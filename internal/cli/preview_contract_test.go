package cli_test

import (
	"context"
	"errors"
	a_admin_group_create "github.com/ahillspace/tadx/actions/admin/group/create"
	a_admin_group_delete "github.com/ahillspace/tadx/actions/admin/group/delete"
	a_admin_group_inspect "github.com/ahillspace/tadx/actions/admin/group/inspect"
	a_admin_group_list "github.com/ahillspace/tadx/actions/admin/group/list"
	a_admin_group_member_add "github.com/ahillspace/tadx/actions/admin/group/member/add"
	a_admin_group_member_remove "github.com/ahillspace/tadx/actions/admin/group/member/remove"
	a_admin_group_update "github.com/ahillspace/tadx/actions/admin/group/update"
	a_admin_permission_create "github.com/ahillspace/tadx/actions/admin/permission/create"
	a_admin_permission_delete "github.com/ahillspace/tadx/actions/admin/permission/delete"
	a_admin_permission_inspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	a_admin_user_create "github.com/ahillspace/tadx/actions/admin/user/create"
	a_admin_user_delete "github.com/ahillspace/tadx/actions/admin/user/delete"
	a_admin_user_inspect "github.com/ahillspace/tadx/actions/admin/user/inspect"
	a_admin_user_list "github.com/ahillspace/tadx/actions/admin/user/list"
	a_admin_user_update "github.com/ahillspace/tadx/actions/admin/user/update"
	a_datasource_delete "github.com/ahillspace/tadx/actions/datasource/delete"
	a_datasource_inspect "github.com/ahillspace/tadx/actions/datasource/inspect"
	a_datasource_list "github.com/ahillspace/tadx/actions/datasource/list"
	a_datasource_move "github.com/ahillspace/tadx/actions/datasource/move"
	a_datasource_publish "github.com/ahillspace/tadx/actions/datasource/publish"
	a_datasource_pull "github.com/ahillspace/tadx/actions/datasource/pull"
	a_datasource_schema "github.com/ahillspace/tadx/actions/datasource/schema"
	a_datasource_update "github.com/ahillspace/tadx/actions/datasource/update"
	a_flow_delete "github.com/ahillspace/tadx/actions/flow/delete"
	a_flow_inspect "github.com/ahillspace/tadx/actions/flow/inspect"
	a_flow_list "github.com/ahillspace/tadx/actions/flow/list"
	a_flow_move "github.com/ahillspace/tadx/actions/flow/move"
	a_flow_publish "github.com/ahillspace/tadx/actions/flow/publish"
	a_flow_pull "github.com/ahillspace/tadx/actions/flow/pull"
	a_flow_update "github.com/ahillspace/tadx/actions/flow/update"
	a_lineage_pull "github.com/ahillspace/tadx/actions/lineage/pull"
	a_project_create "github.com/ahillspace/tadx/actions/project/create"
	a_project_delete "github.com/ahillspace/tadx/actions/project/delete"
	a_project_inspect "github.com/ahillspace/tadx/actions/project/inspect"
	a_project_list "github.com/ahillspace/tadx/actions/project/list"
	a_project_move "github.com/ahillspace/tadx/actions/project/move"
	a_project_update "github.com/ahillspace/tadx/actions/project/update"
	a_pulse_definition_create "github.com/ahillspace/tadx/actions/pulse/definition/create"
	a_pulse_definition_delete "github.com/ahillspace/tadx/actions/pulse/definition/delete"
	a_pulse_definition_inspect "github.com/ahillspace/tadx/actions/pulse/definition/inspect"
	a_pulse_definition_list "github.com/ahillspace/tadx/actions/pulse/definition/list"
	a_pulse_definition_pull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	a_pulse_metric_delete "github.com/ahillspace/tadx/actions/pulse/metric/delete"
	a_pulse_metric_follow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	a_pulse_metric_followers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	a_pulse_metric_fork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	a_pulse_metric_inspect "github.com/ahillspace/tadx/actions/pulse/metric/inspect"
	a_pulse_metric_list "github.com/ahillspace/tadx/actions/pulse/metric/list"
	a_pulse_metric_unfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
	a_workbook_delete "github.com/ahillspace/tadx/actions/workbook/delete"
	a_workbook_inspect "github.com/ahillspace/tadx/actions/workbook/inspect"
	a_workbook_list "github.com/ahillspace/tadx/actions/workbook/list"
	a_workbook_move "github.com/ahillspace/tadx/actions/workbook/move"
	a_workbook_publish "github.com/ahillspace/tadx/actions/workbook/publish"
	a_workbook_update "github.com/ahillspace/tadx/actions/workbook/update"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
	"testing"
)

type previewActionSpy struct{ calls, writes int }

func (s *previewActionSpy) record(preview bool) {
	s.calls++
	if !preview {
		s.writes++
	}
}
func (s *previewActionSpy) Render(any) error { return nil }
func (s *previewActionSpy) MoveWorkbook(_ context.Context, input a_workbook_move.Input, preview bool) (a_workbook_move.Output, error) {
	s.record(preview)
	return a_workbook_move.Output{}, nil
}
func (s *previewActionSpy) UpdateWorkbook(_ context.Context, input a_workbook_update.Input, preview bool) (a_workbook_update.Output, error) {
	s.record(preview)
	return a_workbook_update.Output{}, nil
}
func (s *previewActionSpy) MoveDatasource(_ context.Context, input a_datasource_move.Input, preview bool) (a_datasource_move.Output, error) {
	s.record(preview)
	return a_datasource_move.Output{}, nil
}
func (s *previewActionSpy) UpdateDatasource(_ context.Context, input a_datasource_update.Input, preview bool) (a_datasource_update.Output, error) {
	s.record(preview)
	return a_datasource_update.Output{}, nil
}
func (s *previewActionSpy) UpdateFlow(_ context.Context, input a_flow_update.Input, preview bool) (a_flow_update.Output, error) {
	s.record(preview)
	return a_flow_update.Output{}, nil
}
func (s *previewActionSpy) MoveProject(_ context.Context, input a_project_move.Input, preview bool) (a_project_move.Output, error) {
	s.record(preview)
	return a_project_move.Output{}, nil
}
func (s *previewActionSpy) ListDatasources(_ context.Context, input a_datasource_list.Input) (a_datasource_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectDatasource(_ context.Context, input a_datasource_inspect.Input) (a_datasource_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PullDatasource(_ context.Context, input a_datasource_pull.Input) (a_datasource_pull.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PublishDatasource(_ context.Context, input a_datasource_publish.Input, preview bool) (a_datasource_publish.Output, error) {
	s.record(preview)
	return a_datasource_publish.Output{}, nil
}
func (s *previewActionSpy) DeleteDatasource(_ context.Context, input a_datasource_delete.Input, preview bool) (a_datasource_delete.Output, error) {
	s.record(preview)
	return a_datasource_delete.Output{}, nil
}
func (s *previewActionSpy) GetDatasourceSchema(_ context.Context, input a_datasource_schema.Input) (a_datasource_schema.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ListFlows(_ context.Context, input a_flow_list.Input) (a_flow_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectFlow(_ context.Context, input a_flow_inspect.Input) (a_flow_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PullFlow(_ context.Context, input a_flow_pull.Input) (a_flow_pull.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PublishFlow(_ context.Context, input a_flow_publish.Input, preview bool) (a_flow_publish.Output, error) {
	s.record(preview)
	return a_flow_publish.Output{}, nil
}
func (s *previewActionSpy) MoveFlow(_ context.Context, input a_flow_move.Input, preview bool) (a_flow_move.Output, error) {
	s.record(preview)
	return a_flow_move.Output{}, nil
}
func (s *previewActionSpy) DeleteFlow(_ context.Context, input a_flow_delete.Input, preview bool) (a_flow_delete.Output, error) {
	s.record(preview)
	return a_flow_delete.Output{}, nil
}
func (s *previewActionSpy) PullLineage(_ context.Context, input a_lineage_pull.Input) (a_lineage_pull.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ListProjects(_ context.Context, input a_project_list.Input) (a_project_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectProject(_ context.Context, input a_project_inspect.Input) (a_project_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateProject(_ context.Context, input a_project_create.Input, preview bool) (a_project_create.Output, error) {
	s.record(preview)
	return a_project_create.Output{}, nil
}
func (s *previewActionSpy) UpdateProject(_ context.Context, input a_project_update.Input, preview bool) (a_project_update.Output, error) {
	s.record(preview)
	return a_project_update.Output{}, nil
}
func (s *previewActionSpy) DeleteProject(_ context.Context, input a_project_delete.Input, preview bool) (a_project_delete.Output, error) {
	s.record(preview)
	return a_project_delete.Output{}, nil
}
func (s *previewActionSpy) DeleteWorkbook(_ context.Context, input a_workbook_delete.Input, preview bool) (a_workbook_delete.Output, error) {
	s.record(preview)
	return a_workbook_delete.Output{}, nil
}
func (s *previewActionSpy) ListWorkbooks(_ context.Context, input a_workbook_list.Input) (a_workbook_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectWorkbook(_ context.Context, input a_workbook_inspect.Input) (a_workbook_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ListAdminUsers(_ context.Context, input a_admin_user_list.Input) (a_admin_user_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectAdminUser(_ context.Context, input a_admin_user_inspect.Input) (a_admin_user_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateAdminUser(_ context.Context, input a_admin_user_create.Input, preview bool) (a_admin_user_create.Output, error) {
	s.record(preview)
	return a_admin_user_create.Output{}, nil
}
func (s *previewActionSpy) UpdateAdminUser(_ context.Context, input a_admin_user_update.Input, preview bool) (a_admin_user_update.Output, error) {
	s.record(preview)
	return a_admin_user_update.Output{}, nil
}
func (s *previewActionSpy) DeleteAdminUser(_ context.Context, input a_admin_user_delete.Input, preview bool) (a_admin_user_delete.Output, error) {
	s.record(preview)
	return a_admin_user_delete.Output{}, nil
}
func (s *previewActionSpy) ListAdminGroups(_ context.Context, input a_admin_group_list.Input) (a_admin_group_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectAdminGroup(_ context.Context, input a_admin_group_inspect.Input) (a_admin_group_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateAdminGroup(_ context.Context, input a_admin_group_create.Input, preview bool) (a_admin_group_create.Output, error) {
	s.record(preview)
	return a_admin_group_create.Output{}, nil
}
func (s *previewActionSpy) UpdateAdminGroup(_ context.Context, input a_admin_group_update.Input, preview bool) (a_admin_group_update.Output, error) {
	s.record(preview)
	return a_admin_group_update.Output{}, nil
}
func (s *previewActionSpy) DeleteAdminGroup(_ context.Context, input a_admin_group_delete.Input, preview bool) (a_admin_group_delete.Output, error) {
	s.record(preview)
	return a_admin_group_delete.Output{}, nil
}
func (s *previewActionSpy) AddAdminGroupMember(_ context.Context, input a_admin_group_member_add.Input, preview bool) (a_admin_group_member_add.Output, error) {
	s.record(preview)
	return a_admin_group_member_add.Output{}, nil
}
func (s *previewActionSpy) RemoveAdminGroupMember(_ context.Context, input a_admin_group_member_remove.Input, preview bool) (a_admin_group_member_remove.Output, error) {
	s.record(preview)
	return a_admin_group_member_remove.Output{}, nil
}
func (s *previewActionSpy) InspectAdminPermission(_ context.Context, input a_admin_permission_inspect.Input) (a_admin_permission_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateAdminPermission(_ context.Context, input a_admin_permission_create.Input, preview bool) (a_admin_permission_create.Output, error) {
	s.record(preview)
	return a_admin_permission_create.Output{}, nil
}
func (s *previewActionSpy) DeleteAdminPermission(_ context.Context, input a_admin_permission_delete.Input, preview bool) (a_admin_permission_delete.Output, error) {
	s.record(preview)
	return a_admin_permission_delete.Output{}, nil
}
func (s *previewActionSpy) ListPulseDefinitions(_ context.Context, input a_pulse_definition_list.Input) (a_pulse_definition_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectPulseDefinition(_ context.Context, input a_pulse_definition_inspect.Input) (a_pulse_definition_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PullPulseDefinition(_ context.Context, input a_pulse_definition_pull.Input) (a_pulse_definition_pull.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreatePulseDefinition(_ context.Context, input a_pulse_definition_create.Input, preview bool) (a_pulse_definition_create.Output, error) {
	s.record(preview)
	return a_pulse_definition_create.Output{}, nil
}
func (s *previewActionSpy) DeletePulseDefinition(_ context.Context, input a_pulse_definition_delete.Input) (a_pulse_definition_delete.Output, error) {
	s.record(input.Preview)
	return a_pulse_definition_delete.Output{}, nil
}
func (s *previewActionSpy) ListPulseMetrics(_ context.Context, input a_pulse_metric_list.Input) (a_pulse_metric_list.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectPulseMetric(_ context.Context, input a_pulse_metric_inspect.Input) (a_pulse_metric_inspect.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ForkPulseMetric(_ context.Context, input a_pulse_metric_fork.Input, preview bool) (a_pulse_metric_fork.Output, error) {
	s.record(preview)
	return a_pulse_metric_fork.Output{}, nil
}
func (s *previewActionSpy) DeletePulseMetric(_ context.Context, input a_pulse_metric_delete.Input) (a_pulse_metric_delete.Output, error) {
	s.record(input.Preview)
	return a_pulse_metric_delete.Output{}, nil
}
func (s *previewActionSpy) ListPulseMetricFollowers(_ context.Context, input a_pulse_metric_followers.Input) (a_pulse_metric_followers.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) FollowPulseMetric(_ context.Context, input a_pulse_metric_follow.Input, preview bool) (a_pulse_metric_follow.Output, error) {
	s.record(preview)
	return a_pulse_metric_follow.Output{}, nil
}
func (s *previewActionSpy) UnfollowPulseMetric(_ context.Context, input a_pulse_metric_unfollow.Input, preview bool) (a_pulse_metric_unfollow.Output, error) {
	s.record(preview)
	return a_pulse_metric_unfollow.Output{}, nil
}
func (s *previewActionSpy) Execute(_ context.Context, _ a_workbook_publish.Input, preview bool) (a_workbook_publish.Output, error) {
	s.record(preview)
	return a_workbook_publish.Output{}, nil
}

type registryPreviewPolicy struct{}

func (registryPreviewPolicy) IsRemoteMutation(id string) bool {
	for _, d := range capability.All() {
		if d.ID == id {
			return d.RemoteMutation
		}
	}
	return false
}
func previewDependencies(spy *previewActionSpy) cli.Dependencies {
	return cli.Dependencies{MutationPolicy: registryPreviewPolicy{}, Renderer: spy, WorkbookPublisher: spy, WorkbookPuller: &puller{},
		Content: &contentcli.Dependencies{Renderer: spy,
			WorkbookLister:      spy,
			WorkbookInspector:   spy,
			WorkbookDeleter:     spy,
			WorkbookMover:       spy,
			WorkbookUpdater:     spy,
			DatasourceLister:    spy,
			DatasourceInspector: spy,
			DatasourceSchema:    spy,
			DatasourcePuller:    spy,
			DatasourcePublisher: spy,
			DatasourceDeleter:   spy,
			DatasourceMover:     spy,
			DatasourceUpdater:   spy,
			ProjectLister:       spy,
			ProjectInspector:    spy,
			ProjectCreator:      spy,
			ProjectUpdater:      spy,
			ProjectDeleter:      spy,
			ProjectMover:        spy,
			FlowLister:          spy,
			FlowInspector:       spy,
			FlowPuller:          spy,
			FlowPublisher:       spy,
			FlowMover:           spy,
			FlowDeleter:         spy,
			FlowUpdater:         spy,
			LineagePuller:       spy,
		},
		Admin: &admincli.Dependencies{Renderer: spy,
			PermissionCreator:   spy,
			PermissionDeleter:   spy,
			UserLister:          spy,
			UserInspector:       spy,
			UserCreator:         spy,
			UserUpdater:         spy,
			UserDeleter:         spy,
			GroupLister:         spy,
			GroupInspector:      spy,
			GroupCreator:        spy,
			GroupUpdater:        spy,
			GroupDeleter:        spy,
			GroupMemberAdder:    spy,
			GroupMemberRemover:  spy,
			PermissionInspector: spy,
		},
		Pulse: &pulsecli.Dependencies{Renderer: spy,
			DefinitionLister:    spy,
			DefinitionInspector: spy,
			DefinitionPuller:    spy,
			DefinitionCreator:   spy,
			DefinitionDeleter:   spy,
			MetricLister:        spy,
			MetricInspector:     spy,
			MetricForker:        spy,
			MetricDeleter:       spy,
			MetricFollowers:     spy,
			MetricFollower:      spy,
			MetricUnfollower:    spy,
		},
	}
}

// Actual command bindings must pass preview=true to their action when mutations are off.
func TestEveryRemoteMutationDispatchesPreviewWithoutWrites(t *testing.T) {
	flags := map[string]string{
		"admin.group.create":        "--name New",
		"admin.group.delete":        "--id group",
		"admin.group.member.add":    "--group-id group --user-id user",
		"admin.group.member.remove": "--group-id group --user-id user",
		"admin.group.update":        "--id group --name Renamed",
		"admin.permission.create":   "--kind workbook --id book --principal-type user --principal-id user --capability Read --mode Allow",
		"admin.permission.delete":   "--kind workbook --id book --principal-type user --principal-id user --capability Read --mode Allow",
		"admin.user.create":         "--name person@example.com --site-role Unlicensed",
		"admin.user.delete":         "--id user",
		"admin.user.update":         "--id user --site-role Unlicensed",
		"datasource.delete":         "--id source",
		"datasource.move":           "--id source --destination-project-id destination",
		"datasource.publish":        "--workspace local --artifact artifacts/datasource/Source--id --project-id destination --create",
		"datasource.update":         "--id source --owner-id owner",
		"flow.delete":               "--id flow",
		"flow.move":                 "--id flow --destination-project-id destination",
		"flow.publish":              "--workspace local --artifact artifacts/flow/Flow--id --project-id destination",
		"flow.update":               "--id flow --owner-id owner",
		"project.create":            "--name New",
		"project.delete":            "--project-id project",
		"project.move":              "--project-id project --parent-id destination",
		"project.update":            "--project-id project --name Renamed",
		"pulse.definition.create":   "--name Revenue --datasource-id source --measure-field Sales --date-field Date --dimension Region",
		"pulse.definition.delete":   "--id definition",
		"pulse.metric.delete":       "--id metric",
		"pulse.metric.follow":       "--id metric --user-id user",
		"pulse.metric.fork":         "--id metric --period LAST_30_DAYS",
		"pulse.metric.unfollow":     "--subscription-id subscription",
		"workbook.delete":           "--id book",
		"workbook.move":             "--id book --destination-project-id destination",
		"workbook.publish":          "--workspace local --artifact artifacts/workbook/Book--id --project-id destination",
		"workbook.update":           "--id book --owner-id owner",
	}
	covered := 0
	for _, definition := range capability.All() {
		if !definition.RemoteMutation || len(definition.CommandPath) == 0 {
			continue
		}
		covered++
		arguments, ok := flags[definition.ID]
		if !ok || !definition.SupportsPreview {
			t.Fatalf("missing supported preview contract for %s", definition.ID)
		}
		for _, previewFlag := range []string{"--preview", "--preview=true", "--preview=false", ""} {
			t.Run(definition.ID+"/"+previewFlag, func(t *testing.T) {
				spy := &previewActionSpy{}
				root := cli.NewRoot(previewDependencies(spy))
				args := append(append([]string{}, definition.CommandPath...), strings.Fields(arguments)...)
				args = append(args, "--environment", "test")
				if previewFlag != "" {
					args = append(args, previewFlag)
				}
				root.SetArgs(args)
				err := root.Execute()
				if previewFlag == "--preview" || previewFlag == "--preview=true" {
					if err != nil || spy.calls != 1 {
						t.Fatalf("preview error=%v calls=%d", err, spy.calls)
					}
				} else {
					var structured *errs.Error
					if !errors.As(err, &structured) || structured.ID != "mutation.disabled" || spy.calls != 0 {
						t.Fatalf("execution error=%v calls=%d", err, spy.calls)
					}
				}
				if spy.writes != 0 {
					t.Fatalf("preview dispatched %d consequential writes", spy.writes)
				}
			})
		}
	}
	if covered != len(flags) {
		t.Fatalf("preview contract has %d cases for %d registered remote mutations", len(flags), covered)
	}
}
