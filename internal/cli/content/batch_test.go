package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	dspublish "github.com/ahillspace/tadx/actions/datasource/publish"
	dspull "github.com/ahillspace/tadx/actions/datasource/pull"
	fpublish "github.com/ahillspace/tadx/actions/flow/publish"
	fpull "github.com/ahillspace/tadx/actions/flow/pull"
	wpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	wpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/spf13/cobra"
)

type batchCalls struct {
	selectors        []string
	envs, workspaces []string
	previews         []bool
	fail             bool
	failAt           int
}

var errBatchItem = errors.New("item failed")

func (b *batchCalls) call(selector, env, workspace string) error {
	b.selectors = append(b.selectors, selector)
	b.envs = append(b.envs, env)
	b.workspaces = append(b.workspaces, workspace)
	if b.fail && len(b.selectors) == 2 || b.failAt > 0 && len(b.selectors) == b.failAt {
		return errBatchItem
	}
	return nil
}
func (b *batchCalls) Execute(_ context.Context, in wpull.Input) (wpull.Output, error) {
	return wpull.Output{}, b.call(in.LUID, in.Environment, in.Workspace)
}

type batchWorkbookPublisher struct{ b *batchCalls }

func (p batchWorkbookPublisher) Execute(_ context.Context, in wpublish.Input, preview bool) (wpublish.Output, error) {
	p.b.previews = append(p.b.previews, preview)
	return wpublish.Output{Plan: wpublish.Plan{Operation: "workbook.publish", ArtifactPath: in.ArtifactPath}}, p.b.call(in.ArtifactPath, in.Environment, in.Workspace)
}
func (b *batchCalls) PullDatasource(_ context.Context, in dspull.Input) (dspull.Output, error) {
	return dspull.Output{}, b.call(string(in.Selector.LUID), in.Environment, in.Workspace)
}
func (b *batchCalls) PublishDatasource(_ context.Context, in dspublish.Input, preview bool) (dspublish.Output, error) {
	b.previews = append(b.previews, preview)
	return dspublish.Output{Plan: dspublish.Plan{Operation: "datasource.publish", ArtifactPath: in.ArtifactPath}}, b.call(in.ArtifactPath, in.Environment, in.Workspace)
}
func (b *batchCalls) PullFlow(_ context.Context, in fpull.Input) (fpull.Output, error) {
	return fpull.Output{}, b.call(string(in.Selector.LUID), in.Environment, in.Workspace)
}
func (b *batchCalls) PublishFlow(_ context.Context, in fpublish.Input, preview bool) (fpublish.Output, error) {
	b.previews = append(b.previews, preview)
	return fpublish.Output{Plan: fpublish.Plan{Operation: "flow.publish", ArtifactPath: in.ArtifactPath}}, b.call(in.ArtifactPath, in.Environment, in.Workspace)
}

type batchRenderer struct{ values []any }

func (r *batchRenderer) Render(v any) error { r.values = append(r.values, v); return nil }

func batchCommand(kind, verb string, b *batchCalls, r Renderer) *cobra.Command {
	deps := Dependencies{Puller: b, Publisher: batchWorkbookPublisher{b}, FlowPuller: b, FlowPublisher: b, Renderer: r}
	var command *cobra.Command
	switch kind + "." + verb {
	case "workbook.pull":
		command = newPull(deps)
	case "workbook.publish":
		command = newPublish(deps)
	case "datasource.pull":
		command = newDatasourcePull(datasourceLifecycleDependencies{puller: b, renderer: r})
	case "datasource.publish":
		command = newDatasourcePublish(datasourceLifecycleDependencies{publisher: b, renderer: r})
	case "flow.pull":
		command = newFlowPull(deps)
	default:
		command = newFlowPublish(deps)
	}
	command.SilenceErrors = true
	command.SilenceUsage = true
	return command
}

func TestContentBatchProcessesEverySelectorAndPreservesSharedFlags(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, verb := range []string{"pull", "publish"} {
			t.Run(kind+"."+verb, func(t *testing.T) {
				b, r := &batchCalls{fail: true}, &batchRenderer{}
				cmd := batchCommand(kind, verb, b, r)
				var stderr bytes.Buffer
				cmd.SetErr(&stderr)
				args := []string{"--environment", "dev", "--workspace", "analytics"}
				selectors := []string{"first", "second", "third"}
				flag := "--id"
				if verb == "publish" {
					flag = "--artifact"
					for i := range selectors {
						selectors[i] = "artifacts/" + kind + "/" + selectors[i]
					}
					args = append(args, "--project-id", "project-1", "--preview")
					if kind == "datasource" {
						args = append(args, "--create")
					}
				}
				for _, selector := range selectors {
					args = append(args, flag, selector)
				}
				cmd.SetArgs(args)
				err := cmd.ExecuteContext(context.Background())
				if err == nil || !clierr.IsRendered(err) {
					t.Errorf("expected rendered batch failure, got %v", err)
				}
				if stderr.Len() != 0 {
					t.Errorf("redirected stderr = %q, want empty", stderr.String())
				}
				if !reflect.DeepEqual(b.selectors, selectors) {
					t.Errorf("selectors = %v; want %v", b.selectors, selectors)
				}
				if !reflect.DeepEqual(b.envs, []string{"dev", "dev", "dev"}) || !reflect.DeepEqual(b.workspaces, []string{"analytics", "analytics", "analytics"}) {
					t.Errorf("shared flags changed: %#v", b)
				}
				if len(r.values) != 1 {
					t.Fatalf("render calls = %d", len(r.values))
				}
				out, ok := r.values[0].(contentbatch.Output)
				if !ok || out.Total != 3 || out.Succeeded != 2 || out.Failed != 1 || out.Items[1].Status != "failed" {
					t.Fatalf("aggregate = %#v", r.values[0])
				}
				if verb == "publish" && !reflect.DeepEqual(b.previews, []bool{true, true, true}) {
					t.Errorf("preview flags = %v", b.previews)
				}
			})
		}
	}
}

func TestContentSelectionPreservesSingleOutputAndSuccessfulBatch(t *testing.T) {
	for operation, want := range map[string]any{
		"workbook.pull": wpull.Output{}, "workbook.publish": wpublish.Output{Plan: wpublish.Plan{Operation: "workbook.publish", ArtifactPath: "artifacts/workbook/0"}},
		"datasource.pull": dspull.Output{}, "datasource.publish": dspublish.Output{Plan: dspublish.Plan{Operation: "datasource.publish", ArtifactPath: "artifacts/datasource/0"}},
		"flow.pull": fpull.Output{}, "flow.publish": fpublish.Output{Plan: fpublish.Plan{Operation: "flow.publish", ArtifactPath: "artifacts/flow/0"}},
	} {
		for _, count := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d", operation, count), func(t *testing.T) {
				parts := strings.Split(operation, ".")
				b, r := &batchCalls{}, &batchRenderer{}
				cmd := batchCommand(parts[0], parts[1], b, r)
				var stderr bytes.Buffer
				cmd.SetErr(&stderr)
				args := []string{}
				for i := 0; i < count; i++ {
					selector := fmt.Sprint(i)
					if parts[1] == "pull" {
						args = append(args, "--id", selector)
					} else {
						args = append(args, "--artifact", "artifacts/"+parts[0]+"/"+selector)
					}
				}
				if operation == "datasource.publish" {
					args = append(args, "--create")
				}
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if stderr.Len() != 0 {
					t.Fatalf("redirected stderr = %q, want empty", stderr.String())
				}
				if len(b.selectors) != count || len(r.values) != 1 {
					t.Fatalf("calls=%v renders=%v", b.selectors, r.values)
				}
				if count == 1 {
					if !reflect.DeepEqual(r.values[0], want) {
						t.Fatalf("single result = %#v, want %#v", r.values[0], want)
					}
				} else {
					out, ok := r.values[0].(contentbatch.Output)
					if !ok || out.Status != "succeeded" || out.Succeeded != count {
						t.Fatalf("aggregate=%#v", r.values[0])
					}
				}
			})
		}
	}
}

func TestSinglePublishFailurePreservesErrorAndKeepsRedirectedStderrSilent(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			calls, renderer := &batchCalls{failAt: 1}, &batchRenderer{}
			command := batchCommand(kind, "publish", calls, renderer)
			var stderr bytes.Buffer
			command.SetErr(&stderr)
			args := []string{"--artifact", "artifacts/" + kind + "/Finance--identity"}
			if kind == "datasource" {
				args = append(args, "--create")
			}
			command.SetArgs(args)

			err := command.ExecuteContext(context.Background())

			if !errors.Is(err, errBatchItem) {
				t.Fatalf("error = %v, want original item failure", err)
			}
			if len(renderer.values) != 0 {
				t.Fatalf("rendered values = %#v, want none", renderer.values)
			}
			if stderr.Len() != 0 {
				t.Fatalf("redirected stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestPublishProgressUsesContentTypeAndArtifactBasename(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, preview := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preview=%t", kind, preview), func(t *testing.T) {
				var stderr bytes.Buffer
				reporter := progress.New(&stderr,
					progress.WithTerminalDetector(func(io.Writer) bool { return true }),
					progress.WithInterval(time.Hour),
				)
				renderer := &batchRenderer{}
				artifacts := []string{
					"artifacts/" + kind + "/Finance--identity",
					"artifacts/" + kind + "/Sales--identity",
				}
				var published []string

				err := runPublishSelection(context.Background(), kind+".publish", kind, artifacts, preview, renderer, reporter, func(_ context.Context, artifact string) (string, error) {
					published = append(published, artifact)
					return artifact, nil
				})

				if err != nil {
					t.Fatal(err)
				}
				got := stderr.String()
				first := "Publishing " + kind + " Finance--identity... elapsed 0s"
				second := "Publishing " + kind + " Sales--identity... elapsed 0s"
				if preview {
					first = "Previewing " + kind + " publication Finance--identity... elapsed 0s"
					second = "Previewing " + kind + " publication Sales--identity... elapsed 0s"
				}
				firstPosition, secondPosition := strings.Index(got, first), strings.Index(got, second)
				if firstPosition < 0 || secondPosition <= firstPosition {
					t.Fatalf("stderr = %q, want ordered labels %q then %q", got, first, second)
				}
				if strings.Contains(got, "artifacts/") {
					t.Fatalf("stderr = %q, want workspace-relative basename only", got)
				}
				if !reflect.DeepEqual(published, artifacts) {
					t.Fatalf("published artifacts = %v, want %v", published, artifacts)
				}
			})
		}
	}
}

func TestContentBatchRejectsDuplicateAndInvalidSelectorsBeforeAnyAction(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, tc := range []struct {
			verb string
			args []string
		}{
			{"pull", []string{"--id", "same", "--id", "same"}},
			{"pull", []string{"--id", "first", "--id", ""}},
			{"pull", []string{"--id", "first", "--id", "second", "--name", "Name"}},
			{"publish", []string{"--artifact", "artifacts/" + kind + "/valid", "--artifact", "../invalid", "--overwrite"}},
			{"publish", []string{"--artifact", "artifacts/" + kind + "/first", "--artifact", "artifacts/" + kind + "/second", "--name", "Shared", "--overwrite"}},
		} {
			b := &batchCalls{}
			cmd := batchCommand(kind, tc.verb, b, &batchRenderer{})
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil {
				t.Errorf("%s %s accepted %v", kind, tc.verb, tc.args)
			}
			if len(b.selectors) != 0 {
				t.Errorf("actions ran before validation: %v", b.selectors)
			}
		}
	}
}
