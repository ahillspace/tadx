package status

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
)

type Reader interface {
	ReadMutationStatus(context.Context, string) (value.MutationStatus, error)
}
type Output = value.MutationStatus
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{r} }
func (a *Action) Execute(ctx context.Context, environment string) (Output, error) {
	return a.reader.ReadMutationStatus(ctx, environment)
}
