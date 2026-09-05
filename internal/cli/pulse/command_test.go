package pulse_test

import (
	"context"
	"strings"
	"testing"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitiondelete "github.com/ahillspace/tadx/actions/pulse/definition/delete"
	definitioninspect "github.com/ahillspace/tadx/actions/pulse/definition/inspect"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	metricdelete "github.com/ahillspace/tadx/actions/pulse/metric/delete"
	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	metricfollowers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	metricinspect "github.com/ahillspace/tadx/actions/pulse/metric/inspect"
	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
	metricunfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	"github.com/spf13/cobra"
)

type actions struct {
	definitionListInput     definitionlist.Input
	definitionCreateInput   definitioncreate.Input
	definitionCreatePreview bool
	definitionDeleteInput   definitiondelete.Input
	metricForkInput         metricfork.Input
	metricForkPreview       bool
	metricDeleteInput       metricdelete.Input
	metricFollowInput       metricfollow.Input
	metricFollowPreview     bool
	metricUnfollowInput     metricunfollow.Input
	metricUnfollowPreview   bool
}

func (a *actions) ListPulseDefinitions(_ context.Context, input definitionlist.Input) (definitionlist.Output, error) {
	a.definitionListInput = input
	return definitionlist.Output{}, nil
}

func (*actions) InspectPulseDefinition(context.Context, definitioninspect.Input) (definitioninspect.Output, error) {
	return definitioninspect.Output{}, nil
}

func (*actions) PullPulseDefinition(context.Context, definitionpull.Input) (definitionpull.Output, error) {
	return definitionpull.Output{}, nil
}

func (a *actions) CreatePulseDefinition(_ context.Context, input definitioncreate.Input, preview bool) (definitioncreate.Output, error) {
	a.definitionCreateInput = input
	a.definitionCreatePreview = preview
	return definitioncreate.Output{}, nil
}

func (a *actions) DeletePulseDefinition(_ context.Context, input definitiondelete.Input) (definitiondelete.Output, error) {
	a.definitionDeleteInput = input
	return definitiondelete.Output{}, nil
}

func (*actions) ListPulseMetrics(context.Context, metriclist.Input) (metriclist.Output, error) {
	return metriclist.Output{}, nil
}

func (*actions) InspectPulseMetric(context.Context, metricinspect.Input) (metricinspect.Output, error) {
	return metricinspect.Output{}, nil
}

func (a *actions) ForkPulseMetric(_ context.Context, input metricfork.Input, preview bool) (metricfork.Output, error) {
	a.metricForkInput = input
	a.metricForkPreview = preview
	return metricfork.Output{}, nil
}

func (a *actions) DeletePulseMetric(_ context.Context, input metricdelete.Input) (metricdelete.Output, error) {
	a.metricDeleteInput = input
	return metricdelete.Output{}, nil
}

func (*actions) ListPulseMetricFollowers(context.Context, metricfollowers.Input) (metricfollowers.Output, error) {
	return metricfollowers.Output{}, nil
}

func (a *actions) FollowPulseMetric(_ context.Context, input metricfollow.Input, preview bool) (metricfollow.Output, error) {
	a.metricFollowInput = input
	a.metricFollowPreview = preview
	return metricfollow.Output{}, nil
}

func (a *actions) UnfollowPulseMetric(_ context.Context, input metricunfollow.Input, preview bool) (metricunfollow.Output, error) {
	a.metricUnfollowInput = input
	a.metricUnfollowPreview = preview
	return metricunfollow.Output{}, nil
}

type renderer struct{}

func (renderer) Render(any) error { return nil }

func newCommand(a *actions) *cobra.Command {
	return pulsecli.New(pulsecli.Dependencies{
		DefinitionLister:    a,
		DefinitionInspector: a,
		DefinitionPuller:    a,
		DefinitionCreator:   a,
		DefinitionDeleter:   a,
		MetricLister:        a,
		MetricInspector:     a,
		MetricForker:        a,
		MetricDeleter:       a,
		MetricFollowers:     a,
		MetricFollower:      a,
		MetricUnfollower:    a,
		Renderer:            renderer{},
	})
}

func TestCommandTreeHasExpectedCapabilities(t *testing.T) {
	command := newCommand(&actions{})
	expected := map[string]string{
		"definition/list":    "pulse.definition.list",
		"definition/inspect": "pulse.definition.inspect",
		"definition/pull":    "pulse.definition.pull",
		"definition/create":  "pulse.definition.create",
		"definition/delete":  "pulse.definition.delete",
		"metric/list":        "pulse.metric.list",
		"metric/inspect":     "pulse.metric.inspect",
		"metric/fork":        "pulse.metric.fork",
		"metric/delete":      "pulse.metric.delete",
		"metric/followers":   "pulse.metric.followers",
		"metric/follow":      "pulse.metric.follow",
		"metric/unfollow":    "pulse.metric.unfollow",
	}
	for path, capability := range expected {
		leaf := childAt(t, command, path)
		if got := leaf.Annotations["tadx.capability"]; got != capability {
			t.Fatalf("%s capability = %q, want %q", path, got, capability)
		}
	}
	if !strings.Contains(command.Long, "content datasource schema") {
		t.Fatalf("Pulse help does not point to datasource schema discovery: %q", command.Long)
	}
}

func TestCatalogFlagAppearsOnlyOnEligibleReads(t *testing.T) {
	command := newCommand(&actions{})
	eligible := []string{"definition/list", "definition/inspect", "metric/list", "metric/inspect", "metric/followers"}
	for _, path := range eligible {
		if childAt(t, command, path).Flags().Lookup("catalog") == nil {
			t.Fatalf("%s does not expose --catalog", path)
		}
	}
	other := []string{"definition/pull", "definition/create", "definition/delete", "metric/fork", "metric/delete", "metric/follow", "metric/unfollow"}
	for _, path := range other {
		if childAt(t, command, path).Flags().Lookup("catalog") != nil {
			t.Fatalf("%s unexpectedly exposes --catalog", path)
		}
	}
}

func TestPulseDeletesMapExactTargetAndPreview(t *testing.T) {
	a := &actions{}
	command := newCommand(a)
	command.SetArgs([]string{"definition", "delete", "--environment", "development", "--id", "definition-1", "--preview"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if a.definitionDeleteInput.Environment != "development" || a.definitionDeleteInput.LUID != "definition-1" || !a.definitionDeleteInput.Preview {
		t.Fatalf("definition delete input = %#v", a.definitionDeleteInput)
	}

	command.SetArgs([]string{"metric", "delete", "--environment", "production", "--id", "metric-1"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if a.metricDeleteInput.Environment != "production" || a.metricDeleteInput.LUID != "metric-1" || a.metricDeleteInput.Preview {
		t.Fatalf("metric delete input = %#v", a.metricDeleteInput)
	}
}

func TestDefinitionListMapsCatalogInput(t *testing.T) {
	a := &actions{}
	command := newCommand(a)
	command.SetArgs([]string{"definition", "list", "--environment", "development", "--limit", "12", "--cursor", "next", "--catalog"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := definitionlist.Input{Environment: "development", Limit: 12, Cursor: "next", Catalog: true}
	if a.definitionListInput != want {
		t.Fatalf("list input = %#v, want %#v", a.definitionListInput, want)
	}
}

func TestDefinitionCreateMapsSmallIntentAndPreview(t *testing.T) {
	a := &actions{}
	command := newCommand(a)
	command.SetArgs([]string{
		"definition", "create",
		"--environment", "development",
		"--name", "Revenue",
		"--description", "Recognized revenue",
		"--datasource-id", "datasource-1",
		"--measure-field", "[Revenue]",
		"--aggregation", "SUM",
		"--date-field", "[Order Date]",
		"--dimension", "[Region]",
		"--dimension", "[Segment]",
		"--minimum-granularity", "WEEK",
		"--number-format", "CURRENCY",
		"--currency", "USD",
		"--sentiment", "UP",
		"--temporality", "OVER_TIME",
		"--running-total",
		"--preview",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	input := a.definitionCreateInput
	if input.Environment != "development" || input.Intent.Name != "Revenue" || input.Intent.DatasourceLUID != "datasource-1" || input.Intent.MeasureField != "[Revenue]" || input.Intent.TimeDimension != "[Order Date]" {
		t.Fatalf("definition create input = %#v", input)
	}
	if got := strings.Join(input.Intent.AllowedDimensions, ","); got != "[Region],[Segment]" {
		t.Fatalf("allowed dimensions = %q", got)
	}
	if !input.Intent.RunningTotal || !a.definitionCreatePreview {
		t.Fatalf("running total = %t, preview = %t", input.Intent.RunningTotal, a.definitionCreatePreview)
	}
}

func TestMetricUnfollowMapsEachCompleteSelectorForm(t *testing.T) {
	a := &actions{}
	command := newCommand(a)
	command.SetArgs([]string{"metric", "unfollow", "--environment", "development", "--subscription-id", "sub-1", "--preview"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if a.metricUnfollowInput.Environment != "development" || a.metricUnfollowInput.SubscriptionLUID != "sub-1" || !a.metricUnfollowPreview {
		t.Fatalf("direct unfollow input=%#v preview=%t", a.metricUnfollowInput, a.metricUnfollowPreview)
	}

	a = &actions{}
	command = newCommand(a)
	command.SetArgs([]string{"metric", "unfollow", "--environment", "development", "--id", "metric-1", "--user-id", "user-1"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if a.metricUnfollowInput.MetricLUID != "metric-1" || a.metricUnfollowInput.UserLUID != "user-1" || a.metricUnfollowPreview {
		t.Fatalf("relationship unfollow input=%#v preview=%t", a.metricUnfollowInput, a.metricUnfollowPreview)
	}
}

func TestMetricUnfollowRejectsMixedAndIncompleteSelectorFlags(t *testing.T) {
	tests := [][]string{
		{},
		{"--subscription-id", "sub-1", "--id", "metric-1"},
		{"--subscription-id", "sub-1", "--user-id", "user-1"},
		{"--subscription-id", "sub-1", "--group-id", "group-1"},
		{"--id", "metric-1"},
		{"--user-id", "user-1"},
		{"--group-id", "group-1"},
		{"--id", "metric-1", "--user-id", "user-1", "--group-id", "group-1"},
	}
	for _, flags := range tests {
		a := &actions{}
		command := newCommand(a)
		command.SetArgs(append([]string{"metric", "unfollow", "--environment", "development"}, flags...))
		if err := command.Execute(); err == nil {
			t.Fatalf("invalid selectors accepted: %v", flags)
		}
		if a.metricUnfollowInput != (metricunfollow.Input{}) {
			t.Fatalf("invalid selectors reached action: flags=%v input=%#v", flags, a.metricUnfollowInput)
		}
	}
}

func TestMetricForkGroupsRepeatedFilters(t *testing.T) {
	a := &actions{}
	command := newCommand(a)
	command.SetArgs([]string{
		"metric", "fork",
		"--environment", "development",
		"--id", "metric-1",
		"--period", "LAST_30_DAYS",
		"--filter", "[Region]=West",
		"--filter", "[Region]=East",
		"--exclude-filter", "[Category]=Furniture",
		"--preview",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []metricfork.Filter{
		{Field: "[Category]", Values: []string{"Furniture"}, Exclude: true},
		{Field: "[Region]", Values: []string{"East", "West"}},
	}
	if len(a.metricForkInput.Filters) != len(want) {
		t.Fatalf("filters = %#v", a.metricForkInput.Filters)
	}
	for index := range want {
		got := a.metricForkInput.Filters[index]
		if got.Field != want[index].Field || got.Exclude != want[index].Exclude || strings.Join(got.Values, ",") != strings.Join(want[index].Values, ",") {
			t.Fatalf("filter %d = %#v, want %#v", index, got, want[index])
		}
	}
	if !a.metricForkPreview {
		t.Fatal("fork did not pass --preview")
	}
}

func TestMetricForkRejectsConflictingFilterModes(t *testing.T) {
	command := newCommand(&actions{})
	command.SetArgs([]string{
		"metric", "fork",
		"--environment", "development",
		"--id", "metric-1",
		"--filter", "[Region]=West",
		"--exclude-filter", "[Region]=East",
	})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "both included and excluded") {
		t.Fatalf("error = %v", err)
	}
}

func TestMutationRequiresExplicitEnvironment(t *testing.T) {
	command := newCommand(&actions{})
	command.SetArgs([]string{"metric", "follow", "--id", "metric-1", "--user-id", "user-1"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--environment") {
		t.Fatalf("error = %v", err)
	}
}

func childAt(t *testing.T, command *cobra.Command, path string) *cobra.Command {
	t.Helper()
	current := command
	for _, name := range strings.Split(path, "/") {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			t.Fatalf("command %q not found", path)
		}
		current = next
	}
	return current
}
