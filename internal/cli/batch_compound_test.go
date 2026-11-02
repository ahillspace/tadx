package cli

import (
	"context"
	"strings"
	"sync"
	"testing"

	groupmember "github.com/ahillspace/tadx/actions/admin/group/member"
	permission "github.com/ahillspace/tadx/actions/admin/permission"
	"github.com/ahillspace/tadx/internal/capability"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/spf13/cobra"
)

type batchAdminWriter struct {
	mu     sync.Mutex
	writes int
}

func (w *batchAdminWriter) ResolveBatchResource(_ context.Context, _, _ string, selector identity.Selector) (identity.LUID, error) {
	if selector.Name == "alice" {
		return "u1", nil
	}
	return selector.LUID, nil
}

func (w *batchAdminWriter) AddAdminGroupMember(context.Context, groupmember.Input, bool) (groupmember.Output, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	return groupmember.Output{}, nil
}

func (w *batchAdminWriter) CreateAdminPermission(context.Context, permission.Input, bool) (permission.Output, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	return permission.Output{}, nil
}

func TestBatchCompoundMutationIdentities(t *testing.T) {
	for _, test := range []struct {
		name, capability, body string
		path                   []string
		writes                 int
	}{
		{
			name: "different users in one group", capability: "admin.group.member.add", path: []string{"admin", "group-member", "add"}, writes: 2,
			body: `{"items":[{"group-id":"g1","user-id":"u1"},{"group-id":"g1","user-id":"u2"}]}`,
		},
		{
			name: "one user in different groups", capability: "admin.group.member.add", path: []string{"admin", "group-member", "add"}, writes: 2,
			body: `{"items":[{"group-id":"g1","user-id":"u1"},{"group-id":"g2","user-id":"u1"}]}`,
		},
		{
			name: "same membership pair", capability: "admin.group.member.add", path: []string{"admin", "group-member", "add"},
			body: `{"items":[{"group-id":"g1","user-id":"u1"},{"group-id":"g1","user-id":"u1"}]}`,
		},
		{
			name: "different capabilities for one principal", capability: "admin.permission.create", path: []string{"admin", "permission", "create"}, writes: 2,
			body: `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Write","mode":"Deny"}]}`,
		},
		{
			name: "different principals on one resource", capability: "admin.permission.create", path: []string{"admin", "permission", "create"}, writes: 2,
			body: `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u2","capability":"Read","mode":"Allow"}]}`,
		},
		{
			name: "conflicting mode on one capability", capability: "admin.permission.create", path: []string{"admin", "permission", "create"},
			body: `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Deny"}]}`,
		},
		{
			name: "partially overlapping capability arrays", capability: "admin.permission.create", path: []string{"admin", "permission", "create"},
			body: `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":["Read","Write"],"mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":["Write","Filter"],"mode":"Deny"}]}`,
		},
		{
			name: "separate project default collections", capability: "admin.permission.create", path: []string{"admin", "permission", "create"}, writes: 2,
			body: `{"items":[{"kind":"project","id":"p1","default-for":"workbooks","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"},{"kind":"project","id":"p1","default-for":"datasources","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"}]}`,
		},
		{
			name: "membership ID and username aliases", capability: "admin.group.member.add", path: []string{"admin", "group-member", "add"},
			body: `{"items":[{"group-id":"g1","user-id":"u1"},{"group-id":"g1","username":"alice"}]}`,
		},
		{
			name: "permission principal ID and username aliases", capability: "admin.permission.create", path: []string{"admin", "permission", "create"},
			body: `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-username":"alice","capability":"Read","mode":"Deny"}]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &batchAdminWriter{}
			factory := func(renderer Renderer) *cobra.Command {
				root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
				root.AddCommand(admincli.New(admincli.Dependencies{Renderer: renderer, GroupMemberAdder: writer, PermissionCreator: writer, BatchTargets: writer}, test.capability))
				return root
			}
			renderer := &batchCollector{}
			root := factory(renderer)
			command, _, err := root.Find(test.path)
			if err != nil {
				t.Fatal(err)
			}
			attachBatchWithOptions(root, command, capability.BatchOptions()[test.capability], renderer, factory)
			root.SetArgs(append(append([]string(nil), test.path...), "--environment", "fixture", "--batch-file", batchFile(t, test.body)))
			err = root.ExecuteContext(t.Context())
			if test.writes == 0 {
				if err == nil || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") {
					t.Fatalf("err=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if writer.writes != test.writes {
				t.Fatalf("writes=%d, want %d", writer.writes, test.writes)
			}
		})
	}
}
