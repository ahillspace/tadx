package batchinput

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/ahillspace/tadx/internal/value"
)

// PublishDestination is the shared immutable destination contract used by plans
// and durable operation records.
type PublishDestination = value.PublishDestination

type publicationKey struct{}

// Publication preparation follows the command's deterministic row and native
// selection order. The mutex also protects inspection by dispatch wrappers.
type publicationPreparation struct {
	mu       sync.Mutex
	saved    []value.PublishDestination
	hasSaved bool
	next     int
	prepared []value.PublishDestination
}

func EnsurePublicationPreparation(ctx context.Context) context.Context {
	if _, ok := ctx.Value(publicationKey{}).(*publicationPreparation); ok {
		return ctx
	}
	return context.WithValue(ctx, publicationKey{}, &publicationPreparation{})
}

// WithPublicationExpectations installs the foreground's exact destination order
// in a worker. A nil list preserves records written before this field existed.
func WithPublicationExpectations(ctx context.Context, saved []value.PublishDestination) context.Context {
	return context.WithValue(ctx, publicationKey{}, &publicationPreparation{saved: slices.Clone(saved), hasSaved: saved != nil})
}

func HasPublicationExpectations(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	p, ok := ctx.Value(publicationKey{}).(*publicationPreparation)
	return ok && p.hasSaved
}

func NextPublicationExpectation(ctx context.Context) (*value.PublishDestination, error) {
	if ctx == nil {
		return nil, nil
	}
	p, ok := ctx.Value(publicationKey{}).(*publicationPreparation)
	if !ok || !p.hasSaved {
		return nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.next >= len(p.saved) {
		return nil, errors.New("publication preparation exceeds the saved destination count")
	}
	expected := p.saved[p.next]
	p.next++
	return &expected, nil
}

func RecordPublication(ctx context.Context, destination value.PublishDestination) error {
	if ctx == nil {
		return errors.New("publication preparation context is missing")
	}
	p, ok := ctx.Value(publicationKey{}).(*publicationPreparation)
	if !ok {
		return errors.New("publication preparation context is missing")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepared = append(p.prepared, destination)
	return nil
}

func PreparedPublications(ctx context.Context) []value.PublishDestination {
	if ctx == nil {
		return nil
	}
	p, ok := ctx.Value(publicationKey{}).(*publicationPreparation)
	if !ok {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.prepared)
}

func CheckPublicationPreparation(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	p, ok := ctx.Value(publicationKey{}).(*publicationPreparation)
	if !ok {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hasSaved && p.next != len(p.saved) {
		return fmt.Errorf("publication preparation consumed %d of %d saved destinations", p.next, len(p.saved))
	}
	return nil
}
