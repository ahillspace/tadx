package suite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func contentPull(ctx context.Context, c *harness.Case, kind, action string) (harness.Object, map[string]any, error) {
	variant := ""
	if action == "pull-dependencies" {
		variant = "dependencies"
	}
	if action == "pull-extract-scope" {
		variant = "extract"
	}
	o, err := c.Content(ctx, kind, variant)
	if err != nil {
		return o, nil, err
	}
	if err = contentAwaitListed(ctx, c, o); err != nil {
		return o, nil, err
	}
	args := []string{"--workspace", c.Workspace, "--id", o.ID, "--full"}
	if action == "pull-dependencies" {
		args = append(args, "--include-pds")
	}
	if action == "pull-extract-scope" {
		args = append(args, "--include-extract=false")
	}
	out, err := contentRun(ctx, c, kind, "pull", args...)
	if err != nil {
		return o, out, err
	}
	if action == "pull-extract-scope" {
		resp, e := c.API.Do(ctx, "GET", c.API.REST("workbooks/"+o.ID+"/content")+"?includeExtract=false", nil, "")
		if e != nil {
			return o, out, e
		}
		local, e := contentVerifyPull(ctx, c, kind, o, out, resp.Body)
		if e != nil {
			return o, out, e
		}
		evidence, e := harness.ValidateNative(kind, harness.Native{Bytes: local, Filename: contentString(contentMap(out["artifact"]), "canonical_path")})
		if e != nil {
			return o, out, e
		}
		_ = evidence
		full, e := c.API.Download(ctx, kind, o.ID)
		if e != nil {
			return o, out, e
		}
		if harness.CompareNative(kind, full.Bytes, local) == nil {
			return o, out, errors.New("extract-scope fixture does not distinguish extract-preserving download")
		}
	} else {
		native, e := c.API.Download(ctx, kind, o.ID)
		if e != nil {
			return o, out, e
		}
		if _, e = contentVerifyPull(ctx, c, kind, o, out, native.Bytes); e != nil {
			return o, out, e
		}
	}
	if action == "pull-dependencies" {
		if err = contentVerifyDependencies(ctx, c, o, out); err != nil {
			return o, out, err
		}
	}
	return o, out, contentUnchanged(ctx, c, o)
}

func contentVerifyPull(ctx context.Context, c *harness.Case, kind string, o harness.Object, out map[string]any, oracle []byte) ([]byte, error) {
	if !contentHasIdentity(out[kind], o.ID) {
		return nil, errors.New("pull receipt identifies another source")
	}
	artifact := contentMap(out["artifact"])
	dir, err := contentSafePath(c.WorkspacePath, contentString(artifact, "path"))
	if err != nil {
		return nil, err
	}
	canonical, err := contentSafePath(c.WorkspacePath, contentString(artifact, "canonical_path"))
	if err != nil {
		return nil, err
	}
	metadata, err := contentReadJSON(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	if contentString(metadata, "tableau_id") != o.ID || contentString(metadata, "source_project_id") != o.ProjectID || contentString(metadata, "source_environment") != c.Environment || contentString(metadata, "source_site_luid") == "" || contentString(metadata, "source_server_origin") == "" {
		return nil, errors.New("pulled sidecar omits or changes authoritative source provenance")
	}
	bytes, err := os.ReadFile(canonical)
	if err != nil {
		return nil, err
	}
	if err = harness.CompareNative(kind, oracle, bytes); err != nil {
		return nil, fmt.Errorf("pull differs from independent native download: %w", err)
	}
	if contentString(metadata, "local_baseline_fingerprint") == "" {
		return nil, errors.New("pulled sidecar has no clean baseline")
	}
	status, err := c.Run(ctx, "workspace", "status", "--workspace", c.Workspace, "--all", "--full")
	if err != nil {
		return nil, err
	}
	inventory := contentMap(status["artifacts"])
	rows, _ := inventory["artifacts"].([]any)
	found := 0
	for _, row := range rows {
		item := contentMap(row)
		if contentString(item, "luid") == o.ID {
			found++
			if contentString(item, "state") != "clean" {
				return nil, errors.New("fresh pulled artifact is not clean")
			}
		}
	}
	if found != 1 {
		return nil, errors.New("workspace does not contain exactly one clean artifact for source")
	}
	return bytes, nil
}

func contentVerifyDependencies(ctx context.Context, c *harness.Case, o harness.Object, out map[string]any) error {
	b, err := c.Require("workbook-dependencies")
	if err != nil {
		return err
	}
	expected, ok := b.Expected["direct_ids"].([]any)
	if !ok || len(expected) != 2 {
		return errors.New("dependency manifest requires exactly two independently authored direct_ids")
	}
	artifact := contentMap(out["artifact"])
	refs, ok := artifact["published_datasources"].([]any)
	if !ok || len(refs) != 2 {
		return errors.New("pull does not report exactly two direct dependencies")
	}
	ids := []string{}
	for _, raw := range refs {
		ref := contentMap(raw)
		id := contentString(ref, "luid")
		ids = append(ids, id)
		path, e := contentSafePath(c.WorkspacePath, contentString(ref, "local_artifact_path"))
		if e != nil {
			return e
		}
		metadata, e := contentReadJSON(filepath.Join(path, "metadata.json"))
		if e != nil {
			return e
		}
		if contentString(metadata, "tableau_id") != id {
			return errors.New("dependency sidecar changes remote identity")
		}
		payload, e := os.ReadFile(filepath.Join(path, contentString(metadata, "canonical_payload")))
		if e != nil {
			return e
		}
		native, e := c.API.Download(ctx, "datasource", id)
		if e != nil {
			return e
		}
		if e = harness.CompareNative("datasource", native.Bytes, payload); e != nil {
			return e
		}
	}
	wants := []string{}
	for _, id := range expected {
		value, ok := id.(string)
		if !ok || value == "" {
			return errors.New("invalid direct dependency manifest ID")
		}
		wants = append(wants, value)
	}
	slices.Sort(ids)
	slices.Sort(wants)
	if !reflect.DeepEqual(ids, wants) {
		return errors.New("pulled dependency IDs differ from authored direct relationships")
	}
	grandparent := contentString(b.Expected, "grandparent_id")
	if grandparent == "" {
		return errors.New("dependency manifest omits independently seeded grandparent")
	}
	status, err := c.Run(ctx, "workspace", "status", "--workspace", c.Workspace, "--all", "--full")
	if err != nil {
		return err
	}
	if contentHasIdentity(status, grandparent) {
		return errors.New("pull recursively acquires grandparent")
	}
	dir, err := contentSafePath(c.WorkspacePath, contentString(artifact, "path"))
	if err != nil {
		return err
	}
	lineage, err := contentReadJSON(filepath.Join(dir, "lineage.json"))
	if err != nil {
		return err
	}
	expectedLineage := contentMap(b.Expected["lineage"])
	if len(expectedLineage) == 0 {
		return harness.Blocked("dependency fixture requires authored lineage aliases and graph edges")
	}
	return contentSubset(lineage, expectedLineage)
}

func contentPublish(ctx context.Context, c *harness.Case, kind, action string) error {
	variant := ""
	switch action {
	case "publish-overwrite":
		variant = "b"
	case "publish-large-upload":
		variant = "large"
	case "publish-composed":
		variant = "composed"
	case "publish-append", "publish-replace":
		variant = "hyper-b"
	}
	binding, err := contentBinding(c, kind, variant)
	if err != nil {
		return err
	}
	if action == "publish-append" {
		union, e := c.Require("datasource-append-result")
		if e != nil {
			return e
		}
		if _, ok := union.Expected["rows"]; !ok {
			return harness.Blocked("append fixture requires exact authored union row multiset")
		}
	}
	if action == "publish-replace" {
		if _, ok := binding.Expected["rows"]; !ok {
			return harness.Blocked("replace fixture requires exact authored replacement rows")
		}
	}
	if action == "publish-composed" && (binding.Params["metadata_query"] == "" || len(contentMap(binding.Expected["metadata"])) == 0) {
		return harness.Blocked("composed fixture requires independently authored immediate-parent metadata")
	}
	file := binding.Path
	if action == "publish-large-upload" {
		if _, ok := binding.Expected["rows"]; !ok {
			return harness.Blocked("large upload fixture requires exact extract rows, including payload tail")
		}
		stat, e := os.Stat(file)
		if e != nil {
			return e
		}
		if stat.Size() <= 64<<20 {
			return errors.New("large-upload fixture does not exceed the unmodified 64 MiB threshold")
		}
	}
	p, err := c.Project(ctx, "destination")
	if err != nil {
		return err
	}
	name := c.NamePrefix + "-published"
	args := []string{"--file", file, "--project-id", p.ID, "--name", name}
	mode := "create"
	var source, decoy harness.Object
	var sourceNative []byte
	var localBefore map[string][]byte
	if action == "publish-artifact" {
		var pull map[string]any
		source, pull, err = contentPull(ctx, c, kind, "pull")
		if err != nil {
			return err
		}
		artifact := contentMap(pull["artifact"])
		args = []string{"--workspace", c.Workspace, "--artifact", contentString(artifact, "path"), "--project-id", p.ID, "--name", name}
		native, e := c.API.Download(ctx, kind, source.ID)
		if e != nil {
			return e
		}
		sourceNative = native.Bytes
		localBefore, e = contentSnapshot(c.WorkspacePath)
		if e != nil {
			return e
		}
	} else if action == "publish-overwrite" || action == "publish-append" || action == "publish-replace" {
		baseline := ""
		if action != "publish-overwrite" {
			baseline = "hyper"
		}
		source, err = contentPublishSeed(ctx, c, kind, p, name, baseline)
		if err != nil {
			return err
		}
		q, e := c.Project(ctx, "decoy")
		if e != nil {
			return e
		}
		decoy, err = contentPublishSeed(ctx, c, kind, q, name, baseline)
		if err != nil {
			return err
		}
		native, e := c.API.Download(ctx, kind, source.ID)
		if e != nil {
			return e
		}
		sourceNative = native.Bytes
		switch action {
		case "publish-overwrite":
			mode = "overwrite"
		case "publish-append":
			mode = "append"
		case "publish-replace":
			mode = "replace"
		}
	}
	if kind == "datasource" || mode != "create" {
		args = append(args, "--"+mode)
	}
	if err = c.Intent(kind, name, p.ID); err != nil {
		return err
	}
	out, err := contentRun(ctx, c, kind, "publish", args...)
	if err != nil {
		return err
	}
	published, err := contentFindPublished(ctx, c, kind, p.ID, name, out)
	if err != nil {
		return err
	}
	if source.ID != "" && action != "publish-artifact" && published.ID != source.ID {
		return errors.New("overwrite/data mutation changes destination identity")
	}
	var download harness.Native
	if action == "publish-append" {
		union, e := c.Require("datasource-append-result")
		if e != nil {
			return e
		}
		binding.Expected = union.Expected
	}
	if err = contentObserve(ctx, c, func() error {
		var e error
		download, e = c.API.Download(ctx, kind, published.ID)
		if e != nil {
			return e
		}
		if action == "publish-overwrite" && harness.CompareNative(kind, sourceNative, download.Bytes) == nil {
			return errors.New("overwrite has not replaced distinguishable payload A")
		}
		return harness.CheckNative(ctx, kind, download.Bytes, binding)
	}); err != nil {
		return err
	}
	if action == "publish-artifact" {
		after, e := contentSnapshot(c.WorkspacePath)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(after, localBefore) {
			return errors.New("publishing changes pulled artifact provenance or payload")
		}
		remote, e := c.API.Download(ctx, kind, source.ID)
		if e != nil {
			return e
		}
		if e = harness.CompareNative(kind, sourceNative, remote.Bytes); e != nil {
			return fmt.Errorf("artifact publication changes remote source payload: %w", e)
		}
		return contentUnchanged(ctx, c, source)
	}
	if decoy.ID != "" {
		if err = contentUnchanged(ctx, c, decoy); err != nil {
			return err
		}
	}
	if action == "publish-composed" {
		return contentComposed(ctx, c, published, binding)
	}
	return nil
}

func contentFindPublished(ctx context.Context, c *harness.Case, kind, project, name string, out map[string]any) (harness.Object, error) {
	var object harness.Object
	err := contentObserve(ctx, c, func() error {
		rows, err := c.API.List(ctx, kind, nil)
		if err != nil {
			return err
		}
		var found []harness.Object
		for _, o := range rows {
			if o.ProjectID == project && o.Name == name {
				found = append(found, o)
			}
		}
		if len(found) != 1 {
			return fmt.Errorf("independent inventory finds %d exact destinations, want one", len(found))
		}
		object = found[0]
		if !contentHasIdentity(out, object.ID) {
			return errors.New("publication receipt does not identify independently observed destination")
		}
		return c.Track(object)
	})
	return object, err
}

func contentComposed(ctx context.Context, c *harness.Case, o harness.Object, b harness.Binding) error {
	query := b.Params["metadata_query"]
	if query == "" {
		return errors.New("composed fixture requires independent metadata_query and metadata_expected manifest")
	}
	query = strings.ReplaceAll(query, "${id}", o.ID)
	doc, err := c.API.JSON(ctx, "POST", "/api/metadata/graphql", map[string]any{"query": query})
	if err != nil {
		return err
	}
	if failures, ok := doc["errors"].([]any); ok && len(failures) > 0 {
		return harness.Blocked("independent composed-fixture Metadata query returns errors")
	}
	expected := contentMap(b.Expected["metadata"])
	if len(expected) == 0 {
		return errors.New("composed metadata expectation is empty")
	}
	return contentSubset(doc, expected)
}

func contentSchema(ctx context.Context, c *harness.Case, kind string) error {
	b, err := c.Require(kind + "-schema")
	if err != nil {
		return err
	}
	if b.ID == "" {
		return errors.New("schema binding requires an indexed fixture ID")
	}
	expected := contentMap(b.Expected["cli"])
	if len(expected) == 0 {
		return errors.New("schema fixture requires independent authored CLI field manifest")
	}
	query := strings.ReplaceAll(b.Params["metadata_query"], "${id}", b.ID)
	if query == "" {
		return errors.New("schema fixture requires independent metadata query")
	}
	oracle, err := c.API.JSON(ctx, "POST", "/api/metadata/graphql", map[string]any{"query": query})
	if err != nil {
		return err
	}
	if failures, ok := oracle["errors"].([]any); ok && len(failures) > 0 {
		return harness.Blocked("schema fixture Metadata readiness query returns errors")
	}
	metadata := contentMap(b.Expected["metadata"])
	if len(metadata) == 0 {
		return errors.New("schema fixture requires independently authored metadata expectations")
	}
	if err = contentSubset(oracle, metadata); err != nil {
		return fmt.Errorf("Metadata readiness: %w", err)
	}
	out, err := contentRun(ctx, c, kind, "schema", "--id", b.ID, "--all", "--descriptions", "--tags", "--full")
	if err != nil {
		return err
	}
	return contentSubset(out, expected)
}

// contentSubset compares authored facts structurally and keeps array multiplicity.
func contentSubset(actual, expected any) error {
	switch want := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return errors.New("expected object is missing")
		}
		for key, value := range want {
			item, ok := got[key]
			if !ok {
				return fmt.Errorf("expected field %s is missing", key)
			}
			if err := contentSubset(item, value); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(want) {
			return errors.New("array count differs from independent manifest")
		}
		used := make([]bool, len(got))
		for _, value := range want {
			matched := false
			for i, item := range got {
				if !used[i] && contentSubset(item, value) == nil {
					used[i] = true
					matched = true
					break
				}
			}
			if !matched {
				return errors.New("array item differs from independent manifest")
			}
		}
	default:
		// JSON normalization avoids int/float implementation differences in authored profiles.
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(expected)
		if string(a) != string(b) {
			return errors.New("value differs from independent manifest")
		}
	}
	return nil
}
