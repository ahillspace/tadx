package pulse_test

import (
	"context"
	"strings"
	"testing"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitionget "github.com/ahillspace/tadx/actions/pulse/definition/get"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	metricfollowers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	metricget "github.com/ahillspace/tadx/actions/pulse/metric/get"
	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
	metricunfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	"github.com/spf13/cobra"
)

type actions struct {
	definitionListInput   definitionlist.Input
	definitionCreateInput definitioncreate.Input
	definitionCreateApply bool
	metricForkInput       metricfork.Input
	metricForkApply       bool
	metricFollowInput     metricfollow.Input
	metricFollowApply     bool
}

func (a *actions) ListPulseDefinitions(_ context.Context, input definitionlist.Input) (definitionlist.Output, error) {
	a.definitionListInput = input
	return definitionlist.Output{}, nil
}

func (*actions) GetPulseDefinition(context.Context, definitionget.Input) (definitionget.Output, error) {
	return definitionget.Output{}, nil
}

func (*actions) PullPulseDefinition(context.Context, definitionpull.Input) (definitionpull.Output, error) {
	return definitionpull.Output{}, nil
}

func (a *actions) CreatePulseDefinition(_ context.Context, input definitioncreate.Input, apply bool) (definitioncreate.Output, error) {
	a.definitionCreateInput = input
	a.definitionCreateApply = apply
	return definitioncreate.Output{}, nil
}

func (*actions) ListPulseMetrics(context.Context, metriclist.Input) (metriclist.Output, error) {
	return metriclist.Output{}, nil
}

func (*actions) GetPulseMetric(context.Context, metricget.Input) (metricget.Output, error) {
	return metricget.Output{}, nil
}

func (a *actions) ForkPulseMetric(_ context.Context, input metricfork.Input, apply bool) (metricfork.Output, error) {
	a.metricForkInput = input
	a.metricForkApply = apply
	return metricfork.Output{}, nil
}

func (*actions) ListPulseMetricFollowers(context.Context, metricfollowers.Input) (metricfollowers.Output, error) {
	return metricfollowers.Output{}, nil
}

func (a *actions) FollowPulseMetric(_ context.Context, input metricfollow.Input, apply bool) (metricfollow.Output, error) {
	a.metricFollowInput = input
	a.metricFollowApply = apply
	return metricfollow.Output{}, nil
}

func (*actions) UnfollowPulseMetric(context.Context, metricunfollow.Input, bool) (metricunfollow.Output, error) {
	return metricunfollow.Output{}, nil
}

type renderer struct{}

func (renderer) Render(any) error { return nil }

func newCommand(a *actions) *cobra.Command {
	return pulsecli.New(pulsecli.Dependencies{
		DefinitionLister:  a,
		DefinitionGetter:  a,
		DefinitionPuller:  a,
		DefinitionCreator: a,
		MetricLister:      a,
		MetricGetter:      a,
		MetricForker:      a,
		MetricFollowers:   a,
		MetricFollower:    a,
		MetricUnfollower:  a,
		Renderer:          renderer{},
	})
}

func TestCommandTreeHasExpectedCapabilities(t *testing.T) {
	command := newCommand(&actions{})
	expected := map[string]string{
		"definition/list":   "pulse.definition.list",
		"definition/get":    "pulse.definition.get",
		"definition/pull":   "pulse.definition.pull",
		"definition/create": "pulse.definition.create",
		"metric/list":       "pulse.metric.list",
		"metric/get":        "pulse.metric.get",
		"metric/fork":       "pulse.metric.fork",
		"metric/followers":  "pulse.metric.followers",
		"metric/follow":     "pulse.metric.follow",
		"metric/unfollow":   "pulse.metric.unfollow",
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
	eligible := []string{"definition/list", "definition/get", "metric/list", "metric/get", "metric/followers"}
	for _, path := range eligible {
		if childAt(t, command, path).Flags().Lookup("catalog") == nil {
			t.Fatalf("%s does not expose --catalog", path)
		}
	}
	other := []string{"definition/pull", "definition/create", "metric/fork", "metric/follow", "metric/unfollow"}
	for _, path := range other {
		if childAt(t, command, path).Flags().Lookup("catalog") != nil {
			t.Fatalf("%s unexpectedly exposes --catalog", path)
		}
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

func TestDefinitionCreateMapsSmallIntentAndApply(t *testing.T) {
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
		"--apply",
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
	if !input.Intent.RunningTotal || !a.definitionCreateApply {
		t.Fatalf("running total = %t, apply = %t", input.Intent.RunningTotal, a.definitionCreateApply)
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
		"--apply",
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
	if !a.metricForkApply {
		t.Fatal("fork did not pass --apply")
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
