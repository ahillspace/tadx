package cli_test

import (
	"context"
	"reflect"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/cli"
)

type checker struct{ input authcheck.Input }

func (c *checker) Execute(_ context.Context, input authcheck.Input) (authcheck.Output, error) {
	c.input = input
	return authcheck.Output{Status: "authenticated"}, nil
}

type searcher struct{ input searchaction.Input }

func (s *searcher) Execute(_ context.Context, input searchaction.Input) (searchaction.Output, error) {
	s.input = input
	return searchaction.Output{Items: []searchaction.Item{}, Help: []string{}}, nil
}

type puller struct{ input workbookpull.Input }

func (p *puller) Execute(_ context.Context, input workbookpull.Input) (workbookpull.Output, error) {
	p.input = input
	return workbookpull.Output{Status: "pulled"}, nil
}

type publisher struct {
	input   workbookpublish.Input
	preview bool
}

func (p *publisher) Execute(_ context.Context, input workbookpublish.Input, preview bool) (workbookpublish.Output, error) {
	p.input, p.preview = input, preview
	return workbookpublish.Output{Plan: workbookpublish.Plan{Mode: "preview"}}, nil
}

func TestRootRegistersPhaseOneCapabilities(t *testing.T) {
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.AuthChecker = &checker{}
	deps.Searcher = &searcher{}
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
		{CapabilityID: "workbook.publish", CommandPath: []string{"content", "workbook", "publish"}},
		{CapabilityID: "workbook.pull", CommandPath: []string{"content", "workbook", "pull"}},
		{CapabilityID: "search.run", CommandPath: []string{"search"}},
	}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %#v, want %#v", registered, want)
	}
}

func TestTopLevelSearchMapsCanonicalFlags(t *testing.T) {
	s := &searcher{}
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.Searcher = s
	root := cli.NewRoot(deps)
	root.SetArgs([]string{"search", "revenue", "--environment", "production", "--type", "workbook", "--cache", "--cursor", "next", "--limit", "12"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	want := searchaction.Input{Terms: "revenue", Environment: "production", Type: "workbook", Cache: true, Cursor: "next", Limit: 12}
	if !reflect.DeepEqual(s.input, want) {
		t.Fatalf("search input = %#v, want %#v", s.input, want)
	}
	if child, _, err := root.Find([]string{"cache", "search"}); err == nil && child.Name() == "search" {
		t.Fatal("obsolete cache search command is mounted")
	}
}

func TestWorkbookPublishPerformsByDefaultAndSupportsPreviewFlag(t *testing.T) {
	for _, test := range []struct {
		name        string
		previewFlag []string
		wantPreview bool
	}{
		{name: "perform"},
		{name: "preview", previewFlag: []string{"--preview"}, wantPreview: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &publisher{}
			deps := dependencies(&lister{}, &getter{}, &renderer{})
			deps.AuthChecker, deps.Searcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, &puller{}, p
			root := cli.NewRoot(deps)
			args := []string{"content", "workbook", "publish", "--workspace", "development", "--artifact", "artifacts/workbook/Finance--identity", "--environment", "production", "--project-id", "project-1"}
			args = append(args, test.previewFlag...)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if p.preview != test.wantPreview || p.input.Environment != "production" || p.input.ProjectLUID != "project-1" {
				t.Fatalf("input = %#v, preview = %v", p.input, p.preview)
			}
		})
	}
}

func TestWorkbookPullPreservesExplicitIncludeExtractFalse(t *testing.T) {
	p := &puller{}
	deps := dependencies(&lister{}, &getter{}, &renderer{})
	deps.AuthChecker, deps.Searcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, p, &publisher{}
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
			deps.AuthChecker, deps.Searcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, p, &publisher{}
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
		deps.AuthChecker, deps.Searcher, deps.WorkbookPuller, deps.WorkbookPublisher = &checker{}, &searcher{}, p, &publisher{}
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
