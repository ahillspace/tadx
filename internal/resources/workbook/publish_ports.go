package workbook

import (
	"context"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

// PublishAcceptance binds an exact receipt before the native upload is prepared.
type PublishAcceptance struct {
	AsJob       bool
	Environment string
	Site        string
	Accepted    func(context.Context, string, string) (AcceptedPublication, error)
	Destination func(context.Context, string) (id, name, project string, err error)
}

type AcceptedPublication struct {
	Status, ResourceID, RequestID, ReceiptPath string
}

// NewPublishAcceptance preserves workbook-specific native acceptance translation.
func NewPublishAcceptance(monitor *jobmonitor.Publication, asJob bool, environment, site string, destination FreshPublicationDestination, bulk func(context.Context) bool) PublishAcceptance {
	return PublishAcceptance{
		AsJob: asJob, Environment: environment, Site: site,
		Accepted: func(ctx context.Context, jobID, requestID string) (AcceptedPublication, error) {
			receipt, path, err := monitor.Accepted(ctx, jobID, requestID, "PublishWorkbook", bulk(ctx))
			return AcceptedPublication{Status: receipt.Observation.Status, ResourceID: receipt.Observation.ResourceID, RequestID: receipt.Observation.RequestID, ReceiptPath: path}, err
		},
		Destination: destination.Confirm,
	}
}

// PublishPorts translates workbook action requests at the native boundary.
type PublishPorts struct {
	Adapter  *Adapter
	Projects ProjectResolution
	Fresh    func(context.Context) (*Adapter, error)
	Begin    func(context.Context, workbook.PublishRequest) (PublishAcceptance, error)
	Progress func(context.Context, string)
}

func (p PublishPorts) BeginProjectResolution(ctx context.Context) context.Context {
	return p.Projects.BeginProjectResolution(ctx)
}

func (p PublishPorts) ResolveProject(ctx context.Context, selector identity.Selector) (workbook.Project, error) {
	return p.Projects.ResolveProjectIdentity(ctx, selector)
}

func (p PublishPorts) FindWorkbooks(ctx context.Context, name, project string) ([]workbook.Record, error) {
	adapter := p.Adapter
	if p.Fresh != nil {
		var err error
		adapter, err = p.Fresh(ctx)
		if err != nil {
			return nil, err
		}
	}
	return adapter.FindWorkbooks(ctx, name, project)
}

func (p PublishPorts) Prepare(ctx context.Context, input workbook.PublishRequest) (workbook.PreparedPublish, error) {
	request := tableauworkbook.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Overwrite: input.Overwrite, AsJob: input.AsJob}
	if p.Begin != nil {
		acceptance, err := p.Begin(ctx, input)
		if err != nil {
			return nil, err
		}
		request.AsJob = acceptance.AsJob
		request.Accepted = acceptance.result
	}
	prepared, err := p.Adapter.PrepareWorkbook(ctx, request)
	if err != nil {
		return nil, err
	}
	return preparedWorkbookPublish{prepared: prepared, progress: p.Progress}, nil
}

func (a PublishAcceptance) result(ctx context.Context, jobID, requestID string) (tableauworkbook.PublishResult, error) {
	observed, err := a.Accepted(ctx, jobID, requestID)
	result := tableauworkbook.PublishResult{Status: observed.Status, JobID: jobID, TableauRequestID: observed.RequestID, ReceiptPath: observed.ReceiptPath}
	if err == nil && result.Status == "succeeded" && observed.ResourceID != "" {
		result.WorkbookLUID, result.WorkbookName, result.ProjectLUID, err = a.Destination(ctx, observed.ResourceID)
	}
	return result, errs.PublicationMonitorError("workbook.publish", a.Environment, a.Site, jobID, result.Status, err)
}

type preparedWorkbookPublish struct {
	prepared *tableauworkbook.PreparedPublish
	progress func(context.Context, string)
}

func (p preparedWorkbookPublish) Commit(ctx context.Context) (workbook.PublishResult, error) {
	if p.progress != nil {
		p.progress(ctx, "Uploading and submitting workbook")
	}
	result, err := p.prepared.Commit(ctx)
	warnings := make([]workbook.PublishValidationIssue, len(result.Warnings))
	for index, warning := range result.Warnings {
		warnings[index] = workbook.PublishValidationIssue{Severity: warning.Severity, Message: warning.Message, Line: warning.Line, Column: warning.Column, ElementName: warning.ElementName}
	}
	return workbook.PublishResult{Status: result.Status, WorkbookLUID: result.WorkbookLUID, WorkbookName: result.WorkbookName, ProjectLUID: result.ProjectLUID, JobID: result.JobID, TableauRequestID: result.TableauRequestID, ReceiptPath: result.ReceiptPath, ValidationWarnings: warnings}, err
}
