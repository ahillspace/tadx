package cli_test

import (
	"context"
	authcheck "github.com/ahillspace/tadx/actions/auth"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
)

type checker struct{ input authcheck.CheckInput }

func (c *checker) Check(_ context.Context, input authcheck.CheckInput) (authcheck.CheckOutput, error) {
	c.input = input
	return authcheck.CheckOutput{Status: "authenticated"}, nil
}

type searcher struct{ input searchaction.Input }

func (s *searcher) Execute(_ context.Context, input searchaction.Input) (searchaction.Output, error) {
	s.input = input
	return searchaction.Output{Items: []searchaction.Item{}, Help: []string{}}, nil
}

type puller struct{ input workbookops.PullInput }

func (p *puller) PullWorkbook(_ context.Context, input workbookops.PullInput) (workbookops.PullOutput, error) {
	p.input = input
	return workbookops.PullOutput{Status: "pulled"}, nil
}

type publisher struct {
	input   workbookops.PublishInput
	preview bool
}

func (p *publisher) PublishWorkbook(_ context.Context, input workbookops.PublishInput, preview bool) (workbookops.PublishOutput, error) {
	p.input, p.preview = input, preview
	return workbookops.PublishOutput{Plan: workbookops.PublishPlan{Mode: "preview"}}, nil
}
