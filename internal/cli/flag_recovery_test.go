package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/cli"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestUnknownFlagsProvideActionableRecovery(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"search", "sales", "--site", "staging"}, "--environment"},
		{[]string{"search", "--terms", "sales"}, `tadx search "<term>"`},
		{[]string{"search", "sales", "--tpye", "workbook"}, "tadx search --help"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			read := &searcher{}
			deps := dependencies(&lister{}, &getter{}, &renderer{})
			deps.Searcher = read
			root := cli.NewRoot(deps)
			root.SetArgs(tc.args)
			var structured *errs.Error
			if err := root.Execute(); !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
				t.Fatalf("expected usage error, got %v", err)
			}
			if !strings.Contains(structured.CorrectiveAction, tc.want) {
				t.Fatalf("corrective action = %q, want %q", structured.CorrectiveAction, tc.want)
			}
			if read.input.Terms != "" {
				t.Fatal("invalid flags invoked search")
			}
		})
	}
}
