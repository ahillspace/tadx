//go:build windows

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"golang.org/x/sys/windows"
)

type deletionFailureWithLockedConfig struct {
	*fakePATStore
	path   string
	handle windows.Handle
}

func (s *deletionFailureWithLockedConfig) DeletePAT(context.Context, auth.CredentialReference) error {
	name, err := windows.UTF16PtrFromString(s.path)
	if err != nil {
		return err
	}
	s.handle, err = windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	return errors.New("synthetic credential deletion denied")
}

func TestAuthLogoutReportsOrphanWhenDeletionAndRestoreFail(t *testing.T) {
	t.Setenv("TADX_DEV_PAT_NAME", "")
	t.Setenv("TADX_DEV_PAT_SECRET", "")
	reference := auth.CredentialReference("cred_99999999999999999999999999999999")
	path := authConfig(t, string(reference))
	store := &deletionFailureWithLockedConfig{
		fakePATStore: &fakePATStore{records: map[auth.CredentialReference]auth.PATCredentials{
			reference: {Name: "fixture", Secret: "fixture-only", Source: auth.CredentialSourceOSKeyring},
		}},
		path: path,
	}
	var output bytes.Buffer
	code := Run(t.Context(), []string{"auth", "logout", "--environment", "dev", "--json"}, &output, Options{ConfigPath: path, PATStore: store})
	if store.handle == 0 {
		t.Fatal("credential deletion callback did not lock the installed configuration")
	}
	if err := windows.CloseHandle(store.handle); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 || loaded.Environments["dev"].Auth.CredentialRef != "" {
		t.Fatal("fixture did not reproduce failed deletion with failed restoration")
	}
	if _, exists := store.records[reference]; !exists {
		t.Fatal("credential was unexpectedly removed")
	}
	result := output.String()
	if !strings.Contains(result, "restore configuration after external commit failure") {
		t.Fatal("restore failure absent from CLI result")
	}
	phase := strings.Contains(result, `"phase":"persistence"`)
	outcome := strings.Contains(result, `"outcome":"unknown"`)
	orphan := strings.Contains(result, "Inspect the OS credential store entry") && strings.Contains(result, "remove it if present") && strings.Contains(result, "logout no longer references it")
	if !phase || !outcome || !orphan {
		t.Fatalf("reference cleared and credential retained, but phase=%t outcome=%t orphan_guidance=%t", phase, outcome, orphan)
	}
	if strings.Contains(result, "fixture-only") {
		t.Fatal("synthetic credential secret exposed")
	}
}
