package main

import (
	"context"
	"os"

	"github.com/ahillspace/tadx/internal/app"
)

func main() {
	exitCode := app.Run(context.Background(), os.Args[1:], os.Stdout, app.Options{
		MutationsEnabled: os.Getenv("TADX_ENABLE_MUTATIONS") == "1",
	})
	os.Exit(exitCode)
}
