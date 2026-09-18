package uninstall_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	uninstall "github.com/ahillspace/tadx/actions/agent/uninstall"
	"github.com/ahillspace/tadx/internal/agenttarget"
)

type service struct{ called bool }

func (s *service) Uninstall(context.Context, uninstall.Input) (uninstall.Result, error) {
	s.called = true
	return uninstall.Result{Status: "uninstalled", Skills: []uninstall.Skill{{Name: "tadx", Status: "removed"}}}, nil
}
func TestExecute(t *testing.T) {
	s := &service{}
	out, err := uninstall.New(s).Execute(context.Background(), uninstall.Input{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.called || out.Status != "uninstalled" {
		t.Fatalf("out=%#v called=%t", out, s.called)
	}
}

func TestCompactProjectionRetainsDestinationAndBackup(t *testing.T) {
	s := &serviceWithBackup{}
	out, err := uninstall.New(s).Execute(t.Context(), uninstall.Input{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(out.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"path":".codex/skills/tadx"`)) || !bytes.Contains(data, []byte(`"backup":".agents/.tadx-skill-backups/tadx"`)) {
		t.Fatalf("compact output omits destination or backup: %s", data)
	}
}

type serviceWithBackup struct{}

func (serviceWithBackup) Uninstall(context.Context, uninstall.Input) (uninstall.Result, error) {
	return uninstall.Result{Status: "uninstalled", Skills: []uninstall.Skill{{Name: "tadx", Status: "backed-up", Path: ".codex/skills/tadx", Backup: ".agents/.tadx-skill-backups/tadx"}}}, nil
}

func TestExecuteAcceptsAllSupportedTargets(t *testing.T) {
	for _, target := range agenttarget.SupportedTargets() {
		t.Run(target, func(t *testing.T) {
			s := &service{}
			if _, err := uninstall.New(s).Execute(context.Background(), uninstall.Input{Target: target}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
