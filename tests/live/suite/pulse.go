package suite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

const pulseBase = "/api/-/pulse"

// PulseCases observes saved configuration only. Numerical metric values and
// generated insights remain outside the CLI lifecycle contract.
func PulseCases() map[string]harness.Scenario {
	cases := map[string]harness.Scenario{}
	for _, verb := range []string{"list", "inspect", "create", "delete", "pull", "publish"} {
		cases["live.pulse-definition."+verb] = pulseDefinition(verb)
	}
	for _, verb := range []string{"list", "inspect", "fork", "delete", "followers", "follow", "follow-group", "unfollow"} {
		cases["live.pulse-metric."+verb] = pulseMetric(verb)
	}
	cases["live.pulse-subscription.list"] = pulseSubscriptions
	return cases
}

func pulseDefinition(verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		b, e := c.Require("pulse_datasource")
		if e != nil {
			return e
		}
		if verb == "create" {
			for _, key := range []string{"measure_field", "date_field", "dimension_field"} {
				if b.Params[key] == "" {
					return harness.Blocked("pulse_datasource requires " + key)
				}
			}
			name := c.NamePrefix + "-definition"
			args := []string{"pulse", "definition", "create", "--name", name, "--datasource-id", b.ID, "--measure-field", b.Params["measure_field"], "--date-field", b.Params["date_field"], "--dimension", b.Params["dimension_field"], "--aggregation", "SUM", "--minimum-granularity", "DAY"}
			if e = adminCreate(ctx, c, "pulseDefinition", args, name, nil); e != nil {
				return e
			}
			o, e := named(ctx, c, "pulseDefinition", name)
			if e != nil {
				return e
			}
			d, e := pulseGet(ctx, c, "definitions", o.ID)
			if e != nil {
				return e
			}
			basic := asObject(asObject(d["specification"])["basic_specification"])
			if e = subset(basic, map[string]any{"measure": map[string]any{"field": b.Params["measure_field"], "aggregation": "AGGREGATION_SUM"}, "time_dimension": map[string]any{"field": b.Params["date_field"]}}); e != nil {
				return e
			}
			if e = field(asObject(asObject(d["specification"])["datasource"]), "id", b.ID); e != nil {
				return e
			}
			dimensions := array(asObject(d["extension_options"])["allowed_dimensions"])
			if len(dimensions) != 1 || dimensions[0] != b.Params["dimension_field"] {
				return fmt.Errorf("definition dimensions did not persist")
			}
			_, e = pulseDefault(ctx, c, o.ID)
			return e
		}
		def, e := c.Seed(ctx, "pulseDefinition", "definition")
		if e != nil {
			return e
		}
		before, e := pulseGet(ctx, c, "definitions", def.ID)
		if e != nil {
			return e
		}
		defaultMetric, e := pulseDefault(ctx, c, def.ID)
		if e != nil {
			return e
		}
		switch verb {
		case "list":
			out, e := fullRun(ctx, c, "pulse", "definition", "list", "--name", def.Name, "--all")
			if e != nil {
				return e
			}
			rows := array(out["definitions"])
			if len(rows) != 1 {
				return fmt.Errorf("exact-name Pulse definition list requires one result")
			}
			row, e := record(rows, def.ID)
			if e != nil {
				return e
			}
			if e = field(row, "datasource_luid", b.ID); e != nil {
				return e
			}
			return field(row, "name", def.Name)
		case "inspect":
			out, e := fullRun(ctx, c, "pulse", "definition", "inspect", "--id", def.ID)
			if e != nil {
				return e
			}
			row, e := record(out, def.ID)
			if e != nil {
				return e
			}
			return pulseDefinitionProjection(row, before)
		case "delete":
			sentinel, e := c.Seed(ctx, "pulseDefinition", "sentinel")
			if e != nil {
				return e
			}
			if _, e = fullRun(ctx, c, "pulse", "definition", "delete", "--id", def.ID); e != nil {
				return e
			}
			if _, e = pulseGet(ctx, c, "definitions", def.ID); !harness.IsNotFound(e) {
				return fmt.Errorf("definition deletion lacks authoritative 404: %w", e)
			}
			_, e = pulseGet(ctx, c, "definitions", sentinel.ID)
			return e
		case "pull", "publish":
			if b.Params["dimension_field"] == "" || b.Params["category"] == "" {
				return harness.Blocked("Pulse portable fixture requires dimension_field and category")
			}
			spec := cloneObject(pulseSpecification(defaultMetric))
			spec["measurement_period"] = pulsePeriod(7)
			spec["filters"] = []any{pulseFilter(b.Params["dimension_field"], b.Params["category"], false)}
			if _, e = pulseSeedMetric(ctx, c, def.ID, spec, "included"); e != nil {
				return e
			}
			excluded := cloneObject(spec)
			excluded["measurement_period"] = pulsePeriod(14)
			excluded["filters"] = []any{pulseFilter(b.Params["dimension_field"], b.Params["category"], true)}
			if _, e = pulseSeedMetric(ctx, c, def.ID, excluded, "excluded"); e != nil {
				return e
			}
			before, e = pulseGet(ctx, c, "definitions", def.ID)
			if e != nil {
				return e
			}
			follower, e := c.Require("user_a")
			if e != nil {
				return e
			}
			if _, e = pulseSeedSubscription(ctx, c, pulseID(defaultMetric), "USER", follower.ID, "source"); e != nil {
				return e
			}
			sourceFollowers, e := pulseFollowers(ctx, c, pulseID(defaultMetric))
			if e != nil {
				return e
			}
			metricsBefore, e := pulseMetrics(ctx, c, def.ID)
			if e != nil {
				return e
			}
			out, e := fullRun(ctx, c, "pulse", "definition", "pull", "--id", def.ID, "--workspace", c.Workspace)
			if e != nil {
				return e
			}
			relative, _ := asObject(out["artifact"])["path"].(string)
			directory, e := safeArtifactPath(c.WorkspacePath, relative)
			if e != nil {
				return e
			}
			bundleBytes, e := os.ReadFile(filepath.Join(directory, "bundle.json"))
			if e != nil {
				return e
			}
			var bundle map[string]any
			if e = json.Unmarshal(bundleBytes, &bundle); e != nil {
				return e
			}
			if e = verifyPulseBundle(bundle, before, metricsBefore, def.ID, b.ID); e != nil {
				return e
			}
			if e = pulseUnchanged(ctx, c, def.ID, before, metricsBefore); e != nil {
				return e
			}
			if verb == "pull" {
				return nil
			}
			destination := b.ID
			if target, ok := c.Profile.Bindings["pulse_destination_datasource"]; ok {
				destination = target.ID
			}
			if destination == "" {
				return harness.Blocked("destination datasource binding lacks authoritative ID")
			}
			// Publish deliberately creates a second definition with the same owned name.
			// Register that exact creation before submission; identities distinguish both.
			if e = c.Intent("pulseDefinition", def.Name, ""); e != nil {
				return e
			}
			published, runErr := fullRun(ctx, c, "pulse", "definition", "publish", "--workspace", c.Workspace, "--artifact", relative, "--datasource-map", b.ID+"="+destination)
			mappings := array(published["mappings"])
			newDefinition := ""
			for _, v := range mappings {
				m := asObject(v)
				if m["kind"] == "definition" {
					newDefinition, _ = m["destination_luid"].(string)
					if newDefinition != "" && newDefinition != def.ID {
						o := harness.Object{Kind: "pulseDefinition", ID: newDefinition, Name: def.Name}
						if e = c.Track(o); e != nil {
							return e
						}
					}
				}
			}
			if runErr != nil {
				return runErr
			}
			if len(mappings) != len(metricsBefore)+1 {
				return fmt.Errorf("publish identity mapping omitted bundled objects")
			}
			seenMappings := map[string]bool{}
			for _, v := range mappings {
				m := asObject(v)
				source, _ := m["source_luid"].(string)
				if source == "" || seenMappings[source] {
					return fmt.Errorf("publish mapping omitted or duplicated source identity")
				}
				seenMappings[source] = true
			}
			if newDefinition == "" || newDefinition == def.ID {
				return fmt.Errorf("Pulse publish did not create a distinct definition")
			}
			saved, e := pulseGet(ctx, c, "definitions", newDefinition)
			if e != nil {
				return e
			}
			expected := cloneObject(before)
			asObject(asObject(expected["specification"])["datasource"])["id"] = destination
			for _, key := range pulseBusinessSections {
				want, exists := expected[key]
				if key == "name" || key == "description" {
					want = asObject(before["metadata"])[key]
					exists = want != nil
					if e = field(asObject(saved["metadata"]), key, want); e != nil {
						return e
					}
					continue
				}
				if exists {
					if e = field(saved, key, want); e != nil {
						return e
					}
				}
			}
			for _, v := range mappings {
				m := asObject(v)
				if m["kind"] != "metric" {
					continue
				}
				source, _ := m["source_luid"].(string)
				id, _ := m["destination_luid"].(string)
				if id == "" || id == source {
					return fmt.Errorf("metric identity mapping is not a new object")
				}
				old, e := pulseRecord(metricsBefore, source)
				if e != nil {
					return e
				}
				fresh, e := pulseGet(ctx, c, "metrics", id)
				if e != nil {
					return e
				}
				expectedSpec := cloneObject(pulseSpecification(old))
				if ds, ok := expectedSpec["datasource"].(map[string]any); ok {
					ds["id"] = destination
				}
				if !pulseSpecificationsEqual(expectedSpec, pulseSpecification(fresh)) {
					return fmt.Errorf("published metric lost saved specification")
				}
				if old["is_default"] == true && fresh["is_default"] != true {
					return fmt.Errorf("published default metric marker changed")
				}
				if e = pulseOwnership(fresh, newDefinition); e != nil {
					return e
				}
				followers, e := pulseFollowers(ctx, c, id)
				if e != nil {
					return e
				}
				if len(followers) != 0 {
					return fmt.Errorf("publish recreated source subscriptions")
				}
			}
			freshBytes, e := os.ReadFile(filepath.Join(directory, "bundle.json"))
			if e != nil {
				return e
			}
			if string(freshBytes) != string(bundleBytes) {
				return fmt.Errorf("publish modified source bundle bytes")
			}
			currentFollowers, e := pulseFollowers(ctx, c, pulseID(defaultMetric))
			if e != nil {
				return e
			}
			if e = subset(currentFollowers, sourceFollowers); e != nil {
				return fmt.Errorf("source followers changed: %w", e)
			}
			return pulseUnchanged(ctx, c, def.ID, before, metricsBefore)
		}
		return fmt.Errorf("unknown Pulse definition operation %s", verb)
	}
}

var pulseBusinessSections = []string{"name", "description", "specification", "extension_options", "representation_options", "insights_options", "comparisons", "datasource_goals", "related_links", "certification"}

func pulseDefinitionProjection(row, saved map[string]any) error {
	spec := asObject(saved["specification"])
	basic := asObject(spec["basic_specification"])
	extension := asObject(saved["extension_options"])
	for cli, want := range map[string]any{"name": asObject(saved["metadata"])["name"], "datasource_luid": asObject(spec["datasource"])["id"], "measure_field": asObject(basic["measure"])["field"], "aggregation": asObject(basic["measure"])["aggregation"], "time_dimension": asObject(basic["time_dimension"])["field"], "allowed_dimensions": extension["allowed_dimensions"], "allowed_granularities": extension["allowed_granularities"]} {
		if e := field(row, cli, want); e != nil {
			return e
		}
	}
	configuration := asObject(row["configuration"])
	for _, key := range pulseBusinessSections {
		if want, ok := saved[key]; ok {
			if e := field(configuration, key, want); e != nil {
				return e
			}
		}
	}
	return nil
}

func pulseMetric(verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		def, e := c.Seed(ctx, "pulseDefinition", "definition")
		if e != nil {
			return e
		}
		baseline, e := pulseDefault(ctx, c, def.ID)
		if e != nil {
			return e
		}
		spec := cloneObject(pulseSpecification(baseline))
		spec["measurement_period"] = pulsePeriod(7)
		metric, e := pulseSeedMetric(ctx, c, def.ID, spec, "subject")
		if e != nil {
			return e
		}
		id := pulseID(metric)
		before := cloneObject(metric)
		switch verb {
		case "list":
			expected, e := pulseMetrics(ctx, c, def.ID)
			if e != nil {
				return e
			}
			out, e := fullRun(ctx, c, "pulse", "metric", "list", "--definition-id", def.ID, "--all")
			if e != nil {
				return e
			}
			want := make([]string, 0, len(expected))
			for _, v := range expected {
				want = append(want, pulseID(asObject(v)))
			}
			if e = equalIDs(array(out["metrics"]), want); e != nil {
				return e
			}
			for _, v := range array(out["metrics"]) {
				if e = field(asObject(v), "definition_luid", def.ID); e != nil {
					return e
				}
			}
			return nil
		case "inspect":
			out, e := fullRun(ctx, c, "pulse", "metric", "inspect", "--id", id)
			if e != nil {
				return e
			}
			row, e := record(out, id)
			if e != nil {
				return e
			}
			if e = field(row, "specification", pulseSpecification(before)); e != nil {
				return e
			}
			return field(row, "definition_luid", def.ID)
		case "delete":
			if _, e = fullRun(ctx, c, "pulse", "metric", "delete", "--id", id); e != nil {
				return e
			}
			if _, e = pulseGet(ctx, c, "metrics", id); !harness.IsNotFound(e) {
				return fmt.Errorf("metric deletion lacks authoritative 404: %w", e)
			}
			if _, e = pulseGet(ctx, c, "definitions", def.ID); e != nil {
				return e
			}
			_, e = pulseGet(ctx, c, "metrics", pulseID(baseline))
			return e
		case "fork":
			binding, e := c.Require("pulse_datasource")
			if e != nil {
				return e
			}
			dimension, category := binding.Params["dimension_field"], binding.Params["category"]
			if dimension == "" || category == "" {
				return harness.Blocked("Pulse fork requires dimension_field and category")
			}
			if e = c.Intent("pulseMetric", c.NamePrefix+"-fork", def.ID); e != nil {
				return e
			}
			out, e := fullRun(ctx, c, "pulse", "metric", "fork", "--id", id, "--period", "LAST_30_DAYS", "--filter", dimension+"="+category)
			if e != nil {
				return e
			}
			row, e := recordField(out, "created", true)
			if e != nil {
				return fmt.Errorf("fresh definition must create a distinct variant: %w", e)
			}
			forkID, _ := row["metric_luid"].(string)
			if forkID == "" || forkID == id {
				return fmt.Errorf("fork returned invalid metric identity")
			}
			if e = c.Track(harness.Object{Kind: "pulseMetric", ID: forkID, Name: c.NamePrefix + "-fork", ProjectID: def.ID}); e != nil {
				return e
			}
			saved, e := pulseGet(ctx, c, "metrics", forkID)
			if e != nil {
				return e
			}
			want := cloneObject(pulseSpecification(before))
			want["measurement_period"] = pulsePeriod(30)
			want["filters"] = []any{pulseFilter(dimension, category, false)}
			if !pulseSpecificationsEqual(pulseSpecification(saved), want) {
				return fmt.Errorf("fork saved specification differs from requested period and filter")
			}
			if e = pulseOwnership(saved, def.ID); e != nil {
				return e
			}
			fresh, e := pulseGet(ctx, c, "metrics", id)
			if e != nil {
				return e
			}
			if !jsonEqual(fresh, before) {
				return fmt.Errorf("fork changed source metric")
			}
			return nil
		case "followers", "follow", "follow-group", "unfollow":
			return pulseFollowerCase(ctx, c, verb, def.ID, id)
		}
		return fmt.Errorf("unknown Pulse metric operation %s", verb)
	}
}

func pulseFollowerCase(ctx context.Context, c *harness.Case, verb, definition, metric string) error {
	a, e := c.Require("user_a")
	if e != nil {
		return e
	}
	b, e := c.Require("user_b")
	if e != nil {
		return e
	}
	if a.ID == b.ID {
		return fmt.Errorf("follower fixture users must differ")
	}
	sentinel, e := pulseSeedSubscription(ctx, c, metric, "USER", a.ID, "sentinel")
	if e != nil {
		return e
	}
	target := ""
	followerType := "USER"
	followerID := b.ID
	if verb == "follow-group" {
		group, e := c.Seed(ctx, "group", "followers")
		if e != nil {
			return e
		}
		followerType = "GROUP"
		followerID = group.ID
	}
	if verb == "followers" || verb == "unfollow" {
		target, e = pulseSeedSubscription(ctx, c, metric, "USER", b.ID, "subject")
		if e != nil {
			return e
		}
	}
	before, e := pulseFollowers(ctx, c, metric)
	if e != nil {
		return e
	}
	args := []string{"pulse", "metric", verb, "--id", metric}
	switch verb {
	case "unfollow":
		args = []string{"pulse", "metric", "unfollow", "--subscription-id", target}
	case "follow", "follow-group":
		args = []string{"pulse", "metric", "follow", "--id", metric}
		flag := "--user-id"
		if followerType == "GROUP" {
			flag = "--group-id"
		}
		args = append(args, flag, followerID)
		if e = c.Intent("pulseSubscription", c.NamePrefix+"-follow", metric); e != nil {
			return e
		}
	}
	out, runErr := fullRun(ctx, c, args...)
	after, e := pulseFollowers(ctx, c, metric)
	if e != nil {
		return e
	}
	if verb == "follow" || verb == "follow-group" {
		matches := matchingFollowers(after, followerType, followerID)
		if len(matches) == 1 {
			target = subscriptionID(matches[0])
			if e = c.Track(harness.Object{Kind: "pulseSubscription", ID: target, Name: c.NamePrefix + "-follow", ProjectID: metric}); e != nil {
				return e
			}
		}
		if runErr != nil {
			return runErr
		}
		if len(matches) != 1 {
			return fmt.Errorf("follow must persist one exact follower relationship")
		}
		if _, e = record(out, target); e != nil {
			return e
		}
	} else if runErr != nil {
		return runErr
	}
	if !hasSubscription(after, sentinel) {
		return fmt.Errorf("Pulse operation removed sentinel follower")
	}
	if verb == "unfollow" && hasSubscription(after, target) {
		return fmt.Errorf("unfollow left selected subscription")
	}
	if verb == "followers" {
		rows := array(out["subscriptions"])
		if rows == nil {
			rows = array(out["followers"])
		}
		want := make([]string, 0, len(after))
		for _, v := range after {
			want = append(want, subscriptionID(asObject(v)))
		}
		if e = equalIDs(rows, want); e != nil {
			return e
		}
		for _, v := range after {
			raw := asObject(v)
			row, e := record(rows, subscriptionID(raw))
			if e != nil {
				return e
			}
			typ, principal := followerIdentity(raw)
			if e = field(row, "follower_type", typ); e != nil {
				return e
			}
			if e = field(row, "follower_luid", principal); e != nil {
				return e
			}
		}
	}
	if verb == "follow" || verb == "follow-group" {
		for _, v := range before {
			old := asObject(v)
			row, e := record(after, subscriptionID(old))
			if e != nil {
				return e
			}
			if !jsonEqual(row, old) {
				return fmt.Errorf("follow changed existing follower")
			}
		}
	}
	if _, e = pulseGet(ctx, c, "metrics", metric); e != nil {
		return e
	}
	_, e = pulseGet(ctx, c, "definitions", definition)
	return e
}

func pulseSubscriptions(ctx context.Context, c *harness.Case) error {
	if c.ActorUserID == "" {
		return harness.Blocked("subscription list requires independently authenticated actor user identity")
	}
	def, e := c.Seed(ctx, "pulseDefinition", "subscriptions")
	if e != nil {
		return e
	}
	metric, e := pulseDefault(ctx, c, def.ID)
	if e != nil {
		return e
	}
	id := pulseID(metric)
	direct, e := pulseSeedSubscription(ctx, c, id, "USER", c.ActorUserID, "direct")
	if e != nil {
		return e
	}
	other, e := c.Require("user_b")
	if e != nil {
		return e
	}
	if other.ID == c.ActorUserID {
		return fmt.Errorf("subscription contrasting user must differ from authenticated user")
	}
	foreign, e := pulseSeedSubscription(ctx, c, id, "USER", other.ID, "foreign")
	if e != nil {
		return e
	}
	independent, e := pulsePages(ctx, c, "subscriptions", url.Values{"user_id": {c.ActorUserID}}, "subscriptions")
	if e != nil {
		return e
	}
	out, e := fullRun(ctx, c, "pulse", "subscription", "list", "--all")
	if e != nil {
		return e
	}
	rows := array(out["subscriptions"])
	if _, e = record(rows, direct); e != nil {
		return e
	}
	if _, e = record(rows, foreign); e == nil {
		return fmt.Errorf("user subscription list includes contrasting user's direct subscription")
	}
	want := make([]string, 0, len(independent))
	for _, v := range independent {
		want = append(want, subscriptionID(asObject(v)))
	}
	got := make([]string, 0, len(rows))
	for _, v := range rows {
		row := asObject(v)
		subID, _ := row["subscription_luid"].(string)
		got = append(got, subID)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("authenticated subscription inventory differs from REST")
	}
	row, e := record(rows, direct)
	if e != nil {
		return e
	}
	if e = field(row, "definition_luid", def.ID); e != nil {
		return e
	}
	if e = field(row, "specification", pulseSpecification(metric)); e != nil {
		return e
	}
	coverage := asObject(out["coverage"])
	if _, ok := coverage["group_derived"]; !ok {
		return fmt.Errorf("subscription coverage omitted group-derived scope")
	}
	return nil
}

func pulseGet(ctx context.Context, c *harness.Case, collection, id string) (map[string]any, error) {
	doc, e := c.API.JSON(ctx, http.MethodGet, pulseBase+"/"+collection+"/"+url.PathEscape(id), nil)
	if e != nil {
		return nil, e
	}
	singular := strings.TrimSuffix(collection, "s")
	if nested, ok := doc[singular].(map[string]any); ok {
		doc = nested
	}
	if pulseID(doc) != id {
		return nil, fmt.Errorf("independent Pulse read returned mismatched identity")
	}
	return doc, nil
}
func pulseID(row map[string]any) string {
	if id, ok := row["id"].(string); ok && id != "" {
		return id
	}
	id, _ := asObject(row["metadata"])["id"].(string)
	return id
}
func pulseSpecification(row map[string]any) map[string]any {
	if spec, ok := row["specification"].(map[string]any); ok {
		return spec
	}
	return asObject(row["metric_specification"])
}
func pulseOwnership(row map[string]any, definition string) error {
	id, _ := row["definition_id"].(string)
	if id == "" {
		id, _ = asObject(row["specification_reference"])["definition_id"].(string)
	}
	if id == "" {
		id, _ = asObject(asObject(row["definition"])["metadata"])["id"].(string)
	}
	if id != definition {
		return fmt.Errorf("Pulse metric belongs to wrong definition")
	}
	return nil
}
func pulseMetrics(ctx context.Context, c *harness.Case, definition string) ([]any, error) {
	return pulsePages(ctx, c, "definitions/"+url.PathEscape(definition)+"/metrics", nil, "metrics")
}
func pulsePages(ctx context.Context, c *harness.Case, path string, q url.Values, key string) ([]any, error) {
	if q == nil {
		q = url.Values{}
	}
	if !(path == "subscriptions" && q.Get("metric_id") != "") {
		q.Set("page_size", "100")
	}
	var rows []any
	seen := map[string]bool{}
	for range 100 {
		doc, e := c.API.JSON(ctx, http.MethodGet, pulseBase+"/"+path+"?"+q.Encode(), nil)
		if e != nil {
			return nil, e
		}
		page, ok := doc[key].([]any)
		if !ok {
			return nil, fmt.Errorf("independent Pulse collection omitted explicit %s array", key)
		}
		for _, row := range page {
			id := pulseID(asObject(row))
			if key == "subscriptions" {
				id = subscriptionID(asObject(row))
			}
			if id == "" || seen[id] {
				return nil, fmt.Errorf("independent Pulse collection omitted or duplicated identity")
			}
			seen[id] = true
			rows = append(rows, row)
		}
		next, _ := doc["next_page_token"].(string)
		if next == "" {
			return rows, nil
		}
		if next == q.Get("page_token") {
			return nil, fmt.Errorf("independent Pulse page did not advance")
		}
		q.Set("page_token", next)
	}
	return nil, fmt.Errorf("Pulse observation exceeded 100-page bound")
}
func pulseDefault(ctx context.Context, c *harness.Case, definition string) (map[string]any, error) {
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	observe, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	for {
		rows, e := pulseMetrics(observe, c, definition)
		if e != nil {
			return nil, e
		}
		for _, v := range rows {
			row := asObject(v)
			if row["is_default"] == true {
				return row, nil
			}
		}
		if len(rows) == 1 {
			row := asObject(rows[0])
			if _, known := row["is_default"]; !known {
				return row, nil
			}
		}
		select {
		case <-observe.Done():
			return nil, harness.Inconclusive("default metric did not appear within observation budget")
		case <-time.After(time.Second):
		}
	}
}
func pulsePeriod(days int) map[string]any {
	return map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG", "last_x_period": map[string]any{"period": days, "period_type": "GRANULARITY_BY_DAY", "include_current_period": true}}
}
func pulseFilter(dimension, category string, exclude bool) map[string]any {
	operator := "OPERATOR_EQUAL"
	if exclude {
		operator = "OPERATOR_NOT_EQUAL"
	}
	return map[string]any{"field": dimension, "operator": operator, "categorical_values": []any{map[string]any{"string_value": category}}, "include_null": false}
}
func cloneObject(row map[string]any) map[string]any {
	data, _ := json.Marshal(row)
	var out map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}
func pulseSeedMetric(ctx context.Context, c *harness.Case, definition string, spec map[string]any, suffix string) (map[string]any, error) {
	name := c.NamePrefix + "-metric-" + suffix
	if e := c.Intent("pulseMetric", name, definition); e != nil {
		return nil, e
	}
	doc, e := c.API.JSON(ctx, http.MethodPost, pulseBase+"/metrics:getOrCreate", map[string]any{"definition_id": definition, "specification": spec})
	if e != nil {
		return nil, e
	}
	id := pulseID(doc)
	if nested, ok := doc["metric"].(map[string]any); ok {
		id = pulseID(nested)
	}
	if id == "" {
		id, _ = doc["metric_id"].(string)
	}
	if id == "" {
		return nil, fmt.Errorf("metric fixture creation omitted identity")
	}
	if e = c.Track(harness.Object{Kind: "pulseMetric", ID: id, Name: name, ProjectID: definition}); e != nil {
		return nil, e
	}
	var row map[string]any
	e = c.Observe(ctx, func() error {
		var err error
		row, err = pulseGet(ctx, c, "metrics", id)
		if err != nil {
			return err
		}
		if err = pulseOwnership(row, definition); err != nil {
			return err
		}
		if !pulseSpecificationsEqual(pulseSpecification(row), spec) {
			return fmt.Errorf("independent seeded metric specification differs from requested fixture")
		}
		return nil
	})
	return row, e
}
func pulseFollowers(ctx context.Context, c *harness.Case, metric string) ([]any, error) {
	return pulsePages(ctx, c, "subscriptions", url.Values{"metric_id": {metric}}, "subscriptions")
}
func subscriptionID(row map[string]any) string {
	if id := pulseID(row); id != "" {
		return id
	}
	if id, ok := row["subscription_id"].(string); ok {
		return id
	}
	return pulseID(asObject(row["subscription"]))
}
func followerIdentity(row map[string]any) (string, string) {
	f := asObject(row["follower"])
	if f == nil {
		f = asObject(asObject(row["subscription"])["follower"])
	}
	for _, kind := range []string{"user", "group"} {
		if id, ok := f[kind+"_id"].(string); ok && id != "" {
			return strings.ToUpper(kind), id
		}
		if id, _ := asObject(f[kind])["id"].(string); id != "" {
			return strings.ToUpper(kind), id
		}
	}
	return "", ""
}
func matchingFollowers(rows []any, kind, id string) []map[string]any {
	var out []map[string]any
	for _, v := range rows {
		row := asObject(v)
		typ, principal := followerIdentity(row)
		if typ == kind && principal == id {
			out = append(out, row)
		}
	}
	return out
}
func hasSubscription(rows []any, id string) bool {
	for _, v := range rows {
		if subscriptionID(asObject(v)) == id {
			return true
		}
	}
	return false
}
func pulseSeedSubscription(ctx context.Context, c *harness.Case, metric, kind, id, suffix string) (string, error) {
	name := c.NamePrefix + "-subscription-" + suffix
	if e := c.Intent("pulseSubscription", name, metric); e != nil {
		return "", e
	}
	before, e := pulseFollowers(ctx, c, metric)
	if e != nil {
		return "", e
	}
	if len(matchingFollowers(before, kind, id)) != 0 {
		return "", fmt.Errorf("fixture subscription already exists")
	}
	field := strings.ToLower(kind) + "_id"
	response, e := c.API.JSON(ctx, http.MethodPost, pulseBase+"/subscriptions:batchCreate", map[string]any{"metric_id": metric, "followers": []any{map[string]any{field: id}}})
	if e != nil {
		return "", e
	}
	receiptID := ""
	for _, row := range allMaps(response) {
		typ, principal := followerIdentity(row)
		if typ == kind && principal == id {
			observed := subscriptionID(row)
			if observed != "" {
				if receiptID != "" && receiptID != observed {
					return receiptID, fmt.Errorf("subscription response contains conflicting receipts")
				}
				receiptID = observed
			}
		}
	}
	if receiptID != "" {
		if e = c.Track(harness.Object{Kind: "pulseSubscription", ID: receiptID, Name: name, ProjectID: metric}); e != nil {
			return receiptID, e
		}
	}
	subscription := receiptID
	e = c.Observe(ctx, func() error {
		after, err := pulseFollowers(ctx, c, metric)
		if err != nil {
			return err
		}
		matches := matchingFollowers(after, kind, id)
		if len(matches) != 1 {
			return fmt.Errorf("independent subscription fixture failed exact identity assertion")
		}
		saved := subscriptionID(matches[0])
		if receiptID != "" && saved != receiptID {
			return fmt.Errorf("saved subscription identity differs from creation receipt")
		}
		subscription = saved
		return c.Track(harness.Object{Kind: "pulseSubscription", ID: saved, Name: name, ProjectID: metric})
	})
	return subscription, e
}
func verifyPulseBundle(bundle, definition map[string]any, metrics []any, definitionID, datasource string) error {
	if e := field(bundle, "definition_luid", definitionID); e != nil {
		return e
	}
	if e := field(bundle, "datasource_references", []string{datasource}); e != nil {
		return e
	}
	if !jsonEqual(bundle["definition"], definition) {
		return fmt.Errorf("portable bundle lost full definition configuration")
	}
	rows := array(bundle["metrics"])
	if len(rows) != len(metrics) {
		return fmt.Errorf("bundle metric inventory incomplete")
	}
	for _, v := range metrics {
		raw := asObject(v)
		row, e := record(rows, pulseID(raw))
		if e != nil {
			return e
		}
		if e = field(row, "specification", pulseSpecification(raw)); e != nil {
			return e
		}
		if marker, known := raw["is_default"]; known {
			if e = field(row, "is_default", marker); e != nil {
				return e
			}
		}
	}
	for _, row := range allMaps(bundle) {
		for _, key := range []string{"subscriptions", "subscribers", "access_token", "session_token", "pat", "metric_values", "insights"} {
			if _, present := row[key]; present {
				return fmt.Errorf("bundle includes forbidden nonportable field %s", key)
			}
		}
	}
	return nil
}
func pulseUnchanged(ctx context.Context, c *harness.Case, id string, definition map[string]any, metrics []any) error {
	fresh, e := pulseGet(ctx, c, "definitions", id)
	if e != nil {
		return e
	}
	if !jsonEqual(fresh, definition) {
		return fmt.Errorf("source definition changed")
	}
	current, e := pulseMetrics(ctx, c, id)
	if e != nil {
		return e
	}
	return subset(current, metrics)
}

func pulseRecord(rows []any, id string) (map[string]any, error) {
	for _, v := range rows {
		row := asObject(v)
		if pulseID(row) == id {
			return row, nil
		}
	}
	return nil, fmt.Errorf("missing exact Pulse identity %s", id)
}

// The REST provider can add a datasource reference and a checked legacy string
// filter mirror. Filter order is not part of saved metric identity.
func pulseSpecificationsEqual(left, right map[string]any) bool {
	a, err := normalizedPulseSpecification(left)
	if err != nil {
		return false
	}
	b, err := normalizedPulseSpecification(right)
	return err == nil && jsonEqual(a, b)
}
func normalizedPulseSpecification(input map[string]any) (map[string]any, error) {
	if input == nil {
		return nil, fmt.Errorf("missing metric specification")
	}
	out := cloneObject(input)
	delete(out, "datasource")
	filters, ok := out["filters"].([]any)
	if !ok {
		return nil, fmt.Errorf("metric specification requires explicit filters")
	}
	for _, v := range filters {
		filter, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("non-object filter")
		}
		categorical, hasTyped := filter["categorical_values"].([]any)
		if mirror, hasMirror := filter["string_values"]; hasMirror {
			if !hasTyped {
				return nil, fmt.Errorf("legacy filter mirror lacks typed population")
			}
			var stringsOnly []any
			for _, value := range categorical {
				r, ok := value.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid categorical population")
				}
				value, ok := r["string_value"].(string)
				if !ok {
					return nil, fmt.Errorf("string mirror does not describe typed string population")
				}
				stringsOnly = append(stringsOnly, value)
			}
			if subset(mirror, stringsOnly) != nil {
				return nil, fmt.Errorf("legacy filter mirror conflicts with typed population")
			}
			delete(filter, "string_values")
		}
		if hasTyped {
			sortJSONSet(categorical)
		}
	}
	sortJSONSet(filters)
	return out, nil
}
func sortJSONSet(values []any) {
	slices.SortFunc(values, func(a, b any) int {
		aa, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		return strings.Compare(string(aa), string(bb))
	})
}
