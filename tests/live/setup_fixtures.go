package live

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

// PrepareInspectFixtures builds the retained workbook inspection pair.
// It completes REST and native verification before returning checkpoint bindings.
// Metadata indexing is a separate phase so a slow index cannot lose ownership.
func PrepareInspectFixtures(ctx context.Context, c *harness.Case) error {
	if c == nil || c.API == nil || c.Journal == nil || c.Root == "" {
		return harness.Blocked("workbook inspection setup requires its shared root and ownership journal")
	}
	binding := c.Profile.Bindings["workbook-native"]
	if binding.Path == "" {
		binding.Path = c.Profile.Artifacts["workbook-native"]
	}
	if binding.Path == "" {
		return harness.Blocked("workbook inspection setup requires workbook-native")
	}
	native, err := os.ReadFile(binding.Path)
	if err != nil {
		return err
	}
	if err := harness.CheckNative(ctx, "workbook", native, binding); err != nil {
		return fmt.Errorf("verify authored inspection fixture: %w", err)
	}
	if c.Profile.Bindings == nil {
		c.Profile.Bindings = map[string]harness.Binding{}
	}
	name := c.NamePrefix + "-workbook-inspect"
	for _, role := range []string{"primary", "decoy"} {
		projectName := c.NamePrefix + "-inspect-" + role + "-project"
		project, found, err := inspectOwnedReceipt(c, "project", projectName, c.Root)
		if err != nil {
			return err
		}
		if !found {
			if err := c.Intent("project", projectName, c.Root); err != nil {
				return err
			}
			project, err = c.API.Create(ctx, "project", map[string]any{"name": projectName, "parentProjectId": c.Root, "contentPermissions": "ManagedByOwner"})
			if err != nil {
				return err
			}
			if err := c.ConfirmReceipt("project", projectName, c.Root, project); err != nil {
				return err
			}
		}
		project, err = awaitInspectObject(ctx, c, "project", project, projectName, c.Root, nil)
		if err != nil {
			return err
		}
		workbook, found, err := inspectOwnedReceipt(c, "workbook", name, project.ID)
		if err != nil {
			return err
		}
		if !found {
			if err := c.Intent("workbook", name, project.ID); err != nil {
				return err
			}
			workbook, err = c.API.Publish(ctx, "workbook", binding.Path, project.ID, name, false)
			if err != nil {
				return err
			}
			if err := c.ConfirmReceipt("workbook", name, project.ID, workbook); err != nil {
				return err
			}
		}
		workbook, err = awaitInspectObject(ctx, c, "workbook", workbook, name, project.ID, &binding)
		if err != nil {
			return err
		}
		params := maps.Clone(binding.Params)
		if params == nil {
			params = map[string]string{}
		}
		params["project_id"] = project.ID
		c.Profile.Bindings["workbook-inspect-"+role] = harness.Binding{ID: workbook.ID, Name: name, Path: binding.Path, Params: params, Expected: maps.Clone(binding.Expected)}
	}
	return nil
}

func inspectOwnedReceipt(c *harness.Case, kind, name, parent string) (harness.Object, bool, error) {
	var matches []harness.Entry
	for _, entry := range c.Journal.Entries {
		if entry.Method == "" && entry.Kind == kind && entry.Name == name && entry.Parent == parent {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return harness.Object{}, false, nil
	}
	if len(matches) != 1 {
		return harness.Object{}, false, harness.Inconclusive("inspection fixture has multiple ownership records; retain setup state for cleanup")
	}
	entry := matches[0]
	if entry.Done {
		return harness.Object{}, false, harness.Blocked("inspection fixture pack was removed; clean up and rebuild setup")
	}
	if entry.Object == nil || entry.Object.ID == "" {
		return harness.Object{}, false, harness.Inconclusive("inspection fixture creation is uncertain; retain its intent and reconcile cleanup before rebuilding")
	}
	if entry.Object.Kind != kind {
		return harness.Object{}, false, errors.New("inspection fixture receipt has a different resource kind")
	}
	return *entry.Object, true, nil
}

func awaitInspectObject(ctx context.Context, c *harness.Case, kind string, receipt harness.Object, name, parent string, native *harness.Binding) (harness.Object, error) {
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	budget := time.Duration(seconds) * time.Second
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var observed harness.Object
	err := harness.Observe(bounded, budget, func() error {
		object, err := c.API.Get(bounded, kind, receipt.ID)
		if err != nil {
			return err
		}
		if object.ID != receipt.ID || object.Name != name || object.ProjectID != parent {
			return harness.Blocked("owned inspection fixture identity or parent changed; clean up and rebuild the immutable pack")
		}
		if native != nil {
			download, err := c.API.Download(bounded, "workbook", receipt.ID)
			if err != nil {
				return err
			}
			if err := harness.CheckNative(bounded, "workbook", download.Bytes, *native); err != nil {
				return harness.Blocked("owned inspection fixture payload differs from its authored pack; clean up and rebuild")
			}
		}
		observed = object
		return nil
	})
	if err != nil {
		return receipt, err
	}
	if err := c.Track(observed); err != nil {
		return receipt, err
	}
	return observed, nil
}

// AwaitInspectFixture checks exact primary identity after setup saves its checkpoint.
// This operation only observes; it never creates or repairs remote content.
func AwaitInspectFixture(ctx context.Context, c *harness.Case) error {
	if c == nil || c.API == nil || c.Journal == nil {
		return harness.Blocked("inspection readiness requires the retained setup context")
	}
	binding, ok := c.Profile.Bindings["workbook-inspect-primary"]
	if !ok || binding.ID == "" || binding.Name == "" || binding.Params["project_id"] == "" {
		return harness.Blocked("inspection readiness requires a prepared primary binding")
	}
	receipt, found, err := inspectOwnedReceipt(c, "workbook", binding.Name, binding.Params["project_id"])
	if err != nil {
		return err
	}
	if !found || receipt.ID != binding.ID {
		return harness.Blocked("inspection readiness binding is not owned by its exact setup journal")
	}
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	budget := time.Duration(seconds) * time.Second
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return harness.Observe(bounded, budget, func() error {
		object, err := c.API.Get(bounded, "workbook", binding.ID)
		if err != nil {
			return err
		}
		if object.ID != binding.ID || object.Name != binding.Name || object.ProjectID != binding.Params["project_id"] {
			return harness.Blocked("retained inspection fixture changed after preparation")
		}
		data, err := c.API.Metadata(bounded, `query TadxLiveInspectReadiness($luid: String!) { workbooksConnection(first: 2, filter: {luid: $luid}) { totalCount nodes { luid name } pageInfo { hasNextPage } } }`, map[string]any{"luid": binding.ID})
		if err != nil {
			return err
		}
		connection, ok := data["workbooksConnection"].(map[string]any)
		if !ok {
			return errors.New("inspection Metadata readiness omitted its workbook connection")
		}
		nodes, ok := connection["nodes"].([]any)
		if !ok || len(nodes) != 1 || connection["totalCount"] != float64(1) {
			return harness.Inconclusive("retained workbook is not yet indexed by exact LUID")
		}
		page, _ := connection["pageInfo"].(map[string]any)
		if next, ok := page["hasNextPage"].(bool); !ok || next {
			return errors.New("inspection Metadata readiness omitted complete bounded coverage")
		}
		node, _ := nodes[0].(map[string]any)
		if node["luid"] != binding.ID || node["name"] != binding.Name {
			return harness.Inconclusive("retained workbook Metadata identity is not yet current")
		}
		return nil
	})
}

// VerifyInspectFixtures observes the retained pair without creating or repairing it.
// The Metadata query POST is read-only; no remote mutation occurs here.
func VerifyInspectFixtures(ctx context.Context, c *harness.Case) error {
	if c == nil || c.API == nil || c.Journal == nil || c.Root == "" {
		return harness.Blocked("inspection verification requires its retained setup context")
	}
	native := c.Profile.Bindings["workbook-native"]
	if native.Path == "" {
		native.Path = c.Profile.Artifacts["workbook-native"]
	}
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	var primary harness.Binding
	for _, role := range []string{"primary", "decoy"} {
		binding, ok := c.Profile.Bindings["workbook-inspect-"+role]
		name := c.NamePrefix + "-workbook-inspect"
		if !ok || binding.ID == "" || binding.Name != name || binding.Params["project_id"] == "" {
			return harness.Blocked("inspection verification requires both immutable fixture bindings")
		}
		project, found, err := inspectOwnedReceipt(c, "project", c.NamePrefix+"-inspect-"+role+"-project", c.Root)
		if err != nil {
			return err
		}
		if !found || project.ID != binding.Params["project_id"] {
			return harness.Blocked("inspection fixture project is not owned by its exact setup journal")
		}
		workbook, found, err := inspectOwnedReceipt(c, "workbook", name, project.ID)
		if err != nil {
			return err
		}
		if !found || workbook.ID != binding.ID {
			return harness.Blocked("inspection fixture workbook is not owned by its exact setup journal")
		}
		actual, err := c.API.Get(bounded, "project", project.ID)
		if err != nil {
			return err
		}
		if actual.Name != c.NamePrefix+"-inspect-"+role+"-project" || actual.ProjectID != c.Root {
			return harness.Blocked("inspection fixture project changed; clean up and rebuild")
		}
		actual, err = c.API.Get(bounded, "workbook", binding.ID)
		if err != nil {
			return err
		}
		if actual.Name != name || actual.ProjectID != project.ID {
			return harness.Blocked("inspection fixture workbook changed; clean up and rebuild")
		}
		download, err := c.API.Download(bounded, "workbook", binding.ID)
		if err != nil {
			return err
		}
		if err := harness.CheckNative(bounded, "workbook", download.Bytes, native); err != nil {
			return harness.Blocked("inspection fixture payload changed; clean up and rebuild")
		}
		if role == "primary" {
			primary = binding
		} else if binding.ID == primary.ID || binding.Params["project_id"] == primary.Params["project_id"] {
			return harness.Blocked("inspection fixtures require distinct owned workbooks and projects")
		}
	}
	return AwaitInspectFixture(bounded, c)
}
