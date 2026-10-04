package datasource

import (
	"context"
	"errors"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/value"
)

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

// NewPublishAcceptance preserves datasource-specific native acceptance translation.
func NewPublishAcceptance(monitor *jobmonitor.Publication, asJob bool, environment, site string, destination FreshPublicationDestination, bulk func(context.Context) bool) PublishAcceptance {
	return PublishAcceptance{
		AsJob: asJob, Environment: environment, Site: site,
		Accepted: func(ctx context.Context, jobID, requestID string) (AcceptedPublication, error) {
			receipt, path, err := monitor.Accepted(ctx, jobID, requestID, "PublishDatasource", bulk(ctx))
			return AcceptedPublication{Status: receipt.Observation.Status, ResourceID: receipt.Observation.ResourceID, RequestID: receipt.Observation.RequestID, ReceiptPath: path}, err
		},
		Destination: destination.Confirm,
	}
}

// PublishPorts owns target-bound datasource resolution and native request translation.
type PublishPorts struct {
	Datasources *Adapter
	Projects    interface {
		ResolveProjectIdentity(context.Context, identity.Selector) (value.ProjectIdentity, error)
		BeginProjectResolution(context.Context) context.Context
	}
	Changes  *MutationAdapter
	Fresh    func(context.Context) (*Adapter, error)
	Begin    func(context.Context, datasource.PublishRequest) (PublishAcceptance, error)
	Progress func(context.Context, string)
}

func (p PublishPorts) BeginProjectResolution(ctx context.Context) context.Context {
	return p.Projects.BeginProjectResolution(ctx)
}

func (p PublishPorts) ResolveProject(ctx context.Context, selector identity.Selector) (datasource.Project, error) {
	return p.Projects.ResolveProjectIdentity(ctx, selector)
}

func (p PublishPorts) FindDatasources(ctx context.Context, name, projectLUID string) ([]datasource.Record, error) {
	return p.Datasources.FindDatasources(ctx, name, projectLUID)
}

func (p PublishPorts) ResolvePublishedDatasource(ctx context.Context, name, projectLUID string) (datasource.Record, error) {
	adapter := p.Datasources
	if p.Fresh != nil {
		var err error
		adapter, err = p.Fresh(ctx)
		if err != nil {
			return datasource.Record{}, err
		}
	}
	item, err := adapter.ResolvePublishedDatasource(ctx, name, projectLUID)
	if _, notVisible := errors.AsType[*PublishedDatasourceNotVisibleError](err); notVisible {
		return datasource.Record{}, datasource.PublishErrPublishedDatasourceNotVisible
	}
	return item, err
}

func (p PublishPorts) Prepare(ctx context.Context, input datasource.PublishRequest) (datasource.PreparedPublish, error) {
	mode, err := datasourcePublishMode(input.Mode)
	if err != nil {
		return nil, err
	}
	request := tableaudatasource.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Mode: mode, ParentDataSourceURLs: append([]string(nil), input.ParentDataSourceURLs...), AsJob: input.AsJob}
	if p.Begin != nil {
		acceptance, err := p.Begin(ctx, input)
		if err != nil {
			return nil, err
		}
		request.AsJob = acceptance.AsJob
		request.Accepted = acceptance.result
	}
	prepared, err := p.Changes.PrepareDatasource(ctx, request)
	if err != nil {
		return nil, err
	}
	return preparedDatasourcePublish{prepared: prepared, progress: p.Progress}, nil
}

func (a PublishAcceptance) result(ctx context.Context, jobID, requestID string) (tableaudatasource.PublishResult, error) {
	observed, err := a.Accepted(ctx, jobID, requestID)
	result := tableaudatasource.PublishResult{Status: observed.Status, JobID: jobID, TableauRequestID: observed.RequestID, ReceiptPath: observed.ReceiptPath}
	if err == nil && result.Status == "succeeded" && observed.ResourceID != "" {
		result.DatasourceLUID, result.DatasourceName, result.ProjectLUID, err = a.Destination(ctx, observed.ResourceID)
	}
	return result, errs.PublicationMonitorError("datasource.publish", a.Environment, a.Site, jobID, result.Status, err)
}

func datasourcePublishMode(mode datasource.Mode) (tableaudatasource.PublishMode, error) {
	switch mode {
	case datasource.ModeCreate:
		return tableaudatasource.PublishCreate, nil
	case datasource.ModeOverwrite:
		return tableaudatasource.PublishOverwrite, nil
	case datasource.ModeAppend:
		return tableaudatasource.PublishAppend, nil
	case datasource.ModeReplace:
		return tableaudatasource.PublishReplace, nil
	default:
		return "", errors.New("unsupported datasource publish mode")
	}
}

type preparedDatasourcePublish struct {
	prepared tableaudatasource.PreparedPublish
	progress func(context.Context, string)
}

func (p preparedDatasourcePublish) Commit(ctx context.Context) (datasource.PublishResult, error) {
	if p.progress != nil {
		p.progress(ctx, "Uploading and submitting datasource")
	}
	result, err := p.prepared.Commit(ctx)
	return datasource.PublishResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, ProjectLUID: result.ProjectLUID, JobID: result.JobID, TableauRequestID: result.TableauRequestID, ReceiptPath: result.ReceiptPath}, err
}
