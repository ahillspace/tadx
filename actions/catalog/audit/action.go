package audit

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
	"reflect"
	"slices"
	"strings"
)

type Input struct {
	Environment, Site, Type, ID string
	Checks                      []string
	DirectOnly                  bool
	Limit                       int
}
type Reader interface {
	GetDatabase(context.Context, string) (value.MetadataDatabase, error)
	GetTable(context.Context, string) (value.MetadataTable, error)
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
	DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error)
	DatasourceFieldDescriptions(context.Context, string) (value.MetadataDatasourceDescriptions, error)
}
type Action struct{ reader Reader }

var errAssessmentBound = errors.New("audit assessment bound reached")

func New(r Reader) *Action { return &Action{reader: r} }

type Finding struct {
	Type       string `json:"type"`
	LUID       string `json:"luid"`
	MetadataID string `json:"metadata_id"`
	Name       string `json:"name"`
	Check      string `json:"check"`
	State      string `json:"state"`
	Source     string `json:"source"`
}
type Summary struct {
	Present int `json:"present"`
	Missing int `json:"missing"`
	Unknown int `json:"unknown"`
}
type Output struct {
	Status      string    `json:"status"`
	Environment string    `json:"environment,omitempty"`
	Site        string    `json:"site,omitempty"`
	Type        string    `json:"type"`
	ID          string    `json:"id"`
	Checks      []string  `json:"checks"`
	DirectOnly  bool      `json:"direct_only"`
	Scanned     int       `json:"scanned"`
	Complete    bool      `json:"complete"`
	Summary     Summary   `json:"summary"`
	Findings    []Finding `json:"findings"`
	ObservedAt  string    `json:"observed_at,omitempty"`
	RequestID   string    `json:"tableau_request_id,omitempty"`
}

func (o Output) CompactOutput() any {
	copy := o
	copy.Findings = make([]Finding, 0)
	for _, f := range o.Findings {
		if f.State != "present" {
			copy.Findings = append(copy.Findings, f)
		}
	}
	return struct {
		Output
		Details string `json:"details"`
	}{copy, "--full"}
}
func (o Output) FullOutput() any { return o }
func ValidateInput(in Input) error {
	if in.ID != strings.TrimSpace(in.ID) || strings.ContainsAny(in.ID, "\x00\r\n") {
		return usage("audit identity must be exact and contain no whitespace or control characters")
	}
	if strings.TrimSpace(in.ID) == "" || !slices.Contains([]string{"database", "table", "datasource"}, in.Type) {
		return usage("audit requires one exact --id and --type database, table, or datasource")
	}
	if in.Limit < 0 || in.Limit > 10000 {
		return usage("audit limit must be between 1 and 10000")
	}
	seen := map[string]bool{}
	for _, check := range in.Checks {
		if !slices.Contains([]string{"descriptions", "tags"}, check) || seen[check] {
			return usage("checks must be unique descriptions or tags values")
		}
		seen[check] = true
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if e := ValidateInput(in); e != nil {
		return Output{}, e
	}
	if a == nil || a.reader == nil {
		return Output{}, usage("catalog audit is not configured")
	}
	checks := in.Checks
	if len(checks) == 0 {
		checks = []string{"descriptions", "tags"}
	}
	limit := in.Limit
	if limit == 0 {
		limit = 1000
	}
	out := Output{Status: "audited", Environment: in.Environment, Site: in.Site, Type: in.Type, ID: in.ID, Checks: checks, DirectOnly: in.DirectOnly, Complete: true, Findings: []Finding{}}
	record := func(id value.MetadataIdentity, description *string, tags []string, tagsObserved bool, inherited []value.DescriptionObservation, inheritedObserved bool, field bool) {
		if out.Scanned >= limit {
			out.Complete = false
			return
		}
		out.Scanned++
		if slices.Contains(checks, "descriptions") {
			state, source := "unknown", "direct"
			if description != nil {
				state = "missing"
				if strings.TrimSpace(*description) != "" {
					state = "present"
				}
			}
			if state != "present" && field && !in.DirectOnly {
				if !inheritedObserved {
					state = "unknown"
				}
				for _, v := range inherited {
					if v.Value == nil {
						state = "unknown"
					}
					if v.Value != nil && strings.TrimSpace(*v.Value) != "" {
						state = "present"
						source = "inherited"
						break
					}
				}
			}
			out.add(id, "descriptions", state, source)
		}
		if slices.Contains(checks, "tags") && !field {
			state := "unknown"
			if tagsObserved {
				state = "missing"
				if len(tags) > 0 {
					state = "present"
				}
			}
			out.add(id, "tags", state, "direct")
		}
	}
	columns := func(tableID string) error {
		if tableID == "" {
			out.Complete = false
			return nil
		}
		complete, e := walk(ctx, limit-out.Scanned, value.MetadataQuery{ParentLUID: tableID}, a.reader.DiscoverColumns, func(v value.MetadataColumn) string {
			if v.MetadataID != "" {
				return v.MetadataID
			}
			return v.LUID
		}, func(v value.MetadataColumn) error {
			if v.Table.LUID != "" && v.Table.LUID != tableID {
				return fmt.Errorf("column parent differs from audited table")
			}
			v.Type = "column"
			record(v.MetadataIdentity, v.Description, v.Tags, v.TagsObserved, nil, false, false)
			return nil
		}, func(observed, request string) { out.ObservedAt = observed; out.RequestID = request })
		out.Complete = out.Complete && complete
		return e
	}
	var err error
	switch in.Type {
	case "database":
		var v value.MetadataDatabase
		v, err = a.reader.GetDatabase(ctx, in.ID)
		if err != nil {
			break
		}
		if v.LUID != in.ID {
			err = fmt.Errorf("audited database identity mismatch")
			break
		}
		if v.Type == "" {
			v.Type = "database"
		}
		record(v.MetadataIdentity, v.Description, v.Tags, v.TagsObserved, nil, false, false)
		var complete bool
		complete, err = walk(ctx, limit-out.Scanned, value.MetadataQuery{ParentLUID: in.ID}, a.reader.DiscoverTables, func(v value.MetadataTable) string {
			if v.MetadataID != "" {
				return v.MetadataID
			}
			return v.LUID
		}, func(v value.MetadataTable) error {
			if out.Scanned >= limit {
				return errAssessmentBound
			}
			if v.Database.LUID != "" && v.Database.LUID != in.ID {
				return fmt.Errorf("table parent differs from audited database")
			}
			v.Type = "table"
			record(v.MetadataIdentity, v.Description, v.Tags, v.TagsObserved, nil, false, false)
			return columns(v.LUID)
		}, func(observed, request string) { out.ObservedAt = observed; out.RequestID = request })
		out.Complete = out.Complete && complete
	case "table":
		var v value.MetadataTable
		v, err = a.reader.GetTable(ctx, in.ID)
		if err != nil {
			break
		}
		if v.LUID != in.ID {
			err = fmt.Errorf("audited table identity mismatch")
			break
		}
		v.Type = "table"
		record(v.MetadataIdentity, v.Description, v.Tags, v.TagsObserved, nil, false, false)
		err = columns(v.LUID)
	case "datasource":
		var v value.MetadataDatasourceDescriptions
		v, err = a.reader.DatasourceFieldDescriptions(ctx, in.ID)
		if err != nil && v.Identity.MetadataID == "" {
			break
		}
		if v.LUID != in.ID {
			err = fmt.Errorf("audited datasource identity mismatch")
			break
		}
		out.ObservedAt = v.ObservedAt
		out.RequestID = v.TableauRequestID
		out.Complete = v.Complete
		id := v.Identity
		if id.LUID == "" {
			id.LUID = v.LUID
		}
		id.Type = "datasource"
		record(id, v.Description, v.Tags, v.TagsObserved, nil, false, false)
		if slices.Contains(checks, "descriptions") {
			for _, field := range v.Fields {
				if field.MetadataID == "" {
					out.Complete = false
					continue
				}
				record(value.MetadataIdentity{MetadataID: field.MetadataID, Name: field.Name, Type: "field"}, field.Description, nil, false, field.Inherited, field.InheritedObserved, true)
			}
		}
	}
	if err != nil {
		out.Status = "partial"
		out.Complete = false
		return out, failure(in, err)
	}
	if out.Summary.Unknown > 0 {
		out.Complete = false
	}
	if !out.Complete {
		out.Status = "partial"
	}
	return out, nil
}
func (o *Output) add(id value.MetadataIdentity, check, state, source string) {
	o.Findings = append(o.Findings, Finding{id.Type, id.LUID, id.MetadataID, id.Name, check, state, source})
	switch state {
	case "present":
		o.Summary.Present++
	case "missing":
		o.Summary.Missing++
	default:
		o.Summary.Unknown++
	}
}
func walk[T any](ctx context.Context, limit int, q value.MetadataQuery, read func(context.Context, value.MetadataQuery) (value.MetadataPage[T], error), identity func(T) string, visit func(T) error, observed func(string, string)) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	seen := map[string]bool{}
	items := map[string]T{}
	count := 0
	var coverage paging.MetadataCoverage
	for n := 0; n < 1000; n++ {
		q.Limit = min(100, limit-count)
		page, e := read(ctx, q)
		if e != nil {
			return false, e
		}
		if len(page.Items) > q.Limit {
			return false, fmt.Errorf("audit page exceeds requested limit")
		}
		observed(page.ObservedAt, page.TableauRequestID)
		for _, v := range page.Items {
			key := identity(v)
			if key == "" {
				return false, fmt.Errorf("audit row lacks authoritative identity")
			}
			if prior, ok := items[key]; ok {
				if !reflect.DeepEqual(prior, v) {
					return false, fmt.Errorf("conflicting audit identity")
				}
				continue
			}
			items[key] = v
			count++
			if e := visit(v); e != nil {
				if errors.Is(e, errAssessmentBound) {
					return false, nil
				}
				return false, e
			}
		}
		if e := coverage.Page(page.Total, len(items), page.NextCursor == ""); e != nil {
			return false, e
		}
		if page.NextCursor == "" {
			return page.Complete, nil
		}
		if seen[page.NextCursor] {
			return false, fmt.Errorf("repeated audit continuation")
		}
		seen[page.NextCursor] = true
		if count >= limit {
			return false, nil
		}
		q.Cursor = page.NextCursor
	}
	return false, fmt.Errorf("audit page bound exceeded")
}
func usage(s string) error {
	return &errs.Error{ID: "catalog.audit.usage", Kind: errs.KindUsage, Operation: "catalog.audit", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Select one exact audit scope and supported checks."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review the audit scope and permissions; inaccessible data is not missing metadata.")
	return &errs.Error{ID: "catalog.audit.failed", Kind: errs.KindOperation, Operation: "catalog.audit", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog audit could not complete the selected scope.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
