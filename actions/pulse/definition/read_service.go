package definition

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

// CachedListPort reads cached pages and exposes their observed coverage.
type CachedListPort interface {
	ListReader
	Source() *readsource.Metadata
}

// CachedInspectPort reads one cached record and exposes its observed coverage.
type CachedInspectPort interface {
	InspectReader
	Source() *readsource.Metadata
}

// LiveListPort reads native pages and publishes the observed summaries.
type LiveListPort interface {
	ListReader
	Publish()
}

// LiveInspectPort reads one native record and publishes its detail.
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

// Service owns Pulse definition operations and their target-bound observations.
type Service struct{ ports Ports }

type Ports struct {
	Read     ReadProvider
	Pull     PullProvider
	Mutation MutationProvider
	Publish  PublishProvider
}

func New(ports Ports) *Service { return &Service{ports: ports} }

// ListPulseDefinitions selects cached or native listing after validating input.
func (s *Service) ListPulseDefinitions(ctx context.Context, input ListInput) (ListOutput, error) {
	if err := ListValidateInput(&input); err != nil {
		return ListOutput{}, err
	}
	var target ReadTarget
	if input.Cursor != "" || input.Cache {
		var err error
		target, err = s.ports.Read.CacheTarget(input.Environment)
		if err != nil {
			if input.Cache && input.Cursor == "" {
				return ListOutput{}, s.ports.Read.CacheSetupError("pulse.definition.list", input.Environment, err)
			}
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		if err := ListValidateContinuation(input); err != nil {
			return ListOutput{}, err
		}
	}
	if input.Cache {
		reader := s.ports.Read.CachedList(target)
		output, err := List(ctx, reader, input)
		if err == nil {
			output.Source = reader.Source()
		}
		return output, err
	}
	session, err := s.ports.Read.Open(ctx, input.Environment, input.Site, "pulse.definition.list")
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := List(ctx, session.List, input)
	if err != nil {
		return output, err
	}
	output.Source = new(readsource.Live(s.ports.Read.Now()))
	session.List.Publish()
	return output, nil
}

// InspectPulseDefinition selects one exact cached or native definition.
func (s *Service) InspectPulseDefinition(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if err := inspectValidateInput(input); err != nil {
		return InspectOutput{}, err
	}
	if input.Cache {
		target, err := s.ports.Read.CacheTarget(input.Environment)
		if err != nil {
			return InspectOutput{}, s.ports.Read.CacheSetupError("pulse.definition.inspect", input.Environment, err)
		}
		input.Environment, input.Site = target.Environment, target.Site
		reader := s.ports.Read.CachedInspect(target)
		output, err := inspectValidated(ctx, reader, input)
		if err == nil {
			output.Source = reader.Source()
			output.RequestID = ""
			output.Definition.RequestID = ""
		}
		return output, err
	}
	session, err := s.ports.Read.Open(ctx, input.Environment, input.Site, "pulse.definition.inspect")
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
