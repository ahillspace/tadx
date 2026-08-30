package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/ahillspace/tadx/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	exitCode := app.Run(ctx, os.Args[1:], os.Stdout, app.Options{
		MutationsEnabled: os.Getenv("TADX_ENABLE_MUTATIONS") == "1",
	})
	stop()
	os.Exit(exitCode)
}
