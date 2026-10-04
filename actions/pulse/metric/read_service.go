package metric

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/readsource"
)

// ReadTarget identifies the canonical environment and exact site for one read.
type ReadTarget struct {
	Environment string
	Site        string
}

type CachedListPort interface {
	ListReader
	Source() *readsource.Metadata
}

type CachedInspectPort interface {
	InspectReader
	Source() *readsource.Metadata
}

type LiveListPort interface {
	ListReader
	Publish()
}

type LiveInspectPort interface {
	InspectReader
	Publish()
}

// ReadSession binds native read ports to the authenticated target.
type ReadSession struct {
	ReadTarget
	List    LiveListPort
	Inspect LiveInspectPort
}

// ReadProvider constructs ports only after local input validation.
type ReadProvider interface {
	CacheTarget(string) (ReadTarget, error)
	CachedList(ReadTarget) CachedListPort
	CachedInspect(ReadTarget) CachedInspectPort
	Open(context.Context, string, string, string) (ReadSession, error)
	CacheSetupError(string, string, error) error
	Now() time.Time
}

// Service owns Pulse metric operations and their target-bound observations.
type Service struct{ ports Ports }

type Ports struct {
	Read      ReadProvider
	Mutation  MutationProvider
	Followers FollowerProvider
}

func New(ports Ports) *Service { return &Service{ports: ports} }

// ListPulseMetrics selects cached or native listing after validating input.
func (s *Service) ListPulseMetrics(ctx context.Context, input ListInput) (ListOutput, error) {
	if err := listValidateInput(&input); err != nil {
		return ListOutput{}, err
	}
	var target ReadTarget
	if input.Cursor != "" || input.Cache {
		var err error
		target, err = s.ports.Read.CacheTarget(input.Environment)
		if err != nil {
			if input.Cache && input.Cursor == "" {
				return ListOutput{}, s.ports.Read.CacheSetupError("pulse.metric.list", input.Environment, err)
			}
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		if err := listValidateContinuation(input); err != nil {
			return ListOutput{}, err
		}
	}
	if input.Cache {
		reader := s.ports.Read.CachedList(target)
		output, err := listValidated(ctx, reader, input)
		if err == nil {
			output.Source = reader.Source()
		}
		return output, err
	}
	session, err := s.ports.Read.Open(ctx, input.Environment, input.Site, "pulse.metric.list")
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := listValidated(ctx, session.List, input)
	if err != nil {
		return output, err
	}
	output.Source = new(readsource.Live(s.ports.Read.Now()))
	session.List.Publish()
	return output, nil
}

// InspectPulseMetric selects one exact cached or native metric.
func (s *Service) InspectPulseMetric(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if err := inspectValidateInput(input); err != nil {
		return InspectOutput{}, err
	}
	if input.Cache {
		target, err := s.ports.Read.CacheTarget(input.Environment)
		if err != nil {
			return InspectOutput{}, s.ports.Read.CacheSetupError("pulse.metric.inspect", input.Environment, err)
		}
		input.Environment, input.Site = target.Environment, target.Site
		reader := s.ports.Read.CachedInspect(target)
		output, err := inspectValidated(ctx, reader, input)
		if err == nil {
			output.Source = reader.Source()
			output.RequestID = ""
			output.Metric.RequestID = ""
		}
		return output, err
	}
	session, err := s.ports.Read.Open(ctx, input.Environment, input.Site, "pulse.metric.inspect")
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := inspectValidated(ctx, session.Inspect, input)
	if err != nil {
		return output, err
	}
	output.Source = new(readsource.Live(s.ports.Read.Now()))
	session.Inspect.Publish()
	return output, nil
}
