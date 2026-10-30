//go:build unix

package artifact

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPullExplainsAPayloadOnAnotherFilesystem(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	manager.operations.rename = func(oldPath, newPath string) error {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
	}
	payload := PayloadFile(t, []byte("v1"))
	_, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Finance.tds", PayloadPath: payload, Metadata: validDatasourceMetadata("Finance", "ds-1")})
	if err == nil || !strings.Contains(err.Error(), "on different filesystems; keep both on one filesystem") {
		t.Fatalf("error = %v", err)
	}
}
