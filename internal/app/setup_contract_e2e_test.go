package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingPATIsSetupFailure(t *testing.T) {
	t.Setenv("TADX_DEV_PAT_NAME", "")
	t.Setenv("TADX_DEV_PAT_SECRET", "")
	path := authConfig(t, "")
	options := overviewOptions(t, filepath.Dir(path))
	for _, command := range []string{"auth check --env dev", "job inspect --id job-1 --env dev", "job cancel --id job-1 --env dev --preview"} {
		t.Run(command, func(t *testing.T) {
			var out strings.Builder
			code := Run(t.Context(), append(strings.Fields(command), "--json"), &out, options)
			var result struct {
				Error map[string]any `json:"error"`
			}
			if err := json.Unmarshal([]byte(out.String()), &result); err != nil || code == 0 {
				t.Fatalf("code=%d error=%v output=%s", code, err, out.String())
			}
			for key, want := range map[string]string{"phase": "setup", "outcome": "not_attempted", "environment": "dev", "site": "test-site"} {
				if result.Error[key] != want {
					t.Errorf("%s = %v, want %s: %s", key, result.Error[key], want, out.String())
				}
			}
		})
	}
}

func TestSetupFailuresPreserveKnownPhaseAndContext(t *testing.T) {
	_, options := contentHelpPilotSetup(t)
	for _, command := range []string{"auth check --env dev", "auth logout --env dev --preview", "env list", "env get dev", "workspace list", "job inspect --id job-1 --env dev --site site", "job cancel --id job-1 --env dev --site site --preview"} {
		t.Run(command, func(t *testing.T) {
			var out strings.Builder
			code := Run(t.Context(), append(strings.Fields(command), "--json"), &out, options)
			var result struct {
				Error map[string]any `json:"error"`
			}
			if err := json.Unmarshal([]byte(out.String()), &result); err != nil || code == 0 {
				t.Fatalf("expected structured failure: code=%d error=%v output=%s", code, err, out.String())
			}
			if result.Error["phase"] != "setup" || result.Error["outcome"] != "not_attempted" {
				t.Errorf("incorrect setup contract: %s", out.String())
			}
			if strings.HasPrefix(command, "job ") {
				for key, want := range map[string]string{"operation": strings.Join(strings.Fields(command)[:2], "."), "environment": "dev", "site": "site", "tableau_job_id": "job-1"} {
					if result.Error[key] != want {
						t.Errorf("%s = %v, want %s", key, result.Error[key], want)
					}
				}
			}
		})
	}
}

func TestFocusedHelpCorrectionContracts(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	for _, command := range []string{"pulse metric fork", "pulse metric"} {
		out := contentHelpPilotRun(t, dir, options, append(strings.Fields(command), "--help")...)
		if !strings.Contains(out, "7|14|30|60|90") || strings.Contains(out, "1..3650") {
			t.Errorf("incorrect Pulse days: %s", out)
		}
	}
	out := contentHelpPilotRun(t, dir, options, "workspace", "status", "--help")
	if strings.Contains(out, "tadx workspace status -h") {
		t.Errorf("self-related route: %s", out)
	}
	out = contentHelpPilotRun(t, dir, options, "auth", "logout", "--help")
	if strings.Contains(out, "check/status:") {
		t.Errorf("unrelated auth defaults: %s", out)
	}
}
