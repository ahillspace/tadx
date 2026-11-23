package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/actions/structure"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/twbschema"
)

// structureService answers workbook.structure and datasource.structure from
// the vendored Tableau schemas, with no package or network involved.
type structureService struct{}

func (structureService) DescribeStructure(_ context.Context, document string, query structure.Input) (structure.Report, error) {
	operation := document + ".structure"
	usage := func(id, field, code, summary, corrective string) error {
		return &errs.Error{ID: operation + "." + id, Kind: errs.KindUsage, Operation: operation, Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: corrective, Validation: []errs.ValidationDetail{{Field: field, Code: code, Message: summary}}}
	}
	versions := twbschema.Versions()
	version := query.SchemaVersion
	if version == "" {
		version = versions[len(versions)-1]
	} else if !slices.Contains(versions, version) {
		return structure.Report{}, usage("usage", "schema_version", "unknown", fmt.Sprintf("no Tableau schema for %q; available: %s", version, strings.Join(versions, ", ")), "Pass one of the available Tableau releases, then retry.")
	}
	schema, err := twbschema.Load(version)
	if err != nil {
		return structure.Report{}, &errs.Error{ID: operation + ".schema", Kind: errs.KindRuntime, Operation: operation, Summary: "The Tableau " + version + " schema could not be loaded: " + err.Error(), Retryable: errs.Bool(false), CorrectiveAction: "Reinstall TADX, then retry."}
	}
	elements, err := schema.Lookup(document, query.Element, query.Attribute)
	var notFound *twbschema.NotFoundError
	switch {
	case errors.As(err, &notFound):
		return structure.Report{}, usage("not_found", "element", "not_found", err.Error(), "Look up the parent element to see the children it takes, then retry.")
	case err != nil:
		return structure.Report{}, usage("usage", "element", "invalid", err.Error(), "Pass an element name or a chain such as worksheet > table, and name one attribute with --attribute, then retry.")
	}
	report := structure.Report{Schema: version, Elements: make([]structure.Element, len(elements))}
	for i, element := range elements {
		attributes := make([]structure.Attribute, len(element.Attributes))
		for j, a := range element.Attributes {
			attributes[j] = structure.Attribute{Name: a.Name, Type: a.Type, Required: a.Required, Fixed: a.Fixed, Doc: a.Doc}
		}
		report.Elements[i] = structure.Element{Name: element.Name, Paths: element.Paths, PathsOmitted: element.PathsOmitted, Doc: element.Doc, Children: element.Children, Text: element.Text,
			Attributes: attributes, OpenAttributes: element.OpenAttributes, OpenContent: element.OpenContent, Unique: element.Unique}
	}
	return report, nil
}
