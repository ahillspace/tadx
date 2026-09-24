// Package pulse implements the released Tableau Pulse REST boundary.
package pulse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	pulsePath                      = "/api/-/pulse"
	maxResponseBytes               = 8 * 1024 * 1024
	createDefinitionRequestType    = "application/vnd.tableau.metricqueryservice.v1.CreateDefinitionRequest+json"
	createDefinitionResponseType   = "application/vnd.tableau.metricqueryservice.v1.CreateDefinitionResponse+json"
	getOrCreateRequestType         = "application/vnd.tableau.metricqueryservice.v1.GetOrCreateMetricRequest+json"
	getOrCreateResponseType        = "application/vnd.tableau.metricqueryservice.v1.GetOrCreateMetricResponse+json"
	createSubscriptionsRequestType = "application/vnd.tableau.pulse.subscriptionservice.v1.BatchCreateSubscriptionsRequest+json"
)

// Client is an authenticated Pulse REST client.
type Client struct {
	transport    *tableau.Transport
	session      auth.Session
	serverURL    string
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// NewClient creates a Pulse client with authoritative authenticated site identity.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) (*Client, error) {
	client := &Client{transport: transport, session: session, serverURL: serverURL, pollInterval: time.Second, pollTimeout: 12 * time.Second}
	if err := client.validate(); err != nil {
		return nil, err
	}
	return client, nil
}

// SetPollPolicy sets bounded default-metric and reconciliation polling.
func (c *Client) SetPollPolicy(interval, timeout time.Duration) {
	if interval > 0 {
		c.pollInterval = interval
	}
	if timeout > 0 {
		c.pollTimeout = timeout
	}
}

func (c *Client) validate() error {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return errors.New("authenticated Pulse client is not configured")
	}
	if strings.TrimSpace(c.session.SiteLUID()) == "" {
		return errors.New("authenticated Pulse client requires a site LUID")
	}
	return nil
}

// ListDefinitions returns one bounded provider page.
func (c *Client) ListDefinitions(ctx context.Context, input PageRequest) (DefinitionPage, error) {
	query, err := pageQuery(input)
	if err != nil {
		return DefinitionPage{}, err
	}
	response, err := c.do(ctx, http.MethodGet, pulsePath+"/definitions", query, nil, "", "", "pulse.definition.list")
	if err != nil {
		return DefinitionPage{}, err
	}
	var envelope struct {
		Definitions   []json.RawMessage `json:"definitions"`
		NextPageToken string            `json:"next_page_token"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Definitions == nil {
		return DefinitionPage{}, protocol("pulse.definition.list", response, errors.New("Pulse definition response requires a definitions array"), true)
	}
	items := make([]Definition, len(envelope.Definitions))
	for i, raw := range envelope.Definitions {
		item, err := decodeDefinition(raw, response.TableauRequestID)
		if err != nil {
			return DefinitionPage{}, protocol("pulse.definition.list", response, err, true)
		}
		items[i] = item
	}
	return DefinitionPage{Definitions: items, NextPageToken: envelope.NextPageToken, TableauRequestID: response.TableauRequestID}, nil
}

// GetDefinition returns one exact Pulse definition.
func (c *Client) GetDefinition(ctx context.Context, luid string) (Definition, error) {
	if strings.TrimSpace(luid) == "" {
		return Definition{}, errors.New("Pulse definition LUID is required")
	}
	response, err := c.do(ctx, http.MethodGet, pulsePath+"/definitions/"+url.PathEscape(luid), nil, nil, "", "", "pulse.definition.get")
	if err != nil {
		return Definition{}, err
	}
	var envelope struct {
		Definition json.RawMessage `json:"definition"`
	}
	raw := json.RawMessage(response.Body)
	if json.Unmarshal(response.Body, &envelope) == nil && len(envelope.Definition) > 0 {
		raw = envelope.Definition
	}
	item, err := decodeDefinition(raw, response.TableauRequestID)
	if err != nil {
		return Definition{}, protocol("pulse.definition.get", response, err, true)
	}
	if item.LUID != luid {
		return Definition{}, protocol("pulse.definition.get", response, fmt.Errorf("definition response returned LUID %q, expected %q", item.LUID, luid), true)
	}
	return item, nil
}

// ListMetrics returns one bounded definition-scoped metric page.
func (c *Client) ListMetrics(ctx context.Context, definitionLUID string, input PageRequest) (MetricPage, error) {
	if strings.TrimSpace(definitionLUID) == "" {
		return MetricPage{}, errors.New("Pulse definition LUID is required")
	}
	query, err := pageQuery(input)
	if err != nil {
		return MetricPage{}, err
	}
	response, err := c.do(ctx, http.MethodGet, pulsePath+"/definitions/"+url.PathEscape(definitionLUID)+"/metrics", query, nil, "", "", "pulse.metric.list")
	if err != nil {
		return MetricPage{}, err
	}
	var envelope struct {
		Metrics       []json.RawMessage `json:"metrics"`
		NextPageToken string            `json:"next_page_token"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Metrics == nil {
		return MetricPage{}, protocol("pulse.metric.list", response, errors.New("Pulse metric response requires a metrics array"), true)
	}
	items := make([]Metric, len(envelope.Metrics))
	for i, raw := range envelope.Metrics {
		item, err := decodeMetric(raw, response.TableauRequestID)
		if err != nil {
			return MetricPage{}, protocol("pulse.metric.list", response, err, true)
		}
		if item.DefinitionLUID == "" {
			item.DefinitionLUID = definitionLUID
		}
		items[i] = item
	}
	return MetricPage{Metrics: items, NextPageToken: envelope.NextPageToken, TableauRequestID: response.TableauRequestID}, nil
}

// GetMetric returns one exact Pulse metric and complete specification.
func (c *Client) GetMetric(ctx context.Context, luid string) (Metric, error) {
	if strings.TrimSpace(luid) == "" {
		return Metric{}, errors.New("Pulse metric LUID is required")
	}
	response, err := c.do(ctx, http.MethodGet, pulsePath+"/metrics/"+url.PathEscape(luid), nil, nil, "", "", "pulse.metric.get")
	if err != nil {
		return Metric{}, err
	}
	var envelope struct {
		Metric json.RawMessage `json:"metric"`
	}
	raw := json.RawMessage(response.Body)
	if json.Unmarshal(response.Body, &envelope) == nil && len(envelope.Metric) > 0 {
		raw = envelope.Metric
	}
	item, err := decodeMetric(raw, response.TableauRequestID)
	if err != nil {
		return Metric{}, protocol("pulse.metric.get", response, err, true)
	}
	if item.LUID != luid {
		return Metric{}, protocol("pulse.metric.get", response, fmt.Errorf("metric response returned LUID %q, expected %q", item.LUID, luid), true)
	}
	return item, nil
}

// GetOrCreateMetric creates or reuses one exact desired metric specification.
func (c *Client) GetOrCreateMetric(ctx context.Context, input GetOrCreateRequest) (GetOrCreateResult, error) {
	if strings.TrimSpace(input.DefinitionLUID) == "" || input.Specification == nil {
		return GetOrCreateResult{}, errors.New("Pulse get-or-create requires definition LUID and specification")
	}
	body, err := json.Marshal(struct {
		DefinitionLUID string         `json:"definition_id"`
		Specification  map[string]any `json:"specification"`
	}{input.DefinitionLUID, input.Specification})
	if err != nil {
		return GetOrCreateResult{}, fmt.Errorf("encode Pulse metric get-or-create: %w", err)
	}
	response, err := c.do(ctx, http.MethodPost, pulsePath+"/metrics:getOrCreate", nil, body, getOrCreateRequestType, getOrCreateResponseType, "pulse.metric.fork")
	if err != nil {
		return GetOrCreateResult{}, err
	}
	var envelope struct {
		Metric struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Metadata struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"metric"`
		Created *bool `json:"is_metric_created"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Created == nil {
		return GetOrCreateResult{}, protocol("pulse.metric.fork", response, errors.New("Pulse get-or-create response requires is_metric_created"), false)
	}
	luid := first(envelope.Metric.Metadata.ID, envelope.Metric.ID)
	if luid == "" {
		return GetOrCreateResult{}, protocol("pulse.metric.fork", response, errors.New("Pulse get-or-create response omitted metric identity"), false)
	}
	return GetOrCreateResult{MetricLUID: luid, MetricName: first(envelope.Metric.Metadata.Name, envelope.Metric.Name), Created: *envelope.Created, TableauRequestID: response.TableauRequestID}, nil
}

// ListSubscriptions returns every normalized follower for one exact metric.
func (c *Client) ListSubscriptions(ctx context.Context, metricLUID string) ([]Subscription, error) {
	if strings.TrimSpace(metricLUID) == "" {
		return nil, errors.New("Pulse metric LUID is required")
	}
	response, err := c.do(ctx, http.MethodGet, pulsePath+"/subscriptions", url.Values{"metric_id": {metricLUID}}, nil, "", "", "pulse.metric.followers")
	if err != nil {
		return nil, err
	}
	records, err := subscriptionRecords(response.Body)
	if err != nil {
		return nil, protocol("pulse.metric.followers", response, err, true)
	}
	items := make([]Subscription, len(records))
	for i, record := range records {
		item, err := decodeSubscription(record, metricLUID, response.TableauRequestID)
		if err != nil {
			return nil, protocol("pulse.metric.followers", response, err, true)
		}
		items[i] = item
	}
	return items, nil
}

// ListUserSubscriptions returns one authenticated-user-filtered Pulse page.
func (c *Client) ListUserSubscriptions(ctx context.Context, userLUID string, input PageRequest) (SubscriptionPage, error) {
	if strings.TrimSpace(userLUID) == "" {
		return SubscriptionPage{}, errors.New("Pulse user LUID is required")
	}
	query, err := pageQuery(input)
	if err != nil {
		return SubscriptionPage{}, err
	}
	query.Set("user_id", userLUID)
	response, err := c.do(ctx, http.MethodGet, pulsePath+"/subscriptions", query, nil, "", "", "pulse.subscription.list")
	if err != nil {
		return SubscriptionPage{}, err
	}
	var envelope struct {
		Subscriptions []json.RawMessage `json:"subscriptions"`
		NextPageToken string            `json:"next_page_token"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Subscriptions == nil {
		return SubscriptionPage{}, protocol("pulse.subscription.list", response, errors.New("Pulse subscription response requires a subscriptions array"), true)
	}
	items := make([]Subscription, len(envelope.Subscriptions))
	for i, raw := range envelope.Subscriptions {
		var record struct {
			MetricLUID   string `json:"metric_id"`
			Subscription struct {
				MetricLUID string `json:"metric_id"`
			} `json:"subscription"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			return SubscriptionPage{}, protocol("pulse.subscription.list", response, err, true)
		}
		metricLUID := first(record.MetricLUID, record.Subscription.MetricLUID)
		if metricLUID == "" {
			return SubscriptionPage{}, protocol("pulse.subscription.list", response, errors.New("Pulse subscription omitted metric identity"), true)
		}
		item, err := decodeSubscription(raw, metricLUID, response.TableauRequestID)
		if err != nil || item.LUID == "" {
			return SubscriptionPage{}, protocol("pulse.subscription.list", response, errors.New("Pulse subscription omitted subscription or follower identity"), true)
		}
		items[i] = item
	}
	return SubscriptionPage{Subscriptions: items, NextPageToken: envelope.NextPageToken, TableauRequestID: response.TableauRequestID}, nil
}

// BatchGetMetrics reads only the supplied exact metric IDs.
func (c *Client) BatchGetMetrics(ctx context.Context, ids []string) ([]Metric, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, errors.New("Pulse metric batch requires 1 through 100 IDs")
	}
	body, err := json.Marshal(struct {
		IDs []string `json:"metric_ids"`
	}{ids})
	if err != nil {
		return nil, err
	}
	response, err := c.do(ctx, http.MethodPost, pulsePath+"/metrics:batchGet", nil, body, "application/json", "", "pulse.subscription.list")
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Metrics []json.RawMessage `json:"metrics"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Metrics == nil {
		return nil, protocol("pulse.subscription.list", response, errors.New("Pulse metric batch requires a metrics array"), true)
	}
	items := make([]Metric, len(envelope.Metrics))
	for i, raw := range envelope.Metrics {
		item, err := decodeMetric(raw, response.TableauRequestID)
		if err != nil {
			return nil, protocol("pulse.subscription.list", response, err, true)
		}
		items[i] = item
	}
	return items, nil
}

// BatchGetDefinitions reads only the supplied exact definition IDs.
func (c *Client) BatchGetDefinitions(ctx context.Context, ids []string) ([]Definition, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, errors.New("Pulse definition batch requires 1 through 100 IDs")
	}
	body, err := json.Marshal(struct {
		IDs []string `json:"definition_ids"`
	}{ids})
	if err != nil {
		return nil, err
	}
	response, err := c.do(ctx, http.MethodPost, pulsePath+"/definitions:batchGet", url.Values{"view": {"DEFINITION_VIEW_BASIC"}}, body, "application/json", "", "pulse.subscription.list")
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Definitions []json.RawMessage `json:"definitions"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil || envelope.Definitions == nil {
		return nil, protocol("pulse.subscription.list", response, errors.New("Pulse definition batch requires a definitions array"), true)
	}
	items := make([]Definition, len(envelope.Definitions))
	for i, raw := range envelope.Definitions {
		item, err := decodeDefinition(raw, response.TableauRequestID)
		if err != nil {
			return nil, protocol("pulse.subscription.list", response, err, true)
		}
		items[i] = item
	}
	return items, nil
}

// CreateSubscription converges one exact user or group follow relationship.
func (c *Client) CreateSubscription(ctx context.Context, input CreateSubscriptionRequest) (CreateSubscriptionResult, error) {
	type follower struct {
		UserLUID  string `json:"user_id,omitempty"`
		GroupLUID string `json:"group_id,omitempty"`
	}
	f := follower{}
	switch input.FollowerType {
	case "USER":
		f.UserLUID = input.FollowerLUID
	case "GROUP":
		f.GroupLUID = input.FollowerLUID
	default:
		return CreateSubscriptionResult{}, errors.New("Pulse follower type must be USER or GROUP")
	}
	if strings.TrimSpace(input.MetricLUID) == "" || strings.TrimSpace(input.FollowerLUID) == "" {
		return CreateSubscriptionResult{}, errors.New("Pulse follow requires exact metric and follower LUIDs")
	}
	body, err := json.Marshal(struct {
		MetricLUID string     `json:"metric_id"`
		Followers  []follower `json:"followers"`
	}{input.MetricLUID, []follower{f}})
	if err != nil {
		return CreateSubscriptionResult{}, err
	}
	response, err := c.do(ctx, http.MethodPost, pulsePath+"/subscriptions:batchCreate", nil, body, createSubscriptionsRequestType, "", "pulse.metric.follow")
	if err != nil {
		if alreadyFollowing(err) {
			return CreateSubscriptionResult{Status: "already_following", TableauRequestID: tableau.RequestID(err)}, nil
		}
		return CreateSubscriptionResult{}, err
	}
	var data any
	_ = json.Unmarshal(response.Body, &data)
	return CreateSubscriptionResult{Status: "followed", SubscriptionLUID: findID(data), TableauRequestID: response.TableauRequestID}, nil
}

// DeleteSubscription removes one exact subscription.
func (c *Client) DeleteSubscription(ctx context.Context, subscriptionLUID string) error {
	if strings.TrimSpace(subscriptionLUID) == "" {
		return errors.New("Pulse subscription LUID is required")
	}
	response, err := c.do(ctx, http.MethodDelete, pulsePath+"/subscriptions/"+url.PathEscape(subscriptionLUID), nil, nil, "", "", "pulse.metric.unfollow")
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		return protocol("pulse.metric.unfollow", response, errors.New("Pulse subscription delete requires empty HTTP 204"), false)
	}
	return nil
}

// CreateDefinition creates one definition and resolves its asynchronous default metric.
func (c *Client) CreateDefinition(ctx context.Context, input CreateRequest) (CreateResult, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return CreateResult{}, err
	}
	return c.createDefinitionDocument(ctx, body)
}

// CreateDefinitionDocument preserves verified writable configuration sections,
// including nested specification fields not exposed as individual CLI flags.
func (c *Client) CreateDefinitionDocument(ctx context.Context, body json.RawMessage) (CreateResult, error) {
	var document map[string]json.RawMessage
	if json.Unmarshal(body, &document) != nil || document == nil || len(document["name"]) == 0 || len(document["specification"]) == 0 {
		return CreateResult{}, errors.New("Pulse definition create document requires name and specification")
	}
	allowed := map[string]bool{"name": true, "description": true, "specification": true, "extension_options": true, "representation_options": true, "insights_options": true, "comparisons": true, "datasource_goals": true, "related_links": true, "certification": true}
	for key := range document {
		if !allowed[key] {
			return CreateResult{}, fmt.Errorf("unsupported Pulse definition create section %q", key)
		}
	}
	return c.createDefinitionDocument(ctx, body)
}

func (c *Client) createDefinitionDocument(ctx context.Context, body []byte) (CreateResult, error) {
	response, err := c.do(ctx, http.MethodPost, pulsePath+"/definitions", nil, body, createDefinitionRequestType, createDefinitionResponseType, "pulse.definition.create")
	if err != nil {
		return CreateResult{}, err
	}
	if response.StatusCode != http.StatusCreated {
		return CreateResult{}, protocol("pulse.definition.create", response, fmt.Errorf("Pulse definition create returned HTTP %d, expected 201", response.StatusCode), false)
	}
	var envelope struct {
		Definition struct {
			Metadata struct {
				ID string `json:"id"`
			} `json:"metadata"`
		} `json:"definition"`
	}
	if json.Unmarshal(response.Body, &envelope) != nil || envelope.Definition.Metadata.ID == "" {
		return CreateResult{}, protocol("pulse.definition.create", response, errors.New("Pulse definition create response omitted identity"), false)
	}
	result := CreateResult{Status: "succeeded", DefinitionLUID: envelope.Definition.Metadata.ID, DefaultMetricStatus: "pending", TableauRequestID: response.TableauRequestID}
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	for {
		page, pollErr := c.ListMetrics(pollCtx, result.DefinitionLUID, PageRequest{PageSize: 100})
		if pollErr == nil {
			result.PollRequestID = page.TableauRequestID
			for _, metric := range page.Metrics {
				if metric.IsDefault {
					result.DefaultMetricLUID = metric.LUID
					result.DefaultMetricStatus = "resolved"
					return result, nil
				}
			}
			if len(page.Metrics) == 1 && !page.Metrics[0].DefaultKnown {
				result.DefaultMetricLUID = page.Metrics[0].LUID
				result.DefaultMetricStatus = "resolved"
				return result, nil
			}
		} else if code := statusCode(pollErr); code == http.StatusUnauthorized || code == http.StatusForbidden {
			return result, pollErr
		}
		select {
		case <-pollCtx.Done():
			return result, fmt.Errorf("resolve default Pulse metric: %w", pollCtx.Err())
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte, contentType, accept, operation string) (tableau.Response, error) {
	if err := c.validate(); err != nil {
		return tableau.Response{}, err
	}
	header := http.Header{}
	header.Set("X-Tableau-Site-Id", strings.TrimSpace(c.session.SiteLUID()))
	return c.transport.Do(ctx, c.session, tableau.Request{Method: method, ServerURL: c.serverURL, Path: path, Query: query, Header: header, Body: body, ContentType: contentType, Accept: accept, Operation: operation, MaxResponseBytes: maxResponseBytes})
}

func pageQuery(input PageRequest) (url.Values, error) {
	if input.PageSize < 0 || input.PageSize > 100 {
		return nil, errors.New("Pulse page size must be between 1 and 100")
	}
	query := url.Values{}
	if input.PageSize > 0 {
		query.Set("page_size", fmt.Sprint(input.PageSize))
	}
	if input.PageToken != "" {
		query.Set("page_token", input.PageToken)
	}
	return query, nil
}

func decodeDefinition(raw json.RawMessage, requestID string) (Definition, error) {
	var value struct {
		Metadata      struct{ ID, Name, Description string } `json:"metadata"`
		Specification struct {
			Datasource struct {
				ID string `json:"id"`
			} `json:"datasource"`
			Basic struct {
				Filters json.RawMessage                     `json:"filters"`
				Measure struct{ Field, Aggregation string } `json:"measure"`
				Time    struct {
					Field string `json:"field"`
				} `json:"time_dimension"`
			} `json:"basic_specification"`
			RunningTotal bool   `json:"is_running_total"`
			Temporality  string `json:"temporality"`
		} `json:"specification"`
		Extension struct {
			Dimensions    []string `json:"allowed_dimensions"`
			Granularities []string `json:"allowed_granularities"`
		} `json:"extension_options"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return Definition{}, fmt.Errorf("decode Pulse definition: %w", err)
	}
	if value.Metadata.ID == "" {
		return Definition{}, errors.New("Pulse definition omitted identity")
	}
	configuration := append(json.RawMessage(nil), raw...)
	var fixedFilters []any
	decoder := json.NewDecoder(bytes.NewReader(value.Specification.Basic.Filters))
	decoder.UseNumber()
	known := decoder.Decode(&fixedFilters) == nil && fixedFilters != nil
	return Definition{LUID: value.Metadata.ID, Name: value.Metadata.Name, Description: value.Metadata.Description, DatasourceLUID: value.Specification.Datasource.ID, MeasureField: value.Specification.Basic.Measure.Field, Aggregation: value.Specification.Basic.Measure.Aggregation, TimeDimension: value.Specification.Basic.Time.Field, RunningTotal: value.Specification.RunningTotal, Temporality: value.Specification.Temporality, AllowedDimensions: value.Extension.Dimensions, AllowedGranularities: value.Extension.Granularities, FixedFilters: fixedFilters, FixedFiltersKnown: known, Configuration: configuration, TableauRequestID: requestID}, nil
}

func decodeMetric(raw json.RawMessage, requestID string) (Metric, error) {
	var value struct {
		ID, Name               string
		Metadata               struct{ ID, Name string }
		DefinitionLUID         string `json:"definition_id"`
		SpecificationReference struct {
			DefinitionLUID string `json:"definition_id"`
		} `json:"specification_reference"`
		Definition struct {
			Metadata struct {
				ID string `json:"id"`
			} `json:"metadata"`
		} `json:"definition"`
		SiteLUID string `json:"site_id"`
		Site     struct {
			ID string `json:"id"`
		} `json:"site"`
		IsDefault           *bool          `json:"is_default"`
		Specification       map[string]any `json:"specification"`
		MetricSpecification map[string]any `json:"metric_specification"`
	}
	if !json.Valid(raw) {
		return Metric{}, errors.New("decode Pulse metric: invalid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return Metric{}, fmt.Errorf("decode Pulse metric: %w", err)
	}
	luid := first(value.Metadata.ID, value.ID)
	if luid == "" {
		return Metric{}, errors.New("Pulse metric omitted identity")
	}
	specification := value.Specification
	if specification == nil {
		specification = value.MetricSpecification
	}
	return Metric{LUID: luid, Name: first(value.Metadata.Name, value.Name), DefinitionLUID: first(value.DefinitionLUID, value.SpecificationReference.DefinitionLUID, value.Definition.Metadata.ID), SiteLUID: first(value.SiteLUID, value.Site.ID), IsDefault: value.IsDefault != nil && *value.IsDefault, DefaultKnown: value.IsDefault != nil, Specification: specification, Configuration: append(json.RawMessage(nil), raw...), TableauRequestID: requestID}, nil
}

func subscriptionRecords(body []byte) ([]json.RawMessage, error) {
	var data map[string]json.RawMessage
	if json.Unmarshal(body, &data) != nil {
		return nil, errors.New("decode Pulse subscriptions response")
	}
	for _, key := range []string{"next_page_token", "nextPageToken", "continuation_token"} {
		if token, ok := data[key]; ok && string(token) != `""` && string(token) != "null" {
			return nil, errors.New("Pulse subscriptions response is incomplete; continuation is not supported by the verified full-snapshot endpoint")
		}
	}
	raw, ok := data["subscriptions"]
	_, plural := data["subscriptions"]
	if !ok {
		raw, ok = data["subscription"]
	}
	if !ok {
		return nil, errors.New("Pulse subscriptions response omitted records")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("Pulse subscriptions response requires an explicit array")
	}
	var records []json.RawMessage
	if json.Unmarshal(raw, &records) == nil {
		return records, nil
	}
	if !plural {
		var record map[string]any
		if json.Unmarshal(raw, &record) == nil {
			return []json.RawMessage{raw}, nil
		}
	}
	var nested map[string]json.RawMessage
	if json.Unmarshal(raw, &nested) == nil {
		if leaf, exists := nested["subscription"]; exists {
			if json.Unmarshal(leaf, &records) == nil {
				return records, nil
			}
			return []json.RawMessage{leaf}, nil
		}
	}
	return nil, errors.New("unsupported Pulse subscriptions response shape")
}

func decodeSubscription(raw json.RawMessage, metricLUID, requestID string) (Subscription, error) {
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return Subscription{}, errors.New("invalid Pulse subscription record")
	}
	follower := asMap(data["follower"])
	nested := asMap(data["subscription"])
	if follower == nil {
		follower = asMap(nested["follower"])
	}
	if follower == nil {
		return Subscription{}, errors.New("Pulse subscription omitted follower")
	}
	user := asMap(follower["user"])
	group := asMap(follower["group"])
	userID := first(stringValue(follower["user_id"]), stringValue(follower["userId"]), stringValue(user["id"]), stringValue(user["luid"]))
	groupID := first(stringValue(follower["group_id"]), stringValue(follower["groupId"]), stringValue(group["id"]), stringValue(group["luid"]))
	typeName, luid := "UNKNOWN", first(groupID, userID, stringValue(follower["id"]), stringValue(follower["luid"]))
	if groupID != "" {
		typeName = "GROUP"
	} else if userID != "" {
		typeName = "USER"
	} else {
		explicit := strings.ToUpper(stringValue(follower["type"]))
		if explicit == "GROUP" || explicit == "USER" {
			typeName = explicit
		}
	}
	if luid == "" {
		return Subscription{}, errors.New("Pulse subscription follower omitted identity")
	}
	return Subscription{LUID: first(stringValue(data["id"]), stringValue(data["subscription_id"]), stringValue(nested["id"])), MetricLUID: metricLUID, FollowerType: typeName, FollowerLUID: luid, FollowerName: first(stringValue(follower["name"]), stringValue(user["name"]), stringValue(group["name"])), TableauRequestID: requestID}, nil
}

func protocol(operation string, response tableau.Response, cause error, retryable bool) error {
	return tableau.NewProtocolError(operation, response, cause, retryable)
}
func alreadyFollowing(err error) bool {
	var status interface{ HTTPStatus() int }
	return errors.As(err, &status) && status.HTTPStatus() == http.StatusConflict && strings.Contains(strings.ToLower(err.Error()), "already exists")
}
func asMap(value any) map[string]any { result, _ := value.(map[string]any); return result }
func stringValue(value any) string   { result, _ := value.(string); return strings.TrimSpace(result) }
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func findID(value any) string {
	if data, ok := value.(map[string]any); ok {
		for _, key := range []string{"id", "subscription_id"} {
			if id := stringValue(data[key]); id != "" {
				return id
			}
		}
		for _, child := range data {
			if id := findID(child); id != "" {
				return id
			}
		}
	}
	if values, ok := value.([]any); ok {
		for _, child := range values {
			if id := findID(child); id != "" {
				return id
			}
		}
	}
	return ""
}
