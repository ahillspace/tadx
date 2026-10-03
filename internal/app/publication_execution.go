package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/ahillspace/tadx/internal/jobmonitor"
)

// publicationExecution belongs to one invocation, not one batch item.
// Its deadline ends observation only; the worker retains ownership of writes.
type publicationExecution struct {
	operation   string
	operationID string
	noWait      bool
	deadline    time.Time
	prepare     func(context.Context, jobmonitor.Receipt) error
	accepted    func(context.Context, string) error
	detached    func() bool
}

// publicationReceiptScope binds an intent to the exact pre-write target.
func publicationReceiptScope(receipt jobmonitor.Receipt) string {
	scope := struct {
		Operation, Environment, Server, Site, SiteID, ConfigPath, SourcePath, ProjectID, Name, CoordinationKey string
	}{receipt.Operation, receipt.Environment, receipt.Server, receipt.Site, receipt.SiteID, receipt.ConfigPath, receipt.SourcePath, receipt.ProjectID, receipt.Name, receipt.CoordinationKey}
	data, _ := json.Marshal(scope)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (r *runtimeDependencies) publicationWaitDeadline() time.Time {
	if r.publicationExecution != nil {
		return r.publicationExecution.deadline
	}
	return time.Time{}
}

func (r *runtimeDependencies) publicationNoWait() bool {
	e := r.publicationExecution
	return e != nil && (e.noWait || (e.detached != nil && e.detached()) || (!e.deadline.IsZero() && !time.Now().Before(e.deadline)))
}
