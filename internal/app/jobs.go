package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jobcancel "github.com/ahillspace/tadx/actions/job/cancel"
	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
	jobwait "github.com/ahillspace/tadx/actions/job/wait"
	jobcli "github.com/ahillspace/tadx/internal/cli/job"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaujob "github.com/ahillspace/tadx/internal/tableau/job"
)

type jobCommands struct {
	runtime *runtimeDependencies
	inspect *jobinspect.Action
	wait    *jobwait.Action
	cancel  *jobcancel.Action
}

func newJobCommands(runtime *runtimeDependencies) *jobCommands {
	commands := &jobCommands{runtime: runtime}
	commands.inspect = jobinspect.New(commands)
	commands.wait = jobwait.New(commands)
	commands.cancel = jobcancel.New(commands)
	return commands
}

func (c *jobCommands) dependencies() *jobcli.Dependencies {
	return &jobcli.Dependencies{
		Inspector:  commandsInspector{action: c.inspect},
		Waiter:     commandsWaiter{action: c.wait},
		Canceller:  commandsCanceller{action: c.cancel},
		InspectUse: registryLeafUse("job.inspect"), InspectShort: registryShort("job.inspect"),
		WaitUse: registryLeafUse("job.wait"), WaitShort: registryShort("job.wait"),
		CancelUse: registryLeafUse("job.cancel"), CancelShort: registryShort("job.cancel"),
	}
}

type commandsInspector struct{ action *jobinspect.Action }

func (c commandsInspector) Execute(ctx context.Context, input jobinspect.Input) (jobinspect.Output, error) {
	return c.action.Execute(ctx, input)
}

type commandsWaiter struct{ action *jobwait.Action }

func (c commandsWaiter) Execute(ctx context.Context, input jobwait.Input) (jobwait.Output, error) {
	return c.action.Execute(ctx, input)
}

type commandsCanceller struct{ action *jobcancel.Action }

func (c commandsCanceller) Execute(ctx context.Context, input jobcancel.Input) (jobcancel.Output, error) {
	return c.action.Execute(ctx, input)
}

func (c *jobCommands) Inspect(ctx context.Context, input jobinspect.Input) (jobinspect.Result, error) {
	if input.OperationID != "" {
		return c.runtime.inspectPublicationOperation(ctx, input)
	}
	connection, err := c.connection(ctx, input.Environment, input.Site, "job.inspect", input.ID)
	if err != nil {
		return jobinspect.Result{}, err
	}
	client := tableaujob.NewClient(connection.transport, connection.session, connection.environment.URL)
	status, err := client.Inspect(ctx, input.ID)
	if err != nil {
		return jobinspect.Result{Status: status, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Attempts: 1}, c.observationError("job.inspect", connection.environment.Alias, connection.environment.SiteContentURL, input.ID, "Job status could not be read.", err)
	}
	return jobinspect.Result{Status: status, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Attempts: 1}, nil
}

func (c *jobCommands) Wait(ctx context.Context, input jobwait.Input) (jobwait.Result, error) {
	store, err := c.store()
	if err != nil {
		return jobwait.Result{}, err
	}
	receipt, path, err := c.resolveReceipt(ctx, store, input)
	if err != nil && input.Receipt == "" && errors.Is(err, os.ErrNotExist) {
		receipt, path, err = c.startObservation(ctx, store, input)
	}
	if err != nil {
		return jobwait.Result{}, &errs.Error{ID: "job.wait.receipt", Kind: errs.KindOperation, Operation: "job.wait", Resource: input.ID, Environment: input.Environment, Site: input.Site, Summary: "The accepted job receipt could not be recovered.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Provide the exact durable receipt path or job ID returned by the accepted operation.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if input.Environment != "" && input.Environment != receipt.Environment {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptTargetError(receipt, path, input.Environment, input.Site, "The requested environment does not match the durable job target.", nil)
	}
	if input.Site != "" && input.Site != receipt.Site {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptTargetError(receipt, path, receipt.Environment, input.Site, "The requested site does not match the durable job target.", nil)
	}
	if receipt.Observation.Terminal() {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, nil
	}
	connection, err := c.runtime.tableauConnection(ctx, receipt.Environment, false)
	if err != nil {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptRecoveryError(receipt, path, "The saved job target could not be authenticated for recovery.", err, errs.PhaseSetup, errs.OutcomeNotAttempted)
	}
	if err := verifyReceiptTarget(connection, receipt); err != nil {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptTargetError(receipt, path, receipt.Environment, receipt.Site, "The saved job receipt does not match the authenticated environment and site.", err)
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
	receipt.WaitUntil = time.Now().Add(publicationWaitLimit)
	if _, err := store.Register(ctx, receipt); err != nil {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, &errs.Error{ID: "job.wait.register", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: receipt.Environment, Site: receipt.Site, TableauJobID: receipt.Observation.ID, Summary: "The accepted job could not be restored to the local monitoring pool.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: c.receiptRecoveryHint(receipt, path), Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown}
	}
	if err := c.runtime.commandSessions().Suspend(ctx); err != nil {
		return jobwait.Result{Status: receipt.Observation, Environment: receipt.Environment, Site: receipt.Site, ReceiptPath: filepath.ToSlash(path)}, c.receiptRecoveryError(receipt, path, "Credential coordination could not be released before job monitoring.", err, errs.PhaseSetup, errs.OutcomeUnknown)
	}
	monitor := jobmonitor.Monitor{Store: store, Deadline: receipt.WaitUntil, Observe: c.observer(connection)}
	latest, err := monitor.Wait(ctx, receipt)
	result := jobwait.Result{Status: latest.Observation, Environment: latest.Environment, Site: latest.Site, ReceiptPath: filepath.ToSlash(path)}
	if err != nil && !errors.Is(err, jobmonitor.ErrWaitLimit) {
		return result, c.receiptRecoveryError(receipt, path, "Job monitoring stopped without establishing the remote terminal outcome.", err, errs.PhaseVerification, errs.OutcomeUnknown)
	}
	return result, nil
}

func (c *jobCommands) Cancel(ctx context.Context, input jobcancel.Input) (jobcancel.Result, error) {
	connection, err := c.connection(ctx, input.Environment, input.Site, "job.cancel", input.ID)
	if err != nil {
		return jobcancel.Result{}, err
	}
	client := tableaujob.NewClient(connection.transport, connection.session, connection.environment.URL)
	status, err := client.Inspect(ctx, input.ID)
	if err != nil {
		return jobcancel.Result{}, c.observationError("job.cancel", input.Environment, connection.environment.SiteContentURL, input.ID, "The exact job could not be inspected before cancellation.", err)
	}
	if !supportedCancellation(status.Type) {
		return jobcancel.Result{}, &errs.Error{ID: "job.cancel.unsupported", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, TableauJobID: input.ID, Summary: "Tableau does not document cancellation for this job type.", Retryable: errs.Bool(false), CorrectiveAction: "Use job inspect or job wait for this job; cancellation is limited to documented refresh and flow-run job types.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	}
	if status.Terminal() {
		return jobcancel.Result{}, &errs.Error{ID: "job.cancel.terminal", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, TableauJobID: input.ID, Summary: "The exact job is already terminal and was not cancelled.", Retryable: errs.Bool(false), CorrectiveAction: "Use the authoritative terminal status; do not repeat cancellation.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeConfirmed}
	}
	if input.Preview {
		return jobcancel.Result{Status: status, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Warnings: []string{"Preview only; Tableau was not asked to cancel the job."}}, nil
	}
	requestID, err := client.Cancel(ctx, input.ID)
	if err != nil {
		return jobcancel.Result{Status: status, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, RequestID: requestID}, &errs.Error{ID: "job.cancel.request", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, TableauJobID: input.ID, TableauRequestID: requestID, Summary: "Tableau did not acknowledge the cancellation request.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact job before attempting cancellation again.", Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	confirmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	confirmed, err := client.Inspect(confirmCtx, input.ID)
	if err != nil {
		return jobcancel.Result{Status: status, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, RequestID: requestID}, &errs.Error{ID: "job.cancel.confirmation", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.ID, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, TableauJobID: input.ID, TableauRequestID: requestID, Summary: "Tableau accepted cancellation, but bounded confirmation could not read the exact job state.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Retain the cancellation request ID and inspect the exact job; do not submit a replacement job.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
	}
	if confirmed.Status == "cancelled" {
		return jobcancel.Result{Status: confirmed, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, RequestID: requestID, Confirmed: true}, nil
	}
	return jobcancel.Result{Status: confirmed, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, RequestID: requestID, Warnings: []string{"Cancellation was acknowledged, but the exact job is not yet cancelled; inspect the exact job again before taking further action."}}, nil
}

func (c *jobCommands) connection(ctx context.Context, environment, site, operation, id string) (authenticatedTableau, error) {
	connection, err := c.runtime.tableauConnection(ctx, environment, false)
	if err != nil {
		return connection, err
	}
	if site != "" && site != connection.environment.SiteContentURL {
		return connection, &errs.Error{ID: operation + ".target", Kind: errs.KindUsage, Operation: operation, Resource: id, Environment: connection.environment.Alias, Site: site, Summary: "The requested site does not match the selected environment.", Retryable: errs.Bool(false), CorrectiveAction: "Select the configured environment that owns the exact job site."}
	}
	return connection, nil
}

func (c *jobCommands) store() (jobmonitor.Store, error) {
	directory := c.runtime.jobDirectory
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return jobmonitor.Store{}, errors.New("job recovery directory is unavailable")
		}
		directory = filepath.Join(cache, "tadx", "jobs")
	}
	return jobmonitor.Store{Directory: directory}, nil
}

func (c *jobCommands) resolveReceipt(ctx context.Context, store jobmonitor.Store, input jobwait.Input) (jobmonitor.Receipt, string, error) {
	if input.Receipt != "" {
		receipt, err := store.ReadPath(input.Receipt)
		return receipt, filepath.Clean(input.Receipt), err
	}
	return store.FindByJobID(ctx, input.ID, input.Environment, input.Site)
}

func (c *jobCommands) startObservation(ctx context.Context, store jobmonitor.Store, input jobwait.Input) (jobmonitor.Receipt, string, error) {
	connection, err := c.connection(ctx, input.Environment, input.Site, "job.wait", input.ID)
	if err != nil {
		return jobmonitor.Receipt{}, "", err
	}
	client := tableaujob.NewClient(connection.transport, connection.session, connection.environment.URL)
	status, err := client.Inspect(ctx, input.ID)
	if err != nil {
		return jobmonitor.Receipt{}, "", c.observationError("job.wait", connection.environment.Alias, connection.environment.SiteContentURL, input.ID, "The exact job could not be established for monitoring.", err)
	}
	key, err := c.runtime.commandSessions().CoordinationKey(ctx, authTarget(connection))
	if err != nil {
		return jobmonitor.Receipt{}, "", err
	}
	receipt := jobmonitor.Receipt{
		Version:           1,
		Operation:         "job.wait",
		Environment:       connection.environment.Alias,
		Server:            connection.environment.URL,
		Site:              connection.environment.SiteContentURL,
		SiteID:            connection.session.SiteLUID(),
		ConfigPath:        c.runtime.configPath,
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

func (c *jobCommands) observer(connection authenticatedTableau) func(context.Context, []jobmonitor.Receipt) []jobmonitor.CheckResult {
	return func(ctx context.Context, jobs []jobmonitor.Receipt) []jobmonitor.CheckResult {
		results := make([]jobmonitor.CheckResult, len(jobs))
		sessions := c.runtime.commandSessions()
		for i, receipt := range jobs {
			session, err := sessions.AuthenticateMonitor(ctx, authTarget(connection), tableauauth.NewClient(connection.transport))
			if err == nil {
				client := tableaujob.NewClient(connection.transport, session, connection.environment.URL)
				if normalizeServer(receipt.Server) != normalizeServer(connection.environment.URL) || receipt.SiteID != session.SiteLUID() {
					err = errors.New("job receipt target does not match the authenticated site")
				} else {
					results[i].Status, err = client.Inspect(ctx, receipt.Observation.ID)
					if err == nil && receipt.Observation.Type != "" && results[i].Status.Type != receipt.Observation.Type {
						err = errors.New("job observation changed the accepted job type")
					}
				}
			}
			// Foreground requests may enter between individual exact observations.
			results[i].Err = errors.Join(err, sessions.Suspend(context.WithoutCancel(ctx)))
		}
		return results
	}
}

func (c *jobCommands) receiptTargetError(receipt jobmonitor.Receipt, path, environment, site, summary string, cause error) error {
	return &errs.Error{ID: "job.wait.target", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: environment, Site: site, TableauJobID: receipt.Observation.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: c.receiptRecoveryHint(receipt, path), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}

func (c *jobCommands) receiptRecoveryError(receipt jobmonitor.Receipt, path, summary string, cause error, phase errs.Phase, outcome errs.Outcome) error {
	return &errs.Error{ID: "job.wait.recovery", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: receipt.Environment, Site: receipt.Site, TableauJobID: receipt.Observation.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: c.receiptRecoveryHint(receipt, path), Phase: phase, Outcome: outcome}
}

func (c *jobCommands) receiptRecoveryHint(receipt jobmonitor.Receipt, path string) string {
	args := make([]string, 0, 5)
	if receipt.ConfigPath != "" {
		args = append(args, "--config", receipt.ConfigPath)
	}
	args = append(args, "job", "wait", "--receipt", filepath.ToSlash(path))
	return "Recover the accepted job with its saved target: " + commandhint.Command(args...)
}

func verifyReceiptTarget(connection authenticatedTableau, receipt jobmonitor.Receipt) error {
	if normalizeServer(connection.environment.URL) != normalizeServer(receipt.Server) || connection.environment.SiteContentURL != receipt.Site || connection.session.SiteLUID() != receipt.SiteID {
		return &errs.Error{ID: "job.wait.target", Kind: errs.KindOperation, Operation: "job.wait", Resource: receipt.Observation.ID, Environment: receipt.Environment, Site: receipt.Site, TableauJobID: receipt.Observation.ID, Summary: "The saved job receipt does not match the authenticated environment and site.", Retryable: errs.Bool(false), CorrectiveAction: "Use the original environment configuration and durable receipt; do not redirect recovery to another site.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return nil
}

func (c *jobCommands) observationError(operation, environment, site, id, summary string, cause error) error {
	return &errs.Error{ID: operation + ".inspect", Kind: errs.KindOperation, Operation: operation, Resource: id, Environment: environment, Site: site, TableauJobID: id, TableauRequestID: errs.TableauRequestID(cause), Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect or recover the exact job identity; do not infer success or failure from an unavailable observation.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
}

func normalizeServer(value string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(value)), "/")
}

func supportedCancellation(jobType string) bool {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "refreshextract", "refreshworkbook", "refreshflow", "runflow":
		return true
	default:
		return false
	}
}

var _ jobinspect.Source = (*jobCommands)(nil)
var _ jobwait.Source = (*jobCommands)(nil)
var _ jobcancel.Source = (*jobCommands)(nil)
