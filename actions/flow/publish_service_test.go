package flow

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

func (p *publishServiceProvider) OpenFlowPublishSource(_ context.Context, input PublishInput) (PublishInput, ArtifactReader, string, error) {
	p.calls = append(p.calls, "source")
	return input, nil, "path", nil
}

func (p *publishServiceProvider) OpenFlowPublish(context.Context, PublishInput, string) (PublishSession, error) {
	p.calls = append(p.calls, "target")
	return PublishSession{}, p.err
}

func TestPublishFlowValidatesBeforeSourceAndOpensTargetAfterSource(t *testing.T) {
	sentinel := errors.New("stop at target")
	provider := &publishServiceProvider{err: sentinel}
	service := New(Ports{Publish: provider})
	if _, err := service.PublishFlow(t.Context(), PublishInput{}, false); err == nil || len(provider.calls) != 0 {
		t.Fatalf("invalid input reached provider: calls=%v err=%v", provider.calls, err)
	}
	_, err := service.PublishFlow(t.Context(), PublishInput{File: "flow.tfl"}, true)
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(provider.calls, []string{"source", "target"}) {
		t.Fatalf("calls=%v err=%v", provider.calls, err)
	}
}
