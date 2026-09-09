package clierr_test

import (
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestOutputCarrierPreservesCauseWithoutInventingResults(t *testing.T) {
	cause := &errs.Error{Kind: errs.KindUsage, Summary: "invalid input"}
	for _, value := range []any{nil, (*string)(nil), "", struct{ ID string }{}} {
		if got := clierr.WithOutput(value, cause); got != cause {
			t.Fatalf("empty output changed error: %v", got)
		}
	}
	value := struct{ ID string }{"known-id"}
	if clierr.WithOutput(value, nil) != nil {
		t.Fatal("success became error")
	}
	err := clierr.WithOutput(value, cause)
	if !errors.Is(err, cause) || errs.ExitCode(err) != errs.ExitCode(cause) || clierr.IsRendered(err) {
		t.Fatal("cause or rendering contract lost")
	}
	carrier, ok := err.(interface{ OperationOutput() any })
	if !ok || carrier.OperationOutput() != value {
		t.Fatal("typed result lost")
	}
}
