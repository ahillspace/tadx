package datasource_test

import (
	"context"
	"testing"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
)

type datasourceMutationProvider struct {
	session                       datasourceops.MutationSession
	opens                         *int
	targetEnvironment, targetSite string
}

func (p datasourceMutationProvider) OpenDatasourceMutation(_ context.Context, environment, site, _ string) (datasourceops.MutationSession, error) {
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

func TestMutationServiceUsesCanonicalDatasourceTarget(t *testing.T) {
	resolver := moveResolver{item: datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "source"}, project: datasourceops.Project{LUID: "destination"}}
	provider := datasourceMutationProvider{session: datasourceops.MutationSession{MoveResolver: resolver, Mover: &moveMover{}}, targetEnvironment: "canonical", targetSite: "canonical-site"}
	output, err := datasourceops.New(datasourceops.Ports{Mutation: provider}).MoveDatasource(t.Context(), datasourceops.MoveInput{Environment: "alias", Site: "caller-site", DatasourceSelector: identity.Selector{LUID: "ds-1"}, ProjectSelector: identity.Selector{LUID: "destination"}}, true)
	if err != nil || output.Plan.Environment != "canonical" || output.Plan.Site != "canonical-site" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
}

func TestMutationServiceRejectsInvalidDatasourceInputBeforeOpen(t *testing.T) {
	opens := 0
	service := datasourceops.New(datasourceops.Ports{Mutation: datasourceMutationProvider{opens: &opens}})
	if _, err := service.MoveDatasource(t.Context(), datasourceops.MoveInput{Environment: "dev", DatasourceSelector: identity.Selector{LUID: "ds-1"}}, false); err == nil {
		t.Fatal("move accepted missing destination")
	}
	if _, err := service.UpdateDatasource(t.Context(), datasourceops.UpdateInput{Environment: "dev", Selector: identity.Selector{LUID: "ds-1"}}, false); err == nil {
		t.Fatal("update accepted no changes")
	}
	if _, err := service.DeleteDatasource(t.Context(), datasourceops.DeleteInput{Environment: "dev"}, false); err == nil {
		t.Fatal("delete accepted no selector")
	}
	if opens != 0 {
		t.Fatalf("invalid inputs opened provider %d times", opens)
	}
}

func runMove(ctx context.Context, resolver datasourceops.MoveResolver, mover datasourceops.Mover, input datasourceops.MoveInput, preview bool) (datasourceops.MoveOutput, error) {
	provider := datasourceMutationProvider{session: datasourceops.MutationSession{MoveResolver: resolver, Mover: mover}}
	return datasourceops.New(datasourceops.Ports{Mutation: provider}).MoveDatasource(ctx, input, preview)
}

func runUpdate(ctx context.Context, resolver datasourceops.UpdateResolver, updater datasourceops.Updater, input datasourceops.UpdateInput, preview bool) (datasourceops.UpdateOutput, error) {
	provider := datasourceMutationProvider{session: datasourceops.MutationSession{UpdateResolver: resolver, Updater: updater}}
	return datasourceops.New(datasourceops.Ports{Mutation: provider}).UpdateDatasource(ctx, input, preview)
}

func runDelete(ctx context.Context, resolver datasourceops.Resolver, deleter datasourceops.Deleter, input datasourceops.DeleteInput, preview bool) (datasourceops.DeleteOutput, error) {
	provider := datasourceMutationProvider{session: datasourceops.MutationSession{DeleteResolver: resolver, Deleter: deleter}}
	return datasourceops.New(datasourceops.Ports{Mutation: provider}).DeleteDatasource(ctx, input, preview)
}
