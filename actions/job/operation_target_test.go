package job

import (
	"context"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

func TestRecoveryDefaultSiteIsNotAWildcard(t *testing.T) {
	reads := 0
	recovery := recovery{ports: RecoveryPorts{Open: func(context.Context, string) (RecoverySession, error) {
		return RecoverySession{Server: "https://tableau.example", Site: "named-site", SiteID: "named-id", Inspect: func(context.Context, string) (value.JobStatus, error) { reads++; return value.JobStatus{}, nil }}, nil
	}}}
	receipt := jobmonitor.Receipt{Environment: "dev", Server: "https://tableau.example", Site: "", Observation: value.JobStatus{ID: "job-1"}}
	_, err := recovery.inspectReceiptOnce(t.Context(), receipt, map[string]RecoverySession{})
	if err == nil || !strings.Contains(err.Error(), "target does not match") || reads != 0 {
		t.Fatalf("err=%v reads=%d", err, reads)
	}
}

func TestRecoveryRejectsJobTypeChangesWithoutReplacingSavedEvidence(t *testing.T) {
	original := value.JobStatus{ID: "job-1", Type: "PublishWorkbook", Status: "pending", RequestID: "accepted-request"}
	recovery := recovery{ports: RecoveryPorts{Open: func(context.Context, string) (RecoverySession, error) {
		return RecoverySession{Server: "https://tableau.example", Site: "site", SiteID: "site-id", Inspect: func(context.Context, string) (value.JobStatus, error) {
			return value.JobStatus{ID: "job-1", Type: "RefreshExtract", Status: "succeeded"}, nil
		}}, nil
	}}}
	receipt := jobmonitor.Receipt{Environment: "dev", Server: "https://tableau.example", Site: "site", SiteID: "site-id", Observation: original}
	result, err := recovery.inspectReceiptOnce(t.Context(), receipt, map[string]RecoverySession{})
	if err == nil || !strings.Contains(err.Error(), "job type changed") || result.Observation.Type != original.Type || result.Observation.Status != original.Status || result.Observation.RequestID != original.RequestID {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
