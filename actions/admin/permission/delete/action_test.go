package delete_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/permission/delete"
	"github.com/ahillspace/tadx/internal/errs"
)

type fake struct {
	reads, writes int
	changed       bool
	source, mode  string
	writeErr      error
	status        string
	postMode      *string
}

func TestProjectDefaultPreviewRetainsContentKind(t *testing.T) {
	in := input()
	in.ResourceKind, in.ResourceLUID, in.DefaultFor = "project", "p1", "flows"
	f := &fake{source: "default", mode: "Allow"}
	out, err := action.New(f, f).Execute(context.Background(), in, true)
	if err != nil || out.Plan.Target.DefaultFor != "flows" || !strings.Contains(out.Help[0], "--default-for flows") || f.writes != 0 {
		t.Fatalf("out=%+v err=%v writes=%d", out, err, f.writes)
	}
}

func (f *fake) GetPermission(_ context.Context, in action.Input) (action.Snapshot, error) {
	f.reads++
	mode := f.mode
	if f.writes > 0 {
		mode = ""
	}
	if f.writes > 0 && f.postMode != nil {
		mode = *f.postMode
	}
	if f.changed && f.reads > 1 {
		mode = "Deny"
	}
	return action.Snapshot{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, Source: f.source, Mode: mode}, nil
}
func (f *fake) DeletePermission(_ context.Context, in action.Input) (action.Result, error) {
	f.writes++
	return action.Result{Status: f.status, ResourceLUID: in.ResourceLUID, TableauRequestID: "request-1"}, f.writeErr
}
func input() action.Input {
	return action.Input{Environment: "dev", Site: "site", ResourceKind: "workbook", ResourceLUID: "w1", PrincipalType: "group", PrincipalLUID: "g1", Capability: "Read", Mode: "Allow"}
}
func TestPreviewAndExecution(t *testing.T) {
	for _, preview := range []bool{true, false} {
		f := &fake{source: "direct", mode: "Allow", status: "deleted"}
		out, err := action.New(f, f).Execute(context.Background(), input(), preview)
		if err != nil {
			t.Fatal(err)
		}
		if preview {
			if f.reads != 1 || f.writes != 0 || out.Result != nil {
				t.Fatalf("preview=%+v fake=%+v", out, f)
			}
		} else {
			if f.reads != 3 || f.writes != 1 || out.Result == nil || out.Result.Status != "deleted" || out.Result.Rule != nil {
				t.Fatalf("execute=%+v fake=%+v", out, f)
			}
		}
	}
}
func TestRejectsChangedAndInheritedState(t *testing.T) {
	for _, f := range []*fake{{source: "direct", mode: "Allow", changed: true}, {source: "inherited", mode: "Allow"}, {source: "unknown", mode: "Allow"}} {
		_, err := action.New(f, f).Execute(context.Background(), input(), false)
		var structured *errs.Error
		if !errors.As(err, &structured) || f.writes != 0 {
			t.Fatalf("err=%v fake=%+v", err, f)
		}
	}
}
func TestRequiresExplicitSelectors(t *testing.T) {
	for _, field := range []string{"environment", "kind", "id", "principal-type", "principal-id", "capability", "mode"} {
		in := input()
		switch field {
		case "environment":
			in.Environment = ""
		case "kind":
			in.ResourceKind = "view"
		case "id":
			in.ResourceLUID = " "
		case "principal-type":
			in.PrincipalType = "name"
		case "principal-id":
			in.PrincipalLUID = ""
		case "capability":
			in.Capability = ""
		case "mode":
			in.Mode = "allow"
		}
		f := &fake{source: "direct"}
		_, err := action.New(f, f).Execute(context.Background(), in, true)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || f.reads != 0 {
			t.Fatalf("%s err=%v fake=%+v", field, err, f)
		}
	}
}
func TestUnknownOutcomeRetainsRequestID(t *testing.T) {
	f := &fake{source: "direct", mode: "Allow", status: "unknown", writeErr: errors.New("response incomplete")}
	_, err := action.New(f, f).Execute(context.Background(), input(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "admin.permission.delete.outcome_unknown" || structured.TableauRequestID != "request-1" || *structured.Retryable {
		t.Fatalf("err=%+v", err)
	}
}

func TestAcknowledgedDeleteReportsVerificationFailureWithObservedRule(t *testing.T) {
	mode := "Allow"
	f := &fake{source: "direct", mode: "Allow", status: "deleted", postMode: &mode}
	out, err := action.New(f, f).Execute(context.Background(), input(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeConfirmed || structured.TableauRequestID != "request-1" || out.Result == nil || out.Result.Rule == nil || out.Result.Rule.Mode != mode {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
