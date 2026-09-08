package cli_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/cli"
)

func TestEnvironmentAliasUsesSameReadAndWriteTarget(t *testing.T) {
	for _, flag := range []string{"--environment", "--env"} {
		t.Run(flag, func(t *testing.T) {
			read := &searcher{}
			write := &publisher{}
			deps := dependencies(&lister{}, &getter{}, &renderer{})
			deps.Searcher = read
			deps.WorkbookPuller, deps.WorkbookPublisher = &puller{}, write
			root := cli.NewRoot(deps)
			root.SetArgs([]string{"search", "sales", flag, "staging"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if read.input.Environment != "staging" {
				t.Fatalf("read environment = %q", read.input.Environment)
			}
			root = cli.NewRoot(deps)
			root.SetArgs([]string{"content", "workbook", "publish", flag + "=staging", "--workspace", "analytics", "--artifact", "artifacts/workbook/Sales--identity", "--project-id", "project-1", "--preview"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if write.input.Environment != "staging" || !write.preview {
				t.Fatalf("write environment = %q, preview = %v", write.input.Environment, write.preview)
			}
		})
	}
}
