package flow_test

import (
	"context"
	"testing"

	flowops "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
)

type flowMutationProvider struct {
	session                       flowops.MutationSession
	opens                         *int
	targetEnvironment, targetSite string
}

func (p flowMutationProvider) OpenFlowMutation(_ context.Context, environment, site, _ string) (flowops.MutationSession, error) {
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

func TestMutationServiceUsesCanonicalFlowTarget(t *testing.T) {
	resolver := &moveResolver{flow: flowops.Record{LUID: "flow-1", Name: "Daily", ProjectLUID: "source"}, project: flowops.Project{LUID: "destination"}}
	provider := flowMutationProvider{session: flowops.MutationSession{MoveResolver: resolver, Mover: &moveMover{}}, targetEnvironment: "canonical", targetSite: "canonical-site"}
	output, err := flowops.New(provider).MoveFlow(t.Context(), flowops.MoveInput{Environment: "alias", Site: "caller-site", FlowSelector: identity.Selector{LUID: "flow-1"}, ProjectSelector: identity.Selector{LUID: "destination"}}, true)
	if err != nil || output.Plan.Environment != "canonical" || output.Plan.Site != "canonical-site" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
}

func TestMutationServiceRejectsInvalidFlowInputBeforeOpen(t *testing.T) {
	opens := 0
	service := flowops.New(flowMutationProvider{opens: &opens})
	if _, err := service.MoveFlow(t.Context(), flowops.MoveInput{Environment: "dev", FlowSelector: identity.Selector{LUID: "flow-1"}}, false); err == nil {
		t.Fatal("move accepted missing destination")
	}
	if _, err := service.UpdateFlow(t.Context(), flowops.UpdateInput{Environment: "dev", Selector: identity.Selector{LUID: "flow-1"}}, false); err == nil {
		t.Fatal("update accepted missing owner")
	}
	if _, err := service.DeleteFlow(t.Context(), flowops.DeleteInput{Environment: "dev"}, false); err == nil {
		t.Fatal("delete accepted no selector")
	}
	if opens != 0 {
		t.Fatalf("invalid inputs opened provider %d times", opens)
	}
}

func runMove(ctx context.Context, resolver flowops.MoveResolver, mover flowops.Mover, input flowops.MoveInput, preview bool) (flowops.MoveOutput, error) {
	provider := flowMutationProvider{session: flowops.MutationSession{MoveResolver: resolver, Mover: mover}}
	return flowops.New(provider).MoveFlow(ctx, input, preview)
}

func runUpdate(ctx context.Context, resolver flowops.Resolver, updater flowops.Updater, input flowops.UpdateInput, preview bool) (flowops.UpdateOutput, error) {
	provider := flowMutationProvider{session: flowops.MutationSession{UpdateResolver: resolver, Updater: updater}}
	return flowops.New(provider).UpdateFlow(ctx, input, preview)
}

func runDelete(ctx context.Context, resolver flowops.Resolver, deleter flowops.Deleter, input flowops.DeleteInput, preview bool) (flowops.DeleteOutput, error) {
	provider := flowMutationProvider{session: flowops.MutationSession{DeleteResolver: resolver, Deleter: deleter}}
	return flowops.New(provider).DeleteFlow(ctx, input, preview)
}
