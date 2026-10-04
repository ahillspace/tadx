package cli_test

import (
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cli"
	"reflect"
	"testing"
)

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
	var inputs []workbookops.PullInput
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
