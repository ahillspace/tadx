package app

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/ahillspace/tadx/internal/cli"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/spf13/cobra"
)

// bindPublicationExecution constructs the detached runtime and presentation ports.
func bindPublicationExecution(root *cobra.Command, runtime *runtimeDependencies, capture *lastCapture, argv []string, options Options) {
	cli.BindPublicationExecution(root, cli.PublicationExecution{
		Supports: cli.SupportsPublicationExecution,
		Worker:   options.publicationExecution != nil,
		Enabled:  options.PublicationWorkers,
		Inline: func(noWait bool) error {
			if noWait {
				return operationrun.WorkerError("unconfigured", "Background execution requires the detached worker runtime.", nil)
			}
			runtime.publicationExecution = &operationrun.PublicationExecution{Deadline: time.Now().Add(operationrun.WaitLimit)}
			return nil
		},
		Begin: func(ctx context.Context, operation, batchPath string, noWait bool, stderr io.Writer) (cli.ExecutionOutcome, error) {
			store, err := operationrun.NewStore(options.OperationDirectory)
			if err != nil {
				return cli.ExecutionOutcome{}, operationrun.WorkerError("store", "Operation tracking storage is unavailable; nothing was started.", err)
			}
			request, err := operationrun.PrepareRequest(argv, runtime.configPath, operation, batchPath, noWait)
			if err != nil {
				return cli.ExecutionOutcome{}, err
			}
			launch := options.WorkerLauncher
			if launch == nil {
				launch = func(_ context.Context, directory, id string) error {
					executable, err := os.Executable()
					if err != nil {
						return err
					}
					return operationrun.Launch(executable, []string{"__publication-worker", directory, id}, request.WorkingDirectory)
				}
			}
			coordinator := operationrun.Coordinator{
				Store: store, Launch: launch,
				StartActivity: func(ctx context.Context, operation string) operationrun.Activity {
					return progress.New(stderr).Start(ctx, cli.OperationActivity(operation))
				},
			}
			result, err := coordinator.Begin(ctx, request)
			return cli.ExecutionOutcome{
				Value:   output.OperationRun{Record: result.Record.OutputRecord(), Stopped: result.Stopped},
				Present: result.Present, Rendered: result.Rendered,
			}, err
		},
		Capture: func(value any) { capture.value = value },
	})
}

func runPublicationWorker(ctx context.Context, directory, id string, options Options) int {
	store, err := operationrun.NewStore(directory)
	if err != nil {
		return 1
	}
	worker, err := operationrun.OpenWorker(store, id, cli.SupportsPublicationExecution)
	if err != nil {
		return 1
	}
	defer worker.Close()
	record := worker.Record
	control := &operationrun.PublicationExecution{
		Operation: record.Operation, OperationID: record.ID, NoWait: record.Request.NoWait,
		Deadline: record.RequestedAt.Add(operationrun.WaitLimit), Detached: worker.Detached,
		Prepare: func(_ context.Context, operationID, operation, receiptID, scope string) error {
			return worker.PrepareIntent(operationID, operation, receiptID, scope)
		},
		Accepted: func(_ context.Context, path string) error { return worker.AcceptReceipt(path) },
	}
	options.ConfigPath = record.Request.ConfigPath
	options.OperationDirectory = store.Directory
	options.PublicationWorkers = false
	options.publicationExecution = control
	options.Stderr = io.Discard
	options.publicationResult = func(value any, _ bool, exit int) error {
		compact, full, err := output.CaptureOperationSnapshots(value, options.ConfigPath, int(operationrun.MaxRecordBytes)/3)
		return worker.Finish(compact, full, exit, output.OperationContainsUnfinished(full), err)
	}
	ctx = contentbatch.WithObserver(ctx, func(out contentbatch.Output) {
		compact, _, err := output.CaptureOperationSnapshots(out, options.ConfigPath, int(operationrun.MaxRecordBytes)/3)
		if err != nil {
			return
		}
		_ = worker.SaveProgress(compact, cli.OperationBatchActivity(record.Operation, out.Succeeded, out.Pending, out.Failed))
	})
	ctx = progress.WithObserver(ctx, func(label string) { _ = worker.SetActivity(label) })
	args, err := worker.Args()
	if err != nil {
		return 1
	}
	return Run(ctx, args, io.Discard, options)
}
