package workbook

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type publishServiceProvider struct {
	calls []string
	err   error
}

func (p *publishServiceProvider) ResolveWorkbookPublishTarget(_ context.Context, _, _ string) (string, string, error) {
	p.calls = append(p.calls, "target")
	return "canonical", "site", nil
}

func (p *publishServiceProvider) OpenWorkbookPublishSource(_ context.Context, input PublishInput) (PublishInput, ArtifactReader, string, error) {
	p.calls = append(p.calls, "source")
	if input.Environment != "canonical" || input.Site != "site" || !input.TargetResolved {
		return input, nil, "", errors.New("source did not receive the canonical target")
	}
	return input, nil, "", p.err
}

func (p *publishServiceProvider) OpenWorkbookPublish(context.Context, string, string, string) (PublishSession, error) {
	p.calls = append(p.calls, "open")
	return PublishSession{}, p.err
}

func TestPublishWorkbookValidatesBeforeTargetAndResolvesTargetBeforeSource(t *testing.T) {
	sentinel := errors.New("stop after source")
	provider := &publishServiceProvider{err: sentinel}
	service := New(Ports{Publish: provider})
	if _, err := service.PublishWorkbook(t.Context(), PublishInput{}, false); err == nil || len(provider.calls) != 0 {
		t.Fatalf("invalid input reached provider: calls=%v err=%v", provider.calls, err)
	}
	_, err := service.PublishWorkbook(t.Context(), PublishInput{File: "workbook.twb"}, true)
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(provider.calls, []string{"target", "source"}) {
		t.Fatalf("calls=%v err=%v", provider.calls, err)
	}
}
