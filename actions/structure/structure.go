// Package structure runs workbook.structure and datasource.structure: what
// the Tableau document schema allows at one element of a workbook or
// datasource, so an agent can write XML Tableau accepts.
package structure

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Document names the document root a lookup starts from.
type Document string

const (
	Workbook   Document = "workbook"
	Datasource Document = "datasource"
)

// Describer looks up a query in the Tableau schema. It returns a usage error
// for a schema version or query the schema cannot answer.
type Describer interface {
	DescribeStructure(ctx context.Context, document string, input Input) (Report, error)
}

// Input is one lookup: Element is an element name or a chain such as
// "worksheet > table", Attribute optionally keeps one attribute, and
// SchemaVersion is a Tableau release such as 2026.1, empty for the newest.
type Input struct {
	Element       string
	Attribute     string
	SchemaVersion string
}

// Structure runs the lookup for one kind of document.
type Structure struct {
	document  Document
	describer Describer
}

// New creates the lookup for document.
func New(document Document, describer Describer) *Structure {
	return &Structure{document: document, describer: describer}
}

// Operation is the capability ID the lookup runs as.
func (a *Structure) Operation() string { return string(a.document) + ".structure" }

// Execute looks up the element.
func (a *Structure) Execute(ctx context.Context, input Input) (Report, error) {
	operation := a.Operation()
	if strings.TrimSpace(input.Element) == "" {
		const message = "name an element, such as table, or a chain such as worksheet > table"
		return Report{}, &errs.Error{ID: operation + ".usage", Kind: errs.KindUsage, Operation: operation, Summary: message, Retryable: new(false), CorrectiveAction: "Pass the element to look up, then retry.", Validation: []errs.ValidationDetail{{Field: "element", Code: "required", Message: message}}}
	}
	if a.describer == nil {
		return Report{}, &errs.Error{ID: operation + ".unconfigured", Kind: errs.KindRuntime, Operation: operation, Summary: "Structure lookup is not configured.", Retryable: new(false), CorrectiveAction: "Configure the Tableau schemas before retrying."}
	}
	return a.describer.DescribeStructure(ctx, string(a.document), input)
}

// Report is the result of workbook.structure and datasource.structure: each
// distinct declaration the query names. The schemas declare at most a dozen
// elements under one name, so the list needs no bound.
type Report struct {
	Schema   string    `json:"schema"`
	Elements []Element `json:"elements"`
}

// CompactOutput leaves out the schema's documentation text, which --full adds.
func (r Report) CompactOutput() any {
	elements := make([]Element, len(r.Elements))
	for i, element := range r.Elements {
		element.Doc = ""
		element.Attributes = make([]Attribute, len(r.Elements[i].Attributes))
		for j, attribute := range r.Elements[i].Attributes {
			attribute.Doc = ""
			element.Attributes[j] = attribute
		}
		elements[i] = element
	}
	return struct {
		Schema   string    `json:"schema"`
		Elements []Element `json:"elements"`
		Details  string    `json:"details"`
	}{Schema: r.Schema, Elements: elements, Details: "--full"}
}

// FullOutput is the whole report.
func (r Report) FullOutput() any { return r }

// Element is what the schema allows for one element declaration.
// Children uses the order notation of the schema validator's messages; OpenContent and
// OpenAttributes mean the schema leaves that part unchecked.
type Element struct {
	Name           string      `json:"name"`
	Paths          []string    `json:"paths"`
	PathsOmitted   int         `json:"paths_omitted,omitempty"`
	Doc            string      `json:"doc,omitempty"`
	Children       string      `json:"children,omitempty"`
	Text           string      `json:"text,omitempty"`
	Attributes     []Attribute `json:"attributes"`
	OpenAttributes bool        `json:"open_attributes,omitempty"`
	OpenContent    bool        `json:"open_content,omitempty"`
	Unique         []string    `json:"unique,omitempty"`
}

// Attribute is one declared attribute and the values it takes.
type Attribute struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
	Fixed    string `json:"fixed,omitempty"`
	Doc      string `json:"doc,omitempty"`
}
