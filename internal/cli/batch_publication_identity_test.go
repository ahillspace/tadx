package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/capability"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/identity"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	"github.com/spf13/cobra"
)

// The actual resource adapter gets one stable Tableau inventory record. Both
// aliases therefore resolve through production collision code, not a stubbed
// FindWorkbooks implementation. Only the external inventory and writes are fake.
type publicationIdentityInventory struct {
	resourceworkbook.Client
	items []tableauworkbook.Workbook
}

func (i publicationIdentityInventory) List(_ context.Context, number, size int) (tableauworkbook.WorkbookPage, error) {
	return tableauworkbook.WorkbookPage{
		Page:  tableauworkbook.Page{Number: number, Size: size, Total: len(i.items)},
		Items: i.items,
	}, nil
}

type publicationIdentityResolver struct {
	*publicationBatchBackend
	adapter *resourceworkbook.Adapter
}

func (r publicationIdentityResolver) FindWorkbooks(ctx context.Context, name, project string) ([]workbookops.Record, error) {
	return r.adapter.FindWorkbooks(ctx, name, project)
}

type publicationIdentityService struct {
	backend  *publicationBatchBackend
	resolver publicationIdentityResolver
}

func (publicationIdentityService) ResolveBatchResource(context.Context, string, string, identity.Selector) (identity.LUID, error) {
	return "project", nil
}

func (publicationIdentityService) AuthorizeBatchPreparation(context.Context, string) error {
	return nil
}

func (s publicationIdentityService) Execute(ctx context.Context, input workbookops.PublishInput, preview bool) (workbookops.PublishOutput, error) {
	input.Site, input.TargetResolved, input.ArtifactPath = "site", true, input.File
	return workbookops.NewPublish(publicationBatchReader{}, s.resolver, s.backend).Execute(ctx, input, preview)
}

func TestBatchPublicationRejectsEquivalentDestinationNamesBeforeWriting(t *testing.T) {
	for _, names := range [][2]string{{"Destination", "destination"}, {"\u03a3", "\u03c2"}} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing_%t", names[0], existing), func(t *testing.T) {
				first, second := publicationSource(t, "first"), publicationSource(t, "second")
				var items []tableauworkbook.Workbook
				if existing {
					items = []tableauworkbook.Workbook{{LUID: "same-workbook", Name: names[0], ProjectLUID: "project", ProjectName: "Project"}}
				}
				backend := &publicationBatchBackend{}
				service := publicationIdentityService{backend: backend, resolver: publicationIdentityResolver{publicationBatchBackend: backend, adapter: resourceworkbook.NewAdapter(publicationIdentityInventory{items: items})}}
				for _, name := range names {
					input := workbookops.PublishInput{Environment: "fixture", File: first, Name: name, ProjectLUID: "project", Overwrite: true}
					out, err := service.Execute(t.Context(), input, true)
					prepared := out.Plan.PrepareInput(input)
					if err != nil || out.Plan.Destination().Name != name || prepared.Name != name || prepared.ExpectedDestination.Name != name {
						t.Fatalf("exact execution name changed: plan=%+v err=%v", out.Plan, err)
					}
				}
				factory := func(renderer Renderer) *cobra.Command {
					root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
					root.AddCommand(contentcli.New(contentcli.Dependencies{Publisher: service, Renderer: renderer, BatchTargets: service}))
					return root
				}
				capture := &batchCollector{}
				root := factory(capture)
				command, _, err := root.Find([]string{"content", "workbook", "publish"})
				if err != nil {
					t.Fatal(err)
				}
				attachBatchWithOptions(root, command, capability.BatchOptions()["workbook.publish"], capture, factory)
				body, err := json.Marshal(map[string]any{"items": []map[string]string{{"file": first, "name": names[0]}, {"file": second, "name": names[1]}}})
				if err != nil {
					t.Fatal(err)
				}
				root.SetArgs([]string{"content", "workbook", "publish", "--environment", "fixture", "--project-id", "project", "--overwrite", "--batch-file", batchFile(t, string(body))})
				err = root.ExecuteContext(t.Context())
				if err == nil || !strings.Contains(err.Error(), "duplicates") || len(backend.writes) != 0 {
					t.Fatalf("writes=%v err=%v", backend.writes, err)
				}
			})
		}
	}
}
