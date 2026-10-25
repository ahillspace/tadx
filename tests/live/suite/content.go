package suite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

// ContentCases contains independent real-process content and workspace scenarios.
func ContentCases() map[string]harness.Scenario {
	cases := map[string]harness.Scenario{}
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, action := range []string{"list", "inspect", "delete", "move", "update", "pull", "publish-native", "publish-artifact", "publish-overwrite", "detached-pull", "detached-publish"} {
			cases["live."+kind+"."+action] = contentScenario(kind, action)
		}
	}
	for _, action := range []string{"create", "list", "inspect", "update", "move", "delete"} {
		cases["live.project."+action] = contentProject(action)
	}
	for _, action := range []string{"publish-append", "publish-replace", "publish-composed", "publish-large-upload", "schema"} {
		cases["live.datasource."+action] = contentScenario("datasource", action)
	}
	for _, action := range []string{"publish-large-upload", "pull-dependencies", "pull-extract-scope", "schema"} {
		cases["live.workbook."+action] = contentScenario("workbook", action)
	}
	cases["live.workspace.artifact-roundtrip"] = contentWorkspaceMove
	cases["live.workspace.delete-artifact"] = contentWorkspaceDelete
	for _, action := range []string{"inspect", "wait", "cancel"} {
		cases["live.job."+action] = contentJob(action)
	}
	return cases
}

func contentRun(ctx context.Context, c *harness.Case, kind, action string, args ...string) (map[string]any, error) {
	return c.Run(ctx, append([]string{"content", kind, action, "--environment", c.Environment}, args...)...)
}

func contentObserve(ctx context.Context, c *harness.Case, check func() error) error {
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	return harness.Observe(ctx, time.Duration(seconds)*time.Second, check)
}

func contentAwaitObject(ctx context.Context, c *harness.Case, kind, id string, expected func(harness.Object) error) (harness.Object, error) {
	var object harness.Object
	err := contentObserve(ctx, c, func() error {
		var err error
		object, err = c.API.Get(ctx, kind, id)
		if err != nil {
			return err
		}
		if object.ID != id {
			return errors.New("independent read changes authoritative identity")
		}
		if expected != nil {
			return expected(object)
		}
		return nil
	})
	return object, err
}

func contentAwaitAbsent(ctx context.Context, c *harness.Case, kind, id string) error {
	return contentObserve(ctx, c, func() error {
		_, err := c.API.Get(ctx, kind, id)
		if harness.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("deleted identity remains independently visible")
	})
}

func contentAwaitListed(ctx context.Context, c *harness.Case, expected harness.Object) error {
	return contentObserve(ctx, c, func() error {
		rows, err := c.API.List(ctx, expected.Kind, nil)
		if err != nil {
			return err
		}
		for _, o := range rows {
			if o.ID == expected.ID {
				if o.Name != expected.Name || o.ProjectID != expected.ProjectID {
					return errors.New("seeded resource inventory identity is stale")
				}
				return nil
			}
		}
		return errors.New("seeded resource is not yet visible in inventory")
	})
}

// Workbook and datasource REST inventories do not support projectId filters.
// Exhaust the independent inventory before checking authoritative direct membership.
func contentProjectInventory(ctx context.Context, c *harness.Case, kind, project string) ([]harness.Object, error) {
	objects, err := c.API.List(ctx, kind, nil)
	if err != nil {
		return nil, err
	}
	selected := make([]harness.Object, 0)
	for _, object := range objects {
		if object.ProjectID == project {
			selected = append(selected, object)
		}
	}
	return selected, nil
}

func contentScenario(kind, action string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		switch action {
		case "list":
			return contentList(ctx, c, kind)
		case "inspect":
			return contentInspect(ctx, c, kind)
		case "delete", "move", "update":
			return contentMutate(ctx, c, kind, action)
		case "pull", "pull-dependencies", "pull-extract-scope":
			_, _, err := contentPull(ctx, c, kind, action)
			return err
		case "schema":
			return contentSchema(ctx, c, kind)
		case "detached-pull", "detached-publish":
			return contentDetached(ctx, c, kind, action)
		default:
			return contentPublish(ctx, c, kind, action)
		}
	}
}

func contentBinding(c *harness.Case, kind, variant string) (harness.Binding, error) {
	key := kind + "-native"
	if variant != "" {
		key += "-" + variant
	}
	b, ok := c.Profile.Bindings[key]
	if ok && b.Path != "" {
		if len(b.Expected) == 0 {
			return b, harness.Blocked("native fixture requires independent expected payload markers: " + key)
		}
		return b, nil
	}
	if variant != "" {
		if alternate, found := c.Profile.Bindings[kind+"-"+variant]; found && alternate.Path != "" && len(alternate.Expected) > 0 {
			return alternate, nil
		}
	}
	if path := c.Profile.Artifacts[key]; path != "" {
		b.Path = path
		return b, harness.Blocked("native artifact requires authored payload expectations in binding " + key)
	}
	return b, harness.Blocked("native fixture requires a path and authored payload expectations in binding " + key)
}

func contentPublishSeed(ctx context.Context, c *harness.Case, kind string, p harness.Object, name, variant string) (harness.Object, error) {
	b, err := contentBinding(c, kind, variant)
	if err != nil {
		return harness.Object{}, err
	}
	if err = c.Intent(kind, name, p.ID); err != nil {
		return harness.Object{}, err
	}
	o, err := c.API.Publish(ctx, kind, b.Path, p.ID, name, false)
	if err != nil {
		return o, err
	}
	if err = c.ConfirmReceipt(kind, name, p.ID, o); err != nil {
		return o, err
	}
	o, err = contentAwaitObject(ctx, c, kind, o.ID, func(after harness.Object) error {
		if after.Name != name || after.ProjectID != p.ID {
			return errors.New("published fixture identity is not yet independently visible")
		}
		return nil
	})
	if err != nil {
		return o, err
	}
	if err = c.Track(o); err != nil {
		return o, err
	}
	return o, contentAwaitListed(ctx, c, o)
}

func contentPair(ctx context.Context, c *harness.Case, kind, variant string) (harness.Object, harness.Object, error) {
	o, err := c.Content(ctx, kind, variant)
	if err != nil {
		return o, harness.Object{}, err
	}
	if err = contentAwaitListed(ctx, c, o); err != nil {
		return o, harness.Object{}, err
	}
	p, err := c.Project(ctx, "decoy")
	if err != nil {
		return o, harness.Object{}, err
	}
	decoy, err := contentPublishSeed(ctx, c, kind, p, o.Name, variant)
	return o, decoy, err
}

func contentList(ctx context.Context, c *harness.Case, kind string) error {
	p, err := c.Project(ctx, "inventory")
	if err != nil {
		return err
	}
	a, err := contentPublishSeed(ctx, c, kind, p, c.NamePrefix+"-a", "")
	if err != nil {
		return err
	}
	b, err := contentPublishSeed(ctx, c, kind, p, c.NamePrefix+"-b", "")
	if err != nil {
		return err
	}
	q, err := c.Project(ctx, "decoy")
	if err != nil {
		return err
	}
	decoy, err := contentPublishSeed(ctx, c, kind, q, a.Name, "")
	if err != nil {
		return err
	}
	expected := []string{a.ID, b.ID}
	slices.Sort(expected)
	if err = contentObserve(ctx, c, func() error {
		oracle, err := contentProjectInventory(ctx, c, kind, p.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(contentObjectIDs(oracle), expected) {
			return errors.New("independent fixture inventory does not contain exactly two scoped objects")
		}
		return nil
	}); err != nil {
		return err
	}
	first, err := contentRun(ctx, c, kind, "list", "--project-id", p.ID, "--limit", "1")
	if err != nil {
		return err
	}
	ids, err := contentListIDs(first, kind+"s")
	if err != nil {
		return err
	}
	if len(ids) != 1 {
		return errors.New("first bounded page does not contain exactly one object")
	}
	if contentMap(first["page"])["more_available"] != true {
		return errors.New("bounded inventory does not disclose omitted matches")
	}
	second, err := contentRun(ctx, c, kind, "list", "--project-id", p.ID, "--all")
	if err != nil {
		return err
	}
	next, err := contentListIDs(second, kind+"s")
	if err != nil {
		return err
	}
	if !slices.Contains(next, ids[0]) {
		return errors.New("complete inventory omits bounded result")
	}
	ids = next
	slices.Sort(ids)
	if !reflect.DeepEqual(ids, expected) || slices.Contains(ids, decoy.ID) {
		return errors.New("complete inventory duplicates, omits, or leaks an independently seeded identity")
	}
	return c.Assert(contentMap(second["page"])["more_available"] == false, "complete inventory advertises omitted matches")
}

func contentInspect(ctx context.Context, c *harness.Case, kind string) error {
	var o, decoy harness.Object
	var err error
	if kind == "workbook" {
		o, decoy, err = contentRetainedWorkbookPair(ctx, c)
	} else {
		o, decoy, err = contentPair(ctx, c, kind, "")
	}
	if err != nil {
		return err
	}
	oracle, err := c.API.Get(ctx, kind, o.ID)
	if err != nil {
		return err
	}
	projectPath, err := contentProjectPath(ctx, c, o.ProjectID)
	if err != nil {
		return err
	}
	if kind == "workbook" {
		if err := contentAwaitWorkbookMetadata(ctx, c, oracle); err != nil {
			return err
		}
	}
	out, err := contentRun(ctx, c, kind, "inspect", "--name", o.Name, "--project", projectPath, "--full")
	if err != nil {
		return err
	}
	actual := contentMap(out[kind])
	if err := contentIdentity(actual, oracle); err != nil {
		return err
	}
	if kind == "workbook" && out["datasource_status"] != "complete" {
		return errors.New("workbook inspect does not report complete Metadata enrichment")
	}
	return c.Assert(contentString(actual, "luid", "id") != decoy.ID, "inspect resolves the out-of-scope decoy")
}

func contentRetainedWorkbookPair(ctx context.Context, c *harness.Case) (harness.Object, harness.Object, error) {
	objects := make([]harness.Object, 0, 2)
	for _, key := range []string{"workbook-inspect-primary", "workbook-inspect-decoy"} {
		binding, err := c.Require(key)
		if err != nil {
			return harness.Object{}, harness.Object{}, err
		}
		if binding.ID == "" || binding.Name == "" || binding.Params["project_id"] == "" {
			return harness.Object{}, harness.Object{}, harness.Blocked("setup must resolve retained binding " + key)
		}
		object, err := c.API.Get(ctx, "workbook", binding.ID)
		if err != nil {
			return harness.Object{}, harness.Object{}, err
		}
		if object.ID != binding.ID || object.Name != binding.Name || object.ProjectID != binding.Params["project_id"] {
			return harness.Object{}, harness.Object{}, errors.New("retained workbook identity differs from the setup manifest")
		}
		objects = append(objects, object)
	}
	if objects[0].ID == objects[1].ID || objects[0].Name != objects[1].Name || objects[0].ProjectID == objects[1].ProjectID {
		return harness.Object{}, harness.Object{}, errors.New("retained inspect fixtures require distinct IDs and projects with one exact shared name")
	}
	return objects[0], objects[1], nil
}

func contentAwaitWorkbookMetadata(ctx context.Context, c *harness.Case, workbook harness.Object) error {
	const query = `query LiveWorkbookReadiness($luid:String!){workbooks:workbooksConnection(filter:{luid:$luid},first:2,permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage} nodes{id luid name}}}`
	return contentObserve(ctx, c, func() error {
		data, err := c.API.Metadata(ctx, query, map[string]any{"luid": workbook.ID})
		if err != nil {
			return err
		}
		connection := contentMap(data["workbooks"])
		nodes, _ := connection["nodes"].([]any)
		if connection["totalCount"] != float64(1) || contentMap(connection["pageInfo"])["hasNextPage"] != false || len(nodes) != 1 {
			return errors.New("Metadata indexing does not yet resolve one complete workbook identity")
		}
		actual := contentMap(nodes[0])
		if contentString(actual, "id") == "" || contentString(actual, "luid") != workbook.ID || contentString(actual, "name") != workbook.Name {
			return errors.New("Metadata workbook identity differs from independently seeded REST identity")
		}
		return nil
	})
}

func contentIdentity(actual map[string]any, o harness.Object) error {
	projectKeys := []string{"project_luid", "project_id"}
	if o.Kind == "project" {
		projectKeys = []string{"parent_luid"}
	}
	for _, f := range []struct {
		want string
		keys []string
	}{{o.ID, []string{"luid", "id"}}, {o.Name, []string{"name"}}, {o.ProjectID, projectKeys}, {o.OwnerID, []string{"owner_luid", "owner_id"}}} {
		if f.want != "" && contentString(actual, f.keys...) != f.want {
			return fmt.Errorf("CLI identity field %s differs from independent REST", f.keys[0])
		}
	}
	return nil
}

func contentMutate(ctx context.Context, c *harness.Case, kind, action string) error {
	o, decoy, err := contentPair(ctx, c, kind, "")
	if err != nil {
		return err
	}
	before, err := c.API.Get(ctx, kind, o.ID)
	if err != nil {
		return err
	}
	native, err := c.API.Download(ctx, kind, o.ID)
	if err != nil {
		return err
	}
	args := []string{"--id", o.ID}
	switch action {
	case "delete":
		if _, err = contentRun(ctx, c, kind, action, append(args, "--preview")...); err != nil {
			return err
		}
		if err = contentUnchanged(ctx, c, before); err != nil {
			return fmt.Errorf("preview: %w", err)
		}
	case "move":
		destination, e := c.Project(ctx, "destination")
		if e != nil {
			return e
		}
		args = append(args, "--destination-project-id", destination.ID)
	case "update":
		owner, e := c.Require("alternate-owner")
		if e != nil {
			return e
		}
		args = append(args, "--owner-id", owner.ID)
		if kind != "flow" {
			args = append(args, "--new-name", c.NamePrefix+"-updated")
		}
		if kind == "workbook" {
			args = append(args, "--description", "Live fixture description")
		}
	}
	out, err := contentRun(ctx, c, kind, action, args...)
	if err != nil {
		return err
	}
	if !contentHasIdentity(out, o.ID) {
		return errors.New("mutation receipt omits the selected authoritative identity")
	}
	if action == "delete" {
		if err = contentAwaitAbsent(ctx, c, kind, o.ID); err != nil {
			return err
		}
	} else {
		_, e := contentAwaitObject(ctx, c, kind, o.ID, func(after harness.Object) error {
			if after.ID != o.ID {
				return errors.New("mutation replaced the selected identity")
			}
			if action == "move" {
				if after.ProjectID != args[len(args)-1] {
					return errors.New("move did not reach the exact destination")
				}
				old, e := contentProjectInventory(ctx, c, kind, before.ProjectID)
				if e != nil {
					return e
				}
				if slices.Contains(contentObjectIDs(old), o.ID) {
					return errors.New("moved identity remains in source inventory")
				}
				if after.Name != before.Name || after.OwnerID != before.OwnerID || after.Description != before.Description {
					return errors.New("move changes unrelated metadata")
				}
			} else {
				owner, _ := c.Require("alternate-owner")
				wantedName := c.NamePrefix + "-updated"
				if kind == "flow" {
					wantedName = before.Name
				}
				if after.Name != wantedName || after.OwnerID != owner.ID || after.ProjectID != before.ProjectID {
					return errors.New("metadata update does not match requested owner, name, and preserved project")
				}
				if kind == "workbook" && after.Description != "Live fixture description" {
					return errors.New("description update does not persist")
				}
			}
			return nil
		})
		if e != nil {
			return e
		}
		downloaded, e := c.API.Download(ctx, kind, o.ID)
		if e != nil {
			return e
		}
		if action == "move" {
			if e = harness.CompareNative(kind, native.Bytes, downloaded.Bytes); e != nil {
				return fmt.Errorf("move changed payload: %w", e)
			}
		} else {
			binding, e := contentBinding(c, kind, "")
			if e != nil {
				return e
			}
			if e = harness.CheckNative(ctx, kind, downloaded.Bytes, binding); e != nil {
				return fmt.Errorf("metadata mutation changes authored payload: %w", e)
			}
		}
	}
	return contentUnchanged(ctx, c, decoy)
}

func contentUnchanged(ctx context.Context, c *harness.Case, before harness.Object) error {
	_, err := contentAwaitObject(ctx, c, before.Kind, before.ID, func(after harness.Object) error {
		if after.ID != before.ID || after.Name != before.Name || after.ProjectID != before.ProjectID || after.OwnerID != before.OwnerID || after.Description != before.Description {
			return errors.New("unrelated object identity or metadata changes")
		}
		return nil
	})
	return err
}

func contentProjectPath(ctx context.Context, c *harness.Case, id string) (string, error) {
	var parts []string
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return "", errors.New("independent project hierarchy contains cycle")
		}
		seen[id] = true
		o, err := contentAwaitObject(ctx, c, "project", id, nil)
		if err != nil {
			return "", err
		}
		parts = append(parts, o.Name)
		id = contentString(o.Fields, "parentProjectId", "parent_id", "parent_luid")
	}
	slices.Reverse(parts)
	return strings.Join(parts, "/"), nil
}

func contentMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func contentString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
func contentObjectIDs(objects []harness.Object) []string {
	ids := make([]string, len(objects))
	for i, o := range objects {
		ids[i] = o.ID
	}
	slices.Sort(ids)
	return ids
}
func contentListIDs(out map[string]any, key string) ([]string, error) {
	rows, ok := out[key].([]any)
	if !ok {
		return nil, fmt.Errorf("CLI omits %s array", key)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		id := contentString(contentMap(row), "luid", "id")
		if id == "" {
			return nil, errors.New("CLI list record omits identity")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
func contentHasIdentity(v any, id string) bool {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			if slices.Contains([]string{"id", "luid", "tableau_id", "source_luid", "resource_id", "workbook_luid", "datasource_luid", "flow_luid", "project_luid"}, key) && item == id {
				return true
			}
			if contentHasIdentity(item, id) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if contentHasIdentity(item, id) {
				return true
			}
		}
	}
	return false
}

func contentReadJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}
func contentSafePath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
		return "", errors.New("artifact path is not workspace-relative with forward slashes")
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes workspace")
	}
	return path, nil
}
func contentSnapshot(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("artifact contains symbolic link")
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	return files, err
}
