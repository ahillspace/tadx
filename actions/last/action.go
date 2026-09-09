package last

import (
	"context"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type Reader interface {
	Read(context.Context) (value.SavedExecution, error)
}

type Output = value.SavedExecution
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader} }
func (a *Action) Execute(ctx context.Context) (value.SavedExecution, error) {
	out, err := a.reader.Read(ctx)
	if err != nil {
		return out, &errs.Error{ID: "last.unavailable", Kind: errs.KindOperation, Operation: "last", Summary: "No readable previous result is available.", Cause: err, Retryable: errs.Bool(false)}
	}
	return out, nil
}
