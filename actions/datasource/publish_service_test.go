package datasource

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

func (p *publishServiceProvider) OpenDatasourcePublishSource(_ context.Context, input PublishInput) (PublishInput, ArtifactReader, string, error) {
	p.calls = append(p.calls, "source")
	return input, nil, "path", nil
}

func (p *publishServiceProvider) OpenDatasourcePublish(context.Context, PublishInput, string) (PublishSession, error) {
	p.calls = append(p.calls, "target")
	return PublishSession{}, p.err
}

func TestPublishDatasourceValidatesBeforeSourceAndOpensTargetAfterSource(t *testing.T) {
	sentinel := errors.New("stop at target")
	provider := &publishServiceProvider{err: sentinel}
	service := New(Ports{Publish: provider})
	if _, err := service.PublishDatasource(t.Context(), PublishInput{}, false); err == nil || len(provider.calls) != 0 {
		t.Fatalf("invalid input reached provider: calls=%v err=%v", provider.calls, err)
	}
	_, err := service.PublishDatasource(t.Context(), PublishInput{File: "datasource.tds", Mode: ModeCreate}, true)
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(provider.calls, []string{"source", "target"}) {
		t.Fatalf("calls=%v err=%v", provider.calls, err)
	}
}
