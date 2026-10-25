package suite

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/tests/live/harness"
)

// ContentBindingRequirements names each selected scenario's portable fixture
// inputs. Empty entries are intentional: project fixtures are created by setup.
func ContentBindingRequirements() map[string][]string {
	requirements := map[string][]string{}
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		native := kind + "-native"
		for _, action := range []string{"list", "inspect", "delete", "move", "pull", "publish-native", "publish-artifact", "detached-pull", "detached-publish"} {
			requirements["live."+kind+"."+action] = []string{native}
		}
		requirements["live."+kind+".update"] = []string{native, "alternate-owner"}
		requirements["live."+kind+".publish-overwrite"] = []string{native, native + "-b"}
	}
	for _, action := range []string{"create", "list", "inspect", "update", "delete"} {
		requirements["live.project."+action] = []string{}
	}
	requirements["live.project.move"] = []string{"workbook-native"}
	for _, kind := range []string{"workbook", "datasource"} {
		requirements["live."+kind+".schema"] = []string{kind + "-schema"}
		requirements["live."+kind+".publish-large-upload"] = []string{kind + "-native-large"}
	}
	requirements["live.datasource.publish-append"] = []string{"datasource-native-hyper", "datasource-native-hyper-b", "datasource-append-result"}
	requirements["live.datasource.publish-replace"] = []string{"datasource-native-hyper", "datasource-native-hyper-b"}
	requirements["live.datasource.publish-composed"] = []string{"datasource-native-composed"}
	requirements["live.workbook.pull-dependencies"] = []string{"workbook-native-dependencies", "workbook-dependencies"}
	requirements["live.workbook.pull-extract-scope"] = []string{"workbook-native-extract"}
	for _, action := range []string{"inspect", "wait", "cancel"} {
		requirements["live.job."+action] = []string{"job-" + action}
	}
	requirements["live.workspace.artifact-roundtrip"] = []string{"workbook-native"}
	requirements["live.workspace.delete-artifact"] = []string{"workbook-native"}
	return requirements
}

// ContentFixtureCapabilities declares helper mutations and recovery commands.
// Subject capabilities remain owned by the catalog; these describe actual
// fixture, observation, and local workspace calls made by the scenario.
func ContentFixtureCapabilities(id string) []string {
	if _, ok := ContentBindingRequirements()[id]; !ok {
		return nil
	}
	parts := strings.Split(id, ".")
	group, action := parts[1], parts[2]
	if action == "schema" {
		return nil
	}
	if group == "job" {
		if action == "wait" {
			return []string{"job.inspect", "job.wait"}
		}
		return []string{"job.inspect"}
	}
	capabilities := []string{"project.create", "project.delete"}
	switch group {
	case "project":
		if action == "move" {
			capabilities = append(capabilities, "workbook.publish", "workbook.delete", "project.inspect")
		}
	case "workspace":
		capabilities = append(capabilities, "workbook.publish", "workbook.delete", "workbook.pull", "workspace.status", "job.inspect")
		if action == "artifact-roundtrip" {
			capabilities = append(capabilities, "workspace.create")
		}
	default:
		capabilities = append(capabilities, group+".publish", group+".delete")
		if action == "pull" || strings.HasPrefix(action, "pull-") || strings.HasPrefix(action, "publish-") || strings.HasPrefix(action, "detached-") {
			capabilities = append(capabilities, "job.inspect")
		}
		if action == "publish-artifact" {
			capabilities = append(capabilities, group+".pull", "workspace.status")
		}
		if action == "pull" || strings.HasPrefix(action, "pull-") || action == "detached-pull" {
			capabilities = append(capabilities, "workspace.status")
		}
		if action == "detached-publish" && group != "flow" {
			capabilities = append(capabilities, "job.wait")
		}
	}
	slices.Sort(capabilities)
	return slices.Compact(capabilities)
}

// ValidateContentBinding catches incomplete fixture packs before setup writes.
// It validates declared inputs, without manufacturing expected facts from CLI output.
func ValidateContentBinding(key string, b harness.Binding) error {
	problem := func(message string) error { return harness.Blocked(fmt.Sprintf("binding %s: %s", key, message)) }
	switch {
	case key == "alternate-owner":
		if b.ID == "" {
			return problem("requires a licensed alternate user LUID with cleanup access retained")
		}
	case strings.HasPrefix(key, "job-"):
		if b.Params["submit_method"] != "POST" || !strings.HasPrefix(b.Params["submit_path"], "/api/") || strings.ContainsAny(b.Params["submit_path"], "\r\n\\") || strings.Contains(b.Params["submit_path"], "..") {
			return problem("requires POST and an exact server-relative API submit_path")
		}
		if raw := b.Params["submit_body"]; raw != "" && !json.Valid([]byte(raw)) {
			return problem("submit_body must be valid JSON without credentials")
		}
	case strings.HasSuffix(key, "-schema"):
		if b.ID == "" || b.Params["metadata_query"] == "" || len(contentMap(b.Expected["cli"])) == 0 || len(contentMap(b.Expected["metadata"])) == 0 {
			return problem("requires fixture ID, metadata_query, and authored cli and metadata expectations")
		}
	case key == "workbook-dependencies":
		ids, ok := b.Expected["direct_ids"].([]any)
		if !ok || len(ids) != 2 || contentString(b.Expected, "grandparent_id") == "" || len(contentMap(b.Expected["lineage"])) == 0 {
			return problem("requires two distinct direct_ids, grandparent_id, and authored lineage aliases and edges")
		}
		for _, id := range ids {
			value, ok := id.(string)
			if !ok || value == "" {
				return problem("direct_ids must contain two nonempty datasource LUID strings")
			}
		}
		if ids[0].(string) == ids[1].(string) {
			return problem("direct_ids must identify distinct datasources")
		}
	case key == "datasource-append-result":
		if _, ok := b.Expected["rows"].([]any); !ok {
			return problem("requires exact authored A plus B row multiset")
		}
	case strings.Contains(key, "-native"):
		if b.Path == "" {
			return problem("requires native file path")
		}
		stat, err := os.Stat(b.Path)
		if err != nil || !stat.Mode().IsRegular() {
			return problem("native file must exist and be a regular file")
		}
		if len(b.Expected) == 0 {
			return problem("requires authored native payload expectations")
		}
		if strings.HasPrefix(key, "flow-") {
			if len(contentMap(b.Expected["flow"])) == 0 && len(contentMap(b.Expected["document"])) == 0 {
				return problem("requires authored flow nodes and outputs in expected.flow or expected.document")
			}
		}
		if strings.HasSuffix(key, "-large") {
			if stat.Size() <= 64<<20 {
				return problem("large fixture must exceed 64 MiB after compression")
			}
			if _, ok := b.Expected["rows"].([]any); !ok {
				return problem("large fixture requires exact authored rows, including the payload tail")
			}
		}
		if strings.Contains(key, "-hyper") {
			if _, ok := b.Expected["rows"].([]any); !ok {
				return problem("Hyper fixture requires exact authored rows")
			}
		}
		if strings.HasSuffix(key, "-composed") && (b.Params["metadata_query"] == "" || len(contentMap(b.Expected["metadata"])) == 0) {
			return problem("requires immediate-parent metadata_query and authored expected.metadata")
		}
	default:
		return errors.New("unknown content binding requirement: " + key)
	}
	return nil
}

// ValidateContentRequirements checks exactly the requested contracts.
func ValidateContentRequirements(profile harness.Profile, ids []string) error {
	requirements := ContentBindingRequirements()
	keys := []string{}
	for _, id := range ids {
		required, ok := requirements[id]
		if !ok {
			continue
		}
		keys = append(keys, required...)
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	var failures []error
	for _, key := range keys {
		b, ok := profile.Bindings[key]
		if !ok {
			failures = append(failures, harness.Blocked("target profile requires binding "+key))
			continue
		}
		if err := ValidateContentBinding(key, b); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
