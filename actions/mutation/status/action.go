package status

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
)

type Reader interface {
	ReadMutationSetting(context.Context) (value.MutationSetting, error)
}
type Output = value.MutationSetting
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{r} }
func (a *Action) Execute(ctx context.Context) (value.MutationSetting, error) {
	return a.reader.ReadMutationSetting(ctx)
}
