package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/workspace"
)

// Pause at an existing cancellation boundary after metadata has been copied but
// before the canonical payload is opened. No production copy code is replaced.
type cloneCopyContext struct {
	context.Context
	parent, relativeArtifact string
	paused, resume           chan struct{}
	once                     sync.Once
	metadataSeen             bool
}

func (c *cloneCopyContext) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	stages, _ := filepath.Glob(filepath.Join(c.parent, ".tadx-workspace-stage-*"))
	for _, stage := range stages {
		directory := filepath.Join(stage, c.relativeArtifact)
		if info, err := os.Stat(filepath.Join(directory, "metadata.json")); err != nil || info.Size() == 0 {
			continue
		}
		// The first nonempty observation is the metadata reader's EOF probe.
		// The next cancellation boundary follows closure of both file handles.
		if !c.metadataSeen {
			c.metadataSeen = true
			continue
		}
		if _, err := os.Stat(filepath.Join(directory, "zz-workbook.twb")); !os.IsNotExist(err) {
			continue
		}
		c.once.Do(func() {
			close(c.paused)
			select {
			case <-c.resume:
			case <-c.Context.Done():
			}
		})
	}
	return c.Context.Err()
}

func TestCloneCopiesOneCompleteGenerationDuringNormalPull(t *testing.T) {
	parent := t.TempDir()
	manager := workspace.NewManager(newConfig(t), nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(parent, "source"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.NewWorkbookManager(nil)
	metadata := artifact.WorkbookMetadata{Kind: "workbook", Name: "Workbook", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "fixture", SourceSite: "fixture", SourceProjectName: "Project", SourceProjectID: "project-1"}
	payload := filepath.Join(t.TempDir(), "zz-workbook.twb")
	oldBytes, newBytes := []byte(`<workbook generation="old"/>`), []byte(`<workbook generation="new"/>`)
	if err := os.WriteFile(payload, oldBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	request := artifact.WorkbookPull{Workspace: source.Root, Filename: "zz-workbook.twb", PayloadPath: payload, Metadata: metadata}
	first, err := artifacts.Pull(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(source.Root, first.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	copyContext := &cloneCopyContext{Context: ctx, parent: parent, relativeArtifact: relative, paused: make(chan struct{}), resume: make(chan struct{})}
	defer close(copyContext.resume)
	type cloneResult struct {
		record workspace.Record
		err    error
	}
	cloned := make(chan cloneResult, 1)
	go func() {
		record, err := manager.Clone(copyContext, workspace.CloneRequest{Source: "source", Name: "clone", Root: filepath.Join(parent, "clone"), Reconcile: artifact.PrepareWorkspaceSnapshot})
		cloned <- cloneResult{record, err}
	}()
	select {
	case <-copyContext.paused:
	case result := <-cloned:
		t.Fatalf("clone completed before reaching its metadata/payload boundary: %+v", result)
	case <-ctx.Done():
		t.Fatal("clone did not reach its metadata/payload boundary")
	}
	if err := os.WriteFile(payload, newBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	probe, probeErr := lock.TryAcquire(filepath.Join(source.Root, workspace.ArtifactLockName))
	protected := errors.Is(probeErr, lock.ErrLocked)
	if probe != nil {
		_ = probe.Release()
	} else if !protected {
		t.Fatal(probeErr)
	}
	pulled := make(chan error, 1)
	go func() {
		_, err := artifacts.Pull(ctx, request)
		pulled <- err
	}()
	pullFinished := false
	if !protected {
		select {
		case err := <-pulled:
			if err != nil {
				t.Fatal(err)
			}
			pullFinished = true
		case <-ctx.Done():
			t.Fatal("normal pull did not finish while clone lacked artifact ownership")
		}
	}
	if protected {
		select {
		case err := <-pulled:
			if err != nil {
				t.Fatal(err)
			}
			pullFinished = true
		case <-time.After(150 * time.Millisecond):
		}
	}
	copyContext.resume <- struct{}{}
	result := <-cloned
	if result.err != nil {
		t.Fatal(result.err)
	}
	if !pullFinished {
		if err := <-pulled; err != nil {
			t.Fatal(err)
		}
	}
	item, err := artifact.Resolve(t.Context(), result.record.Root, artifact.Selector{Kind: "workbook", LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join(result.record.Root, filepath.FromSlash(item.CanonicalPath)))
	if err != nil {
		t.Fatal(err)
	}
	if pullFinished || item.State != artifact.StateClean || item.BaselineFingerprint != first.BaselineFingerprint || string(bytes) != string(oldBytes) || result.record.ID == source.ID {
		t.Fatalf("clone mixed concurrent pull generations: pull_completed_during_copy=%t state=%s baseline=%s old_baseline=%s payload=%s clone_id=%s source_id=%s", pullFinished, item.State, item.BaselineFingerprint, first.BaselineFingerprint, bytes, result.record.ID, source.ID)
	}
}

func TestCloneRecoversInterruptedIndividualArtifactBeforeCopying(t *testing.T) {
	parent := t.TempDir()
	manager := workspace.NewManager(newConfig(t), nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(parent, "source"))
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(t.TempDir(), "workbook.twb")
	if err := os.WriteFile(payload, []byte(`<workbook generation="original"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{Workspace: source.Root, Filename: "workbook.twb", PayloadPath: payload, Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Workbook", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "fixture", SourceSite: "fixture", SourceProjectName: "Project", SourceProjectID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	// A standalone replacement may stop after moving the original aside and
	// before installing its stage. Preserve the actual on-disk artifact receipt.
	backup := filepath.Join(filepath.Dir(first.ArtifactPath), ".tadx-workbook-backup-interrupted")
	if err := os.Rename(first.ArtifactPath, backup); err != nil {
		t.Fatal(err)
	}
	clone, err := manager.Clone(t.Context(), workspace.CloneRequest{Source: "source", Name: "clone", Root: filepath.Join(parent, "clone"), Reconcile: artifact.PrepareWorkspaceSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{source.Root, clone.Root} {
		item, err := artifact.Resolve(t.Context(), root, artifact.Selector{Kind: "workbook", LUID: "wb-1"})
		if err != nil || item.State != artifact.StateClean || item.BaselineFingerprint != first.BaselineFingerprint {
			t.Fatalf("clone did not copy the recovered complete artifact: %+v, %v", item, err)
		}
	}
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Fatalf("recovered backup remains as a second copy: %v", err)
	}
}
