package last

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type savedReader struct {
	record value.SavedExecution
	err    error
	calls  int
}

func (r *savedReader) Read(context.Context) (value.SavedExecution, error) {
	r.calls++
	return r.record, r.err
}

func TestReadLastChecksOperationThenRecordedPrerequisites(t *testing.T) {
	record := value.SavedExecution{Operation: "workbook.publish", RequiredCapabilities: []string{"project.inspect", "workbook.inspect"}, Result: json.RawMessage(`{"status":"published"}`)}
	reader := &savedReader{record: record}
	var checked []string
	service := New(reader.Read, func(id string) error { checked = append(checked, id); return nil })
	out, err := service.ReadLast(t.Context())
	if err != nil || !reflect.DeepEqual(out, record) || reader.calls != 1 || !reflect.DeepEqual(checked, []string{"workbook.publish", "project.inspect", "workbook.inspect"}) {
		t.Fatalf("out=%+v reads=%d checks=%v err=%v", out, reader.calls, checked, err)
	}
}

func TestReadLastDenialNeverExposesSavedPayload(t *testing.T) {
	for _, denied := range []string{"workbook.publish", "project.inspect", "workbook.inspect"} {
		t.Run(denied, func(t *testing.T) {
			want := errors.New("denied")
			reader := &savedReader{record: value.SavedExecution{Operation: "workbook.publish", RequiredCapabilities: []string{"project.inspect", "workbook.inspect"}, Result: json.RawMessage(`{"identity":"restricted"}`)}}
			var checked []string
			service := New(reader.Read, func(id string) error {
				checked = append(checked, id)
				if id == denied {
					return want
				}
				return nil
			})
			out, err := service.ReadLast(t.Context())
			failure, ok := errors.AsType[*errs.Error](err)
			if !errors.Is(err, want) || !ok || failure.ID != "last.unavailable" || !reflect.DeepEqual(out, Output{}) || checked[len(checked)-1] != denied {
				t.Fatalf("out=%+v checks=%v err=%v", out, checked, err)
			}
		})
	}
}

func TestReadLastPreservesReaderFailureBeforeAuthorization(t *testing.T) {
	want := errors.New("unreadable")
	partial := value.SavedExecution{Operation: "last-incomplete"}
	reader := &savedReader{record: partial, err: want}
	checks := 0
	out, err := New(reader.Read, func(string) error { checks++; return nil }).ReadLast(t.Context())
	if !errors.Is(err, want) || !reflect.DeepEqual(out, partial) || checks != 0 || reader.calls != 1 {
		t.Fatalf("out=%+v checks=%d reads=%d err=%v", out, checks, reader.calls, err)
	}
}

func TestReadLastAuthorizesLegacySearchRowsNotOwnerFields(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation string
		payload   string
		want      []string
	}{
		{"top-level-and-partial", "search.run", `{"items":[{"type":"user"},{"type":"group"},{"type":"user"}],"output":{"items":[{"type":"group"}]}}`, []string{"search.run", "admin.group.list", "admin.user.list"}},
		{"partial-only", "search.run", `{"output":{"items":[{"type":"user"}]}}`, []string{"search.run", "admin.user.list"}},
		{"owner-fields", "search.run", `{"items":[{"type":"workbook","owner":{"type":"user"}}]}`, []string{"search.run"}},
		{"malformed", "search.run", `{`, []string{"search.run"}},
		{"not-search", "workbook.list", `{"items":[{"type":"user"}]}`, []string{"workbook.list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &savedReader{record: value.SavedExecution{Operation: tc.operation, Result: json.RawMessage(tc.payload)}}
			var checked []string
			_, err := New(reader.Read, func(id string) error { checked = append(checked, id); return nil }).ReadLast(t.Context())
			if err != nil || !reflect.DeepEqual(checked, tc.want) {
				t.Fatalf("checks=%v want=%v err=%v", checked, tc.want, err)
			}
		})
	}
}

func TestReadLastLegacySearchDenialDiscardsPayload(t *testing.T) {
	want := errors.New("legacy group access denied")
	reader := &savedReader{record: value.SavedExecution{Operation: "search.run", Result: json.RawMessage(`{"output":{"items":[{"type":"group"}]}}`)}}
	out, err := New(reader.Read, func(id string) error {
		if id == "admin.group.list" {
			return want
		}
		return nil
	}).ReadLast(t.Context())
	if !errors.Is(err, want) || !reflect.DeepEqual(out, Output{}) {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
