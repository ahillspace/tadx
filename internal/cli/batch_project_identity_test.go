package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/capability"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/spf13/cobra"
)

type projectIdentityCreator struct{ calls atomic.Int32 }

func (*projectIdentityCreator) ListProjects(context.Context, projectops.ListInput) (projectops.ListOutput, error) {
	return projectops.ListOutput{}, nil
}
func (*projectIdentityCreator) InspectProject(context.Context, projectops.InspectInput) (projectops.InspectOutput, error) {
	return projectops.InspectOutput{}, nil
}

func (p *projectIdentityCreator) CreateProject(context.Context, projectops.CreateInput, bool) (projectops.CreateOutput, error) {
	p.calls.Add(1)
	return projectops.CreateOutput{}, nil
}

func (*projectIdentityCreator) ResolveBatchResource(context.Context, string, string, identity.Selector) (identity.LUID, error) {
	return "parent-1", nil
}

func TestProjectCreateBatchRejectsSiblingNameAliases(t *testing.T) {
	for _, tc := range []struct {
		name, first, second, secondParent string
		conflict                          bool
	}{
		{"ASCII siblings", "Destination", "destination", "parent-1", true},
		{"Unicode siblings", "\u03a3", "\u03c2", "parent-1", true},
		{"different parents", "Destination", "destination", "parent-2", false},
		{"different names", "Destination", "Other", "parent-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &projectIdentityCreator{}
			factory := func(renderer Renderer) *cobra.Command {
				root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
				root.AddCommand(contentcli.New(contentcli.Dependencies{ProjectCreator: service, ProjectLister: service, ProjectInspector: service, BatchTargets: service, Renderer: renderer}, "project.create"))
				return root
			}
			capture := &batchCollector{}
			root := factory(capture)
			command, _, err := root.Find([]string{"content", "project", "create"})
			if err != nil {
				t.Fatal(err)
			}
			attachBatchWithOptions(root, command, capability.BatchOptions()["project.create"], capture, factory)
			body, err := json.Marshal(map[string]any{"items": []map[string]string{{"name": tc.first, "parent-id": "parent-1"}, {"name": tc.second, "parent-id": tc.secondParent}}})
			if err != nil {
				t.Fatal(err)
			}
			root.SetArgs([]string{"content", "project", "create", "--environment", "test", "--batch-file", batchFile(t, string(body))})
			err = root.ExecuteContext(t.Context())
			if tc.conflict {
				if err == nil || !strings.Contains(err.Error(), "duplicates") || service.calls.Load() != 0 {
					t.Fatalf("calls=%d err=%v", service.calls.Load(), err)
				}
			} else if err != nil || service.calls.Load() != 2 {
				t.Fatalf("calls=%d err=%v", service.calls.Load(), err)
			}
		})
	}
}
