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

	"github.com/ahillspace/tadx/tests/live/harness"
)

// CatalogCases includes catalog assets, attached labels, audits, and saved
// lineage graphs. Authored truth manifests are independent of CLI output.
func CatalogCases() map[string]harness.Scenario {
	cases := map[string]harness.Scenario{}
	for _, kind := range []string{"database", "table", "column"} {
		cases["live.catalog."+kind+"-read"] = catalogRead(kind)
		cases["live.catalog."+kind+"-update"] = catalogUpdate(kind)
	}
	for _, kind := range []string{"database", "table", "datasource", "workbook"} {
		cases["live.catalog.audit-"+kind] = catalogAudit(kind)
	}
	cases["live.catalog.search-upstream"] = catalogSearch(false)
	cases["live.catalog.search-column-scope"] = catalogSearch(true)
	for _, verb := range []string{"read", "update", "delete"} {
		cases["live.labels."+verb+"-attachment"] = catalogLabel(verb)
	}
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		cases["live.lineage."+kind+"-graph"] = catalogLineage(kind)
	}
	return cases
}

func catalogRead(kind string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		b, e := c.Require("catalog_" + kind)
		if e != nil {
			return e
		}
		if b.ID == "" || b.Name == "" || b.Params["metadata_id"] == "" {
			return harness.Blocked("catalog asset requires REST ID, name, and metadata_id")
		}
		fresh, e := c.API.Get(ctx, catalogKind(kind, b), b.ID)
		if e != nil {
			return e
		}
		list := []string{"catalog", kind, "list", "--name", b.Name, "--all"}
		inspect := []string{"catalog", kind, "inspect", "--id", b.ID}
		if kind == "table" {
			if b.Params["parent_id"] == "" {
				return harness.Blocked("table requires parent_id")
			}
			list = append(list, "--database-id", b.Params["parent_id"])
		}
		if kind == "column" {
			if b.Params["parent_id"] == "" {
				return harness.Blocked("column requires parent_id")
			}
			list = append(list, "--table-id", b.Params["parent_id"])
			inspect = append(inspect, "--table-id", b.Params["parent_id"])
		}
		for _, args := range [][]string{list, inspect} {
			out, e := fullRun(ctx, c, args...)
			if e != nil {
				return e
			}
			row, e := record(out, b.ID)
			if e != nil {
				return e
			}
			if e = field(row, "name", fresh.Name); e != nil {
				return e
			}
			if e = field(row, "metadata_id", b.Params["metadata_id"]); e != nil {
				return e
			}
			if args[2] == "inspect" {
				if description, exists := fresh.Fields["description"]; exists {
					if e = field(row, "description", description); e != nil {
						return e
					}
				}
			}
			if expected, ok := b.Expected["read"].(map[string]any); ok {
				if e = subset(row, expected); e != nil {
					return e
				}
			}
		}
		return catalogOracle(ctx, c, b)
	}
}
func catalogKind(kind string, b harness.Binding) string {
	if kind == "column" {
		return "tables/" + url.PathEscape(b.Params["parent_id"]) + "/columns"
	}
	return kind
}
func catalogPath(c *harness.Case, kind string, b harness.Binding) string {
	if kind == "column" {
		return c.API.REST("tables/" + url.PathEscape(b.Params["parent_id"]) + "/columns/" + url.PathEscape(b.ID))
	}
	return c.API.REST(kind + "s/" + url.PathEscape(b.ID))
}
func catalogUpdate(kind string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		b, e := c.Require("catalog_" + kind)
		if e != nil {
			return e
		}
		sibling, e := c.Require("catalog_" + kind + "_sibling")
		if e != nil {
			return e
		}
		if sibling.ID == b.ID {
			return fmt.Errorf("catalog sentinel must differ from subject")
		}
		before, e := c.API.Get(ctx, catalogKind(kind, b), b.ID)
		if e != nil {
			return e
		}
		other, e := c.API.Get(ctx, catalogKind(kind, sibling), sibling.ID)
		if e != nil {
			return e
		}
		path := catalogPath(c, kind, b)
		if kind != "column" {
			description, _ := before.Fields["description"].(string)
			contactID, _ := asObject(before.Fields["contact"])["id"].(string)
			if description == "" || contactID == "" {
				return harness.Blocked("database/table update fixture requires existing nonempty description and contact for verified restoration")
			}
		}
		writable := map[string]any{"description": ""}
		if value, ok := before.Fields["description"]; ok {
			writable["description"] = value
		}
		if kind != "column" {
			contact := map[string]any{"id": ""}
			if value, ok := before.Fields["contact"]; ok {
				contact = asObject(value)
			}
			writable["contact"] = contact
		}
		if e = c.RestoreRequest(http.MethodPut, path, map[string]any{kind: writable}); e != nil {
			return e
		}
		oldTag, sentinelTag, newTag := c.NamePrefix+"old", c.NamePrefix+"sentinel", c.NamePrefix+"new"
		tagPath := c.API.REST(kind + "s/" + url.PathEscape(b.ID) + "/tags")
		for _, tag := range []string{oldTag, sentinelTag, newTag} {
			if e = c.RestoreRequest(http.MethodDelete, tagPath+"/"+url.PathEscape(tag), nil); e != nil {
				return e
			}
		}
		if _, e = c.API.JSON(ctx, http.MethodPut, tagPath, map[string]any{"tags": map[string]any{"tag": []any{map[string]any{"label": oldTag}, map[string]any{"label": sentinelTag}}}}); e != nil {
			return e
		}
		args := []string{"catalog", kind, "update", "--id", b.ID, "--description", c.NamePrefix + "description", "--add-tag", newTag, "--remove-tag", oldTag}
		if kind == "column" {
			args = append(args, "--table-id", b.Params["parent_id"])
		} else {
			contact, e := c.Require("user_b")
			if e != nil {
				return e
			}
			if asObject(before.Fields["contact"])["id"] == contact.ID {
				return harness.Blocked("catalog update user_b must differ from existing fixture contact")
			}
			args = append(args, "--contact-id", contact.ID)
		}
		if _, e = fullRun(ctx, c, args...); e != nil {
			return e
		}
		after, e := c.API.Get(ctx, catalogKind(kind, b), b.ID)
		if e != nil {
			return e
		}
		if e = field(after.Fields, "description", c.NamePrefix+"description"); e != nil {
			return e
		}
		if kind != "column" {
			contact, _ := c.Require("user_b")
			if e = field(asObject(after.Fields["contact"]), "id", contact.ID); e != nil {
				return e
			}
		}
		tags := tagNames(after.Fields)
		if !slices.Contains(tags, newTag) || !slices.Contains(tags, sentinelTag) || slices.Contains(tags, oldTag) {
			return fmt.Errorf("catalog update did not preserve exact tag changes")
		}
		for _, tag := range tagNames(before.Fields) {
			if !slices.Contains(tags, tag) {
				return fmt.Errorf("catalog update removed unrelated tag")
			}
		}
		freshSibling, e := c.API.Get(ctx, catalogKind(kind, sibling), sibling.ID)
		if e != nil {
			return e
		}
		return stableCatalog(other, freshSibling)
	}
}
func tagNames(data any) []string {
	var names []string
	for _, row := range allMaps(data) {
		if label, ok := row["label"].(string); ok {
			names = append(names, label)
		}
	}
	return slices.Sorted(slices.Values(names))
}
func stableCatalog(a, b harness.Object) error {
	for _, key := range []string{"description", "contact", "tags"} {
		if !jsonEqual(a.Fields[key], b.Fields[key]) {
			return fmt.Errorf("catalog sentinel %s changed", key)
		}
	}
	return nil
}

func catalogAudit(kind string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		b, e := c.Require("audit_" + kind)
		if e != nil {
			return e
		}
		truth, ok := b.Expected["output"].(map[string]any)
		if !ok || len(array(truth["findings"])) == 0 {
			return harness.Blocked("audit requires authored nonempty Expected.output.findings truth table")
		}
		for _, key := range []string{"scanned", "summary", "complete"} {
			if _, ok := truth[key]; !ok {
				return harness.Blocked("audit truth requires scanned, summary, and complete")
			}
		}
		if e = catalogOracle(ctx, c, b); e != nil {
			return e
		}
		args := []string{"catalog", "audit", "--type", kind, "--id", b.ID}
		if kind != "database" {
			args = append(args, "--check", "descriptions")
		}
		out, e := fullRun(ctx, c, args...)
		if e != nil {
			return e
		}
		if e = subset(out, truth); e != nil {
			return e
		}
		if e = field(out, "id", b.ID); e != nil {
			return e
		}
		if e = field(out, "type", kind); e != nil {
			return e
		}
		findings := array(out["findings"])
		counts := map[string]int{"present": 0, "missing": 0, "unknown": 0}
		for _, v := range findings {
			row := asObject(v)
			state, _ := row["state"].(string)
			if _, ok := counts[state]; !ok {
				return fmt.Errorf("unknown assessment state %q", state)
			}
			counts[state]++
		}
		summary := asObject(out["summary"])
		for key, count := range counts {
			if !jsonEqual(summary[key], count) {
				return fmt.Errorf("audit summary %s does not reconcile with findings", key)
			}
		}
		return nil
	}
}
func catalogSearch(columns bool) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		key := "catalog_search"
		if columns {
			key = "catalog_column_search"
		}
		b, e := c.Require(key)
		if e != nil {
			return e
		}
		if b.Params["query"] == "" {
			return harness.Blocked("catalog search requires authored query")
		}
		truth, ok := b.Expected["output"].(map[string]any)
		if !ok || len(array(truth["items"])) == 0 {
			return harness.Blocked("catalog search requires exact nonempty Expected.output.items")
		}
		if e = catalogOracle(ctx, c, b); e != nil {
			return e
		}
		args := []string{"catalog", "search", b.Params["query"], "--all"}
		if columns {
			if b.Params["parent_id"] == "" {
				return harness.Blocked("column search requires parent_id")
			}
			args = append(args, "--type", "column", "--table-id", b.Params["parent_id"])
		} else {
			args = append(args, "--type", "database", "--type", "table")
		}
		out, e := fullRun(ctx, c, args...)
		if e != nil {
			return e
		}
		return subset(out, truth)
	}
}

func catalogLabel(verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		ds, e := c.Content(ctx, "datasource", "native")
		if e != nil {
			return e
		}
		value, e := c.Seed(ctx, "labelValue", "attachment")
		if e != nil {
			return e
		}
		makeLabel := func(suffix string) (harness.Object, error) {
			name := c.NamePrefix + suffix
			category, _ := value.Fields["category"].(string)
			if e := c.IntentLabel(name, "datasource", ds.ID, value.Name, category, false, false); e != nil {
				return harness.Object{}, e
			}
			object, e := c.API.CreateLabel(ctx, harness.LabelAttachment{ContentType: "datasource", ContentID: ds.ID, Value: value.Name, Category: category, Message: name, Active: false, Elevated: false})
			if e != nil {
				return harness.Object{}, e
			}
			if e := c.ConfirmReceipt("label", name, ds.ID, object); e != nil {
				return harness.Object{}, e
			}
			return object, nil
		}
		label, e := makeLabel("attachment")
		if e != nil {
			return e
		}
		// A different value produces a second attachment on the same datasource.
		secondValue, e := c.Seed(ctx, "labelValue", "sentinel-attachment")
		if e != nil {
			return e
		}
		firstValue := value
		value = secondValue
		sentinel, e := makeLabel("sentinel")
		value = firstValue
		if e != nil {
			return e
		}
		observeLabel := func(id string) (harness.Object, error) {
			labels, err := c.API.QueryLabels(ctx, "datasource", ds.ID)
			if err != nil {
				return harness.Object{}, err
			}
			var matches []harness.Object
			for _, candidate := range labels {
				if candidate.ID == id {
					matches = append(matches, candidate)
				}
			}
			if len(matches) != 1 {
				return harness.Object{}, harness.Inconclusive("scoped label query did not return exactly one requested label ID")
			}
			return matches[0], nil
		}
		before, e := observeLabel(label.ID)
		if e != nil {
			return e
		}
		args := []string{"catalog", "label", verb, "--id", label.ID}
		if verb == "read" {
			args = []string{"catalog", "label", "inspect", "--id", label.ID}
			list, e := fullRun(ctx, c, "catalog", "label", "list", "--type", "datasource", "--target-id", ds.ID, "--all")
			if e != nil {
				return e
			}
			if _, e = record(list, label.ID); e != nil {
				return e
			}
		}
		if verb == "update" {
			args = append(args, "--message", c.NamePrefix+"updated", "--active=true")
		}
		out, e := fullRun(ctx, c, args...)
		if e != nil {
			return e
		}
		if _, e = c.API.Get(ctx, "datasource", ds.ID); e != nil {
			return e
		}
		if _, e = observeLabel(sentinel.ID); e != nil {
			return e
		}
		if verb == "delete" {
			labels, e := c.API.QueryLabels(ctx, "datasource", ds.ID)
			if e != nil {
				return e
			}
			for _, candidate := range labels {
				if candidate.ID == label.ID {
					return fmt.Errorf("deleted label remains attached")
				}
			}
			return nil
		}
		after, e := observeLabel(label.ID)
		if e != nil {
			return e
		}
		for _, key := range []string{"contentId", "contentType", "value", "category"} {
			if !jsonEqual(before.Fields[key], after.Fields[key]) {
				return fmt.Errorf("attachment ownership changed")
			}
		}
		if verb == "update" {
			if e = field(after.Fields, "message", c.NamePrefix+"updated"); e != nil {
				return e
			}
			return field(after.Fields, "active", true)
		}
		row, e := record(out, label.ID)
		if e != nil {
			return e
		}
		for cli, rest := range map[string]string{"target_luid": "contentId", "type": "contentType", "value": "value", "category": "category", "message": "message", "active": "active"} {
			want := after.Fields[rest]
			if cli == "type" {
				want, e = catalogLabelType(contentString(after.Fields, rest))
				if e != nil {
					return e
				}
			}
			if e = field(row, cli, want); e != nil {
				return e
			}
		}
		return nil
	}
}

func catalogLabelType(contentType string) (string, error) {
	if contentType != "DATASOURCE" {
		return "", fmt.Errorf("unexpected REST label content type")
	}
	return "datasource", nil
}

func catalogLineage(kind string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		b, e := c.Require("lineage_" + kind)
		if e != nil {
			return e
		}
		truth, ok := b.Expected["graph"].(map[string]any)
		if !ok || len(array(truth["nodes"])) < 2 || len(array(truth["edges"])) == 0 || b.Params["metadata_id"] == "" {
			return harness.Blocked("lineage requires independently authored nodes, edges, and root metadata_id")
		}
		if e = catalogOracle(ctx, c, b); e != nil {
			return e
		}
		args := []string{"catalog", "lineage", "pull", "--kind", kind, "--id", b.ID, "--workspace", c.Workspace}
		if kind != "flow" {
			args = append(args, "--fields")
		}
		out, e := fullRun(ctx, c, args...)
		if e != nil {
			return e
		}
		artifact := asObject(out["artifact"])
		relative, _ := artifact["lineage_path"].(string)
		path, e := safeArtifactPath(c.WorkspacePath, relative)
		if e != nil {
			return e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var graph map[string]any
		if e = json.Unmarshal(data, &graph); e != nil {
			return e
		}
		if e = subsetGraph(graph, truth); e != nil {
			return e
		}
		nodes := map[string]map[string]any{}
		for _, v := range array(graph["nodes"]) {
			n := asObject(v)
			id, _ := n["metadata_id"].(string)
			if id == "" || nodes[id] != nil {
				return fmt.Errorf("graph node omitted or duplicated Metadata identity")
			}
			nodes[id] = n
		}
		root := nodes[b.Params["metadata_id"]]
		if e = field(root, "rest_luid", b.ID); e != nil {
			return e
		}
		if e = field(root, "kind", kind); e != nil {
			return e
		}
		for _, v := range array(graph["edges"]) {
			edge := asObject(v)
			from, _ := edge["from_metadata_id"].(string)
			to, _ := edge["to_metadata_id"].(string)
			if nodes[from] == nil || nodes[to] == nil {
				return fmt.Errorf("graph edge has missing endpoint")
			}
		}
		if kind != "flow" {
			if e = field(graph, "fields", true); e != nil {
				return e
			}
			found := false
			for _, node := range nodes {
				if node["kind"] == "field" {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("requested lineage omitted field nodes")
			}
		}
		return nil
	}
}
func subsetGraph(actual, expected map[string]any) error {
	for _, key := range []string{"nodes", "edges"} {
		for _, want := range array(expected[key]) {
			found := false
			for _, got := range array(actual[key]) {
				if subset(got, want) == nil {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("lineage missing authored %s member %v", key, want)
			}
		}
	}
	return nil
}
func catalogOracle(ctx context.Context, c *harness.Case, b harness.Binding) error {
	query := b.Params["oracle_query"]
	truth, ok := b.Expected["oracle"].(map[string]any)
	if query == "" || !ok || len(truth) == 0 {
		return harness.Blocked("catalog fixture requires independent oracle_query and nonempty Expected.oracle")
	}
	var variables map[string]any
	if raw := b.Params["oracle_variables"]; raw != "" {
		if e := json.Unmarshal([]byte(raw), &variables); e != nil {
			return fmt.Errorf("invalid oracle_variables: %w", e)
		}
	}
	out, e := c.API.Metadata(ctx, query, variables)
	if e != nil {
		return e
	}
	return subset(out, truth)
}
func asObject(v any) map[string]any { row, _ := v.(map[string]any); return row }
func safeArtifactPath(root, relative string) (string, error) {
	if relative == "" || strings.Contains(relative, "\\") || filepath.IsAbs(relative) {
		return "", fmt.Errorf("artifact path must be workspace-relative with forward slashes")
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, e := filepath.Rel(root, path)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path escapes workspace")
	}
	return path, nil
}

// subset compares authored map fields and complete unordered array sets. Array
// cardinality is significant, and an actual element cannot satisfy two rows.
func subset(actual, expected any) error {
	switch want := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object, got %T", actual)
		}
		for key, v := range want {
			value, exists := got[key]
			if !exists {
				return fmt.Errorf("missing expected field %s", key)
			}
			if e := subset(value, v); e != nil {
				return fmt.Errorf("%s: %w", key, e)
			}
		}
		return nil
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(want) {
			return fmt.Errorf("expected exact %d-element set, got %v", len(want), actual)
		}
		used := make([]bool, len(got))
		for _, v := range want {
			found := false
			for i, other := range got {
				if !used[i] && subset(other, v) == nil {
					used[i] = true
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("missing expected set member %v", v)
			}
		}
		return nil
	default:
		if !jsonEqual(actual, expected) {
			return fmt.Errorf("got %v, want %v", actual, expected)
		}
		return nil
	}
}
