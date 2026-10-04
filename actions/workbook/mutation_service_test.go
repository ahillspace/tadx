package workbook_test

import (
	"context"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
)

type workbookMutationProvider struct {
	session                       workbookops.MutationSession
	opens                         *int
	targetEnvironment, targetSite string
}

func (p workbookMutationProvider) OpenWorkbookMutation(_ context.Context, environment, site, _ string) (workbookops.MutationSession, error) {
	if p.opens != nil {
		*p.opens++
	}
	p.session.Environment, p.session.Site = environment, site
	if p.targetEnvironment != "" {
		p.session.Environment = p.targetEnvironment
	}
	if p.targetSite != "" {
		p.session.Site = p.targetSite
	}
	return p.session, nil
}

func TestMutationServiceUsesCanonicalWorkbookTarget(t *testing.T) {
	resolver := &moveResolver{workbook: workbookops.Record{LUID: "wb-1", Name: "Sales", ProjectLUID: "source"}, project: workbookops.Project{LUID: "destination"}}
	provider := workbookMutationProvider{session: workbookops.MutationSession{MoveResolver: resolver, Mover: &moveMover{}}, targetEnvironment: "canonical", targetSite: "canonical-site"}
	output, err := workbookops.New(workbookops.Ports{Mutation: provider}).MoveWorkbook(t.Context(), workbookops.MoveInput{Environment: "alias", Site: "caller-site", WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "destination"}}, true)
	if err != nil || output.Plan.Environment != "canonical" || output.Plan.Site != "canonical-site" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
}

func TestMutationServiceDeleteBindsDefaultSiteAfterOpen(t *testing.T) {
	resolver := &deleteResolver{results: []workbookops.Record{{LUID: "wb-1", Name: "Sales"}}}
	provider := workbookMutationProvider{session: workbookops.MutationSession{DeleteResolver: resolver, Deleter: &deleteDeleter{}}, targetSite: "canonical-site"}
	output, err := workbookops.New(workbookops.Ports{Mutation: provider}).DeleteWorkbook(t.Context(), workbookops.DeleteInput{Environment: "dev", Selector: identity.Selector{LUID: "wb-1"}}, true)
	if err != nil || output.Plan.Site != "canonical-site" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
}

func TestMutationServiceRejectsInvalidWorkbookInputBeforeOpen(t *testing.T) {
	opens := 0
	service := workbookops.New(workbookops.Ports{Mutation: workbookMutationProvider{opens: &opens}})
	if _, err := service.MoveWorkbook(t.Context(), workbookops.MoveInput{Environment: "dev", WorkbookSelector: identity.Selector{LUID: "wb-1"}}, false); err == nil {
		t.Fatal("move accepted missing destination")
	}
	if _, err := service.UpdateWorkbook(t.Context(), workbookops.UpdateInput{Environment: "dev", Selector: identity.Selector{LUID: "wb-1"}}, false); err == nil {
		t.Fatal("update accepted no changes")
	}
	if _, err := service.DeleteWorkbook(t.Context(), workbookops.DeleteInput{Environment: "dev"}, false); err == nil {
		t.Fatal("delete accepted no selector")
	}
	if _, err := service.DeleteWorkbook(t.Context(), workbookops.DeleteInput{Selector: identity.Selector{LUID: "wb-1"}}, false); err == nil {
		t.Fatal("delete accepted no environment")
	}
	if _, err := service.DeleteWorkbook(t.Context(), workbookops.DeleteInput{Environment: "  ", Selector: identity.Selector{LUID: "wb-1"}}, false); err == nil {
		t.Fatal("delete accepted whitespace environment")
	}
	if opens != 0 {
		t.Fatalf("invalid inputs opened provider %d times", opens)
	}
}

func runMove(ctx context.Context, resolver workbookops.MoveResolver, mover workbookops.Mover, input workbookops.MoveInput, preview bool) (workbookops.MoveOutput, error) {
	provider := workbookMutationProvider{session: workbookops.MutationSession{MoveResolver: resolver, Mover: mover}}
	return workbookops.New(workbookops.Ports{Mutation: provider}).MoveWorkbook(ctx, input, preview)
}

func runUpdate(ctx context.Context, resolver workbookops.UpdateResolver, updater workbookops.Updater, input workbookops.UpdateInput, preview bool) (workbookops.UpdateOutput, error) {
	provider := workbookMutationProvider{session: workbookops.MutationSession{UpdateResolver: resolver, Updater: updater}}
	return workbookops.New(workbookops.Ports{Mutation: provider}).UpdateWorkbook(ctx, input, preview)
}

func runDelete(ctx context.Context, resolver workbookops.Resolver, deleter workbookops.Deleter, input workbookops.DeleteInput, preview bool) (workbookops.DeleteOutput, error) {
	provider := workbookMutationProvider{session: workbookops.MutationSession{DeleteResolver: resolver, Deleter: deleter}}
	return workbookops.New(workbookops.Ports{Mutation: provider}).DeleteWorkbook(ctx, input, preview)
}
