package live

import (
	"context"
	"github.com/ahillspace/tadx/tests/live/harness"
	"os"
)

//go:generate go run ./generate

func RunFromEnvironment(ctx context.Context, id string) (Report, error) {
	target, binary := os.Getenv("TADX_LIVECLI_TARGET"), os.Getenv("TADX_LIVECLI_BINARY")
	if target == "" || binary == "" {
		return Report{}, harness.Blocked("explicit TADX_LIVECLI_TARGET and TADX_LIVECLI_BINARY are required")
	}
	state := os.Getenv("TADX_LIVECLI_STATE")
	if state == "" {
		state = ".tadx-live"
	}
	p, err := harness.LoadProfile(target)
	if err != nil {
		return Report{}, err
	}
	plan, err := Select(Selectors{IDs: []string{id}})
	if err != nil {
		return Report{}, err
	}
	return Run(ctx, p, binary, state, plan)
}
