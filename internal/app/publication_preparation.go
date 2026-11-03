package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/ahillspace/tadx/actions/batchinput"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type publicationPreparationSnapshot struct {
	Version      int                        `json:"version"`
	Operation    string                     `json:"operation"`
	Destinations []value.PublishDestination `json:"destinations"`
	Scopes       []publicationTargetScope   `json:"scopes"`
}

type publicationTargetScope struct {
	Environment string `json:"environment"`
	Site        string `json:"site"`
	Server      string `json:"server"`
	SiteLUID    string `json:"site_luid"`
}

type publicationScopesKey struct{}
type publicationScopes struct {
	mu       sync.Mutex
	expected []publicationTargetScope
	observed []publicationTargetScope
}

func ensurePublicationScopes(ctx context.Context) context.Context {
	if _, ok := ctx.Value(publicationScopesKey{}).(*publicationScopes); ok {
		return ctx
	}
	return context.WithValue(ctx, publicationScopesKey{}, &publicationScopes{})
}

func withPublicationScopes(ctx context.Context, expected []publicationTargetScope) context.Context {
	return context.WithValue(ctx, publicationScopesKey{}, &publicationScopes{expected: slices.Clone(expected)})
}

func observedPublicationScopes(ctx context.Context) []publicationTargetScope {
	scopes, ok := ctx.Value(publicationScopesKey{}).(*publicationScopes)
	if !ok {
		return nil
	}
	scopes.mu.Lock()
	defer scopes.mu.Unlock()
	return slices.Clone(scopes.observed)
}

func bindPublicationScope(ctx context.Context, operation string, environment config.Environment, siteLUID string) error {
	scopes, ok := ctx.Value(publicationScopesKey{}).(*publicationScopes)
	if !ok {
		return nil
	}
	server, err := config.CanonicalMutationServer(environment.URL)
	if err != nil {
		return err
	}
	current := publicationTargetScope{Environment: environment.Alias, Site: environment.SiteContentURL, Server: server, SiteLUID: siteLUID}
	failure := func() error {
		return &errs.Error{ID: operation + ".scope_changed", Kind: errs.KindOperation, Operation: operation, Environment: environment.Alias, Site: environment.SiteContentURL, Summary: "The authenticated publication target changed after batch preparation.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Retryable: new(false), CorrectiveAction: "Review a new batch plan for the exact server and site before publishing."}
	}
	scopes.mu.Lock()
	defer scopes.mu.Unlock()
	if current.SiteLUID == "" {
		return failure()
	}
	if scopes.expected != nil {
		matched := false
		for _, expected := range scopes.expected {
			if current == expected {
				matched = true
				break
			}
		}
		if !matched {
			return failure()
		}
	}
	for _, observed := range scopes.observed {
		if observed.Environment == current.Environment && observed.Site == current.Site {
			if observed != current {
				return failure()
			}
			return nil
		}
	}
	scopes.observed = append(scopes.observed, current)
	return nil
}

func encodePublicationPreparation(ctx context.Context, operation string) (json.RawMessage, error) {
	destinations := batchinput.PreparedPublications(ctx)
	if destinations == nil {
		return nil, nil
	}
	snapshot := publicationPreparationSnapshot{Version: 1, Operation: operation, Destinations: destinations, Scopes: observedPublicationScopes(ctx)}
	if err := validatePublicationPreparation(snapshot, operation); err != nil {
		return nil, err
	}
	return json.Marshal(snapshot)
}

func decodePublicationPreparation(data json.RawMessage, operation string) (publicationPreparationSnapshot, error) {
	if len(data) == 0 {
		return publicationPreparationSnapshot{}, nil
	}
	var snapshot publicationPreparationSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return publicationPreparationSnapshot{}, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return publicationPreparationSnapshot{}, errors.New("publication preparation contains trailing data")
	}
	if err := validatePublicationPreparation(snapshot, operation); err != nil {
		return publicationPreparationSnapshot{}, err
	}
	return snapshot, nil
}

func validatePublicationPreparation(snapshot publicationPreparationSnapshot, operation string) error {
	if snapshot.Version != 1 || snapshot.Operation != operation || !nativePublication(operation) {
		return errors.New("publication preparation version or operation does not match the accepted request")
	}
	if len(snapshot.Destinations) == 0 || len(snapshot.Destinations) > contentbatch.MaxItems {
		return errors.New("publication preparation has an invalid destination count")
	}
	if len(snapshot.Scopes) == 0 || len(snapshot.Scopes) > len(snapshot.Destinations) {
		return errors.New("publication preparation has an invalid authenticated scope count")
	}
	seen := make(map[[2]string]bool)
	used := make(map[[2]string]bool)
	for _, scope := range snapshot.Scopes {
		server, err := config.CanonicalMutationServer(scope.Server)
		key := [2]string{scope.Environment, scope.Site}
		if err != nil || server != scope.Server || scope.Environment == "" || scope.SiteLUID == "" || seen[key] {
			return errors.New("publication preparation has an invalid authenticated scope")
		}
		seen[key] = true
	}
	for _, target := range snapshot.Destinations {
		if strings.TrimSpace(target.Environment) == "" || strings.TrimSpace(target.ProjectLUID) == "" || target.Name == "" {
			return errors.New("publication preparation omitted an authoritative destination")
		}
		if !seen[[2]string{target.Environment, target.Site}] {
			return errors.New("publication preparation destination has no matching authenticated scope")
		}
		used[[2]string{target.Environment, target.Site}] = true
	}
	if len(used) != len(seen) {
		return errors.New("publication preparation contains an unrelated authenticated scope")
	}
	return nil
}
