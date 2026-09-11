package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/guidancenotice"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	if guidancenotice.ShouldPrint(guidancenotice.Options{Args: os.Args[1:]}) {
		_, _ = fmt.Fprint(os.Stderr, guidancenotice.Message())
	}
	exitCode := app.Run(ctx, os.Args[1:], os.Stdout, app.Options{
		MutationEnvironment: func() (string, bool) { return os.LookupEnv("TADX_ENABLE_MUTATIONS") },
		Stderr:              os.Stderr,
	})
	stop()
	os.Exit(exitCode)
}
