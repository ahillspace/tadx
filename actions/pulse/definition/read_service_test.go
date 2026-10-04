package definition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/readsource"
)

type definitionReadPort struct {
	page      ListPage
	item      Definition
	listError error
	published int
	source    *readsource.Metadata
}

func (p *definitionReadPort) ListDefinitions(context.Context, ListPageRequest) (ListPage, error) {
	return p.page, p.listError
}

func (p *definitionReadPort) GetDefinition(context.Context, string) (Definition, error) {
	return p.item, nil
}

func (p *definitionReadPort) Source() *readsource.Metadata { return p.source }
func (p *definitionReadPort) Publish()                     { p.published++ }

type definitionReadProvider struct {
	port        *definitionReadPort
	cacheCalls  int
	openCalls   int
	openedAlias string
}

func (p *definitionReadProvider) CacheTarget(string) (ReadTarget, error) {
	p.cacheCalls++
	return ReadTarget{Environment: "canonical", Site: "sales"}, nil
}

func (p *definitionReadProvider) CachedList(ReadTarget) CachedListPort {
	return p.port
}

func (p *definitionReadProvider) CachedInspect(ReadTarget) CachedInspectPort {
	return p.port
}

func (p *definitionReadProvider) Open(_ context.Context, alias, _, _ string) (ReadSession, error) {
	p.openCalls++
	p.openedAlias = alias
	return ReadSession{ReadTarget: ReadTarget{Environment: "canonical", Site: "sales"}, List: p.port, Inspect: p.port}, nil
}

func (p *definitionReadProvider) CacheSetupError(_, _ string, err error) error { return err }
func (p *definitionReadProvider) Now() time.Time {
	return time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
}

func TestDefinitionServiceRejectsLocalInputsBeforePortAcquisition(t *testing.T) {
	provider := &definitionReadProvider{port: &definitionReadPort{}}
	service := New(Ports{Read: provider})
	if _, err := service.ListPulseDefinitions(t.Context(), ListInput{Limit: -1}); err == nil {
		t.Fatal("invalid limit was accepted")
	}
	if _, err := service.InspectPulseDefinition(t.Context(), InspectInput{}); err == nil {
		t.Fatal("missing exact LUID was accepted")
	}
	if provider.cacheCalls != 0 || provider.openCalls != 0 {
		t.Fatalf("invalid input acquired ports: cache=%d open=%d", provider.cacheCalls, provider.openCalls)
	}
}

func TestDefinitionServiceBindsContinuationBeforeNativeOpen(t *testing.T) {
	provider := &definitionReadProvider{port: &definitionReadPort{page: ListPage{NextPageToken: "opaque"}}}
	service := New(Ports{Read: provider})
	first, err := service.ListPulseDefinitions(t.Context(), ListInput{Environment: "alias", Limit: 7})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "sales" {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	if provider.port.published != 1 {
		t.Fatalf("successful native page publication = %d", provider.port.published)
	}
	_, err = service.ListPulseDefinitions(t.Context(), ListInput{Environment: "alias", Limit: 8, Cursor: first.Page.NextCursor})
	if err == nil {
		t.Fatal("cursor changed limit was accepted")
	}
	if provider.openCalls != 1 {
		t.Fatalf("mismatched cursor opened native provider %d times", provider.openCalls)
	}
}

func TestDefinitionServiceDoesNotPublishFailedNativeRead(t *testing.T) {
	provider := &definitionReadProvider{port: &definitionReadPort{listError: errors.New("upstream failed")}}
	_, err := New(Ports{Read: provider}).ListPulseDefinitions(t.Context(), ListInput{Environment: "alias"})
	if err == nil || provider.port.published != 0 {
		t.Fatalf("failed read = %v; publication count = %d", err, provider.port.published)
	}
}
