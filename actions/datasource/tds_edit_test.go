package datasource

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
	"testing"
)

var editTarget = EditTarget{Server: "https://tableau.example.test", SiteLUID: "00000000-0000-4000-8000-000000000001", WorkCopyLUID: "00000000-0000-4000-8000-000000000002"}

type editFixture struct {
	values     map[string]value.TDSElement
	writes     []string
	afterWrite func()
	failWrite  bool
}

func (f *editFixture) ReadTDSElements(_ context.Context, _, _, kind, name string) (value.TDSRead, error) {
	e, ok := f.values[kind+name]
	if !ok {
		return value.TDSRead{Absent: true}, nil
	}
	return value.TDSRead{Elements: []value.TDSElement{e}}, nil
}
func (f *editFixture) EditTDSElement(_ context.Context, _ string, verb string, e value.TDSElement) (value.TDSReceipt, error) {
	f.writes = append(f.writes, verb+" "+e.Name)
	if verb == "delete" {
		delete(f.values, e.ElementType+e.Name)
	} else {
		f.values[e.ElementType+e.Name] = e
	}
	if f.afterWrite != nil {
		f.afterWrite()
	}
	if f.failWrite {
		return value.TDSReceipt{Status: "unknown"}, errors.New("response lost")
	}
	return value.TDSReceipt{Status: "acknowledged"}, nil
}
func editFields(names ...string) *editFixture {
	f := &editFixture{values: map[string]value.TDSElement{}}
	for _, name := range names {
		f.values["Column"+name] = value.TDSElement{Name: name, Column: &value.TDSColumn{Caption: new(name)}}
	}
	return f
}
func TestTDSEditOrderedEffects(t *testing.T) {
	f := editFields("Sales", "Date")
	changes := []EditChange{{Op: "set-field-description", Field: "Sales", Description: "Amount"}, {Op: "set-field-description", Field: "Date", Description: "Placed on"}}
	preview, err := Edit(t.Context(), f, editTarget, "demo", changes, true, "")
	if err != nil || len(f.writes) > 0 || preview.PreviewToken == "" {
		t.Fatalf("preview %#v %v", preview, err)
	}
	f.afterWrite = func() {
		f.values["FieldDescriptionDate"] = value.TDSElement{Name: "Date", Description: new("competing edit")}
	}
	out, err := Edit(t.Context(), f, editTarget, "demo", changes, false, preview.PreviewToken)
	if err == nil || strings.Join(f.writes, ",") != "add Sales" || out.Items[0].Status != "verified" || out.Items[1].Status != "stale" {
		t.Fatalf("apply %#v writes %v err %v", out, f.writes, err)
	}
}
func TestTDSEditUnknownIsNeverReplayed(t *testing.T) {
	f := editFields("outside collection", "B")
	f.failWrite = true
	out, err := Edit(t.Context(), f, editTarget, "demo", []EditChange{{Op: "set-field-description", Field: "outside collection", Description: "value"}, {Op: "set-field-description", Field: "B", Description: "next"}}, false, "")
	if err == nil || len(f.writes) != 1 || out.Items[0].Status != "unknown" || out.Items[0].Observed.Value != "value" || out.Items[1].Status != "unattempted" || !strings.Contains(out.Help[0], "--field") {
		t.Fatalf("uncertainty %#v %v", out, err)
	}
}

func TestTDSEditMethodsAndReview(t *testing.T) {
	for _, tc := range []struct {
		change   EditChange
		previous *string
		want     string
	}{
		{EditChange{Op: "set-field-description", Field: "X", Description: "new"}, nil, "add"},
		{EditChange{Op: "set-field-description", Field: "X", Description: "new"}, new(""), "update"},
		{EditChange{Op: "set-table-description", Table: "X", Description: "new"}, nil, "add"},
		{EditChange{Op: "set-table-description", Table: "X", Description: "new"}, new("old"), "update"},
		{EditChange{Op: "set-table-description", Table: "X", Description: "new"}, new("new"), "unchanged"},
		{EditChange{Op: "remove-table-description", Table: "X"}, new("old"), "delete"},
		{EditChange{Op: "remove-field-description", Field: "X"}, new("old"), "delete"},
		{EditChange{Op: "remove-field-description", Field: "X"}, nil, "unchanged"},
		{EditChange{Op: "set-column-caption", Field: "X", Caption: "new"}, new("old"), "update"},
		{EditChange{Op: "set-column-type", Field: "X", Type: "FIELD_TYPE_STRING"}, new("old"), "update"},
		{EditChange{Op: "set-column-role", Field: "X", Role: "FIELD_ROLE_DIMENSION"}, new("old"), "update"},
		{EditChange{Op: "delete-column", Field: "X"}, new("old"), "delete"},
	} {
		t.Run(tc.change.Op+"/"+tc.want, func(t *testing.T) {
			f := editFields("X")
			s := tc.change.Selector()
			if tc.previous != nil {
				if s.ElementType == "Column" {
					f.values["ColumnX"] = value.TDSElement{Name: "X", Column: &value.TDSColumn{Caption: tc.previous, Role: tc.previous, Type: tc.previous}}
				} else {
					f.values[s.ElementType+"X"] = value.TDSElement{Name: "X", Description: tc.previous}
				}
			}
			ops := []EditChange{tc.change}
			out, err := Edit(t.Context(), f, editTarget, "demo", ops, false, "")
			if err != nil || out.Items[0].Action != tc.want || out.Items[0].Status != "verified" && out.Items[0].Status != "unchanged" {
				t.Fatal(out, err)
			}
			n := 1
			if tc.want == "unchanged" {
				n = 0
			}
			if len(f.writes) != n {
				t.Fatal(f.writes)
			}
			review, err := ReviewEdits(t.Context(), f, editTarget, "demo", ops)
			if err != nil || review.Items[0].Status != "matched" || len(f.writes) != n {
				t.Fatal(review, err, f.writes)
			}
		})
	}
}
func TestTDSEditInitialStalenessAndMissingSuffix(t *testing.T) {
	f := editFields("X", "Y")
	changes := []EditChange{{Op: "set-field-description", Field: "X", Description: "next"}, {Op: "set-table-description", Table: "Y", Description: "next table"}}
	preview, err := Edit(t.Context(), f, editTarget, "demo", changes, true, "")
	if err != nil {
		t.Fatal(err)
	}
	f.values["FieldDescriptionX"] = value.TDSElement{Name: "X", Description: new("")}
	out, err := Edit(t.Context(), f, editTarget, "demo", changes, false, preview.PreviewToken)
	if err == nil || len(f.writes) != 0 {
		t.Fatal(out, err)
	}
	changes[1] = EditChange{Op: "set-field-description", Field: "Missing", Description: "new"}
	out, err = Edit(t.Context(), f, editTarget, "demo", changes, false, "")
	if err == nil || len(f.writes) != 0 || out.Items[1].Status != "unreadable" {
		t.Fatal(out, err)
	}
	f.values["TableDescriptionY"] = value.TDSElement{Name: "Y", Description: new("matches")}
	changes = []EditChange{{Op: "set-field-description", Field: "X", Description: "next"}, {Op: "set-table-description", Table: "Y", Description: "matches"}}
	review, err := ReviewEdits(t.Context(), f, editTarget, "demo", changes)
	if err == nil || review.Items[0].Status != "different" || review.Items[1].Status != "matched" {
		t.Fatal(review, err)
	}
}

type refusal struct {
	*editFixture
	status string
}

func (r refusal) EditTDSElement(_ context.Context, _ string, _ string, _ value.TDSElement) (value.TDSReceipt, error) {
	return value.TDSReceipt{Status: r.status, TableauRequestID: "request-1", Failure: &value.TDSStatus{Code: 403}}, errors.New("insufficient rights")
}
func TestTDSEditRejectionRetainsReceipt(t *testing.T) {
	for _, tc := range []struct{ wire, want string }{{"rejected", "rejected"}, {"not_attempted", "unattempted"}} {
		client := refusal{editFields("X", "Y"), tc.wire}
		out, err := Edit(t.Context(), client, editTarget, "test", []EditChange{{Op: "set-field-description", Field: "X", Description: "amount"}, {Op: "set-field-description", Field: "Y", Description: "later"}}, false, "")
		if err == nil || out.Items[0].Status != tc.want || out.Items[0].Failure.Code != 403 || out.Items[0].TableauRequestID != "request-1" || out.Items[1].Status != "unattempted" {
			t.Fatal(out, err)
		}
	}
}
