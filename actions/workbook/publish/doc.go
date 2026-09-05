// Package publish implements workbook.publish with an explicit preview mode.
//
// Mutation-shape contract: workbook.publish is a consequential remote mutation,
// so it deliberately uses a different Execute signature than read-only actions.
// Read actions expose Execute(ctx, Input) (Output, error). Mutations instead
// split the work into Plan(ctx, Input) (Plan, error), which performs only
// authoritative reads and returns a deterministic preview, and Apply(ctx, Plan)
// (Result, error), which performs only the exact mutation captured by that Plan.
// Execute(ctx, Input, preview bool) composes the two: it always plans and performs
// the mutation unless preview is true. The result stays attached to the plan.
// Future mutating actions should copy this Plan/Apply/Execute-with-preview shape.
package publish
