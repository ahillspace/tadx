package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

const recoveryWaitLimit = 20 * time.Minute

// NativeJobs is the exact-job operation contract; it cannot submit a new job.
type NativeJobs interface {
	Inspect(context.Context, string) (value.JobStatus, error)
	Cancel(context.Context, string) (string, error)
}

type Session struct {
	Environment, Site, Server, SiteID, ConfigPath string
	Native                                        NativeJobs
	CoordinationKey                               func(context.Context) (string, error)
	Observe                                       func(context.Context, []jobmonitor.Receipt) []jobmonitor.CheckResult
}

type Provider interface {
	Open(context.Context, string) (Session, error)
	Store() (jobmonitor.Store, error)
	Suspend(context.Context) error
	RecoveryPorts() RecoveryPorts
}

type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }
func (s *Service) Inspect(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if s == nil || s.provider == nil {
		return inspectOutput(ctx, nil, input)
	}
	return inspectOutput(ctx, s.inspect, input)
}
func (s *Service) Wait(ctx context.Context, input WaitInput) (WaitOutput, error) {
	if s == nil || s.provider == nil {
		return waitOutput(ctx, nil, input)
	}
	return waitOutput(ctx, s.wait, input)
}
func (s *Service) Cancel(ctx context.Context, input CancelInput) (CancelOutput, error) {
	if s == nil || s.provider == nil {
		return cancelOutput(ctx, nil, input)
	}
	return cancelOutput(ctx, s.cancel, input)
}

func (c *Service) inspect(ctx context.Context, input InspectInput) (InspectResult, error) {
	if input.OperationID != "" {
		return (recovery{ports: c.provider.RecoveryPorts()}).inspectOperation(ctx, input)
	}
	connection, err := c.connection(ctx, input.Environment, input.Site, "job.inspect", input.ID)
	if err != nil {
		return InspectResult{}, err
	}
	client := connection.Native
	status, err := client.Inspect(ctx, input.ID)
	if err != nil {
		return InspectResult{Status: status, Environment: connection.Environment, Site: connection.Site, Attempts: 1}, c.observationError("job.inspect", connection.Environment, connection.Site, input.ID, "Job status could not be read.", err)
	}
	return InspectResult{Status: status, Environment: connection.Environment, Site: connection.Site, Attempts: 1}, nil
}

func (c *Service) wait(ctx context.Context, input WaitInput) (WaitResult, error) {
	store, err := c.provider.Store()
	if err != nil {
		return WaitResult{}, err
	}
	receipt, path, err := c.resolveReceipt(ctx, store, input)
	if err != nil && input.Receipt == "" && errors.Is(err, os.ErrNotExist) {
		receipt, path, err = c.startObservation(ctx, store, input)
	}
	if err != nil {
		return WaitResult{}, &errs.Error{ID: "job.wait.receipt", Kind: errs.KindOperation, Operation: "job.wait", Resource: input.ID, Environment: input.Environment, Site: input.Site, Summary: "The accepted job receipt could not be recovered.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Provide the exact durable receipt path or job ID returned by the accepted operation.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if input.Environment != "" && input.Environment != receipt.Environment {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptTargetError(receipt, path, input.Environment, input.Site, "The requested environment does not match the durable job target.", nil)
	}
	if input.Site != "" && input.Site != receipt.Site {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptTargetError(receipt, path, receipt.Environment, input.Site, "The requested site does not match the durable job target.", nil)
	}
	if receipt.Observation.Terminal() {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, nil
	}
	connection, err := c.provider.Open(ctx, receipt.Environment)
	if err != nil {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptRecoveryError(receipt, path, "The saved job target could not be authenticated for recovery.", err, errs.PhaseSetup, errs.OutcomeNotAttempted)
	}
	if err := verifyReceiptTarget(connection, receipt); err != nil {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptTargetError(receipt, path, receipt.Environment, receipt.Site, "The saved job receipt does not match the authenticated environment and site.", err)
	}
	// An explicit recovery request is a deliberate fresh-read authorization.
	// Clear only local backoff/error state; the accepted remote identity and
	// pending observation remain unchanged, and no request is resubmitted.
	if receipt.ReadFailures != 0 || receipt.LastReadError != "" || !receipt.NextCheck.IsZero() {
		receipt.ReadFailures = 0
		receipt.LastReadError = ""
		receipt.NextCheck = time.Time{}
	}
	// Re-register every recovered active receipt. Acceptance may have been
	// durably saved even when the original active-index write failed.
	receipt.ManualOnly = false
	receipt.WaitUntil = time.Now().Add(recoveryWaitLimit)
	if _, err := store.Register(ctx, receipt); err != nil {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, &errs.Error{ID: "job.wait.register", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: receipt.Environment, Site: receipt.Site, TableauJobID: receipt.Observation.ID, Summary: "The accepted job could not be restored to the local monitoring pool.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: c.receiptRecoveryHint(receipt, path), Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown}
	}
	if err := c.provider.Suspend(ctx); err != nil {
		return WaitResult{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptRecoveryError(receipt, path, "Credential coordination could not be released before job monitoring.", err, errs.PhaseSetup, errs.OutcomeUnknown)
	}
	monitor := jobmonitor.Monitor{Store: store, Deadline: receipt.WaitUntil, Observe: connection.Observe}
	latest, err := monitor.Wait(ctx, receipt)
	result := WaitResult{Status: latest.Observation, Environment: latest.Environment, Site: latest.Site, ReceiptPath: filepath.ToSlash(path)}
	if err != nil && !errors.Is(err, jobmonitor.ErrWaitLimit) {
		return result, c.receiptRecoveryError(receipt, path, "Job monitoring stopped without establishing the remote terminal outcome.", err, errs.PhaseVerification, errs.OutcomeUnknown)
	}
	return result, nil
}

func (c *Service) cancel(ctx context.Context, input CancelInput) (CancelResult, error) {
	connection, err := c.connection(ctx, input.Environment, input.Site, "job.cancel", input.ID)
	if err != nil {
		return CancelResult{}, err
	}
	client := connection.Native
	status, err := client.Inspect(ctx, input.ID)
	if err != nil {
		return CancelResult{}, &errs.Error{ID: "job.cancel.inspect", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.Environment, Site: connection.Site, TableauJobID: input.ID, Summary: "The exact job could not be inspected before cancellation.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact job before attempting cancellation again.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if !supportedCancellation(status.Type) {
		return CancelResult{}, &errs.Error{ID: "job.cancel.unsupported", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.Environment, Site: connection.Site, TableauJobID: input.ID, Summary: "Tableau does not document cancellation for this job type.", Retryable: errs.Bool(false), CorrectiveAction: "Use job inspect or job wait for this job; cancellation is limited to documented refresh and flow-run job types.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	}
	if status.Terminal() {
		return CancelResult{}, &errs.Error{ID: "job.cancel.terminal", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.Environment, Site: connection.Site, TableauJobID: input.ID, Summary: "The exact job is already terminal and was not cancelled.", Retryable: errs.Bool(false), CorrectiveAction: "Use the authoritative terminal status; do not repeat cancellation.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeConfirmed}
	}
	if input.Preview {
		return CancelResult{Status: status, Environment: connection.Environment, Site: connection.Site, Warnings: []string{"Preview only; Tableau was not asked to cancel the job."}}, nil
	}
	requestID, err := client.Cancel(ctx, input.ID)
	if err != nil {
		return CancelResult{Status: status, Environment: connection.Environment, Site: connection.Site, RequestID: requestID}, &errs.Error{ID: "job.cancel.request", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.Environment, Site: connection.Site, TableauJobID: input.ID, TableauRequestID: requestID, Summary: "Tableau did not acknowledge the cancellation request.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact job before attempting cancellation again.", Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	confirmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	confirmed, err := client.Inspect(confirmCtx, input.ID)
	if err != nil {
		return CancelResult{Status: status, Environment: connection.Environment, Site: connection.Site, RequestID: requestID}, &errs.Error{ID: "job.cancel.confirmation", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.Environment, Site: connection.Site, TableauJobID: input.ID, TableauRequestID: requestID, Summary: "Tableau accepted cancellation, but bounded confirmation could not read the exact job state.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Retain the cancellation request ID and inspect the exact job; do not submit a replacement job.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
	}
	if confirmed.Status == "cancelled" {
		return CancelResult{Status: confirmed, Environment: connection.Environment, Site: connection.Site, RequestID: requestID, Confirmed: true}, nil
	}
	return CancelResult{Status: confirmed, Environment: connection.Environment, Site: connection.Site, RequestID: requestID, Warnings: []string{"Cancellation was acknowledged, but the exact job is not yet cancelled; inspect the exact job again before taking further action."}}, nil
}

func (c *Service) connection(ctx context.Context, environment, site, operation, id string) (Session, error) {
	connection, err := c.provider.Open(ctx, environment)
	if err != nil {
		resolvedSite := site
		if connection.Environment != "" {
			environment = connection.Environment
		}
		if connection.Site != "" || connection.Environment != "" {
			resolvedSite = connection.Site
		}
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the environment, site, and PAT configuration.")
		return connection, &errs.Error{ID: operation + ".setup", Kind: errs.KindOperation, Operation: operation, Resource: id, Environment: environment, Site: resolvedSite, TableauJobID: id, Summary: "Job connection setup failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if site != "" && site != connection.Site {
		return connection, &errs.Error{ID: operation + ".target", Kind: errs.KindUsage, Operation: operation, Resource: id, Environment: connection.Environment, Site: site, TableauJobID: id, Summary: "The requested site does not match the selected environment.", Retryable: errs.Bool(false), CorrectiveAction: "Select the configured environment that owns the exact job site.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return connection, nil
}

func (c *Service) resolveReceipt(ctx context.Context, store jobmonitor.Store, input WaitInput) (jobmonitor.Receipt, string, error) {
	if input.Receipt != "" {
		receipt, err := store.ReadPath(input.Receipt)
		return receipt, filepath.Clean(input.Receipt), err
	}
	return store.FindByJobID(ctx, input.ID, input.Environment, input.Site)
}

func (c *Service) startObservation(ctx context.Context, store jobmonitor.Store, input WaitInput) (jobmonitor.Receipt, string, error) {
	connection, err := c.connection(ctx, input.Environment, input.Site, "job.wait", input.ID)
	if err != nil {
		return jobmonitor.Receipt{}, "", err
	}
	client := connection.Native
	status, err := client.Inspect(ctx, input.ID)
	if err != nil {
		return jobmonitor.Receipt{}, "", c.observationError("job.wait", connection.Environment, connection.Site, input.ID, "The exact job could not be established for monitoring.", err)
	}
	key, err := connection.CoordinationKey(ctx)
	if err != nil {
		return jobmonitor.Receipt{}, "", err
	}
	receipt := jobmonitor.Receipt{
		Version:           1,
		Operation:         "job.wait",
		Environment:       connection.Environment,
		Server:            connection.Server,
		Site:              connection.Site,
		SiteID:            connection.SiteID,
		ConfigPath:        connection.ConfigPath,
		CoordinationKey:   key,
		TrackingStartedAt: time.Now().UTC(),
		Observation:       status,
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	path, err := store.Save(saveCtx, receipt)
	if err != nil {
		return receipt, path, fmt.Errorf("save observation receipt: %w", err)
	}
	return receipt, path, nil
}

func (c *Service) receiptTargetError(receipt jobmonitor.Receipt, path, environment, site, summary string, cause error) error {
	return &errs.Error{ID: "job.wait.target", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: environment, Site: site, TableauJobID: receipt.Observation.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: c.receiptRecoveryHint(receipt, path), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}

func (c *Service) receiptRecoveryError(receipt jobmonitor.Receipt, path, summary string, cause error, phase errs.Phase, outcome errs.Outcome) error {
	return &errs.Error{ID: "job.wait.recovery", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: receipt.Environment, Site: receipt.Site, TableauJobID: receipt.Observation.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: c.receiptRecoveryHint(receipt, path), Phase: phase, Outcome: outcome}
}

func (c *Service) receiptRecoveryHint(receipt jobmonitor.Receipt, path string) string {
	args := make([]string, 0, 5)
	if receipt.ConfigPath != "" {
		args = append(args, "--config", receipt.ConfigPath)
	}
	args = append(args, "job", "wait", "--receipt", filepath.ToSlash(path))
	return "Recover the accepted job with its saved target: " + commandhint.Command(args...)
}

func verifyReceiptTarget(connection Session, receipt jobmonitor.Receipt) error {
	if jobmonitor.NormalizeServer(connection.Server) != jobmonitor.NormalizeServer(receipt.Server) || connection.Site != receipt.Site || connection.SiteID != receipt.SiteID {
		return &errs.Error{ID: "job.wait.target", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: receipt.Environment, Site: receipt.Site, TableauJobID: receipt.Observation.ID, Summary: "The saved job receipt does not match the authenticated environment and site.", Retryable: errs.Bool(false), CorrectiveAction: "Use the original environment configuration and durable receipt; do not redirect recovery to another site.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return nil
}

func (c *Service) observationError(operation, environment, site, id, summary string, cause error) error {
	return &errs.Error{ID: operation + ".inspect", Kind: errs.KindOperation, Operation: operation, Resource: id, Environment: environment, Site: site, TableauJobID: id, TableauRequestID: errs.TableauRequestID(cause), Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect or recover the exact job identity; do not infer success or failure from an unavailable observation.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
}

func supportedCancellation(jobType string) bool {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "refreshextract", "refreshworkbook", "refreshflow", "runflow":
		return true
	default:
		return false
	}
}
