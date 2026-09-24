package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/guidancenotice"
	"github.com/ahillspace/tadx/internal/releasenotice"
)

func main() {
	if handled, code := app.RunPolicyInstallHelper(os.Args[1:]); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	if guidancenotice.ShouldPrint(guidancenotice.Options{Args: os.Args[1:]}) {
		_, _ = fmt.Fprint(os.Stderr, guidancenotice.Message())
	}
	exitCode := runWithReleaseNotice(ctx, os.Args[1:], os.Stdout, os.Stderr, releasenotice.Options{}, func() int {
		return app.Run(ctx, os.Args[1:], os.Stdout, app.Options{
			Stderr:             os.Stderr,
			PublicationWorkers: true,
		})
	})
	stop()
	os.Exit(exitCode)
}

func runWithReleaseNotice(ctx context.Context, args []string, stdout, stderr io.Writer, options releasenotice.Options, command func() int) int {
	options.Args, options.Stdout, options.Stderr = args, stdout, stderr
	check := releasenotice.Start(ctx, options)
	exitCode := command()
	if notice := check.Finish(exitCode == 0); notice != "" {
		_, _ = io.WriteString(stderr, notice)
	}
	return exitCode
}
