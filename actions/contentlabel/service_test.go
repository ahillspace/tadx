package contentlabel

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type serviceOpen struct {
	environment, operation string
	explicit               bool
}

type serviceProvider struct {
	session Session
	err     error
	opens   []serviceOpen
}

func (p *serviceProvider) Open(_ context.Context, environment, operation string, explicit bool) (Session, error) {
	p.opens = append(p.opens, serviceOpen{environment, operation, explicit})
	return p.session, p.err
}

type serviceLabelPorts struct {
	Ports
	label value.ContentLabel
}

func (p serviceLabelPorts) GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error) {
	return []value.ContentLabel{p.label}, nil
}

func (p serviceLabelPorts) GetLabel(context.Context, string) (value.ContentLabel, error) {
	return p.label, nil
}

func (p serviceLabelPorts) GetLabelValue(context.Context, string) (value.LabelValue, error) {
	return value.LabelValue{Name: p.label.Value}, nil
}

func TestServiceRejectsInvalidInputBeforePolicyAndSetup(t *testing.T) {
	provider := &serviceProvider{}
	checks := 0
	service := New(provider, func() error { checks++; return nil })
	operations := []func() error{
		func() error { _, err := service.ListLabels(t.Context(), ListInput{}); return err },
		func() error { _, err := service.InspectLabel(t.Context(), InspectInput{}); return err },
		func() error { _, err := service.UpdateLabel(t.Context(), UpdateInput{}, true); return err },
		func() error { _, err := service.DeleteLabel(t.Context(), DeleteInput{}, true); return err },
	}
	for index, operation := range operations {
		if err := operation(); err == nil || checks != 0 || len(provider.opens) != 0 {
			t.Fatalf("operation=%d err=%v checks=%d opens=%v", index, err, checks, provider.opens)
		}
	}
}

func TestServiceChecksDefinitionAccessBeforePreviewOrMutationSetup(t *testing.T) {
	want := errors.New("definition access denied")
	provider := &serviceProvider{}
	checks := 0
	service := New(provider, func() error { checks++; return want })
	for _, preview := range []bool{true, false} {
		if _, err := service.UpdateLabel(t.Context(), UpdateInput{ID: "label", Message: new("new")}, preview); !errors.Is(err, want) {
			t.Fatalf("update preview=%v err=%v", preview, err)
		}
		if _, err := service.DeleteLabel(t.Context(), DeleteInput{ID: "label"}, preview); !errors.Is(err, want) {
			t.Fatalf("delete preview=%v err=%v", preview, err)
		}
	}
	if checks != 4 || len(provider.opens) != 0 {
		t.Fatalf("checks=%d opens=%v", checks, provider.opens)
	}
}

func TestServiceBindsCanonicalTargetAndKeepsPreviewReadOnly(t *testing.T) {
	label := value.ContentLabel{LUID: "label", TargetLUID: "target", Type: "table", Value: "quality", Message: "before", Active: true}
	// The embedded write port is nil: any write during preview fails the test.
	provider := &serviceProvider{session: Session{Environment: "canonical", Site: "site", Ports: serviceLabelPorts{label: label}}}
	checks := 0
	service := New(provider, func() error { checks++; return nil })
	listed, err := service.ListLabels(t.Context(), ListInput{Environment: "alias", Site: "untrusted", Type: "table", TargetID: "target"})
	if err != nil || listed.Environment != "canonical" || listed.Site != "site" || listed.Returned != 1 {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	inspected, err := service.InspectLabel(t.Context(), InspectInput{Environment: "alias", Site: "untrusted", ID: "label"})
	if err != nil || inspected.Environment != "canonical" || inspected.Site != "site" || inspected.Item == nil || inspected.Item.LUID != "label" || checks != 0 {
		t.Fatalf("inspected=%+v checks=%d err=%v", inspected, checks, err)
	}
	updated, err := service.UpdateLabel(t.Context(), UpdateInput{Environment: "alias", Site: "untrusted", ID: "label", Message: new("")}, true)
	if err != nil || updated.Plan == nil || updated.Plan.Environment != "canonical" || updated.Plan.Site != "site" || updated.Plan.Desired.Message != "" || !updated.Plan.Desired.Active || updated.Result != nil {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	deleted, err := service.DeleteLabel(t.Context(), DeleteInput{Environment: "alias", Site: "untrusted", ID: "label"}, true)
	if err != nil || deleted.Environment != "canonical" || deleted.Site != "site" || deleted.Mode != "preview" || checks != 2 {
		t.Fatalf("deleted=%+v checks=%d err=%v", deleted, checks, err)
	}
	want := []serviceOpen{{"alias", "content.label.list", false}, {"alias", "content.label.inspect", false}, {"alias", "content.label.update", true}, {"alias", "content.label.delete", true}}
	if !reflect.DeepEqual(provider.opens, want) {
		t.Fatalf("opens=%+v want=%+v", provider.opens, want)
	}
}

func TestServicePreservesProviderFailure(t *testing.T) {
	want := errors.New("setup failure")
	provider := &serviceProvider{err: want}
	service := New(provider, func() error { return nil })
	operations := []func() error{
		func() error {
			_, err := service.ListLabels(t.Context(), ListInput{Type: "table", TargetID: "target"})
			return err
		},
		func() error { _, err := service.InspectLabel(t.Context(), InspectInput{ID: "label"}); return err },
		func() error {
			_, err := service.UpdateLabel(t.Context(), UpdateInput{ID: "label", Message: new("new")}, false)
			return err
		},
		func() error { _, err := service.DeleteLabel(t.Context(), DeleteInput{ID: "label"}, false); return err },
	}
	for index, operation := range operations {
		if err := operation(); !errors.Is(err, want) {
			t.Fatalf("operation=%d err=%v", index, err)
		}
	}
}
