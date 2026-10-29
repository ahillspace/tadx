package app

import "context"

// publicationExecution belongs to one detached worker invocation, not one
// batch item. It links each saved receipt to the worker's operation record.
type publicationExecution struct {
	operation    string
	receiptSaved func(context.Context, string) error
}
