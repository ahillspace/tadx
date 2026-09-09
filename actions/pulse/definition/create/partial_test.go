package create_test

import (
	"context"
	"errors"
	"fmt"
	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	"github.com/ahillspace/tadx/internal/errs"
	"testing"
)

type partialCreator struct {
	result definitioncreate.CreateResult
	err    error
	calls  int
}

func (c *partialCreator) CreateDefinition(context.Context, definitioncreate.CreateRequest) (definitioncreate.CreateResult, error) {
	c.calls++
	return c.result, c.err
}

func TestConfirmedCreateRemainsTypedWhileUnknownWriteDoesNotBecomeSuccess(t *testing.T) {
	for _, id := range []string{"confirmed", ""} {
		creator := &partialCreator{result: definitioncreate.CreateResult{DefinitionLUID: id, TableauRequestID: "write-request"}, err: errors.New("readback unavailable")}
		input := definitioncreate.Input{Environment: "selected", Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "source", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: []string{"Region"}}}
		output, err := definitioncreate.New(&validator{}, &finder{}, creator).Execute(context.Background(), input, false)
		var structured *errs.Error
		if !errors.As(err, &structured) || creator.calls != 1 || structured.Retryable == nil || *structured.Retryable {
			t.Fatalf("err=%v calls=%d", err, creator.calls)
		}
		if id == "" {
			if output.Result != nil {
				t.Fatal("unknown write acquired a successful outcome")
			}
			continue
		}
		if output.Result == nil || output.Result.Status != "created" || output.Result.DefaultMetricStatus != "unresolved" || output.Result.DefinitionLUID != id || structured.ID != "pulse.definition.create.verification_failed" {
			t.Fatalf("output=%#v err=%v", output, err)
		}
	}
}

func TestCompactCreateDimensionsAreBoundedWithoutChangingRequest(t *testing.T) {
	dimensions := make([]string, 51)
	for i := range dimensions {
		dimensions[i] = fmt.Sprintf("Dimension %d", i)
	}
	input := definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "source", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: dimensions}}
	output, err := definitioncreate.New(&validator{}, &finder{}, &creator{}).Execute(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(definitioncreate.CompactResult)
	if compact.Plan.ReviewComplete || !compact.Plan.RequiresFull || compact.Plan.DimensionsOmitted != 1 || len(compact.Plan.Dimensions) != 50 || len(output.Plan.Request.ExtensionOptions.AllowedDimensions) != 51 {
		t.Fatalf("compact=%#v", compact.Plan)
	}
}
