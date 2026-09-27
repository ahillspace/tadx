package labelcategory

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type ListInput struct {
	Environment, Site string
	Limit             int
	All               bool
}
type ListReader interface {
	ListLabelCategories(context.Context) ([]value.LabelCategory, error)
}

type ListOutput struct {
	Status        string                `json:"status"`
	Environment   string                `json:"environment,omitempty"`
	Site          string                `json:"site,omitempty"`
	Items         []value.LabelCategory `json:"items,omitempty"`
	Returned      int                   `json:"returned"`
	MoreAvailable bool                  `json:"more_available"`
	Total         int                   `json:"total"`
	NextCommand   string                `json:"next_command,omitempty"`
}

type CompactLabel struct {
	Name string `json:"name"`
}

func compact(v value.LabelCategory) CompactLabel { return CompactLabel{v.Name} }
func (o ListOutput) CompactOutput() any {
	var items []CompactLabel
	if o.Items != nil {
		items = make([]CompactLabel, 0, len(o.Items))
		for _, v := range o.Items {
			items = append(items, compact(v))
		}
	}
	return struct {
		Status        string         `json:"status"`
		Environment   string         `json:"environment,omitempty"`
		Site          string         `json:"site,omitempty"`
		Items         []CompactLabel `json:"items,omitempty"`
		Returned      int            `json:"returned"`
		MoreAvailable bool           `json:"more_available"`
		Total         int            `json:"total"`
		NextCommand   string         `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, items, o.Returned, o.MoreAvailable, o.Total, o.NextCommand}
}
func (o ListOutput) FullOutput() any { return o }
func ValidateListInput(in ListInput) error {
	if in.Limit < 0 || in.Limit > 10000 {
		return listUsage("limit must be between 1 and 10000, or omitted")
	}
	if in.All && in.Limit != 0 {
		return listUsage("--all cannot be combined with --limit")
	}
	return nil
}
func List(ctx context.Context, reader ListReader, in ListInput) (ListOutput, error) {
	out := ListOutput{Status: "listed", Environment: in.Environment, Site: in.Site}
	items, err := reader.ListLabelCategories(ctx)
	if err != nil {
		return out, listFailure(in.Environment, in.Site, err)
	}
	items = slices.Clone(items)
	slices.SortFunc(items, func(a, b value.LabelCategory) int { return strings.Compare(a.Name, b.Name) })
	limit := in.Limit
	if in.All {
		limit = 10000
	} else if limit == 0 {
		limit = 20
	}
	out.MoreAvailable = len(items) > limit
	out.Total = len(items)
	if out.MoreAvailable {
		items = items[:limit]
	}
	out.Items = items
	if out.MoreAvailable {
		out.NextCommand = commandhint.Environment(in.Environment, "admin", "label-category", "list", "--limit", strconv.Itoa(out.Total))
	}
	out.Returned = len(items)
	return out, nil
}

func listUsage(s string) error {
	return &errs.Error{ID: "admin.label.category.list.usage", Kind: errs.KindUsage, Operation: "admin.label.category.list", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func listFailure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "admin.label.category.list.failed", Kind: errs.KindOperation, Operation: "admin.label.category.list", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
