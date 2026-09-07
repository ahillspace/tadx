package agent

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Verify the ownership allowlist against original release trees when repository
// history is available. Source archives and shallow checkouts retain the other
// migration tests without depending on Git or network access.
func TestLegacyReleaseTreesMigrateWithoutForce(t *testing.T) {
	for _, revision := range []string{"4d57544", "89287ae", "0a0a371", "6701e49"} {
		t.Run(revision, func(t *testing.T) {
			if err := exec.Command("git", "cat-file", "-e", revision+"^{commit}").Run(); err != nil {
				t.Skip("release history unavailable")
			}
			for _, crlf := range []bool{false, true} {
				for _, operation := range []string{"install", "uninstall"} {
					home := t.TempDir()
					for _, name := range []string{"tadx", "tadx-pulse"} {
						prefix := "internal/agent/skills/" + name + "/"
						listing, err := exec.Command("git", "ls-tree", "-r", "--name-only", revision, "--", "skills/"+name).Output()
						if err != nil {
							t.Fatal(err)
						}
						if len(bytes.TrimSpace(listing)) == 0 {
							t.Fatal("release package is missing")
						}
						for _, relative := range strings.Fields(string(listing)) {
							// ls-tree reports paths relative to this package directory.
							file := "internal/agent/" + relative
							data, err := exec.Command("git", "show", revision+":"+file).Output()
							if err != nil {
								t.Fatal(err)
							}
							if crlf {
								data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
							}
							location := filepath.Join(home, ".agents", "skills", name, filepath.FromSlash(strings.TrimPrefix(file, prefix)))
							if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(location, data, 0o600); err != nil {
								t.Fatal(err)
							}
						}
					}
					in := Installer{Home: func() (string, error) { return home, nil }}
					result, err := runOperation(in, operation, false, false)
					if err != nil || len(result.Skills) != 4 || len(result.Warnings) != 0 {
						t.Fatalf("%s CRLF=%t: %#v, %v", operation, crlf, result, err)
					}
				}
			}
		})
	}
}
