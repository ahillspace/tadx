package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
)

type Input struct {
	Environment, Site, Name string
	Limit                   int
	All                     bool
}
type Reader interface {
	DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func ValidateInput(in Input) error {
	if in.Limit < 0 || in.Limit > 10000 {
		return usage("limit must be between 1 and 10000")
	}
	if in.All && in.Limit != 0 {
		return usage("--all cannot be combined with --limit")
	}

	return nil
}

type Output struct {
	Status      string                   `json:"status"`
	Environment string                   `json:"environment,omitempty"`
	Site        string                   `json:"site,omitempty"`
	Page        output.Page              `json:"page"`
	Items       []value.MetadataDatabase `json:"items"`
	Complete    bool                     `json:"complete"`
	ObservedAt  string                   `json:"observed_at,omitempty"`
	RequestID   string                   `json:"tableau_request_id,omitempty"`
	NextCommand string                   `json:"next_command,omitempty"`
}
type CompactItem struct {
	LUID       string `json:"luid"`
	MetadataID string `json:"metadata_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

func (o Output) CompactOutput() any {
	rows := make([]CompactItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = CompactItem{LUID: v.LUID, MetadataID: v.MetadataID, Name: v.Name, Type: v.Type}
	}
	return struct {
		Status      string        `json:"status"`
		Environment string        `json:"environment,omitempty"`
		Site        string        `json:"site,omitempty"`
		Page        output.Page   `json:"page"`
		Items       []CompactItem `json:"items"`
		Complete    bool          `json:"complete"`
		ObservedAt  string        `json:"observed_at,omitempty"`
		Details     string        `json:"details"`
		NextCommand string        `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.ObservedAt, "--full", o.NextCommand}
}
func (o Output) FullOutput() any { return o }
func (a *Action) Execute(ctx context.Context, in Input) (out Output, err error) {
	defer func() {
		if err != nil && out.Status != "" {
			out.Status = "partial"
			out.Complete = false
			out.Page.MoreAvailable = true
			out.Page.Returned = len(out.Items)
		}
	}()
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	defer func() {
		if err == nil && out.Page.MoreAvailable {
			out.NextCommand = nextCommand(in)
		}
	}()
	out = Output{Status: "listed", Environment: in.Environment, Site: in.Site, Items: []value.MetadataDatabase{}}
	if a == nil || a.reader == nil {
		return out, usage("catalog database reader is not configured")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 25
	}
	if in.All {
		limit = 10000
	}
	out.Page.Limit = limit
	result, readErr := paging.CollectMetadata(ctx, limit, func(ctx context.Context, size int, cursor string) (value.MetadataPage[value.MetadataDatabase], error) {
		return a.reader.DiscoverDatabases(ctx, value.MetadataQuery{Name: in.Name, Limit: size, Cursor: cursor})
	}, func(item value.MetadataDatabase) string {
		if item.MetadataID != "" {
			return item.MetadataID
		}
		return item.LUID
	})
	out.Items = result.Items
	out.Page.Returned = len(result.Items)
	out.Page.Total = result.Total
	out.Page.MoreAvailable = result.MoreAvailable
	out.Complete = result.Complete
	out.ObservedAt = result.ObservedAt
	out.RequestID = result.TableauRequestID
	if readErr != nil {
		return out, failure(in, readErr)
	}
	if !out.Complete && !out.Page.MoreAvailable {
		out.Status = "partial"
	}
	return out, nil
}

func nextCommand(in Input) string {
	args := []string{"catalog", "database", "list", "--all"}
	if in.Name != "" {
		args = append(args, "--name", in.Name)
	}
	return commandhint.Environment(in.Environment, args...)
}
func usage(message string) error {
	return &errs.Error{ID: "catalog.database.list.usage", Kind: errs.KindUsage, Operation: "catalog.database.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the list selectors or bounds."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review the exact scope and retry the read.")
	return &errs.Error{ID: "catalog.database.list.failed", Kind: errs.KindOperation, Operation: "catalog.database.list", Environment: in.Environment, Site: in.Site, Summary: "Catalog database listing failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
