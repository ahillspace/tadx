package workbook

import (
	"context"
	"errors"
	"strings"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

// WorkbookChanges is the exact native capability required by workbook mutations.
type WorkbookChanges interface {
	Update(context.Context, tableauworkbook.UpdateRequest) (tableauworkbook.MutationResult, error)
	Delete(context.Context, string) (tableauworkbook.MutationResult, error)
}

// MutationPort combines authoritative resolution with one native mutation client.
type MutationPort struct {
	*Adapter
	changes WorkbookChanges
}

func NewMutationPort(reader *Adapter, changes WorkbookChanges) *MutationPort {
	return &MutationPort{Adapter: reader, changes: changes}
}

func (p *MutationPort) MoveWorkbook(ctx context.Context, luid, projectLUID string) (workbookops.MoveResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(luid) == "" || strings.TrimSpace(projectLUID) == "" {
		return workbookops.MoveResult{}, errors.New("workbook move requires configured client and exact workbook and project LUIDs")
	}
	result, err := p.changes.Update(ctx, tableauworkbook.UpdateRequest{LUID: luid, ProjectLUID: &projectLUID})
	return workbookops.MoveResult{Status: result.Status, WorkbookLUID: result.WorkbookLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

func (p *MutationPort) UpdateWorkbook(ctx context.Context, input workbookops.UpdateRequest) (workbookops.UpdateResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(input.LUID) == "" {
		return workbookops.UpdateResult{}, errors.New("workbook LUID and configured client are required")
	}
	result, err := p.changes.Update(ctx, tableauworkbook.UpdateRequest{LUID: input.LUID, Name: input.Name, OwnerLUID: input.OwnerLUID, Description: input.Description})
	return workbookops.UpdateResult{Status: result.Status, WorkbookLUID: result.WorkbookLUID, WorkbookName: result.WorkbookName, ProjectLUID: result.ProjectLUID, OwnerLUID: result.OwnerLUID, Description: result.Description, EvidenceSource: result.EvidenceSource, TableauRequestID: result.TableauRequestID}, err
}

func (p *MutationPort) DeleteWorkbook(ctx context.Context, luid string) (workbookops.DeleteResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(luid) == "" {
		return workbookops.DeleteResult{}, errors.New("workbook LUID and configured client are required")
	}
	result, err := p.changes.Delete(ctx, luid)
	return workbookops.DeleteResult{Status: result.Status, WorkbookLUID: result.WorkbookLUID, TableauRequestID: result.TableauRequestID}, err
}
