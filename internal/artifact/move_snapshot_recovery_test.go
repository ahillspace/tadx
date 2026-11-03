package artifact

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestCloneRejectsSourceRetainedByRealMoveRecovery(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "failed install and failed restoration"
		if committed {
			name = "committed destination and failed source cleanup"
		}
		t.Run(name, func(t *testing.T) {
			manager, configPath, source, destination, created := moveSnapshotFixture(t)
			var retained string
			operations := defaultMoveOperations()
			if committed {
				operations.removeAll = func(path string) error {
					if strings.HasPrefix(filepath.Base(path), ".tadx-move-source-") {
						retained = path
						return errors.New("source cleanup unavailable")
					}
					return os.RemoveAll(path)
				}
			} else {
				operations.rename = func(from, to string) error {
					switch {
					case from == created.ArtifactPath:
						retained = to
						return os.Rename(from, to)
					case strings.HasPrefix(filepath.Base(from), ".tadx-move-stage-"):
						return errors.New("destination installation unavailable")
					case from == retained:
						return errors.New("source restoration unavailable")
					default:
						return os.Rename(from, to)
					}
				}
			}
			moved, err := moveWithOperations(t.Context(), MoveRequest{SourceWorkspace: source.Root, DestinationWorkspace: destination.Root, Selector: Selector{Kind: "workbook", LUID: "wb-1"}}, operations)
			if committed && (err != nil || len(moved.Warnings) != 1) || !committed && (err == nil || !strings.Contains(err.Error(), "restoration failed")) {
				t.Fatalf("unexpected production move boundary: committed=%t output=%+v error=%v", committed, moved, err)
			}
			if retained == "" {
				t.Fatal("move did not retain its source recovery evidence")
			}
			assertMoveEvidenceBlocksClone(t, manager, configPath, source, retained)
			if _, err := os.Lstat(created.ArtifactPath); !os.IsNotExist(err) {
				t.Fatalf("snapshot preparation restored a source without knowing the other workspace outcome: %v", err)
			}
			if committed {
				installed, err := Resolve(t.Context(), destination.Root, Selector{Kind: "workbook", LUID: "wb-1"})
				if err != nil || installed.State != StateClean {
					t.Fatalf("snapshot preparation changed the committed destination: %+v, %v", installed, err)
				}
			}
		})
	}
}

type interruptedMoveContext struct {
	context.Context
	stageParent string
}

func (c interruptedMoveContext) Err() error {
	stages, _ := filepath.Glob(filepath.Join(c.stageParent, ".tadx-move-stage-*"))
	for _, stage := range stages {
		if entries, err := os.ReadDir(stage); err == nil && len(entries) > 0 {
			os.Exit(73)
		}
	}
	return c.Context.Err()
}

func TestCloneRejectsStageRetainedByInterruptedRealMove(t *testing.T) {
	if os.Getenv("TADX_MOVE_SNAPSHOT_CHILD") == "1" {
		source, destination := os.Getenv("TADX_MOVE_SNAPSHOT_SOURCE"), os.Getenv("TADX_MOVE_SNAPSHOT_DESTINATION")
		ctx := interruptedMoveContext{Context: t.Context(), stageParent: filepath.Join(destination, "artifacts", "workbook")}
		_, err := Move(ctx, MoveRequest{SourceWorkspace: source, DestinationWorkspace: destination, Selector: Selector{Kind: "workbook", LUID: "wb-1"}})
		t.Fatalf("move did not reach its real staging boundary: %v", err)
	}
	manager, configPath, source, destination, _ := moveSnapshotFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), executable, "-test.run=^TestCloneRejectsStageRetainedByInterruptedRealMove$")
	command.Env = append(os.Environ(), "TADX_MOVE_SNAPSHOT_CHILD=1", "TADX_MOVE_SNAPSHOT_SOURCE="+source.Root, "TADX_MOVE_SNAPSHOT_DESTINATION="+destination.Root)
	output, err := command.CombinedOutput()
	if exited, ok := errors.AsType[*exec.ExitError](err); !ok || exited.ExitCode() != 73 {
		t.Fatalf("move interruption did not retain the staged artifact: %v, %s", err, output)
	}
	stages, err := filepath.Glob(filepath.Join(destination.Root, "artifacts", "workbook", ".tadx-move-stage-*"))
	if err != nil || len(stages) != 1 {
		t.Fatalf("interrupted move stages = %v, %v", stages, err)
	}
	assertMoveEvidenceBlocksClone(t, manager, configPath, destination, stages[0])
	if item, err := Resolve(t.Context(), source.Root, Selector{Kind: "workbook", LUID: "wb-1"}); err != nil || item.State != StateClean {
		t.Fatalf("snapshot preparation changed the active source: %+v, %v", item, err)
	}
}

func moveSnapshotFixture(t *testing.T) (*workspacecore.Manager, string, workspacecore.Record, workspacecore.Record, WorkbookPullResult) {
	t.Helper()
	parent := t.TempDir()
	configPath := filepath.Join(parent, "config.yaml")
	manager := workspacecore.NewManager(configPath, nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(parent, "source"))
	if err != nil {
		t.Fatal(err)
	}
	destination, err := manager.Create(t.Context(), "destination", filepath.Join(parent, "destination"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := NewWorkbookManager(nil).Pull(t.Context(), WorkbookPull{Workspace: source.Root, Filename: "Finance.twbx", PayloadPath: PayloadFile(t, []byte("move payload")), Metadata: moveTestMetadata()})
	if err != nil {
		t.Fatal(err)
	}
	return manager, configPath, source, destination, created
}

func assertMoveEvidenceBlocksClone(t *testing.T, manager *workspacecore.Manager, configPath string, selected workspacecore.Record, retained string) {
	t.Helper()
	before, err := artifactTreeFingerprint(t.Context(), retained)
	if err != nil {
		t.Fatal(err)
	}
	cloneRoot := filepath.Join(filepath.Dir(selected.Root), "clone")
	cloned, err := manager.Clone(t.Context(), workspacecore.CloneRequest{Source: selected.Name, Name: "clone", Root: cloneRoot, Reconcile: PrepareWorkspaceSnapshot})
	if err == nil || !strings.Contains(err.Error(), "unresolved artifact recovery evidence") || !strings.Contains(err.Error(), filepath.Base(retained)) {
		t.Fatalf("clone accepted Move recovery state: output=%+v error=%v", cloned, err)
	}
	if _, err := os.Lstat(cloneRoot); !os.IsNotExist(err) {
		t.Fatalf("rejected snapshot created a clone destination: %v", err)
	}
	current, err := config.Load(configPath)
	if err != nil || len(current.Workspaces) != 2 {
		t.Fatalf("rejected snapshot changed registrations: %+v, %v", current.Workspaces, err)
	}
	after, err := artifactTreeFingerprint(t.Context(), retained)
	if err != nil || after != before {
		t.Fatalf("snapshot preparation changed Move evidence: before=%s after=%s error=%v", before, after, err)
	}
}
