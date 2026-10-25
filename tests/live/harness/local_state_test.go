package harness

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

func localStateFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func assertLocalStateFile(t *testing.T, path, wanted string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != wanted {
		t.Fatalf("local state changed unexpectedly at %s", filepath.Base(path))
	}
}

func TestLocalCleanupRemovesOnlyOwnedWorkspaceAndHome(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	for _, name := range []string{"workspace", "home"} {
		localStateFile(t, filepath.Join(root, name, "nested", "state.json"), "disposable")
	}
	kept := map[string]string{"journal.json": "ownership", "evidence.json": "commands", "config.yaml": "target", "neighbor/keep.txt": "neighbor"}
	for name, value := range kept {
		localStateFile(t, filepath.Join(root, filepath.FromSlash(name)), value)
	}
	localStateFile(t, filepath.Join(outside, "outside.txt"), "outside")
	c := &Case{RunDir: root, WorkspacePath: outside}
	if err := c.cleanLocalState(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"workspace", "home"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned %s was not removed: %v", name, err)
		}
	}
	for name, value := range kept {
		assertLocalStateFile(t, filepath.Join(root, filepath.FromSlash(name)), value)
	}
	assertLocalStateFile(t, filepath.Join(outside, "outside.txt"), "outside")
}

func TestLocalCleanupIsIdempotentWhenOwnedDirectoriesAreAbsent(t *testing.T) {
	root := t.TempDir()
	localStateFile(t, filepath.Join(root, "journal.json"), "ownership")
	localStateFile(t, filepath.Join(root, "evidence.json"), "commands")
	c := &Case{RunDir: root}
	for range 2 {
		if err := c.cleanLocalState(); err != nil {
			t.Fatal(err)
		}
	}
	assertLocalStateFile(t, filepath.Join(root, "journal.json"), "ownership")
	assertLocalStateFile(t, filepath.Join(root, "evidence.json"), "commands")
}

func TestLocalCleanupRefusesSubstitutedFile(t *testing.T) {
	for _, name := range []string{"workspace", "home"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			substitute := filepath.Join(root, name)
			localStateFile(t, substitute, "substituted-user-data")
			localStateFile(t, filepath.Join(root, "journal.json"), "ownership")
			c := &Case{RunDir: root}
			if err := c.cleanLocalState(); err == nil {
				t.Fatal("substituted local-state file was removed")
			}
			assertLocalStateFile(t, substitute, "substituted-user-data")
			assertLocalStateFile(t, filepath.Join(root, "journal.json"), "ownership")
		})
	}
}

func makeLocalStateSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, os.ErrPermission) || (runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314))) {
			t.Skipf("directory symlink check unavailable under current native privileges: %v", err)
		}
		t.Fatal(err)
	}
}

func TestLocalCleanupRefusesOwnedDirectorySymlinkWithoutTouchingTarget(t *testing.T) {
	for _, name := range []string{"workspace", "home"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			localStateFile(t, filepath.Join(outside, "private.txt"), "outside-user-data")
			makeLocalStateSymlink(t, outside, filepath.Join(root, name))
			localStateFile(t, filepath.Join(root, "journal.json"), "ownership")
			c := &Case{RunDir: root}
			if err := c.cleanLocalState(); err == nil {
				t.Fatal("substituted local-state symlink was accepted")
			}
			assertLocalStateFile(t, filepath.Join(outside, "private.txt"), "outside-user-data")
			assertLocalStateFile(t, filepath.Join(root, "journal.json"), "ownership")
			if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
				t.Fatal("refused substitution was unexpectedly removed")
			}
		})
	}
}

func TestLocalCleanupRefusesRunDirectorySymlink(t *testing.T) {
	owned := t.TempDir()
	localStateFile(t, filepath.Join(owned, "workspace", "private.txt"), "existing-workspace")
	localStateFile(t, filepath.Join(owned, "home", "private.txt"), "existing-home")
	alias := filepath.Join(t.TempDir(), "case-link")
	makeLocalStateSymlink(t, owned, alias)
	if err := (&Case{RunDir: alias}).cleanLocalState(); err == nil {
		t.Fatal("substituted case-directory symlink was accepted")
	}
	assertLocalStateFile(t, filepath.Join(owned, "workspace", "private.txt"), "existing-workspace")
	assertLocalStateFile(t, filepath.Join(owned, "home", "private.txt"), "existing-home")
}

func TestLocalCleanupDoesNotFollowNestedSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	localStateFile(t, filepath.Join(outside, "private.txt"), "outside-user-data")
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0700); err != nil {
		t.Fatal(err)
	}
	makeLocalStateSymlink(t, outside, filepath.Join(root, "workspace", "linked-data"))
	localStateFile(t, filepath.Join(root, "journal.json"), "ownership")
	if err := (&Case{RunDir: root}).cleanLocalState(); err != nil {
		t.Fatal(err)
	}
	assertLocalStateFile(t, filepath.Join(outside, "private.txt"), "outside-user-data")
	assertLocalStateFile(t, filepath.Join(root, "journal.json"), "ownership")
	if _, err := os.Lstat(filepath.Join(root, "workspace")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned workspace containing a nested link was not removed")
	}
}

func TestIsolatedCandidateUsesExplicitProfileActorCredentials(t *testing.T) {
	p, dir := testProfileConfig(t, "https://tableau.example.test", "", new(true))
	source, err := config.Load(p.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	environment := source.Environments[p.Environment]
	environment.Auth.CredentialRef = "cred_00000000000000000000000000000000"
	source.Environments[p.Environment] = environment
	if err := config.Save(p.ConfigPath, source); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIVE_TEST_ACTOR_NAME", "bootstrap-name")
	t.Setenv("LIVE_TEST_ACTOR_SECRET", "bootstrap-secret")
	t.Setenv("LIVE_RESTRICTED_ACTOR_NAME", "restricted-name")
	t.Setenv("LIVE_RESTRICTED_ACTOR_SECRET", "restricted-secret")
	p.ActorPAT = Credential{NameEnv: "LIVE_RESTRICTED_ACTOR_NAME", SecretEnv: "LIVE_RESTRICTED_ACTOR_SECRET"}
	c := &Case{Profile: p, Environment: p.Environment, ConfigPath: filepath.Join(dir, "isolated.yaml")}
	if err := c.makeConfig(); err != nil {
		t.Fatal(err)
	}
	isolated, err := config.Load(c.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := isolated.ResolveEnvironment(p.Environment)
	if err != nil {
		t.Fatal(err)
	}
	if actor.Auth.PATNameEnv != p.ActorPAT.NameEnv || actor.Auth.PATSecretEnv != p.ActorPAT.SecretEnv || actor.Auth.CredentialRef != "" {
		t.Fatal("candidate isolated configuration retained bootstrap credentials instead of selected profile actor")
	}
	resolver := auth.NewPATSourceResolver(auth.LookupEnvFunc(os.LookupEnv), nil)
	credentials, err := resolver.Resolve(t.Context(), auth.Target{Environment: p.Environment, ServerURL: actor.URL, SiteContentURL: actor.SiteContentURL, PATNameVariable: actor.Auth.PATNameEnv, PATSecretVariable: actor.Auth.PATSecretEnv, CredentialReference: actor.Auth.CredentialRef})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Name != "restricted-name" || credentials.Secret != "restricted-secret" {
		t.Fatal("candidate credential references resolve a different actor")
	}
	after, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("actor isolation changed source credentials")
	}
	data, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"bootstrap-name", "bootstrap-secret", "restricted-name", "restricted-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("isolated configuration persisted raw actor credentials")
		}
	}
}
