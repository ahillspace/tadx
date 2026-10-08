package commandhint_test

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/commandhint"
)

func TestRecoveryBindsEveryCommandAndQuotesIdentities(t *testing.T) {
	commands := [][]string{{"env", "remove", "space name"}, {"auth", "logout", "--environment", "space name"}}
	path := "config with spaces.yaml"
	got := commandhint.Recovery(commands, "Revoke the exposed PAT.", path)
	for _, args := range commands {
		want := commandhint.Command(append([]string{"--config", path}, args...)...)
		if !strings.Contains(got, want) {
			t.Fatalf("repair hint omitted bound command %q", want)
		}
	}
	if strings.Count(got, "--config") != 2 || !strings.HasSuffix(got, "Revoke the exposed PAT.") {
		t.Fatalf("unexpected recovery: %s", got)
	}
	if commands[0][0] != "env" {
		t.Fatal("recovery changed source arguments")
	}
}
