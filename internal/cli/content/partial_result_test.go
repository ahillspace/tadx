package content

import (
	"bytes"
	"context"
	"errors"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"strings"
	"testing"
)

type partialPublisher struct{ unknown bool }

type warningPublisher struct{}

func (warningPublisher) Execute(_ context.Context, _ workbookops.PublishInput, _ bool) (workbookops.PublishOutput, error) {
	return workbookops.PublishOutput{Plan: workbookops.PublishPlan{Mode: "preview", Operation: "workbook.publish", ArtifactFingerprint: "diagnostic-fingerprint", Warnings: []string{"Workbook retains published datasource bindings to its source site."}}}, nil
}

func TestPublishPreviewDisplaysConsequentialWarningWithoutFull(t *testing.T) {
	var rendered bytes.Buffer
	command := newPublish(Dependencies{Publisher: warningPublisher{}, Renderer: partialRenderer{&rendered, false}})
	command.SetArgs([]string{"--environment", "chosen", "--project-id", "project", "--artifact", "artifacts/workbook/one", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "Workbook retains published datasource bindings") || strings.Contains(rendered.String(), "diagnostic-fingerprint") {
		t.Fatalf("preview hides decision evidence: %s", rendered.String())
	}
}

func (p partialPublisher) Execute(_ context.Context, input workbookops.PublishInput, _ bool) (workbookops.PublishOutput, error) {
	result := &workbookops.PublishResult{Status: "created", WorkbookLUID: "created-workbook", TableauRequestID: "readback-request"}
	if p.unknown {
		result = &workbookops.PublishResult{Status: "unknown", JobID: "accepted-job"}
	}
	return workbookops.PublishOutput{Plan: workbookops.PublishPlan{Operation: "workbook.publish", ArtifactPath: input.ArtifactPath}, Result: result},
		&errs.Error{ID: "workbook.publish.verify", Kind: errs.KindOperation, Summary: "Verification failed.", Retryable: errs.Bool(false)}
}

type partialRenderer struct {
	buffer *bytes.Buffer
	full   bool
}

func (r partialRenderer) Render(value any) error {
	return output.RenderWithOptions(r.buffer, value, output.Options{Full: r.full})
}

func TestPublishCLIRespectsPartialAndUnknownResultsThroughSingleAndBatchRendering(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, full := range []bool{false, true} {
			for _, unknown := range []bool{false, true} {
				var rendered bytes.Buffer
				command := newPublish(Dependencies{Publisher: partialPublisher{unknown}, Renderer: partialRenderer{&rendered, full}})
				command.SilenceErrors, command.SilenceUsage = true, true
				args := []string{"--environment", "chosen", "--project-id", "project", "--artifact", "artifacts/workbook/one"}
				if batch {
					args = append(args, "--artifact", "artifacts/workbook/two")
				}
				command.SetArgs(args)
				err := command.ExecuteContext(context.Background())
				if err == nil || errs.ExitCode(err) == 0 {
					t.Fatal("follow-up failure must retain nonzero exit")
				}
				if !clierr.IsRendered(err) {
					if renderErr := output.RenderError(&rendered, err, output.Options{Full: full}); renderErr != nil {
						t.Fatal(renderErr)
					}
				}
				text := rendered.String()
				identity := "created-workbook"
				if unknown {
					identity = "accepted-job"
				}
				if !strings.Contains(text, identity) || !strings.Contains(text, "Verification failed.") {
					t.Fatalf("batch=%t full=%t unknown=%t lost known outcome: %s", batch, full, unknown, text)
				}
				if unknown && strings.Contains(text, "status: created") {
					t.Fatalf("unknown became successful: %s", text)
				}
				if !unknown && strings.Contains(text, "readback-request") != full {
					t.Fatalf("projection lost: %s", text)
				}
				if batch && (!strings.Contains(text, "succeeded: 0") || !strings.Contains(text, "failed: 2")) {
					t.Fatalf("failed followups counted as success: %s", text)
				}
				if !batch {
					var structured *errs.Error
					if !errors.As(err, &structured) || structured.ID != "workbook.publish.verify" {
						t.Fatal("typed error chain lost")
					}
				}
			}
		}
	}
}
