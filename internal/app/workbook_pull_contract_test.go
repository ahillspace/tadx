package app

import (
	"context"

	workbook "github.com/ahillspace/tadx/actions/workbook"
)

type workbookPullTestProvider struct {
	input     workbook.PullInput
	reader    workbook.PullReader
	writer    workbook.ArtifactWriter
	previewer workbook.PullPreviewer
}

func (p workbookPullTestProvider) ResolveWorkbookWorkspace(context.Context, string, string, string) (workbook.PullWorkspace, error) {
	return workbook.PullWorkspace{Root: p.input.Workspace, Name: p.input.WorkspaceName}, nil
}

func (p workbookPullTestProvider) OpenWorkbookPull(context.Context, string, string) (workbook.PullSession, error) {
	return workbook.PullSession{Environment: p.input.Environment, Site: p.input.Site, SiteLUID: p.input.SiteLUID, ServerOrigin: p.input.ServerOrigin, Reader: p.reader, Writer: p.writer, Previewer: p.previewer}, nil
}

func runWorkbookPull(ctx context.Context, reader workbook.PullReader, writer workbook.ArtifactWriter, previewer workbook.PullPreviewer, input workbook.PullInput) (workbook.PullOutput, error) {
	return workbook.New(workbook.Ports{Pull: workbookPullTestProvider{input: input, reader: reader, writer: writer, previewer: previewer}}).PullWorkbook(ctx, input)
}
