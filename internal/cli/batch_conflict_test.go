package cli

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/batchspec"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/spf13/cobra"
)

type batchWorkbookBackend struct {
	mu             sync.Mutex
	items          map[string]workbookops.Record
	entered        chan workbookops.UpdateRequest
	committed      chan string
	permits        map[string]chan struct{}
	onBatchResolve func()
}

func (b *batchWorkbookBackend) ResolveWorkbook(_ context.Context, selector identity.Selector) (workbookops.Record, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if selector.LUID == "" {
		for _, item := range b.items {
			if selector.Name == item.Name && selector.ProjectPath == "Project" {
				return item, nil
			}
		}
	}
	return b.items[string(selector.LUID)], nil
}

func (b *batchWorkbookBackend) FindWorkbooks(context.Context, string, string) ([]workbookops.Record, error) {
	return nil, nil
}

func (b *batchWorkbookBackend) UpdateWorkbook(ctx context.Context, request workbookops.UpdateRequest) (workbookops.UpdateResult, error) {
	b.entered <- request
	select {
	case <-b.permits[*request.Description]:
	case <-ctx.Done():
		return workbookops.UpdateResult{}, ctx.Err()
	}
	b.mu.Lock()
	item := b.items[request.LUID]
	item.Description = *request.Description
	b.items[request.LUID] = item
	b.mu.Unlock()
	b.committed <- item.Description
	return workbookops.UpdateResult{Status: "succeeded", WorkbookLUID: item.LUID, WorkbookName: item.Name, ProjectLUID: item.ProjectLUID, OwnerLUID: item.OwnerLUID, Description: request.Description}, nil
}

type batchWorkbookService struct {
	backend *batchWorkbookBackend
}

func (s batchWorkbookService) ResolveBatchResource(ctx context.Context, environment, kind string, selector identity.Selector) (identity.LUID, error) {
	item, err := s.backend.ResolveWorkbook(ctx, selector)
	if s.backend.onBatchResolve != nil {
		s.backend.onBatchResolve()
	}
	return identity.LUID(item.LUID), err
}

func TestBatchPreparedWorkbookNameCannotRetargetAnotherSelectedID(t *testing.T) {
	backend := &batchWorkbookBackend{
		items: map[string]workbookops.Record{
			"one": {LUID: "one", Name: "Workbook", ProjectLUID: "project", OwnerLUID: "owner", Description: "initial"},
			"two": {LUID: "two", Name: "Two", ProjectLUID: "project", OwnerLUID: "owner", Description: "initial"},
		},
		entered: make(chan workbookops.UpdateRequest, 2), committed: make(chan string, 2),
		permits: map[string]chan struct{}{"A": make(chan struct{}), "B": make(chan struct{})},
	}
	backend.onBatchResolve = func() {
		backend.mu.Lock()
		defer backend.mu.Unlock()
		first, second := backend.items["one"], backend.items["two"]
		first.Name, second.Name = "Renamed", "Workbook"
		backend.items["one"], backend.items["two"] = first, second
	}
	close(backend.permits["A"])
	close(backend.permits["B"])
	root, renderer := workbookBatchRoot(t, backend, `{"items":[{"name":"Workbook","project":"Project","description":"A"},{"id":"two","description":"B"}]}`)
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	out := renderer.value.(contentbatch.Output)
	if out.Succeeded != 2 || backend.items["one"].Description != "A" || backend.items["two"].Description != "B" {
		t.Fatalf("items=%v output=%#v", backend.items, out)
	}
}

func (s batchWorkbookService) UpdateWorkbook(ctx context.Context, input workbookops.UpdateInput, preview bool) (workbookops.UpdateOutput, error) {
	input.Site = "site"
	input.TargetResolved = true
	return workbookops.Update(ctx, s.backend, s.backend, input, preview)
}

// Exercise the actual command wrapper and action. Conflicting desired values
// must never reach the provider, even though both rows are individually valid.
func TestBatchRejectsConflictingWorkbookUpdatesBeforeAnyWrite(t *testing.T) {
	const luid = "e271b581-a44d-4f45-afb5-ddc6d2f9dcdf"
	backend := &batchWorkbookBackend{
		items:     map[string]workbookops.Record{luid: {LUID: luid, Name: "Workbook", ProjectLUID: "project", OwnerLUID: "owner", Description: "initial"}},
		entered:   make(chan workbookops.UpdateRequest, 2),
		committed: make(chan string, 2),
		permits:   map[string]chan struct{}{"A": make(chan struct{}), "B": make(chan struct{})},
	}
	root, _ := workbookBatchRoot(t, backend, `{"items":[{"id":"`+luid+`","description":"A"},{"id":"`+luid+`","description":"B"}]}`)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := root.ExecuteContext(ctx)
	if err == nil || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") {
		t.Fatalf("err=%v", err)
	}
	if len(backend.entered) != 0 {
		t.Fatalf("conflicting writes reached provider: %d", len(backend.entered))
	}
	if backend.items[luid].Description != "initial" {
		t.Fatalf("description=%q", backend.items[luid].Description)
	}
}

func workbookBatchRoot(t *testing.T, backend *batchWorkbookBackend, body string) (*cobra.Command, *batchCollector) {
	t.Helper()
	service := batchWorkbookService{backend: backend}
	factory := func(renderer Renderer) *cobra.Command {
		root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(contentcli.New(contentcli.Dependencies{Renderer: renderer, WorkbookUpdater: service, BatchTargets: service}))
		return root
	}
	renderer := &batchCollector{}
	root := factory(renderer)
	command, _, err := root.Find([]string{"content", "workbook", "update"})
	if err != nil {
		t.Fatal(err)
	}
	attachBatchWithOptions(root, command, batchspec.Options{Selectors: []string{"id"}}, renderer, factory)
	file := batchFile(t, body)
	root.SetArgs([]string{"content", "workbook", "update", "--environment", "fixture", "--batch-file", file})
	return root, renderer
}

func TestBatchIndependentWorkbookUpdatesRemainConcurrent(t *testing.T) {
	backend := &batchWorkbookBackend{
		items: map[string]workbookops.Record{
			"one": {LUID: "one", Name: "One", ProjectLUID: "project", OwnerLUID: "owner", Description: "initial"},
			"two": {LUID: "two", Name: "Two", ProjectLUID: "project", OwnerLUID: "owner", Description: "initial"},
		},
		entered: make(chan workbookops.UpdateRequest, 2), committed: make(chan string, 2),
		permits: map[string]chan struct{}{"A": make(chan struct{}), "B": make(chan struct{})},
	}
	root, renderer := workbookBatchRoot(t, backend, `{"items":[{"id":"one","description":"A"},{"id":"two","description":"B"}]}`)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	seen := map[string]bool{}
	for range 2 {
		select {
		case request := <-backend.entered:
			seen[request.LUID] = true
		case err := <-done:
			t.Fatalf("finished before both writes reached provider: %v", err)
		case <-ctx.Done():
			t.Fatal("independent writes did not run concurrently")
		}
	}
	close(backend.permits["A"])
	close(backend.permits["B"])
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	out := renderer.value.(contentbatch.Output)
	if !seen["one"] || !seen["two"] || out.Succeeded != 2 || out.Items[0].Selector != "id=one" || out.Items[1].Selector != "id=two" {
		t.Fatalf("seen=%v output=%#v", seen, out)
	}
}

func TestBatchValidatesEveryWorkbookSelectorBeforeWriting(t *testing.T) {
	backend := &batchWorkbookBackend{entered: make(chan workbookops.UpdateRequest, 2)}
	root, _ := workbookBatchRoot(t, backend, `{"items":[{"id":"one","description":"A"},{"name":"missing-project","description":"B"}]}`)
	if err := root.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "batch item 2") {
		t.Fatalf("err=%v", err)
	}
	if len(backend.entered) != 0 {
		t.Fatal("wrote before discovering invalid selector")
	}
}

func TestBatchRejectsWorkbookSelectorAliasesBeforeWriting(t *testing.T) {
	backend := &batchWorkbookBackend{
		items:   map[string]workbookops.Record{"one": {LUID: "one", Name: "Workbook", ProjectLUID: "project", OwnerLUID: "owner", Description: "initial"}},
		entered: make(chan workbookops.UpdateRequest, 2), committed: make(chan string, 2),
		permits: map[string]chan struct{}{"A": make(chan struct{}), "B": make(chan struct{})},
	}
	root, _ := workbookBatchRoot(t, backend, `{"items":[{"id":"one","description":"A"},{"name":"Workbook","project":"Project","description":"B"}]}`)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err := root.ExecuteContext(ctx)
	if err == nil || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") || len(backend.entered) != 0 {
		t.Fatalf("writes=%d err=%v", len(backend.entered), err)
	}
}
