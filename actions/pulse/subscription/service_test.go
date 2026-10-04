package subscription

import (
	"context"
	"errors"
	"testing"
	"time"
)

type serviceProvider struct {
	session                             Session
	err                                 error
	calls                               int
	requestedEnvironment, requestedSite string
}

func (p *serviceProvider) Open(_ context.Context, environment, site string) (Session, error) {
	p.calls++
	p.requestedEnvironment, p.requestedSite = environment, site
	return p.session, p.err
}

type serviceReader struct {
	*reader
	users []string
}

func (r *serviceReader) ListUserSubscriptions(ctx context.Context, user string, request PageRequest) (Page, error) {
	r.users = append(r.users, user)
	return r.reader.ListUserSubscriptions(ctx, user, request)
}

func TestServiceRejectsInvalidLocalOptionsBeforeProvider(t *testing.T) {
	for _, input := range []Input{{Limit: -1}, {Limit: maximumLimit + 1}, {All: true, Limit: 1}, {All: true, Cursor: "cursor"}} {
		provider := &serviceProvider{}
		if _, err := New(provider, nil).ListPulseSubscriptions(t.Context(), input); err == nil || provider.calls != 0 {
			t.Fatalf("input=%+v err=%v provider calls=%d", input, err, provider.calls)
		}
	}
}

func TestServiceBindsAuthenticatedIdentityAndCanonicalCursor(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := &serviceReader{reader: &reader{pages: []Page{
		{Subscriptions: []Subscription{{LUID: "subscription-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "authenticated-user"}}, NextPageToken: "next"},
		{},
	}}}
	provider := &serviceProvider{session: Session{Environment: "canonical", Site: "canonical-site", UserLUID: "authenticated-user", Reader: r}}
	service := New(provider, func() time.Time { return now })
	input := Input{Environment: "requested-alias", Site: "requested-site", UserLUID: "untrusted-user", Limit: 1}
	output, err := service.ListPulseSubscriptions(t.Context(), input)
	if err != nil || output.Environment != "canonical" || output.Site != "canonical-site" || output.UserLUID != "authenticated-user" || output.Coverage.NextCursor == "" {
		t.Fatalf("output=%+v err=%v", output, err)
	}
	if provider.requestedEnvironment != "requested-alias" || provider.requestedSite != "requested-site" || len(r.users) != 1 || r.users[0] != "authenticated-user" {
		t.Fatalf("provider=%+v users=%v", provider, r.users)
	}
	if output.Source == nil || output.Source.Mode != "tableau" || output.Source.ObservedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("source=%+v", output.Source)
	}
	input.Cursor = output.Coverage.NextCursor
	if _, err := service.ListPulseSubscriptions(t.Context(), input); err != nil || len(r.queries) != 2 || r.queries[1].PageToken != "next" {
		t.Fatalf("canonical continuation err=%v queries=%+v", err, r.queries)
	}
	provider.session.UserLUID = "different-user"
	if _, err := service.ListPulseSubscriptions(t.Context(), input); err == nil || len(r.queries) != 2 {
		t.Fatalf("foreign continuation err=%v queries=%+v", err, r.queries)
	}
}

func TestServicePreservesSetupAndReadFailures(t *testing.T) {
	want := errors.New("unavailable")
	provider := &serviceProvider{err: want}
	output, err := New(provider, nil).ListPulseSubscriptions(t.Context(), Input{})
	if !errors.Is(err, want) || output.Source != nil {
		t.Fatalf("setup output=%+v err=%v", output, err)
	}
	provider.err = nil
	provider.session = Session{Environment: "canonical", Site: "site", UserLUID: "user", Reader: &reader{pageErr: want}}
	output, err = New(provider, nil).ListPulseSubscriptions(t.Context(), Input{})
	if !errors.Is(err, want) || output.Status != "partial" || output.Source == nil || output.Source.Mode != "tableau" {
		t.Fatalf("read output=%+v err=%v", output, err)
	}
}
