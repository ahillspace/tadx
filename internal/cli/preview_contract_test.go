package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	labelcategoryops "github.com/ahillspace/tadx/actions/admin/labelcategory"
	labelvalueops "github.com/ahillspace/tadx/actions/admin/labelvalue"
	permission "github.com/ahillspace/tadx/actions/admin/permission"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	catalogupdate "github.com/ahillspace/tadx/actions/catalog"
	"github.com/ahillspace/tadx/actions/contentlabel"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	jobactions "github.com/ahillspace/tadx/actions/job"
	a_lineage_pull "github.com/ahillspace/tadx/actions/lineage/pull"
	projectops "github.com/ahillspace/tadx/actions/project"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	jobcli "github.com/ahillspace/tadx/internal/cli/job"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	"github.com/ahillspace/tadx/internal/errs"
)

type contentlabel_updateSpy struct{ spy *previewActionSpy }

func (s contentlabel_updateSpy) UpdateLabel(_ context.Context, _ contentlabel.UpdateInput, preview bool) (contentlabel.UpdateOutput, error) {
	s.spy.record(preview)
	return contentlabel.UpdateOutput{}, nil
}

type contentlabel_deleteSpy struct{ spy *previewActionSpy }

func (s contentlabel_deleteSpy) DeleteLabel(_ context.Context, _ contentlabel.DeleteInput, preview bool) (contentlabel.DeleteOutput, error) {
	s.spy.record(preview)
	return contentlabel.DeleteOutput{}, nil
}

type admin_labelvalue_updateSpy struct{ spy *previewActionSpy }

func (s admin_labelvalue_updateSpy) UpdateLabelValue(_ context.Context, _ labelvalueops.UpdateInput, preview bool) (labelvalueops.UpdateOutput, error) {
	s.spy.record(preview)
	return labelvalueops.UpdateOutput{}, nil
}

type admin_labelvalue_deleteSpy struct{ spy *previewActionSpy }

func (s admin_labelvalue_deleteSpy) DeleteLabelValue(_ context.Context, _ labelvalueops.DeleteInput, preview bool) (labelvalueops.DeleteOutput, error) {
	s.spy.record(preview)
	return labelvalueops.DeleteOutput{}, nil
}

type admin_labelcategory_createSpy struct{ spy *previewActionSpy }

func (s admin_labelcategory_createSpy) CreateLabelCategory(_ context.Context, _ labelcategoryops.CreateInput, preview bool) (labelcategoryops.WriteOutput, error) {
	s.spy.record(preview)
	return labelcategoryops.WriteOutput{}, nil
}

type admin_labelcategory_updateSpy struct{ spy *previewActionSpy }

func (s admin_labelcategory_updateSpy) UpdateLabelCategory(_ context.Context, _ labelcategoryops.UpdateInput, preview bool) (labelcategoryops.WriteOutput, error) {
	s.spy.record(preview)
	return labelcategoryops.WriteOutput{}, nil
}

type admin_labelcategory_deleteSpy struct{ spy *previewActionSpy }

func (s admin_labelcategory_deleteSpy) DeleteLabelCategory(_ context.Context, _ labelcategoryops.DeleteInput, preview bool) (labelcategoryops.DeleteOutput, error) {
	s.spy.record(preview)
	return labelcategoryops.DeleteOutput{}, nil
}

type previewActionSpy struct{ calls, writes int }

type jobCancelPreviewSpy struct{ spy *previewActionSpy }

func (s jobCancelPreviewSpy) Execute(_ context.Context, input jobactions.CancelInput) (jobactions.CancelOutput, error) {
	s.spy.record(input.Preview)
	return jobactions.CancelOutput{}, nil
}

func (s *previewActionSpy) UpdateCatalogDatabase(_ context.Context, _ catalogupdate.DatabaseInput, preview bool) (catalogupdate.DatabaseOutput, error) {
	s.record(preview)
	return catalogupdate.DatabaseOutput{}, nil
}
func (s *previewActionSpy) UpdateCatalogTable(_ context.Context, _ catalogupdate.TableInput, preview bool) (catalogupdate.TableOutput, error) {
	s.record(preview)
	return catalogupdate.TableOutput{}, nil
}
func (s *previewActionSpy) UpdateCatalogColumn(_ context.Context, _ catalogupdate.ColumnInput, preview bool) (catalogupdate.ColumnOutput, error) {
	s.record(preview)
	return catalogupdate.ColumnOutput{}, nil
}

func (s *previewActionSpy) record(preview bool) {
	s.calls++
	if !preview {
		s.writes++
	}
}
func (s *previewActionSpy) Render(any) error { return nil }
func (s *previewActionSpy) MoveWorkbook(_ context.Context, input workbookops.MoveInput, preview bool) (workbookops.MoveOutput, error) {
	s.record(preview)
	return workbookops.MoveOutput{}, nil
}
func (s *previewActionSpy) UpdateWorkbook(_ context.Context, input workbookops.UpdateInput, preview bool) (workbookops.UpdateOutput, error) {
	s.record(preview)
	return workbookops.UpdateOutput{}, nil
}
func (s *previewActionSpy) MoveDatasource(_ context.Context, input datasourceops.MoveInput, preview bool) (datasourceops.MoveOutput, error) {
	s.record(preview)
	return datasourceops.MoveOutput{}, nil
}
func (s *previewActionSpy) UpdateDatasource(_ context.Context, input datasourceops.UpdateInput, preview bool) (datasourceops.UpdateOutput, error) {
	s.record(preview)
	return datasourceops.UpdateOutput{}, nil
}
func (s *previewActionSpy) UpdateFlow(_ context.Context, input flowops.UpdateInput, preview bool) (flowops.UpdateOutput, error) {
	s.record(preview)
	return flowops.UpdateOutput{}, nil
}
func (s *previewActionSpy) MoveProject(_ context.Context, input projectops.MoveInput, preview bool) (projectops.MoveOutput, error) {
	s.record(preview)
	return projectops.MoveOutput{}, nil
}
func (s *previewActionSpy) ListDatasources(_ context.Context, input datasourceops.ListInput) (datasourceops.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectDatasource(_ context.Context, input datasourceops.InspectInput) (datasourceops.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PullDatasource(_ context.Context, input datasourceops.PullInput) (datasourceops.PullOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PublishDatasource(_ context.Context, input datasourceops.PublishInput, preview bool) (datasourceops.PublishOutput, error) {
	s.record(preview)
	return datasourceops.PublishOutput{}, nil
}
func (s *previewActionSpy) DeleteDatasource(_ context.Context, input datasourceops.DeleteInput, preview bool) (datasourceops.DeleteOutput, error) {
	s.record(preview)
	return datasourceops.DeleteOutput{}, nil
}
func (s *previewActionSpy) GetDatasourceSchema(_ context.Context, input datasourceops.SchemaInput) (datasourceops.SchemaOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ListFlows(_ context.Context, input flowops.ListInput) (flowops.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectFlow(_ context.Context, input flowops.InspectInput) (flowops.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PullFlow(_ context.Context, input flowops.PullInput) (flowops.PullOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PublishFlow(_ context.Context, input flowops.PublishInput, preview bool) (flowops.PublishOutput, error) {
	s.record(preview)
	return flowops.PublishOutput{}, nil
}
func (s *previewActionSpy) MoveFlow(_ context.Context, input flowops.MoveInput, preview bool) (flowops.MoveOutput, error) {
	s.record(preview)
	return flowops.MoveOutput{}, nil
}
func (s *previewActionSpy) DeleteFlow(_ context.Context, input flowops.DeleteInput, preview bool) (flowops.DeleteOutput, error) {
	s.record(preview)
	return flowops.DeleteOutput{}, nil
}
func (s *previewActionSpy) PullLineage(_ context.Context, input a_lineage_pull.Input) (a_lineage_pull.Output, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ListProjects(_ context.Context, input projectops.ListInput) (projectops.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectProject(_ context.Context, input projectops.InspectInput) (projectops.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateProject(_ context.Context, input projectops.CreateInput, preview bool) (projectops.CreateOutput, error) {
	s.record(preview)
	return projectops.CreateOutput{}, nil
}
func (s *previewActionSpy) UpdateProject(_ context.Context, input projectops.UpdateInput, preview bool) (projectops.UpdateOutput, error) {
	s.record(preview)
	return projectops.UpdateOutput{}, nil
}
func (s *previewActionSpy) DeleteProject(_ context.Context, input projectops.DeleteInput, preview bool) (projectops.DeleteOutput, error) {
	s.record(preview)
	return projectops.DeleteOutput{}, nil
}
func (s *previewActionSpy) DeleteWorkbook(_ context.Context, input workbookops.DeleteInput, preview bool) (workbookops.DeleteOutput, error) {
	s.record(preview)
	return workbookops.DeleteOutput{}, nil
}
func (s *previewActionSpy) ListWorkbooks(_ context.Context, input workbookops.ListInput) (workbookops.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectWorkbook(_ context.Context, input workbookops.InspectInput) (workbookops.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ListAdminUsers(_ context.Context, input userops.ListInput) (userops.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectAdminUser(_ context.Context, input userops.InspectInput) (userops.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateAdminUser(_ context.Context, input userops.CreateInput, preview bool) (userops.CreateOutput, error) {
	s.record(preview)
	return userops.CreateOutput{}, nil
}
func (s *previewActionSpy) UpdateAdminUser(_ context.Context, input userops.UpdateInput, preview bool) (userops.UpdateOutput, error) {
	s.record(preview)
	return userops.UpdateOutput{}, nil
}
func (s *previewActionSpy) DeleteAdminUser(_ context.Context, input userops.DeleteInput, preview bool) (userops.DeleteOutput, error) {
	s.record(preview)
	return userops.DeleteOutput{}, nil
}
func (s *previewActionSpy) ListAdminGroups(_ context.Context, input groupops.ListInput) (groupops.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectAdminGroup(_ context.Context, input groupops.InspectInput) (groupops.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateAdminGroup(_ context.Context, input groupops.CreateInput, preview bool) (groupops.CreateOutput, error) {
	s.record(preview)
	return groupops.CreateOutput{}, nil
}
func (s *previewActionSpy) UpdateAdminGroup(_ context.Context, input groupops.UpdateInput, preview bool) (groupops.UpdateOutput, error) {
	s.record(preview)
	return groupops.UpdateOutput{}, nil
}
func (s *previewActionSpy) DeleteAdminGroup(_ context.Context, input groupops.DeleteInput, preview bool) (groupops.DeleteOutput, error) {
	s.record(preview)
	return groupops.DeleteOutput{}, nil
}
func (s *previewActionSpy) AddAdminGroupMember(_ context.Context, input groupops.MembershipInput, preview bool) (groupops.MembershipOutput, error) {
	s.record(preview)
	return groupops.MembershipOutput{}, nil
}
func (s *previewActionSpy) RemoveAdminGroupMember(_ context.Context, input groupops.MembershipInput, preview bool) (groupops.MembershipOutput, error) {
	s.record(preview)
	return groupops.MembershipOutput{}, nil
}
func (s *previewActionSpy) InspectAdminPermission(_ context.Context, input permission.InspectInput) (permission.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) CreateAdminPermission(_ context.Context, input permission.Input, preview bool) (permission.Output, error) {
	s.record(preview)
	return permission.Output{}, nil
}
func (s *previewActionSpy) DeleteAdminPermission(_ context.Context, input permission.Input, preview bool) (permission.Output, error) {
	s.record(preview)
	return permission.Output{}, nil
}
func (s *previewActionSpy) ListPulseDefinitions(_ context.Context, input pulsedefinition.ListInput) (pulsedefinition.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectPulseDefinition(_ context.Context, input pulsedefinition.InspectInput) (pulsedefinition.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) PullPulseDefinition(_ context.Context, input pulsedefinition.PullInput) (pulsedefinition.PullOutput, error) {
	panic("unexpected read action")
}

func (s *previewActionSpy) PublishPulseDefinition(_ context.Context, input pulsedefinition.PublishInput) (pulsedefinition.PublishOutput, error) {
	s.record(input.Preview)
	return pulsedefinition.PublishOutput{}, nil
}
func (s *previewActionSpy) CreatePulseDefinition(_ context.Context, input pulsedefinition.CreateInput, preview bool) (pulsedefinition.CreateOutput, error) {
	s.record(preview)
	return pulsedefinition.CreateOutput{}, nil
}
func (s *previewActionSpy) DeletePulseDefinition(_ context.Context, input pulsedefinition.DeleteInput) (pulsedefinition.DeleteOutput, error) {
	s.record(input.Preview)
	return pulsedefinition.DeleteOutput{}, nil
}
func (s *previewActionSpy) ListPulseMetrics(_ context.Context, input pulsemetric.ListInput) (pulsemetric.ListOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) InspectPulseMetric(_ context.Context, input pulsemetric.InspectInput) (pulsemetric.InspectOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) ForkPulseMetric(_ context.Context, input pulsemetric.ForkInput, preview bool) (pulsemetric.ForkOutput, error) {
	s.record(preview)
	return pulsemetric.ForkOutput{}, nil
}
func (s *previewActionSpy) DeletePulseMetric(_ context.Context, input pulsemetric.DeleteInput) (pulsemetric.DeleteOutput, error) {
	s.record(input.Preview)
	return pulsemetric.DeleteOutput{}, nil
}
func (s *previewActionSpy) ListPulseMetricFollowers(_ context.Context, input pulsemetric.FollowersInput) (pulsemetric.FollowersOutput, error) {
	panic("unexpected read action")
}
func (s *previewActionSpy) FollowPulseMetric(_ context.Context, input pulsemetric.FollowInput, preview bool) (pulsemetric.FollowOutput, error) {
	s.record(preview)
	return pulsemetric.FollowOutput{}, nil
}
func (s *previewActionSpy) UnfollowPulseMetric(_ context.Context, input pulsemetric.UnfollowInput, preview bool) (pulsemetric.UnfollowOutput, error) {
	s.record(preview)
	return pulsemetric.UnfollowOutput{}, nil
}
func (s *previewActionSpy) Execute(_ context.Context, _ workbookops.PublishInput, preview bool) (workbookops.PublishOutput, error) {
	s.record(preview)
	return workbookops.PublishOutput{}, nil
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
		Jobs:           &jobcli.Dependencies{Cancel: (jobCancelPreviewSpy{spy: spy}).Execute},
		CatalogLabels:  &catalogcli.LabelDependencies{Renderer: spy, Updater: contentlabel_updateSpy{spy}, Deleter: contentlabel_deleteSpy{spy}},
		CatalogLineage: spy,
		AdminLabels:    &admincli.LabelDependencies{Renderer: spy, ValueUpdater: admin_labelvalue_updateSpy{spy}, ValueDeleter: admin_labelvalue_deleteSpy{spy}, CategoryCreator: admin_labelcategory_createSpy{spy}, CategoryUpdater: admin_labelcategory_updateSpy{spy}, CategoryDeleter: admin_labelcategory_deleteSpy{spy}},
		Catalog:        &catalogcli.Dependencies{Renderer: spy, DatabaseUpdater: spy, TableUpdater: spy, ColumnUpdater: spy},
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
			DefinitionPublisher: spy,
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
		"content.label.update":        "--id label --message Meaning",
		"content.label.delete":        "--id label",
		"admin.label.value.update":    "--name Warning --description Meaning",
		"admin.label.value.delete":    "--name Warning",
		"admin.label.category.create": "--name Custom --description Meaning",
		"admin.label.category.update": "--name Custom --description Meaning",
		"admin.label.category.delete": "--name Custom",

		"catalog.database.update":   "--id database --description Meaning",
		"catalog.table.update":      "--id table --description Meaning",
		"catalog.column.update":     "--table-id table --id column --description Meaning",
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
		"pulse.definition.publish":  "--workspace local --artifact artifacts/pulse-definition/example --datasource-map source=destination",
		"pulse.metric.delete":       "--id metric",
		"pulse.metric.follow":       "--id metric --user-id user",
		"pulse.metric.fork":         "--id metric --period LAST_30_DAYS",
		"pulse.metric.unfollow":     "--subscription-id subscription",
		"workbook.delete":           "--id book",
		"workbook.move":             "--id book --destination-project-id destination",
		"workbook.publish":          "--workspace local --artifact artifacts/workbook/Book--id --project-id destination",
		"workbook.update":           "--id book --owner-id owner",
		"job.cancel":                "--id job",
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
