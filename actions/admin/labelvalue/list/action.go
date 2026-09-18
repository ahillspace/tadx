package list

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"sort"
	"strconv"
)

type Input struct {
	Environment, Site string
	Limit             int
}
type Reader interface {
	ListLabelValues(context.Context) ([]value.LabelValue, error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }

type Output struct {
	Status        string             `json:"status"`
	Environment   string             `json:"environment,omitempty"`
	Site          string             `json:"site,omitempty"`
	Items         []value.LabelValue `json:"items,omitempty"`
	Returned      int                `json:"returned"`
	MoreAvailable bool               `json:"more_available"`
	Total         int                `json:"total"`
	NextCommand   string             `json:"next_command,omitempty"`
}

type CompactLabel struct {
	Name            string `json:"name"`
	Category        string `json:"category"`
	Internal        bool   `json:"internal"`
	ElevatedDefault bool   `json:"elevated_default"`
	BuiltIn         bool   `json:"built_in"`
}

func compact(v value.LabelValue) CompactLabel {
	return CompactLabel{v.Name, v.Category, v.Internal, v.ElevatedDefault, v.BuiltIn}
}
func (o Output) CompactOutput() any {
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
func (o Output) FullOutput() any { return o }
func ValidateInput(in Input) error {
	if in.Limit < 0 || in.Limit > 10000 {
		return usage("limit must be between 1 and 10000, or omitted")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Status: "listed", Environment: in.Environment, Site: in.Site}
	if a == nil || a.reader == nil {
		return out, usage("label value reader is not configured")
	}
	items, err := a.reader.ListLabelValues(ctx)
	if err != nil {
		return out, failure(in.Environment, in.Site, err)
	}
	if len(items) > 10000 {
		return out, failure(in.Environment, in.Site, fmt.Errorf("label collection exceeds 10000-item bound"))
	}
	seen := map[string]bool{}
	for _, v := range items {
		if v.Name == "" {
			return out, failure(in.Environment, in.Site, fmt.Errorf("label value omitted its exact name"))
		}
		if seen[v.Name] {
			return out, failure(in.Environment, in.Site, fmt.Errorf("duplicate label value identity"))
		}
		seen[v.Name] = true
	}
	items = append([]value.LabelValue{}, items...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	out.MoreAvailable = len(items) > limit
	out.Total = len(items)
	if out.MoreAvailable {
		items = items[:limit]
	}
	out.Items = items
	if out.MoreAvailable {
		out.NextCommand = commandhint.Environment(in.Environment, "admin", "label-value", "list", "--limit", strconv.Itoa(out.Total))
	}
	out.Returned = len(items)
	return out, nil
}

func usage(s string) error {
	return &errs.Error{ID: "admin.label.value.list.usage", Kind: errs.KindUsage, Operation: "admin.label.value.list", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func failure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "admin.label.value.list.failed", Kind: errs.KindOperation, Operation: "admin.label.value.list", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
