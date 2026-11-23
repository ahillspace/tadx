package structure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/actions/structure"
	"github.com/ahillspace/tadx/internal/errs"
)

type describer struct{ document string }

func (d *describer) DescribeStructure(_ context.Context, document string, input structure.Input) (structure.Report, error) {
	d.document = document
	return structure.Report{Schema: "2026.1", Elements: []structure.Element{{Name: input.Element, Doc: "Holds the shelves.", Attributes: []structure.Attribute{{Name: "class", Doc: "Mark type."}}}}}, nil
}

func TestStructureRequiresAnElement(t *testing.T) {
	_, err := structure.New(structure.Workbook, &describer{}).Execute(context.Background(), structure.Input{Element: "  "})
	var failure *errs.Error
	if !errors.As(err, &failure) || failure.ID != "workbook.structure.usage" || failure.Kind != errs.KindUsage {
		t.Fatalf("err = %#v", err)
	}
}

// Compact output drops the schema's documentation text and points at --full,
// which keeps it.
func TestStructureCompactOutputLeavesOutDocumentation(t *testing.T) {
	d := &describer{}
	report, err := structure.New(structure.Datasource, d).Execute(context.Background(), structure.Input{Element: "mark"})
	if err != nil || d.document != "datasource" {
		t.Fatalf("document = %q, err = %v", d.document, err)
	}
	compact := report.CompactOutput().(struct {
		Schema   string              `json:"schema"`
		Elements []structure.Element `json:"elements"`
		Details  string              `json:"details"`
	})
	if compact.Details != "--full" || compact.Elements[0].Doc != "" || compact.Elements[0].Attributes[0].Doc != "" {
		t.Fatalf("compact = %#v", compact)
	}
	if full := report.FullOutput().(structure.Report); full.Elements[0].Doc != "Holds the shelves." || full.Elements[0].Attributes[0].Doc != "Mark type." {
		t.Fatalf("full = %#v", full)
	}
}
