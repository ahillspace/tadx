// Package delete deletes one exact Pulse definition.
package delete

import "context"

// Input identifies one authoritative target. Preview disables the mutation.
type Input struct {
	Environment string
	Site        string
	LUID        string
	Preview     bool
}

// Definition contains bounded authoritative target facts.
type Definition struct {
	LUID           string `json:"luid"`
	Name           string `json:"name,omitempty"`
	DatasourceLUID string `json:"datasource_luid,omitempty"`
}

// Reader obtains one exact live target.
type Reader interface {
	GetDefinition(context.Context, string) (Definition, error)
}

// Deleter sends one exact delete request without cascading client-side.
type Deleter interface {
	DeleteDefinition(context.Context, string) (Result, error)
}

// Plan describes the target selected for deletion.
type Plan struct {
	Mode        string     `json:"mode"`
	Operation   string     `json:"operation"`
	Environment string     `json:"environment"`
	Site        string     `json:"site"`
	Target      Definition `json:"target"`
}

// Result preserves the upstream delete outcome.
type Result struct {
	Status           string `json:"status"`
	DefinitionLUID   string `json:"definition_luid"`
	HTTPStatus       int    `json:"http_status"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// Output contains the exact plan and optional remote outcome.
type Output struct {
	Plan     Plan     `json:"plan"`
	Result   *Result  `json:"result,omitempty"`
	Warnings []string `json:"warnings"`
	Help     []string `json:"help"`
}

// CompactDeleteResult retains the status and authoritative identity.
type CompactDeleteResult struct {
	Status         string `json:"status"`
	DefinitionLUID string `json:"definition_luid"`
}

// CompactResult omits bounded HTTP diagnostics.
type CompactResult struct {
	Plan     Plan                 `json:"plan"`
	Result   *CompactDeleteResult `json:"result,omitempty"`
	Warnings []string             `json:"warnings"`
	Details  string               `json:"details"`
	Help     []string             `json:"help"`
}

// CompactOutput projects bounded decision fields.
func (o Output) CompactOutput() any {
	var result *CompactDeleteResult
	if o.Result != nil {
		result = &CompactDeleteResult{Status: o.Result.Status, DefinitionLUID: o.Result.DefinitionLUID}
	}
	return CompactResult{Plan: o.Plan, Result: result, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput includes the bounded upstream diagnostics for the same operation.
func (o Output) FullOutput() any { return o }
