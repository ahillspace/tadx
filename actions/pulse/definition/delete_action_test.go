package definition

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type deleteBackend struct {
	targets     []DeleteDefinition
	calls       []string
	readError   error
	deleteError error
}

func (b *deleteBackend) GetDefinition(_ context.Context, luid string) (Definition, error) {
	b.calls = append(b.calls, "get:"+luid)
	if b.readError != nil {
		return Definition{}, b.readError
	}
	if len(b.targets) == 0 {
		return Definition{}, errors.New("missing target")
	}
	target := b.targets[0]
	b.targets = b.targets[1:]
	return Definition{LUID: target.LUID, Name: target.Name, DatasourceLUID: target.DatasourceLUID}, nil
}
func (b *deleteBackend) DeleteDefinition(_ context.Context, luid string) (DeleteResult, error) {
	b.calls = append(b.calls, "delete:"+luid)
	return DeleteResult{Status: "deleted", DefinitionLUID: luid, HTTPStatus: 204, TableauRequestID: "request-1"}, b.deleteError
}
func deleteInput() DeleteInput {
	return DeleteInput{Environment: "dev", Site: "sandbox", LUID: "definition-1"}
}
func deleteTarget() DeleteDefinition {
	return DeleteDefinition{LUID: "definition-1", Name: "Revenue"}
}

func TestDeleteRunsByDefaultWithExactRevalidation(t *testing.T) {
	b := &deleteBackend{targets: []DeleteDefinition{deleteTarget(), deleteTarget()}}
	output, err := deleteWorkflow(context.Background(), b, b, deleteInput())
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || output.Result.DefinitionLUID != "definition-1" {
		t.Fatalf("output=%#v", output)
	}
	want := []string{"get:definition-1", "get:definition-1", "delete:definition-1"}
	if !reflect.DeepEqual(b.calls, want) {
		t.Fatalf("calls=%v, want %v", b.calls, want)
	}
}
func TestDeletePreviewDoesNotDelete(t *testing.T) {
	b := &deleteBackend{targets: []DeleteDefinition{deleteTarget()}}
	in := deleteInput()
	in.Preview = true
	output, err := deleteWorkflow(context.Background(), b, b, in)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || !reflect.DeepEqual(b.calls, []string{"get:definition-1"}) {
		t.Fatalf("output=%#v calls=%v", output, b.calls)
	}
	if output.Plan.Mode != "preview" || len(output.Warnings) == 0 {
		t.Fatalf("preview=%#v", output)
	}
}
func TestDeleteRejectsWrongOrChangedIdentity(t *testing.T) {
	for _, targets := range [][]DeleteDefinition{
		{{LUID: "wrong"}},
		{deleteTarget(), {LUID: "wrong"}},
		{deleteTarget(), {}},
	} {
		b := &deleteBackend{targets: targets}
		_, err := deleteWorkflow(context.Background(), b, b, deleteInput())
		if err == nil {
			t.Fatal("expected exact identity failure")
		}
		for _, call := range b.calls {
			if call == "delete:definition-1" {
				t.Fatalf("deleted changed target: %v", b.calls)
			}
		}
	}
}
func TestDeleteRequiresExactSelector(t *testing.T) {
	for _, in := range []DeleteInput{
		{Environment: "dev", Site: "sandbox", LUID: "  "},
	} {
		b := &deleteBackend{}
		_, err := deleteWorkflow(context.Background(), b, b, in)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || len(b.calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, b.calls)
		}
	}
}
func TestDeletePreservesUpstreamFailureWithoutRetry(t *testing.T) {
	upstream := errors.New("upstream dependency rejection")
	b := &deleteBackend{targets: []DeleteDefinition{deleteTarget(), deleteTarget()}, deleteError: upstream}
	_, err := deleteWorkflow(context.Background(), b, b, deleteInput())
	if !errors.Is(err, upstream) || len(b.calls) != 3 {
		t.Fatalf("err=%v calls=%v", err, b.calls)
	}
}
func TestDeletePreservesMissingTarget(t *testing.T) {
	upstream := errors.New("upstream target not found")
	b := &deleteBackend{readError: upstream}
	_, err := deleteWorkflow(context.Background(), b, b, deleteInput())
	if !errors.Is(err, upstream) || len(b.calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, b.calls)
	}
}
