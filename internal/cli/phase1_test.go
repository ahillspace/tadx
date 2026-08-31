package cli_test

import (
	"context"
	"reflect"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/cli"
)

type checker struct{ input authcheck.Input }

func (c *checker) Execute(_ context.Context, input authcheck.Input) (authcheck.Output, error) {
	c.input = input
	return authcheck.Output{Status: "authenticated"}, nil
}

type searcher struct{ input catalogsearch.Input }

func (s *searcher) Execute(_ context.Context, input catalogsearch.Input) (catalogsearch.Output, error) {
	s.input = input
	return catalogsearch.Output{Items: []catalogsearch.Item{}, Help: []string{}}, nil
}

type puller struct{ input workbookpull.Input }

func (p *puller) Execute(_ context.Context, input workbookpull.Input) (workbookpull.Output, error) {
	p.input = input
	return workbookpull.Output{Status: "pulled"}, nil
}

type publisher struct {
	input workbookpublish.Input
	apply bool
}

func (p *publisher) Execute(_ context.Context, input workbookpublish.Input, apply bool) (workbookpublish.Output, error) {
	p.input, p.apply = input, apply
	return workbookpublish.Output{Plan: workbookpublish.Plan{Mode: "preview"}, Applied: apply}, nil
}

func TestRootRegistersPhaseOneCapabilities(t *testing.T) {
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.AuthChecker = &checker{}
	deps.CatalogSearcher = &searcher{}
	deps.WorkbookPuller = &puller{}
	deps.WorkbookPublisher = &publisher{}
	root := cli.NewRoot(deps)
	registered, err := cli.RegisteredCommands(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []cli.RegisteredCommand{
		{CapabilityID: "auth.check", CommandPath: []string{"auth", "check"}},
		{CapabilityID: "capability.get", CommandPath: []string{"capability", "get"}},
		{CapabilityID: "capability.list", CommandPath: []string{"capability", "list"}},
		{CapabilityID: "catalog.search", CommandPath: []string{"catalog", "search"}},
		{CapabilityID: "workbook.publish", CommandPath: []string{"content", "workbook", "publish"}},
		{CapabilityID: "workbook.pull", CommandPath: []string{"content", "workbook", "pull"}},
	}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %#v, want %#v", registered, want)
	}
}

func TestWorkbookPublishPreviewsByDefaultAndRequiresApplyFlagToApply(t *testing.T) {
	for _, test := range []struct {
		name      string
		applyFlag []string
		wantApply bool
	}{
		{name: "preview"},
		{name: "apply", applyFlag: []string{"--apply"}, wantApply: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &publisher{}
			deps := dependencies(&lister{}, &getter{}, &renderer{})
			deps.AuthChecker, deps.CatalogSearcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, &puller{}, p
			root := cli.NewRoot(deps)
			args := []string{"content", "workbook", "publish", "--artifact", "artifact", "--environment", "production", "--project-id", "project-1"}
			args = append(args, test.applyFlag...)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if p.apply != test.wantApply || p.input.Environment != "production" || p.input.ProjectLUID != "project-1" {
				t.Fatalf("input = %#v, apply = %v", p.input, p.apply)
			}
		})
	}
}

func TestWorkbookPullPreservesExplicitIncludeExtractFalse(t *testing.T) {
	p := &puller{}
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.AuthChecker, deps.CatalogSearcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, p, &publisher{}
	root := cli.NewRoot(deps)
	root.SetArgs([]string{"content", "workbook", "pull", "--environment", "production", "--workspace", "workspace", "--id", "wb-1", "--include-extract=false"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if p.input.IncludeExtract == nil || *p.input.IncludeExtract {
		t.Fatalf("include extract = %#v", p.input.IncludeExtract)
	}
}

func TestWorkbookPullPassesIncludePublishedDatasourcesChoice(t *testing.T) {
	for _, test := range []struct {
		name string
		flag []string
		want bool
	}{
		{name: "default"},
		{name: "enabled", flag: []string{"--include-pds"}, want: true},
		{name: "explicit false", flag: []string{"--include-pds=false"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &puller{}
			deps := dependencies(&lister{}, &getter{}, &renderer{})
			deps.AuthChecker, deps.CatalogSearcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, p, &publisher{}
			root := cli.NewRoot(deps)
			args := []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "workspace", "--id", "wb-1"}
			root.SetArgs(append(args, test.flag...))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if p.input.IncludePDS != test.want {
				t.Fatalf("include PDS = %v, want %v", p.input.IncludePDS, test.want)
			}
		})
	}
}

func TestFullChangesPresentationStateWithoutChangingWorkbookPullInput(t *testing.T) {
	var inputs []workbookpull.Input
	for _, full := range []bool{false, true} {
		p := &puller{}
		mode := &cli.RenderOptions{}
		deps := dependencies(&lister{}, &getter{}, &renderer{})
		deps.RenderOptions = mode
		deps.AuthChecker, deps.CatalogSearcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, p, &publisher{}
		root := cli.NewRoot(deps)
		args := []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "workspace", "--id", "wb-1", "--include-pds"}
		if full {
			args = append(args, "--full")
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if mode.Full != full {
			t.Fatalf("full mode = %v, want %v", mode.Full, full)
		}
		inputs = append(inputs, p.input)
	}
	if !reflect.DeepEqual(inputs[0], inputs[1]) {
		t.Fatalf("presentation flag changed action input: compact=%#v full=%#v", inputs[0], inputs[1])
	}
}
