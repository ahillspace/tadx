package definition

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type createPartialCreator struct {
	result CreateResult
	err    error
	calls  int
}

func (c *createPartialCreator) CreateDefinition(context.Context, CreateRequest) (CreateResult, error) {
	c.calls++
	return c.result, c.err
}

func TestCreateConfirmedCreateRemainsTypedWhileUnknownWriteDoesNotBecomeSuccess(t *testing.T) {
	for _, id := range []string{"confirmed", ""} {
		creator := &createPartialCreator{result: CreateResult{DefinitionLUID: id, TableauRequestID: "write-request"}, err: errors.New("readback unavailable")}
		input := CreateInput{Environment: "selected", Intent: CreateIntent{Name: "Revenue", DatasourceLUID: "source", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: []string{"Region"}}}
		output, err := create(context.Background(), &createValidator{}, &createFinder{}, creator, input, false)
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

func TestCreateCompactCreateDimensionsAreBoundedWithoutChangingRequest(t *testing.T) {
	dimensions := make([]string, 51)
	for i := range dimensions {
		dimensions[i] = fmt.Sprintf("Dimension %d", i)
	}
	input := CreateInput{Intent: CreateIntent{Name: "Revenue", DatasourceLUID: "source", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: dimensions}}
	output, err := create(context.Background(), &createValidator{}, &createFinder{}, &createCreator{}, input, true)
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(CreateCompactResult)
	if compact.Plan.ReviewComplete || !compact.Plan.RequiresFull || compact.Plan.DimensionsOmitted != 1 || len(compact.Plan.Dimensions) != 50 || len(output.Plan.Request.ExtensionOptions.AllowedDimensions) != 51 {
		t.Fatalf("compact=%#v", compact.Plan)
	}
}
