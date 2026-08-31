// Package publish implements preview-by-default workbook.publish.
//
// Mutation-shape contract: workbook.publish is a consequential remote mutation,
// so it deliberately uses a different Execute signature than read-only actions.
// Read actions expose Execute(ctx, Input) (Output, error). Mutations instead
// split the work into Plan(ctx, Input) (Plan, error), which performs only
// authoritative reads and returns a deterministic preview, and Apply(ctx, Plan)
// (Result, error), which performs only the exact mutation captured by that Plan.
// Execute(ctx, Input, apply bool) composes the two: it always plans, and applies
// only when apply is true, so preview is the safe default and the applied result
// stays attached to the previewed plan. Future mutating actions should copy this
// Plan/Apply/Execute-with-apply shape deliberately rather than the read shape.
package publish
