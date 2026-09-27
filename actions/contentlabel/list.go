package contentlabel

import (
	"cmp"
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"slices"
	"strconv"
	"strings"
)

type ListInput struct {
	Environment, Site string
	Limit             int
	All               bool
	Type, TargetID    string
	Categories        []string
}
type ListReader interface {
	GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error)
}
type ListOutput struct {
	Status        string               `json:"status"`
	Environment   string               `json:"environment,omitempty"`
	Site          string               `json:"site,omitempty"`
	Target        ListRequestedTarget  `json:"target"`
	Items         []value.ContentLabel `json:"items,omitempty"`
	Returned      int                  `json:"returned"`
	MoreAvailable bool                 `json:"more_available"`
	Total         int                  `json:"total"`
	NextCommand   string               `json:"next_command,omitempty"`
}
type ListRequestedTarget struct {
	Type       string   `json:"type"`
	TargetID   string   `json:"target_id"`
	Categories []string `json:"categories,omitempty"`
}

type ListCompactLabel struct {
	LUID       string `json:"luid"`
	TargetLUID string `json:"target_luid"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	Active     bool   `json:"active"`
	Elevated   bool   `json:"elevated"`
}

func compactListLabel(v value.ContentLabel) ListCompactLabel {
	return ListCompactLabel{v.LUID, v.TargetLUID, v.Type, v.Value, v.Category, v.Active, v.Elevated}
}
func (o ListOutput) CompactOutput() any {
	var items []ListCompactLabel
	if o.Items != nil {
		items = make([]ListCompactLabel, 0, len(o.Items))
		for _, v := range o.Items {
			items = append(items, compactListLabel(v))
		}
	}
	return struct {
		Status        string              `json:"status"`
		Environment   string              `json:"environment,omitempty"`
		Site          string              `json:"site,omitempty"`
		Target        ListRequestedTarget `json:"target"`
		Items         []ListCompactLabel  `json:"items,omitempty"`
		Returned      int                 `json:"returned"`
		MoreAvailable bool                `json:"more_available"`
		Total         int                 `json:"total"`
		NextCommand   string              `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, ListRequestedTarget{Type: o.Target.Type, TargetID: o.Target.TargetID, Categories: slices.Clone(o.Target.Categories)}, items, o.Returned, o.MoreAvailable, o.Total, o.NextCommand}
}
func (o ListOutput) FullOutput() any { return o }
func ValidateListInput(in ListInput) error {
	if in.Limit < 0 || in.Limit > 10000 {
		return listUsage("limit must be between 1 and 10000, or omitted")
	}
	if in.All && in.Limit != 0 {
		return listUsage("--all cannot be combined with --limit")
	}
	if !supportedKind(in.Type) || strings.TrimSpace(in.TargetID) == "" || strings.TrimSpace(in.TargetID) != in.TargetID {
		return listUsage("--type must be database, table, column, datasource, or flow, with an exact --target-id")
	}
	if len(in.Categories) > 100 {
		return listUsage("select at most 100 categories")
	}
	for _, c := range in.Categories {
		if strings.TrimSpace(c) == "" || strings.Contains(c, ",") {
			return listUsage("categories must be nonempty names without commas")
		}
	}
	return nil
}
func List(ctx context.Context, reader ListReader, in ListInput) (ListOutput, error) {
	out := ListOutput{Status: "listed", Environment: in.Environment, Site: in.Site, Target: ListRequestedTarget{Type: in.Type, TargetID: in.TargetID, Categories: slices.Clone(in.Categories)}}
	if reader == nil {
		return out, listUsage("label reader is not configured")
	}
	items, err := reader.GetLabels(ctx, value.LabelTarget{Type: in.Type, LUID: in.TargetID}, in.Categories)
	if err != nil {
		return out, listFailure(in.Environment, in.Site, err)
	}
	if len(items) > 10000 {
		return out, listFailure(in.Environment, in.Site, fmt.Errorf("label collection exceeds 10000-item bound"))
	}
	seen := map[string]bool{}
	for i := range items {
		items[i].Type = value.CanonicalContentType(items[i].Type)
		v := items[i]
		if v.LUID == "" || v.TargetLUID != in.TargetID || v.Type != in.Type {
			return out, listFailure(in.Environment, in.Site, fmt.Errorf("label target mismatch: requested type=%q target=%q, returned attachment=%q type=%q target=%q", in.Type, in.TargetID, v.LUID, v.Type, v.TargetLUID))
		}
		if seen[v.LUID] {
			return out, listFailure(in.Environment, in.Site, fmt.Errorf("duplicate label identity"))
		}
		seen[v.LUID] = true
	}
	items = append([]value.ContentLabel{}, items...)
	slices.SortFunc(items, func(a, b value.ContentLabel) int { return cmp.Compare(a.LUID, b.LUID) })
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
		out.NextCommand = listNextCommand(in, out.Total)
	}
	out.Returned = len(items)
	return out, nil
}
func listNextCommand(in ListInput, total int) string {
	args := []string{"content", "label", "list", "--type", in.Type, "--target-id", in.TargetID, "--limit", strconv.Itoa(total)}
	for _, category := range in.Categories {
		args = append(args, "--category", category)
	}
	return commandhint.Environment(in.Environment, args...)
}
func listUsage(s string) error {
	return &errs.Error{ID: "content.label.list.usage", Kind: errs.KindUsage, Operation: "content.label.list", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func listFailure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "content.label.list.failed", Kind: errs.KindOperation, Operation: "content.label.list", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
func supportedKind(kind string) bool {
	return kind == "database" || kind == "table" || kind == "column" || kind == "datasource" || kind == "flow"
}
