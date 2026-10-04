package search

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type serviceSource struct{ input Input }

func (s *serviceSource) Search(_ context.Context, input Input, _ []string) (Result, error) {
	s.input = input
	return Result{}, nil
}

type serviceProvider struct {
	events                          []string
	session                         Session
	targetErr, sourceErr, policyErr error
}

func (p *serviceProvider) CheckCapability(id string) error {
	p.events = append(p.events, id)
	return p.policyErr
}

func (p *serviceProvider) Target(string) (Target, error) {
	p.events = append(p.events, "target")
	return p.session.Target, p.targetErr
}

func (p *serviceProvider) Cached(string) (Session, error) {
	p.events = append(p.events, "cache")
	return p.session, p.sourceErr
}

func (p *serviceProvider) Complete(string) (Session, error) {
	p.events = append(p.events, "complete")
	return p.session, p.sourceErr
}

func (p *serviceProvider) Live(context.Context, string) (Session, error) {
	p.events = append(p.events, "live")
	return p.session, p.sourceErr
}

func TestServiceSelectsSourceAndPreservesTargetRules(t *testing.T) {
	for _, tc := range []struct {
		name, terms, wantRoute, wantSite string
		cache                            bool
	}{
		{name: "cache", terms: "Sales", cache: true, wantRoute: "cache", wantSite: "requested"},
		{name: "complete", wantRoute: "complete", wantSite: ""},
		{name: "live", terms: "Sales", wantRoute: "live", wantSite: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &serviceSource{}
			provider := &serviceProvider{session: Session{Target: Target{Environment: "canonical", Site: ""}, Source: source}}
			_, err := New(provider).Execute(t.Context(), Input{Type: "workbook", Terms: tc.terms, Cache: tc.cache, Environment: "alias", Site: "requested", Limit: 1})
			if err != nil || !slices.Equal(provider.events, []string{tc.wantRoute}) {
				t.Fatalf("events=%v error=%v", provider.events, err)
			}
			if source.input.Environment != "canonical" || source.input.Site != tc.wantSite || !source.input.SiteResolved {
				t.Fatalf("source target=%+v", source.input)
			}
		})
	}
}

func TestServiceRejectsBeforeSourceConstruction(t *testing.T) {
	denied := errors.New("policy denied")
	for _, tc := range []struct {
		name      string
		input     Input
		policyErr error
		want      []string
	}{
		{name: "invalid type", input: Input{Type: "invalid"}},
		{name: "administrative prerequisite", input: Input{Type: "admin"}, policyErr: denied, want: []string{"admin.group.list"}},
		{name: "invalid cursor", input: Input{Type: "workbook", Cursor: "invalid", Limit: 1}, want: []string{"target"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &serviceProvider{policyErr: tc.policyErr, session: Session{Target: Target{Environment: "canonical"}}}
			_, err := New(provider).Execute(t.Context(), tc.input)
			if err == nil || !slices.Equal(provider.events, tc.want) {
				t.Fatalf("events=%v error=%v", provider.events, err)
			}
			if tc.policyErr != nil && !errors.Is(err, denied) {
				t.Fatalf("lost policy error: %v", err)
			}
		})
	}
}

func TestServiceSetupErrorsRetainResolvedDefaultSite(t *testing.T) {
	cause := errors.New("provider unavailable")
	for _, terms := range []string{"", "Sales"} {
		provider := &serviceProvider{session: Session{Target: Target{Environment: "canonical"}}, sourceErr: cause}
		_, err := New(provider).Execute(t.Context(), Input{Type: "workbook", Terms: terms, Environment: "alias", Site: "wrong"})
		failure, ok := errors.AsType[*errs.Error](err)
		if !ok || failure.ID != "search.setup" || failure.Environment != "canonical" || failure.Site != "" ||
			failure.Phase != errs.PhaseSetup || failure.Outcome != errs.OutcomeNotAttempted || !errors.Is(err, cause) {
			t.Fatalf("error=%+v", failure)
		}
	}
}
