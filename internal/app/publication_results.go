package app

import (
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func (p *publication) acceptWorkbook(ctx context.Context, id, requestID string) (tableauworkbook.PublishResult, error) {
	r, path, err := p.accepted(ctx, id, requestID, "PublishWorkbook")
	result := tableauworkbook.PublishResult{Status: r.Observation.Status, JobID: id, TableauRequestID: r.Observation.RequestID, ReceiptPath: path}
	if err == nil && result.Status == "succeeded" && r.Observation.ResourceID != "" {
		result.WorkbookLUID, result.WorkbookName, result.ProjectLUID, err = p.destination(ctx, r)
	}
	return result, publicationError(p.base.Operation, p.base.Environment, p.base.Site, id, result.Status, err)
}

func (p *publication) acceptDatasource(ctx context.Context, id, requestID string) (tableaudatasource.PublishResult, error) {
	r, path, err := p.accepted(ctx, id, requestID, "PublishDatasource")
	result := tableaudatasource.PublishResult{Status: r.Observation.Status, JobID: id, TableauRequestID: r.Observation.RequestID, ReceiptPath: path}
	if err == nil && result.Status == "succeeded" && r.Observation.ResourceID != "" {
		result.DatasourceLUID, result.DatasourceName, result.ProjectLUID, err = p.destination(ctx, r)
	}
	return result, publicationError(p.base.Operation, p.base.Environment, p.base.Site, id, result.Status, err)
}

func (p *publication) destination(ctx context.Context, r jobmonitor.Receipt) (string, string, string, error) {
	progress.SetLabel(ctx, "Confirming published content")
	connection, err := newRemoteContentCommands(p.runtime).connect(ctx, p.base.Environment, true)
	if err != nil {
		return r.Observation.ResourceID, "", "", err
	}
	selector := identity.Selector{LUID: identity.LUID(r.Observation.ResourceID)}
	var id, name, project string
	if p.base.Operation == "workbook.publish" {
		item, readErr := connection.workbooks.ResolveWorkbook(ctx, selector)
		id, name, project, err = item.LUID, item.Name, item.ProjectLUID, readErr
	} else {
		item, readErr := connection.datasources.ResolveDatasource(ctx, selector)
		id, name, project, err = item.LUID, item.Name, item.ProjectLUID, readErr
	}
	if id == "" {
		id = r.Observation.ResourceID
	}
	if err == nil && (id != r.Observation.ResourceID || name != p.base.Name || project != p.base.ProjectID) {
		err = errors.New("published destination does not match the accepted target")
	}
	return id, name, project, err
}

func (p *publication) completeWorkbook(ctx context.Context, action *workbookops.Publisher, out workbookops.PublishOutput) (workbookops.PublishOutput, error) {
	accepted := p.base
	accepted.Observation.ID = out.Result.JobID
	r, err := p.wait(ctx, accepted)
	if r.Observation.Status != "" {
		out.Result.Status, out.Result.TableauRequestID = r.Observation.Status, r.Observation.RequestID
	}
	if err == nil && r.Observation.ResourceID != "" {
		out.Result.WorkbookLUID, out.Result.WorkbookName, out.Result.ProjectLUID, err = p.destination(ctx, r)
	}
	if err != nil {
		if out.Result.Status == "succeeded" {
			out.Result.Verification = "destination_unavailable"
		}
		err = publicationError(p.base.Operation, p.base.Environment, p.base.Site, out.Result.JobID, out.Result.Status, err)
	} else {
		out, err = action.Complete(ctx, out)
	}
	path, saveErr := p.record(ctx, out.Result.JobID, out.Result.Status, out.Result.WorkbookLUID, out.Result.TableauRequestID, out.Result.Verification)
	out.Result.ReceiptPath = path
	err = errors.Join(err, saveErr)
	return out, err
}

func (p *publication) completeDatasource(ctx context.Context, action *datasourceops.Publisher, out datasourceops.PublishOutput) (datasourceops.PublishOutput, error) {
	accepted := p.base
	accepted.Observation.ID = out.Result.JobID
	r, err := p.wait(ctx, accepted)
	if r.Observation.Status != "" {
		out.Result.Status, out.Result.TableauRequestID = r.Observation.Status, r.Observation.RequestID
	}
	if err == nil && r.Observation.ResourceID != "" {
		out.Result.DatasourceLUID, out.Result.DatasourceName, out.Result.ProjectLUID, err = p.destination(ctx, r)
	}
	if err != nil {
		if out.Result.Status == "succeeded" {
			out.Result.Verification = "destination_unavailable"
		}
		err = publicationError(p.base.Operation, p.base.Environment, p.base.Site, out.Result.JobID, out.Result.Status, err)
	} else {
		out, err = action.Complete(ctx, out)
	}
	path, saveErr := p.record(ctx, out.Result.JobID, out.Result.Status, out.Result.DatasourceLUID, out.Result.TableauRequestID, out.Result.Verification)
	out.Result.ReceiptPath = path
	err = errors.Join(err, saveErr)
	return out, err
}
