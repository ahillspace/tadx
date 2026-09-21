package list

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type readerStub struct {
	items []value.ContentLabel
	calls int
}

func (s *readerStub) GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error) {
	s.calls++
	return s.items, nil
}
func TestValidationBeforeRead(t *testing.T) {
	r := &readerStub{}
	_, e := New(r).Execute(context.Background(), Input{Limit: -1})
	if e == nil || r.calls != 0 {
		t.Fatalf("invalid input contacted reader: %v", e)
	}
}
func TestBoundedOrderedListAndDuplicateRejection(t *testing.T) {
	v := value.ContentLabel{LUID: "label-1", Type: "table", TargetLUID: "table-1", Value: "Warning"}
	r := &readerStub{items: []value.ContentLabel{v}}
	out, e := New(r).Execute(context.Background(), Input{Type: "table", TargetID: "table-1"})
	if e != nil || out.Returned != 1 {
		t.Fatalf("list: %+v %v", out, e)
	}
	r.items = append(r.items, v)
	if _, e := New(r).Execute(context.Background(), Input{Type: "table", TargetID: "table-1"}); e == nil {
		t.Fatal("duplicate identity accepted")
	}
	r.items = r.items[:1]
	for i := 0; i < 21; i++ {
		copy := v
		copy.LUID = string(rune('a' + i))
		r.items = append(r.items, copy)
	}
	out, e = New(r).Execute(context.Background(), Input{Type: "table", TargetID: "table-1"})
	if e != nil || out.Returned != 20 || !out.MoreAvailable {
		t.Fatalf("limit: %+v %v", out, e)
	}
}

func TestAllReturnsCompleteInventory(t *testing.T) {
	items := make([]value.ContentLabel, 25)
	for i := range items {
		items[i] = value.ContentLabel{LUID: string(rune('a' + i)), Type: "table", TargetLUID: "table-1"}
	}
	out, err := New(&readerStub{items: items}).Execute(t.Context(), Input{Type: "table", TargetID: "table-1", All: true})
	if err != nil || len(out.Items) != len(items) || out.Returned != len(items) || out.Total != len(items) || out.MoreAvailable || out.NextCommand != "" {
		t.Fatalf("all: %+v %v", out, err)
	}
}

func TestAllRejectsLimit(t *testing.T) {
	if _, err := New(&readerStub{}).Execute(t.Context(), Input{Type: "table", TargetID: "table-1", All: true, Limit: 1}); err == nil {
		t.Fatal("--all accepted with --limit")
	}
}

type failingReader struct{}

func (failingReader) GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error) {
	return nil, errors.New("fixture read failed")
}

func TestFailureRetainsRequestedTargetWithoutFabricatedItems(t *testing.T) {
	out, err := New(failingReader{}).Execute(t.Context(), Input{Type: "table", TargetID: "table-1", Categories: []string{"warning"}})
	if err == nil || out.Target.Type != "table" || out.Target.TargetID != "table-1" {
		t.Fatalf("failure lost requested target: %+v %v", out, err)
	}
	encoded, marshalErr := json.Marshal(out.CompactOutput())
	if marshalErr != nil || string(encoded) == "" || string(encoded) != `{"status":"listed","target":{"type":"table","target_id":"table-1","categories":["warning"]},"returned":0,"more_available":false,"total":0}` {
		t.Fatalf("failure fabricated or changed compact payload: %s (%v)", encoded, marshalErr)
	}
}

func TestNativeDatasourceTypeIsCanonicalized(t *testing.T) {
	r := &readerStub{items: []value.ContentLabel{{LUID: "label-1", Type: "DATASOURCE", TargetLUID: "source-1", Value: "Warning"}}}
	out, err := New(r).Execute(t.Context(), Input{Type: "datasource", TargetID: "source-1"})
	if err != nil || len(out.Items) != 1 || out.Items[0].Type != "datasource" {
		t.Fatalf("native datasource type rejected: %+v %v", out, err)
	}
}
