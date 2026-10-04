package content

import (
	"errors"
	"path"
	"strings"

	"github.com/ahillspace/tadx/internal/pathspec"
)

func managedArtifactFlagHelp(kind, exampleName string) string {
	return "managed " + kind + " directory relative to the logical workspace, using forward slashes; for example, artifacts/" + kind + "/" + exampleName
}

func validateManagedArtifactPath(value, kind string) error {
	if pathspec.IsAbs(value) || strings.Contains(value, `\`) || path.Clean(value) != value {
		return errors.New("--artifact must be a workspace-relative slash-delimited managed " + kind + " path")
	}
	parts := strings.Split(value, "/")
	if len(parts) != 3 || parts[0] != "artifacts" || parts[1] != kind || parts[2] == "" {
		return errors.New("--artifact must identify one managed " + kind + " directory under artifacts/" + kind)
	}
	return nil
}
