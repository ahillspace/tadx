// Package list lists the authenticated user's bounded Pulse subscriptions.
package list

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

const (
	defaultLimit = 25
	maximumLimit = 10000
)

type Input struct {
	Environment string
	Site        string
	UserLUID    string
	Limit       int
	Cursor      string
	All         bool
}

type PageRequest struct {
	PageSize  int
	PageToken string
}

type Subscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type,omitempty"`
	FollowerLUID string `json:"follower_luid,omitempty"`
}

type Page struct {
	Subscriptions []Subscription
	NextPageToken string
	RequestID     string
}

type Metric struct {
	LUID           string
	Name           string
	DefinitionLUID string
	Specification  map[string]any
}

type Definition struct {
	LUID string
	Name string
}

type Reader interface {
	ListUserSubscriptions(context.Context, string, PageRequest) (Page, error)
	GetMetrics(context.Context, []string) ([]Metric, error)
	GetDefinitions(context.Context, []string) ([]Definition, error)
}

type Item struct {
	SubscriptionLUID string         `json:"subscription_luid"`
	MetricLUID       string         `json:"metric_luid"`
	MetricName       string         `json:"metric_name,omitempty"`
	DefinitionName   string         `json:"definition_name,omitempty"`
	DefinitionLUID   string         `json:"definition_luid,omitempty"`
	FollowerType     string         `json:"follower_type,omitempty"`
	FollowerLUID     string         `json:"follower_luid,omitempty"`
	Filters          any            `json:"filters,omitempty"`
	Period           any            `json:"period,omitempty"`
	Specification    map[string]any `json:"specification,omitempty"`
	Enrichment       string         `json:"enrichment"`
}

type Coverage struct {
	Scope         string `json:"scope"`
	GroupDerived  string `json:"group_derived"`
	Complete      bool   `json:"complete"`
	MoreAvailable bool   `json:"more_available"`
	NextPageToken string `json:"-"`
	NextCursor    string `json:"next_cursor,omitempty"`
}

type Output struct {
	Status        string               `json:"status"`
	Environment   string               `json:"environment"`
	Site          string               `json:"site"`
	UserLUID      string               `json:"user_luid"`
	Count         int                  `json:"count"`
	Coverage      Coverage             `json:"coverage"`
	Subscriptions []Item               `json:"subscriptions"`
	Warnings      []string             `json:"warnings,omitempty"`
	Help          []string             `json:"help,omitempty"`
	RequestID     string               `json:"tableau_request_id,omitempty"`
	Source        *readsource.Metadata `json:"source,omitempty"`
}

type compactItem struct {
	SubscriptionLUID string `json:"subscription_luid"`
	MetricLUID       string `json:"metric_luid"`
	MetricName       string `json:"metric_name,omitempty"`
	DefinitionName   string `json:"definition_name,omitempty"`
	DefinitionLUID   string `json:"definition_luid,omitempty"`
	FollowerType     string `json:"follower_type,omitempty"`
	FollowerLUID     string `json:"follower_luid,omitempty"`
	Filters          any    `json:"filters,omitempty"`
	Period           any    `json:"period,omitempty"`
	Enrichment       string `json:"enrichment"`
}

func (o Output) CompactOutput() any {
	items := make([]compactItem, len(o.Subscriptions))
	for i, item := range o.Subscriptions {
		items[i] = compactItem{item.SubscriptionLUID, item.MetricLUID, item.MetricName, item.DefinitionName, item.DefinitionLUID, item.FollowerType, item.FollowerLUID, item.Filters, item.Period, item.Enrichment}
	}
	return struct {
		Status        string               `json:"status"`
		Environment   string               `json:"environment"`
		Site          string               `json:"site"`
		UserLUID      string               `json:"user_luid"`
		Count         int                  `json:"count"`
		Coverage      Coverage             `json:"coverage"`
		Subscriptions []compactItem        `json:"subscriptions"`
		Warnings      []string             `json:"warnings,omitempty"`
		Details       string               `json:"details"`
		Help          []string             `json:"help,omitempty"`
		Source        *readsource.Metadata `json:"source,omitempty"`
	}{o.Status, o.Environment, o.Site, o.UserLUID, o.Count, o.Coverage, items, o.Warnings, "--full", o.Help, o.Source}
}

func (o Output) FullOutput() any { return o }

type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return Output{}, usage(input, "--all cannot be combined with --limit or --cursor.")
		}
		limit = maximumLimit
	}
	if limit < 1 || limit > maximumLimit || strings.TrimSpace(input.UserLUID) == "" {
		return Output{}, usage(input, "Pulse subscription list requires an authenticated user and a limit from 1 through 10000.")
	}
	token, err := decodeCursor(input.Cursor, input, limit)
	if err != nil {
		return Output{}, usage(input, "Pulse subscription cursor does not match this user, environment, site, and limit.")
	}
	if a == nil || a.reader == nil {
		return Output{}, errors.New("Pulse subscription listing is not configured")
	}
	output := Output{Status: "listed", Environment: input.Environment, Site: input.Site, UserLUID: input.UserLUID, Subscriptions: []Item{}, Coverage: Coverage{Scope: "authenticated_user_filter", GroupDerived: "unverified"}}
	seenIDs := map[string]bool{}
	seenTokens := map[string]bool{}
	for page := 0; page < 100 && len(output.Subscriptions) < limit; page++ {
		pageSize := min(100, limit-len(output.Subscriptions))
		result, err := a.reader.ListUserSubscriptions(ctx, input.UserLUID, PageRequest{PageSize: pageSize, PageToken: token})
		if err != nil {
			output.Warnings = append(output.Warnings, "Subscription inventory could not be completed.")
			return partialFailure(output, &errs.Error{ID: "pulse.subscription.list.failed", Kind: errs.KindOperation, Operation: "pulse.subscription.list", Environment: input.Environment, Site: input.Site, Summary: "Pulse subscription listing failed.", Cause: err, TableauRequestID: errs.TableauRequestID(err)})
		}
		if len(result.Subscriptions) > pageSize {
			return partialFailure(output, invalidResponse(input, "Tableau returned more subscriptions than requested."))
		}
		for _, subscription := range result.Subscriptions {
			if subscription.LUID == "" || subscription.MetricLUID == "" || seenIDs[subscription.LUID] || subscription.FollowerType == "USER" && subscription.FollowerLUID != input.UserLUID {
				return partialFailure(output, invalidResponse(input, "Tableau returned an incomplete, mismatched, or duplicate subscription identity."))
			}
			seenIDs[subscription.LUID] = true
			output.Subscriptions = append(output.Subscriptions, Item{SubscriptionLUID: subscription.LUID, MetricLUID: subscription.MetricLUID, FollowerType: subscription.FollowerType, FollowerLUID: subscription.FollowerLUID, Enrichment: "pending"})
		}
		output.Count = len(output.Subscriptions)
		output.RequestID = result.RequestID
		output.Coverage.NextPageToken = result.NextPageToken
		if result.NextPageToken == "" {
			output.Coverage.Complete = true
			break
		}
		if strings.TrimSpace(result.NextPageToken) == "" || seenTokens[result.NextPageToken] {
			return partialFailure(output, invalidResponse(input, "Tableau returned an invalid or repeated continuation token."))
		}
		seenTokens[result.NextPageToken] = true
		token = result.NextPageToken
	}
	output.Count = len(output.Subscriptions)
	output.Coverage.MoreAvailable = !output.Coverage.Complete
	if input.All && !output.Coverage.Complete {
		output.Status = "partial"
		output.Warnings = append(output.Warnings, "Subscription inventory reached its 100-page or 10000-record bound.")
	}
	if output.Coverage.NextPageToken != "" {
		output.Coverage.NextCursor, err = encodeCursor(output.Coverage.NextPageToken, input, limit)
		if err != nil {
			return partialFailure(output, err)
		}
		output.Help = []string{commandhint.Environment(input.Environment, "pulse", "subscription", "list", "--limit", strconv.Itoa(limit), "--cursor", output.Coverage.NextCursor)}
	}
	if len(output.Subscriptions) == 0 && output.Coverage.Complete {
		return output, nil
	}
	metricIDs := make([]string, 0, len(output.Subscriptions))
	seenMetrics := map[string]bool{}
	for _, item := range output.Subscriptions {
		if !seenMetrics[item.MetricLUID] {
			metricIDs = append(metricIDs, item.MetricLUID)
			seenMetrics[item.MetricLUID] = true
		}
	}
	metrics := map[string]Metric{}
	for start := 0; start < len(metricIDs); start += 100 {
		ids := metricIDs[start:min(start+100, len(metricIDs))]
		batchIDs := make(map[string]bool, len(ids))
		for _, id := range ids {
			batchIDs[id] = true
		}
		batch, err := a.reader.GetMetrics(ctx, ids)
		if err != nil {
			output.Warnings = append(output.Warnings, "A bounded metric batch could not be read; subscription identities are retained.")
			continue
		}
		for _, metric := range batch {
			if !batchIDs[metric.LUID] || metrics[metric.LUID].LUID != "" {
				output.Warnings = append(output.Warnings, "Metric batch returned an unexpected or duplicate identity.")
				continue
			}
			metrics[metric.LUID] = metric
		}
	}
	definitionIDs := []string{}
	seenDefinitions := map[string]bool{}
	for _, id := range metricIDs {
		metric := metrics[id]
		if metric.DefinitionLUID != "" && !seenDefinitions[metric.DefinitionLUID] {
			definitionIDs = append(definitionIDs, metric.DefinitionLUID)
			seenDefinitions[metric.DefinitionLUID] = true
		}
	}
	definitions := map[string]Definition{}
	for start := 0; start < len(definitionIDs); start += 100 {
		ids := definitionIDs[start:min(start+100, len(definitionIDs))]
		batchIDs := make(map[string]bool, len(ids))
		for _, id := range ids {
			batchIDs[id] = true
		}
		batch, err := a.reader.GetDefinitions(ctx, ids)
		if err != nil {
			output.Warnings = append(output.Warnings, "A bounded definition batch could not be read; metric identities are retained.")
			continue
		}
		for _, definition := range batch {
			if !batchIDs[definition.LUID] || definitions[definition.LUID].LUID != "" {
				output.Warnings = append(output.Warnings, "Definition batch returned an unexpected or duplicate identity.")
				continue
			}
			definitions[definition.LUID] = definition
		}
	}
	for index := range output.Subscriptions {
		item := &output.Subscriptions[index]
		metric, known := metrics[item.MetricLUID]
		if !known {
			output.Warnings = append(output.Warnings, fmt.Sprintf("Metric %s could not be enriched; its subscription identity is retained.", item.MetricLUID))
			item.Enrichment = "unavailable"
			continue
		}
		item.MetricName = metric.Name
		item.DefinitionLUID = metric.DefinitionLUID
		item.DefinitionName = definitions[metric.DefinitionLUID].Name
		item.Specification = metric.Specification
		if metric.Specification != nil {
			item.Filters = metric.Specification["filters"]
			item.Period = metric.Specification["measurement_period"]
		}
		item.Enrichment = "complete"
		if metric.DefinitionLUID == "" || item.DefinitionName == "" {
			item.Enrichment = "partial"
			output.Warnings = append(output.Warnings, fmt.Sprintf("Metric %s has no available definition name.", item.MetricLUID))
		}
	}
	if len(output.Warnings) > 0 {
		output.Status = "partial"
	}
	return output, nil
}

func invalidResponse(input Input, summary string) error {
	return &errs.Error{ID: "pulse.subscription.list.invalid_response", Kind: errs.KindOperation, Operation: "pulse.subscription.list", Environment: input.Environment, Site: input.Site, Summary: summary}
}

func partialFailure(output Output, err error) (Output, error) {
	output.Status = "partial"
	output.Count = len(output.Subscriptions)
	output.Coverage.Complete = false
	output.Coverage.MoreAvailable = true
	return output, err
}

func usage(input Input, summary string) error {
	return &errs.Error{ID: "pulse.subscription.list.usage", Kind: errs.KindUsage, Operation: "pulse.subscription.list", Environment: input.Environment, Site: input.Site, Summary: summary}
}

type cursorState struct {
	Version     int    `json:"v"`
	Token       string `json:"t"`
	Environment string `json:"e"`
	Site        string `json:"s"`
	UserLUID    string `json:"u"`
	Limit       int    `json:"l"`
}

func encodeCursor(token string, input Input, limit int) (string, error) {
	data, err := json.Marshal(cursorState{Version: 1, Token: token, Environment: input.Environment, Site: input.Site, UserLUID: input.UserLUID, Limit: limit})
	return base64.RawURLEncoding.EncodeToString(data), err
}

func decodeCursor(value string, input Input, limit int) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > 4096 {
		return "", errors.New("cursor too long")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var state cursorState
	if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Token == "" || state.Environment != input.Environment || state.Site != input.Site || state.UserLUID != input.UserLUID || state.Limit != limit {
		return "", errors.New("invalid cursor")
	}
	return state.Token, nil
}
