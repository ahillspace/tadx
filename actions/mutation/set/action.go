package set

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
)

type Writer interface {
	WriteMutationSetting(context.Context, bool) (value.MutationSetting, error)
}
type Output = value.MutationSetting
type Action struct{ writer Writer }

func New(w Writer) *Action { return &Action{w} }
func (a *Action) Execute(ctx context.Context, enabled bool) (value.MutationSetting, error) {
	return a.writer.WriteMutationSetting(ctx, enabled)
}
