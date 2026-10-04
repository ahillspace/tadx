// Package last reads saved output without replaying the original operation.
package last

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type Output = value.SavedExecution

// Service authorizes both the saved operation and its recorded prerequisites.
type Service struct {
	read            func(context.Context) (Output, error)
	checkCapability func(string) error
}

func New(read func(context.Context) (Output, error), checkCapability func(string) error) *Service {
	return &Service{read: read, checkCapability: checkCapability}
}

func (s *Service) ReadLast(ctx context.Context) (Output, error) {
	record, err := s.read(ctx)
	if err != nil {
		return record, unavailable(err)
	}
	if err := s.checkCapability(record.Operation); err != nil {
		return Output{}, unavailable(err)
	}
	for _, id := range record.RequiredCapabilities {
		if err := s.checkCapability(id); err != nil {
			return Output{}, unavailable(err)
		}
	}
	if record.Operation == "search.run" {
		for _, id := range legacySearchPrerequisites(record.Result) {
			if err := s.checkCapability(id); err != nil {
				return Output{}, unavailable(err)
			}
		}
	}
	return record, nil
}

func unavailable(err error) error {
	return &errs.Error{ID: "last.unavailable", Kind: errs.KindOperation, Operation: "last", Summary: "No readable previous result is available.", Cause: err, Retryable: new(false)}
}

// Old search snapshots have no prerequisite ledger. Inspect only the known
// search row shape, including the partial-result envelope, never owner fields.
func legacySearchPrerequisites(data json.RawMessage) []string {
	type row struct {
		Type string `json:"type"`
	}
	type searchSnapshot struct {
		Items []row `json:"items"`
	}
	var snapshot struct {
		Items  []row          `json:"items"`
		Output searchSnapshot `json:"output"`
	}
	if json.Unmarshal(data, &snapshot) != nil {
		return nil
	}
	ids := map[string]struct{}{}
	for _, item := range append(snapshot.Items, snapshot.Output.Items...) {
		switch item.Type {
		case "user":
			ids["admin.user.list"] = struct{}{}
		case "group":
			ids["admin.group.list"] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(ids))
}
