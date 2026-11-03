package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/batchspec"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
	"github.com/spf13/cobra"
)

type publicationBatchBackend struct {
	mu               sync.Mutex
	writes           []workbookops.PublishRequest
	ids              map[string]string
	entered          chan string
	release          chan struct{}
	replaceOnExecute bool
}

func (*publicationBatchBackend) ResolveProject(context.Context, identity.Selector) (workbookops.Project, error) {
	return workbookops.Project{LUID: "project", Path: "Project"}, nil
}

func (b *publicationBatchBackend) FindWorkbooks(_ context.Context, name, project string) ([]workbookops.Record, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := "existing-" + name
	if b.ids != nil {
		id = b.ids[name]
	}
	if id == "" {
		return nil, nil
	}
	return []workbookops.Record{{LUID: id, Name: name, ProjectLUID: project}}, nil
}

func (*publicationBatchBackend) ReadDatasource(context.Context, string) (value.Datasource, error) {
	return value.Datasource{}, nil
}

func (*publicationBatchBackend) FindDatasourcesByContentURL(context.Context, []string) ([]value.Datasource, error) {
	return nil, nil
}

func (*publicationBatchBackend) FindDatasourcesByName(context.Context, []string) ([]value.Datasource, error) {
	return nil, nil
}

func (b *publicationBatchBackend) Prepare(_ context.Context, request workbookops.PublishRequest) (workbookops.PreparedPublish, error) {
	return publicationBatchCommit{backend: b, request: request}, nil
}

type publicationBatchCommit struct {
	backend *publicationBatchBackend
	request workbookops.PublishRequest
}

func (p publicationBatchCommit) Commit(ctx context.Context) (workbookops.PublishResult, error) {
	if p.backend.entered != nil {
		p.backend.entered <- p.request.Name
		select {
		case <-p.backend.release:
		case <-ctx.Done():
			return workbookops.PublishResult{}, ctx.Err()
		}
	}
	p.backend.mu.Lock()
	p.backend.writes = append(p.backend.writes, p.request)
	p.backend.mu.Unlock()
	return workbookops.PublishResult{Status: "succeeded", WorkbookLUID: "existing-" + p.request.Name, WorkbookName: p.request.Name, ProjectLUID: p.request.ProjectLUID}, nil
}

func (publicationBatchCommit) Close() error { return nil }

type publicationBatchReader struct{}

func (publicationBatchReader) ReadWorkbook(ctx context.Context, path string) (workbookops.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "workbook")
	return workbookops.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint}, err
}

type publicationBatchService struct{ backend *publicationBatchBackend }

func (publicationBatchService) ResolveBatchResource(context.Context, string, string, identity.Selector) (identity.LUID, error) {
	return "project", nil
}

func (publicationBatchService) AuthorizeBatchPreparation(context.Context, string) error { return nil }

func (s publicationBatchService) Execute(ctx context.Context, input workbookops.PublishInput, preview bool) (workbookops.PublishOutput, error) {
	if !preview && s.backend.replaceOnExecute {
		s.backend.mu.Lock()
		s.backend.ids = map[string]string{input.Name: "replacement"}
		s.backend.mu.Unlock()
	}
	input.Site, input.TargetResolved = "site", true
	input.ArtifactPath = input.File
	return workbookops.NewPublish(publicationBatchReader{}, s.backend, s.backend).Execute(ctx, input, preview)
}

func TestBatchRejectsDifferentPublicationSourcesForOneDestinationBeforeWriting(t *testing.T) {
	backend := &publicationBatchBackend{}
	root, _ := publicationBatchRoot(t, backend, []map[string]string{
		{"file": publicationSource(t, "first"), "name": "Destination"},
		{"file": publicationSource(t, "second"), "name": "Destination"},
	})
	err := root.ExecuteContext(t.Context())
	if err == nil || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") || len(backend.writes) != 0 {
		t.Fatalf("writes=%d error=%v", len(backend.writes), err)
	}
}

func TestBatchPublicationRetainsExistingGenerationAndExpectedAbsence(t *testing.T) {
	for _, existing := range []string{"", "original"} {
		t.Run(existing, func(t *testing.T) {
			backend := &publicationBatchBackend{ids: map[string]string{"Destination": existing}, replaceOnExecute: true}
			root, capture := publicationBatchRoot(t, backend, []map[string]string{{"file": publicationSource(t, "source"), "name": "Destination"}})
			err := root.ExecuteContext(t.Context())
			out, ok := capture.value.(contentbatch.Output)
			if err == nil || !ok || len(out.Items) != 1 || out.Items[0].Error == nil || out.Items[0].Error.ID != "workbook.publish.destination_changed" || len(backend.writes) != 0 {
				t.Fatalf("existing=%q writes=%d error=%v", existing, len(backend.writes), err)
			}
		})
	}
}

func TestBatchPublicationValidatesEverySourceBeforeWriting(t *testing.T) {
	backend := &publicationBatchBackend{}
	root, _ := publicationBatchRoot(t, backend, []map[string]string{
		{"file": publicationSource(t, "source"), "name": "First"},
		{"file": filepath.Join(t.TempDir(), "missing.twb"), "name": "Second"},
	})
	if err := root.ExecuteContext(t.Context()); err == nil || len(backend.writes) != 0 {
		t.Fatalf("writes=%d error=%v", len(backend.writes), err)
	}
}

func TestBatchPublicationSameSourceToIndependentDestinationsRemainsConcurrent(t *testing.T) {
	backend := &publicationBatchBackend{entered: make(chan string, 2), release: make(chan struct{})}
	source := publicationSource(t, "source")
	root, capture := publicationBatchRoot(t, backend, []map[string]string{{"file": source, "name": "First"}, {"file": source, "name": "Second"}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	seen := make(map[string]bool)
	for range 2 {
		select {
		case name := <-backend.entered:
			seen[name] = true
		case err := <-done:
			t.Fatalf("completed before both independent writes entered: %v", err)
		case <-ctx.Done():
			t.Fatal("independent destinations did not execute concurrently")
		}
	}
	close(backend.release)
	if err := <-done; err != nil || !seen["First"] || !seen["Second"] || len(backend.writes) != 2 {
		t.Fatalf("targets=%v writes=%d error=%v", seen, len(backend.writes), err)
	}
	out := capture.value.(contentbatch.Output)
	if out.Total != 2 || out.Items[0].Result.(workbookops.PublishOutput).Plan.WorkbookName != "First" || out.Items[1].Result.(workbookops.PublishOutput).Plan.WorkbookName != "Second" {
		t.Fatalf("batch output lost input order or leaked preparation results: %#v", out)
	}
}

func publicationSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".twb")
	if err := os.WriteFile(path, []byte("<workbook/>"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func publicationBatchRoot(t *testing.T, backend *publicationBatchBackend, items []map[string]string) (*cobra.Command, *batchCollector) {
	t.Helper()
	service := publicationBatchService{backend: backend}
	factory := func(renderer Renderer) *cobra.Command {
		root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(contentcli.New(contentcli.Dependencies{Publisher: service, Renderer: renderer, BatchTargets: service}))
		return root
	}
	renderer := &batchCollector{}
	root := factory(renderer)
	command, _, err := root.Find([]string{"content", "workbook", "publish"})
	if err != nil {
		t.Fatal(err)
	}
	attachBatchWithOptions(root, command, batchspec.Options{Selectors: []string{"artifact", "file", "id", "artifact-name"}}, renderer, factory)
	body, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"content", "workbook", "publish", "--environment", "fixture", "--project-id", "project", "--overwrite", "--batch-file", batchFile(t, string(body))})
	return root, renderer
}
