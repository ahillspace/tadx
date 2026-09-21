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
		Stderr:             os.Stderr,
		PublicationWorkers: true,
	})
	stop()
	os.Exit(exitCode)
}
