package workspace_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/errs"
)

type installedConfigurationError struct{}

func (installedConfigurationError) Error() string {
	return "configuration took effect but was not synced"
}

func (installedConfigurationError) ConfigurationInstalled() bool { return true }

var errInstalled = fmt.Errorf("register workspace: %w", installedConfigurationError{})

const installedID = "ws_22222222222222222222222222222222"

type installedCreator struct{}

func (installedCreator) Create(_ context.Context, input workspaceaction.CreateInput) (workspaceaction.Registration, error) {
	return workspaceaction.Registration{Workspace: workspaceaction.Workspace{Name: input.Name, ID: installedID, Root: "root"}, Registered: true}, errInstalled
}

func (installedCreator) Clone(_ context.Context, input workspaceaction.CloneInput) (workspaceaction.Registration, error) {
	return workspaceaction.Registration{Workspace: workspaceaction.Workspace{Name: input.Name, ID: installedID, Root: "root"}, Registered: true}, errInstalled
}

type installedDeleteStore struct{ deleteWorkspaceStore }

func (*installedDeleteStore) DeleteWorkspace(context.Context, workspaceaction.WorkspaceDeleteRequest) error {
	return errInstalled
}

func assertConfirmedPersistence(t *testing.T, err error, id, resource, verb string) {
	t.Helper()
	payload := errs.Structure(err).Error
	if payload.ID != id || payload.Resource != resource || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "failed") || !strings.Contains(payload.Summary, verb) {
		t.Fatalf("error = %#v", payload)
	}
}

func TestCreateReportsInstalledRegistrationAsConfirmed(t *testing.T) {
	_, err := (&workspaceaction.Service{Creator: installedCreator{}}).Create(t.Context(), workspaceaction.CreateInput{Name: "dev"})
	assertConfirmedPersistence(t, err, "workspace.create.failed", installedID, "created")
}

func TestCloneReportsInstalledRegistrationAsConfirmed(t *testing.T) {
	_, err := (&workspaceaction.Service{Cloner: installedCreator{}}).Clone(t.Context(), workspaceaction.CloneInput{Source: "source", Name: "copy"})
	assertConfirmedPersistence(t, err, "workspace.clone.failed", installedID, "cloned")
}

func TestDeleteReportsInstalledUnregistrationAsConfirmed(t *testing.T) {
	_, err := (&workspaceaction.Service{WorkspaceStore: &installedDeleteStore{}}).DeleteWorkspace(t.Context(), workspaceaction.DeleteWorkspaceInput{Name: "dev"}, false)
	assertConfirmedPersistence(t, err, "workspace.delete.failed", "ws_1", "unregistered")
}
