package cli

import (
	"context"
	"sync"
	"testing"

	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/capability"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/spf13/cobra"
)

type batchReadRequests struct {
	mu          sync.Mutex
	permissions []permissioninspect.Input
	pulls       []workbookops.PullInput
}

func (r *batchReadRequests) InspectAdminPermission(_ context.Context, input permissioninspect.Input) (permissioninspect.Output, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissions = append(r.permissions, input)
	return permissioninspect.Output{}, nil
}
func (r *batchReadRequests) Execute(_ context.Context, input workbookops.PullInput) (workbookops.PullOutput, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pulls = append(r.pulls, input)
	return workbookops.PullOutput{}, nil
}

func TestBatchReadRequestsCanInspectDifferentRulesOnOneResource(t *testing.T) {
	requests := &batchReadRequests{}
	factory := func(renderer Renderer) *cobra.Command {
		root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(admincli.New(admincli.Dependencies{Renderer: renderer, PermissionInspector: requests}, "admin.permission.inspect"))
		return root
	}
	renderer := &batchCollector{}
	root := factory(renderer)
	command, _, err := root.Find([]string{"admin", "permission", "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	attachBatchWithOptions(root, command, capability.BatchOptions()["admin.permission.inspect"], renderer, factory)
	root.SetArgs([]string{"admin", "permission", "inspect", "--environment", "test", "--kind", "workbook", "--id", "w1", "--principal-type", "user", "--principal-id", "u1", "--batch-file", batchFile(t, `{"items":[{"capability":"Read"},{"capability":"Write"}]}`)})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(requests.permissions) != 2 {
		t.Fatalf("reads=%v", requests.permissions)
	}
}

func TestBatchPullDestinationsRemainIndependent(t *testing.T) {
	requests := &batchReadRequests{}
	factory := func(renderer Renderer) *cobra.Command {
		root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(contentcli.New(contentcli.Dependencies{Renderer: renderer, Puller: requests}, "workbook.pull"))
		return root
	}
	renderer := &batchCollector{}
	root := factory(renderer)
	command, _, err := root.Find([]string{"content", "workbook", "pull"})
	if err != nil {
		t.Fatal(err)
	}
	attachBatchWithOptions(root, command, capability.BatchOptions()["workbook.pull"], renderer, factory)
	root.SetArgs([]string{"content", "workbook", "pull", "--environment", "test", "--batch-file", batchFile(t, `{"items":[{"id":"w1","workspace":"first"},{"id":"w1","workspace":"second"}]}`)})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(requests.pulls) != 2 || requests.pulls[0].Workspace == requests.pulls[1].Workspace {
		t.Fatalf("pulls=%v", requests.pulls)
	}
}
