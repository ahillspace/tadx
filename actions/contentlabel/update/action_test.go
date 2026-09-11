package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type labelStub struct {
	item                  value.ContentLabel
	writes                int
	readbackErr, conflict bool
	reads                 int
	internal              bool
	writeErr              error
}

func (s *labelStub) GetLabel(context.Context, string) (value.ContentLabel, error) {
	s.reads++
	if s.writes > 0 && s.readbackErr {
		return value.ContentLabel{}, errors.New("readback unavailable")
	}
	v := s.item
	if s.conflict && s.reads > 1 {
		v.Message = "concurrent change"
	}
	return v, nil
}
func (s *labelStub) GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error) {
	if s.item.LUID == "" {
		return nil, nil
	}
	return []value.ContentLabel{s.item}, nil
}
func (s *labelStub) GetLabelValue(_ context.Context, n string) (value.LabelValue, error) {
	return value.LabelValue{Name: n, Internal: s.internal, ElevatedDefault: true}, nil
}
func (s *labelStub) SetLabel(_ context.Context, target value.LabelTarget, u value.LabelUpdate) (value.ContentLabel, error) {
	s.writes++
	s.item = value.ContentLabel{LUID: "label-1", Type: target.Type, TargetLUID: target.LUID, Value: u.Value, Message: u.Message, Active: u.Active, Elevated: u.Elevated}
	return s.item, s.writeErr
}
func (s *labelStub) UpdateLabel(ctx context.Context, _ string, u value.LabelUpdate) (value.ContentLabel, error) {
	return s.SetLabel(ctx, value.LabelTarget{Type: s.item.Type, LUID: s.item.TargetLUID}, u)
}
func fixture() *labelStub {
	return &labelStub{item: value.ContentLabel{LUID: "label-1", Type: "table", TargetLUID: "table-1", Value: "Warning", Message: "old", Active: true, Elevated: true}}
}

type acknowledgedWriteError struct{}

func (acknowledgedWriteError) Error() string           { return "Write acknowledged but response invalid" }
func (acknowledgedWriteError) WriteAcknowledged() bool { return true }

func TestWriteResponseFailureDistinguishesAcknowledgementFromTransport(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		s := fixture()
		s.writeErr = errors.New("connection lost")
		if confirmed {
			s.writeErr = acknowledgedWriteError{}
		}
		msg := "new"
		out, e := New(s, s).Execute(context.Background(), Input{Environment: "test", TargetResolved: true, ID: "label-1", Message: &msg}, false)
		var classified *errs.Error
		if !errors.As(e, &classified) || out.Result == nil || s.writes != 1 {
			t.Fatalf("missing outcome: %+v %v", out, e)
		}
		want := errs.OutcomeUnknown
		status := "unknown"
		if confirmed {
			want = errs.OutcomeConfirmed
			status = "verification_pending"
		}
		if classified.Outcome != want || out.Result.Status != status {
			t.Fatalf("confirmed=%v: %+v %v", confirmed, out, e)
		}
	}
}

func TestPreviewPreservesOmittedFlagsAndExplicitClear(t *testing.T) {
	s := fixture()
	empty := ""
	in := Input{Environment: "test", TargetResolved: true, ID: "label-1", Message: &empty}
	out, e := New(s, s).Execute(context.Background(), in, true)
	if e != nil || s.writes != 0 || out.Plan.Desired.Message != "" || !out.Plan.Desired.Active || !out.Plan.Desired.Elevated {
		t.Fatalf("preview: %+v %v", out, e)
	}
}
func TestAcknowledgedWriteSurvivesReadbackFailure(t *testing.T) {
	s := fixture()
	s.readbackErr = true
	msg := "new"
	out, e := New(s, s).Execute(context.Background(), Input{Environment: "test", TargetResolved: true, ID: "label-1", Message: &msg}, false)
	var classified *errs.Error
	if !errors.As(e, &classified) || classified.Outcome != errs.OutcomeConfirmed || out.Result == nil || out.Result.Item.LUID != "label-1" || s.writes != 1 {
		t.Fatalf("receipt: %+v %v", out, e)
	}
}
func TestConflictAndSystemManagedValueNeverWrite(t *testing.T) {
	for _, mode := range []string{"conflict", "internal"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture()
			s.conflict = mode == "conflict"
			s.internal = mode == "internal"
			msg := "new"
			_, e := New(s, s).Execute(context.Background(), Input{Environment: "test", TargetResolved: true, ID: "label-1", Message: &msg}, false)
			if e == nil || s.writes != 0 {
				t.Fatalf("unsafe write: %v", e)
			}
		})
	}
}
func TestNewAttachmentDefaultsAndExplicitEnvironment(t *testing.T) {
	s := &labelStub{}
	name := "Warning"
	in := Input{Environment: "test", TargetResolved: true, Type: "table", TargetID: "table-1", Value: &name}
	out, e := New(s, s).Execute(context.Background(), in, false)
	if e != nil || out.Result == nil || !out.Result.Item.Active || !out.Result.Item.Elevated {
		t.Fatalf("create: %+v %v", out, e)
	}
	in.TargetResolved = false
	before := s.writes
	if _, e := New(s, s).Execute(context.Background(), in, false); e == nil || s.writes != before {
		t.Fatal("unresolved write permitted")
	}
}
