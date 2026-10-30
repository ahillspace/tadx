package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

func TestPullStopsWaitingForHeldWorkspaceLockWhenCanceled(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	held, err := lock.Acquire(filepath.Join(workspace, workspaceLockName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewWorkbookManager(time.Now).Pull(ctx, WorkbookPull{Workspace: workspace, Filename: "Finance.twb", PayloadPath: PayloadFile(t, []byte("payload")), Metadata: WorkbookMetadata{
			Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceProjectName: "Ops", SourceProjectID: "project-1",
		}})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Pull() error = %v, want context deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Pull() kept waiting for the workspace lock after its context ended")
	}
}
