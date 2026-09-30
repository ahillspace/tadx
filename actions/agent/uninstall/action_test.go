package uninstall_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	uninstall "github.com/ahillspace/tadx/actions/agent/uninstall"
	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type service struct {
	called bool
	input  uninstall.Input
}

func (s *service) Uninstall(_ context.Context, target string, preview, force bool) (value.AgentGuidanceResult, error) {
	s.called = true
	s.input = uninstall.Input{Target: target, Preview: preview, Force: force}
	return value.AgentGuidanceResult{Status: "uninstalled", Skills: []value.AgentGuidanceSkill{{Name: "tadx", Status: "removed"}}}, nil
}
func TestExecute(t *testing.T) {
	s := &service{}
	input := uninstall.Input{Target: "codex", Preview: true, Force: true}
	out, err := uninstall.New(s).Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !s.called || s.input != input || out.Status != "uninstalled" {
		t.Fatalf("out=%#v called=%t input=%#v", out, s.called, s.input)
	}
}

func TestUninstallFailureWithoutPackageStateReturnsOnlyTheError(t *testing.T) {
	cause := errors.New("target locked")
	result, err := uninstall.New(failedService{err: cause}).Execute(t.Context(), uninstall.Input{Target: "codex"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "agent.uninstall.failed" || !errors.Is(err, cause) {
		t.Fatalf("error=%v", err)
	}
	if !reflect.ValueOf(result).IsZero() {
		t.Fatalf("failure returned a success-shaped result: %#v", result)
	}
}

func TestUninstallIncompleteRollbackKeepsConfirmedPackageState(t *testing.T) {
	cause := errors.New("uninstall failed and rollback is incomplete; inspect target packages")
	state := value.AgentGuidanceResult{Status: "partial", Skills: []value.AgentGuidanceSkill{{Name: "tadx", Status: "backed-up", Path: ".codex/skills/tadx", Backup: ".codex/.tadx-skill-staging/tadx-A"}, {Name: "tadx-pulse", Status: "unchanged", Path: ".codex/skills/tadx-pulse"}}}
	result, err := uninstall.New(failedService{result: state, err: cause}).Execute(t.Context(), uninstall.Input{Target: "codex"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "agent.uninstall.failed" || !errors.Is(err, cause) {
		t.Fatalf("error=%v", err)
	}
	if result.Status != "partial" || len(result.Skills) != 2 || result.Skills[0].Backup != ".codex/.tadx-skill-staging/tadx-A" || result.Skills[1].Status != "unchanged" {
		t.Fatalf("confirmed package state was not kept: %#v", result)
	}
}

type failedService struct {
	result value.AgentGuidanceResult
	err    error
}

func (s failedService) Uninstall(context.Context, string, bool, bool) (value.AgentGuidanceResult, error) {
	return s.result, s.err
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
	full, err := json.Marshal(out.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"status":"uninstalled","target":"codex","skills":[{"name":"tadx","status":"backed-up","path":".codex/skills/tadx","backup":".agents/.tadx-skill-backups/tadx"}],"help":["tadx agent install --target codex --preview"]}`
	if string(full) != want {
		t.Fatalf("full=%s", full)
	}
}

type serviceWithBackup struct{}

func (serviceWithBackup) Uninstall(context.Context, string, bool, bool) (value.AgentGuidanceResult, error) {
	return value.AgentGuidanceResult{Status: "uninstalled", Skills: []value.AgentGuidanceSkill{{Name: "tadx", Status: "backed-up", Path: ".codex/skills/tadx", Backup: ".agents/.tadx-skill-backups/tadx"}}}, nil
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
