package suite

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func contentCreateProject(ctx context.Context, c *harness.Case, name, parent string) (harness.Object, error) {
	if err := c.Intent("project", name, parent); err != nil {
		return harness.Object{}, err
	}
	fields := map[string]any{"name": name, "description": "Live project fixture", "contentPermissions": "ManagedByOwner"}
	if parent != "" {
		fields["parentProjectId"] = parent
	}
	o, err := c.API.Create(ctx, "project", fields)
	if err != nil {
		return o, err
	}
	if err = c.ConfirmReceipt("project", name, parent, o); err != nil {
		return o, err
	}
	return contentAwaitObject(ctx, c, "project", o.ID, func(after harness.Object) error {
		if after.Name != name || contentParent(after) != parent || after.Description != "Live project fixture" || contentString(after.Fields, "contentPermissions") != "ManagedByOwner" {
			return errors.New("seeded project metadata is not yet visible")
		}
		return nil
	})
}

func contentObserveCreatedProject(ctx context.Context, c *harness.Case, parent harness.Object, name string, out map[string]any) error {
	receipt := contentMap(contentMap(out["result"])["project"])
	id := contentString(receipt, "luid", "id")
	if id == "" || contentString(receipt, "name") != name || contentString(receipt, "parent_luid") != parent.ID {
		return errors.New("project create receipt omits or changes authoritative requested identity")
	}
	// Acceptance precedes eventually consistent inventory visibility.
	if err := c.Track(harness.Object{Kind: "project", ID: id, Name: name, ProjectID: parent.ID}); err != nil {
		return err
	}
	err := contentObserve(ctx, c, func() error {
		rows, err := c.API.List(ctx, "project", nil)
		if err != nil {
			return err
		}
		matches := 0
		for _, o := range rows {
			if o.Name != name || contentParent(o) != parent.ID {
				continue
			}
			matches++
			if o.ID != id || o.Description != "Live project fixture" || contentString(o.Fields, "contentPermissions") != "ManagedByOwner" {
				return errors.New("created project metadata or receipt differs from independent REST")
			}
			if err = c.Track(o); err != nil {
				return err
			}
		}
		if matches != 1 {
			return fmt.Errorf("independent inventory finds %d created children", matches)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return contentUnchanged(ctx, c, parent)
}

func contentProject(action string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		parent, err := c.Project(ctx, "parent")
		if err != nil {
			return err
		}
		if action == "create" {
			name := c.NamePrefix + "-child"
			if err = c.Intent("project", name, parent.ID); err != nil {
				return err
			}
			before, err := c.API.List(ctx, "project", nil)
			if err != nil {
				return err
			}
			for _, o := range before {
				if o.Name == name {
					return errors.New("project create name exists before submission")
				}
			}
			out, err := contentRun(ctx, c, "project", "create", "--parent-id", parent.ID, "--name", name, "--description", "Live project fixture", "--content-permissions", "ManagedByOwner")
			if err != nil {
				return err
			}
			return contentObserveCreatedProject(ctx, c, parent, name, out)
		}
		child, err := contentCreateProject(ctx, c, c.NamePrefix+"-child", parent.ID)
		if err != nil {
			return err
		}
		q, err := c.Project(ctx, "other-parent")
		if err != nil {
			return err
		}
		decoy, err := contentCreateProject(ctx, c, child.Name, q.ID)
		if err != nil {
			return err
		}
		switch action {
		case "list":
			second, err := contentCreateProject(ctx, c, c.NamePrefix+"-second", parent.ID)
			if err != nil {
				return err
			}
			if err = contentObserve(ctx, c, func() error {
				rows, err := c.API.List(ctx, "project", nil)
				if err != nil {
					return err
				}
				ids := []string{}
				for _, o := range rows {
					if contentParent(o) == parent.ID {
						ids = append(ids, o.ID)
					}
				}
				slices.Sort(ids)
				want := []string{child.ID, second.ID}
				slices.Sort(want)
				if !slices.Equal(ids, want) {
					return errors.New("seeded child inventory is not yet complete")
				}
				return nil
			}); err != nil {
				return err
			}
			out, err := contentRun(ctx, c, "project", "list", "--parent-id", parent.ID, "--all", "--full")
			if err != nil {
				return err
			}
			got, err := contentListIDs(out, "projects")
			if err != nil {
				return err
			}
			slices.Sort(got)
			want := []string{child.ID, second.ID}
			slices.Sort(want)
			if !reflect.DeepEqual(got, want) {
				return errors.New("project list differs from exact independent child set")
			}
			rows, _ := out["projects"].([]any)
			for _, raw := range rows {
				if contentString(contentMap(raw), "parent_luid") != parent.ID {
					return errors.New("project list changes parent identity")
				}
			}
		case "inspect":
			path, err := contentProjectPath(ctx, c, child.ID)
			if err != nil {
				return err
			}
			out, err := contentRun(ctx, c, "project", "inspect", "--project", path, "--full")
			if err != nil {
				return err
			}
			oracle, err := c.API.Get(ctx, "project", child.ID)
			if err != nil {
				return err
			}
			actual := contentMap(out["project"])
			if err = contentIdentity(actual, oracle); err != nil {
				return err
			}
			if contentString(actual, "parent_luid") != parent.ID || contentString(actual, "description") != oracle.Description || contentString(actual, "content_permissions") != contentString(oracle.Fields, "contentPermissions") {
				return errors.New("project inspection metadata differs from independent REST")
			}
		case "update":
			out, err := contentRun(ctx, c, "project", "update", "--id", child.ID, "--new-name", c.NamePrefix+"-updated", "--description", "Updated live fixture", "--content-permissions", "LockedToProject")
			if err != nil {
				return err
			}
			if !contentHasIdentity(out, child.ID) {
				return errors.New("project update receipt loses identity")
			}
			if _, err = contentAwaitObject(ctx, c, "project", child.ID, func(after harness.Object) error {
				if after.Name != c.NamePrefix+"-updated" || after.Description != "Updated live fixture" || contentParent(after) != parent.ID || contentString(after.Fields, "contentPermissions") != "LockedToProject" {
					return errors.New("project update fields differ from requested metadata")
				}
				return nil
			}); err != nil {
				return err
			}
		case "delete":
			if _, err = contentRun(ctx, c, "project", "delete", "--id", child.ID, "--preview"); err != nil {
				return err
			}
			if err = contentUnchanged(ctx, c, child); err != nil {
				return err
			}
			out, err := contentRun(ctx, c, "project", "delete", "--id", child.ID)
			if err != nil {
				return err
			}
			if !contentHasIdentity(out, child.ID) {
				return errors.New("delete receipt loses project identity")
			}
			if err = contentAwaitAbsent(ctx, c, "project", child.ID); err != nil {
				return err
			}
		case "move":
			destination, err := c.Project(ctx, "move-destination")
			if err != nil {
				return err
			}
			grandchild, err := contentCreateProject(ctx, c, c.NamePrefix+"-grandchild", child.ID)
			if err != nil {
				return err
			}
			workbook, err := contentPublishSeed(ctx, c, "workbook", grandchild, c.NamePrefix+"-workbook", "")
			if err != nil {
				return err
			}
			oldPath, err := contentProjectPath(ctx, c, grandchild.ID)
			if err != nil {
				return err
			}
			if _, err = contentRun(ctx, c, "project", "move", "--id", child.ID, "--parent-id", destination.ID); err != nil {
				return err
			}
			if _, err = contentAwaitObject(ctx, c, "project", child.ID, func(moved harness.Object) error {
				if contentParent(moved) != destination.ID {
					return errors.New("project does not move to exact parent")
				}
				return nil
			}); err != nil {
				return err
			}
			if err = contentObserve(ctx, c, func() error {
				after, err := c.API.Get(ctx, "project", grandchild.ID)
				if err != nil {
					return err
				}
				if contentParent(after) != child.ID {
					return errors.New("project move rewrites descendant parent")
				}
				if err = contentUnchanged(ctx, c, workbook); err != nil {
					return err
				}
				newPath, err := contentProjectPath(ctx, c, grandchild.ID)
				if err != nil {
					return err
				}
				if newPath == oldPath {
					return errors.New("independent hierarchy path does not change after move")
				}
				out, err := contentRun(ctx, c, "project", "inspect", "--id", grandchild.ID, "--full")
				if err != nil {
					return err
				}
				if contentString(contentMap(out["project"]), "path") != newPath {
					return errors.New("CLI descendant path remains stale after project move")
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if err = contentUnchanged(ctx, c, parent); err != nil {
			return err
		}
		return contentUnchanged(ctx, c, decoy)
	}
}

func contentParent(o harness.Object) string {
	return contentString(o.Fields, "parentProjectId", "parent_id", "parent_luid")
}
