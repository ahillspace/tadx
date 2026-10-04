package search

import (
	"context"
	"errors"
	"testing"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/errs"
)

type absentCacheRecoverySource struct{}

func (absentCacheRecoverySource) Search(context.Context, searchaction.Input, []string) (searchaction.Result, error) {
	return searchaction.Result{}, cacheSearchScopeUnavailable{resourceType: "flow", cause: errors.New("read failed")}
}

func TestCachedSearchRecoveryRetainsGenericAdviceWithoutObservedEvidence(t *testing.T) {
	_, err := executeSearchAction(t.Context(), absentCacheRecoverySource{}, searchaction.Input{Type: "content", Environment: "dev", Site: "test-site", SiteResolved: true, Cache: true, Limit: 20})
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.CorrectiveAction != "Review the search source and filters, then retry." || structured.Resource != "" {
		t.Fatalf("error=%#v", structured)
	}
}
