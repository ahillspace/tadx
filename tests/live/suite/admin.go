package suite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/tests/live/harness"
)

// AdminCases exercises administration through the installed CLI and reads the
// independent REST client for every remote postcondition.
func AdminCases() map[string]harness.Scenario {
	cases := map[string]harness.Scenario{}
	for _, verb := range []string{"list", "inspect", "create", "update", "delete", "converge-members"} {
		cases["live.admin-group."+verb] = adminGroup(verb)
	}
	for _, verb := range []string{"list", "inspect", "create", "update", "delete"} {
		cases["live.admin-user."+verb] = adminUser(verb)
	}
	for _, verb := range []string{"add", "remove"} {
		cases["live.admin-group-member."+verb] = adminMember(verb)
	}
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "defaults-workbooks", "defaults-datasources", "defaults-flows"} {
		cases["live.admin-permission."+kind] = adminPermission(kind)
	}
	for _, verb := range []string{"list", "inspect", "create", "update", "delete"} {
		cases["live.admin-label-category."+verb] = adminLabel("labelCategory", verb)
	}
	for _, verb := range []string{"list", "inspect", "upsert", "delete"} {
		cases["live.admin-label-value."+verb] = adminLabel("labelValue", verb)
	}
	return cases
}

func adminGroup(verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		if verb == "create" {
			return adminCreate(ctx, c, "group", []string{"admin", "group", "create", "--name", c.NamePrefix + "group"}, c.NamePrefix+"group", nil)
		}
		g, err := c.Seed(ctx, "group", "subject")
		if err != nil {
			return err
		}
		a, err := c.Require("user_a")
		if err != nil {
			return err
		}
		if err = addMember(ctx, c, g.ID, a.ID); err != nil {
			return err
		}
		before, err := members(ctx, c, g.ID)
		if err != nil {
			return err
		}
		args := []string{"admin", "group", verb, "--id", g.ID}
		switch verb {
		case "list":
			args = []string{"admin", "group", "list", "--name", g.Name, "--all"}
		case "inspect":
			args = append(args, "--members")
		case "update":
			args = append(args, "--new-name", g.Name+"-renamed")
		case "converge-members":
			b, e := c.Require("user_b")
			if e != nil {
				return e
			}
			if _, e = fullRun(ctx, c, "admin", "group", "update", "--id", g.ID, "--set-members", "--member-id", b.ID); e != nil {
				return e
			}
			if e = wantMembers(ctx, c, g.ID, []string{b.ID}); e != nil {
				return e
			}
			if _, e = fullRun(ctx, c, "admin", "group", "update", "--id", g.ID, "--set-members"); e != nil {
				return e
			}
			if e = wantMembers(ctx, c, g.ID, nil); e != nil {
				return e
			}
			_, e = c.API.Get(ctx, "user", b.ID)
			return e
		}
		out, err := fullRun(ctx, c, args...)
		if err != nil {
			return err
		}
		if verb == "delete" {
			if err = absent(ctx, c, "group", g.ID); err != nil {
				return err
			}
			_, err = c.API.Get(ctx, "user", a.ID)
			return err
		}
		fresh, err := c.API.Get(ctx, "group", g.ID)
		if err != nil {
			return err
		}
		name := g.Name
		if verb == "update" {
			name += "-renamed"
		}
		for _, key := range []string{"domain", "minimumSiteRole", "grantLicenseMode", "externalUserEnabled"} {
			if !jsonEqual(g.Fields[key], fresh.Fields[key]) {
				return fmt.Errorf("group update changed unrelated %s", key)
			}
		}
		if fresh.Name != name {
			return fmt.Errorf("group name %q, want %q", fresh.Name, name)
		}
		if err = wantMembers(ctx, c, g.ID, before); err != nil {
			return err
		}
		if verb == "inspect" {
			row, e := record(out, g.ID)
			if e != nil {
				return e
			}
			if e = field(row, "name", fresh.Name); e != nil {
				return e
			}
			return equalIDs(array(row["members"]), before)
		}
		if verb == "list" {
			rows := array(out["groups"])
			if len(rows) != 1 {
				return fmt.Errorf("exact-name group list returned %d records", len(rows))
			}
			row, e := record(rows, g.ID)
			if e != nil {
				return e
			}
			if e = field(row, "name", fresh.Name); e != nil {
				return e
			}
			if domain, ok := fresh.Fields["domain"].(map[string]any); ok {
				return field(row, "domain", domain["name"])
			}
		}
		return nil
	}
}

func adminUser(verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		if verb == "list" || verb == "inspect" {
			b, e := c.Require("user_a")
			if e != nil {
				return e
			}
			fresh, e := c.API.Get(ctx, "user", b.ID)
			if e != nil {
				return e
			}
			args := []string{"admin", "user", verb, "--id", b.ID}
			if verb == "list" {
				args = []string{"admin", "user", "list", "--name", fresh.Name, "--all"}
			}
			out, e := fullRun(ctx, c, args...)
			if e != nil {
				return e
			}
			var rows any = out
			if verb == "list" {
				rows = out["users"]
				if len(array(rows)) != 1 {
					return fmt.Errorf("exact-name user inventory must contain one record")
				}
			}
			row, e := record(rows, b.ID)
			if e != nil {
				return e
			}
			if e = field(row, "name", fresh.Name); e != nil {
				return e
			}
			return field(row, "site_role", fresh.Fields["siteRole"])
		}
		b, e := c.Require("controlled_user")
		if e != nil {
			return e
		}
		if b.Name == "" || b.Params["auth_setting"] == "" {
			return harness.Blocked("controlled_user requires Name and auth_setting")
		}
		if verb == "create" {
			login, err := c.ControlledUserLogin("admin-user-create")
			if err != nil {
				return err
			}
			args := []string{"admin", "user", "create", "--name", login, "--site-role", "Unlicensed", "--auth-setting", b.Params["auth_setting"]}
			return adminCreate(ctx, c, "user", args, login, map[string]any{"siteRole": "Unlicensed", "authSetting": b.Params["auth_setting"]})
		}
		u, e := c.Seed(ctx, "user", "subject")
		if e != nil {
			return e
		}
		before, e := c.API.Get(ctx, "user", u.ID)
		if e != nil {
			return e
		}
		args := []string{"admin", "user", verb, "--id", u.ID}
		if verb == "update" {
			if before.Fields["siteRole"] == "Viewer" {
				return fmt.Errorf("user update requires initial role different from Viewer")
			}
			args = append(args, "--site-role", "Viewer")
		}
		out, e := fullRun(ctx, c, args...)
		if e != nil {
			return e
		}
		if verb == "delete" {
			if e = absent(ctx, c, "user", u.ID); e != nil {
				return e
			}
			sentinel, e := c.Require("user_a")
			if e != nil {
				return e
			}
			_, e = c.API.Get(ctx, "user", sentinel.ID)
			return e
		}
		after, e := c.API.Get(ctx, "user", u.ID)
		if e != nil {
			return e
		}
		for _, key := range []string{"fullName", "email", "authSetting", "identityPoolName", "idpConfigurationId", "language", "locale"} {
			if !jsonEqual(before.Fields[key], after.Fields[key]) {
				return fmt.Errorf("user role update changed unrelated %s", key)
			}
		}
		if after.Name != before.Name {
			return fmt.Errorf("role update changed login")
		}
		if after.Fields["siteRole"] != "Viewer" {
			return fmt.Errorf("role did not persist")
		}
		_, e = record(out, u.ID)
		return e
	}
}

func adminMember(verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		g, e := c.Seed(ctx, "group", "members")
		if e != nil {
			return e
		}
		a, e := c.Require("user_a")
		if e != nil {
			return e
		}
		b, e := c.Require("user_b")
		if e != nil {
			return e
		}
		if a.ID == b.ID {
			return fmt.Errorf("member fixtures must have distinct identities")
		}
		if e = addMember(ctx, c, g.ID, a.ID); e != nil {
			return e
		}
		args := []string{"admin", "group-member", verb, "--group-id", g.ID, "--user-id", b.ID}
		want := []string{a.ID, b.ID}
		if verb == "remove" {
			if e = addMember(ctx, c, g.ID, b.ID); e != nil {
				return e
			}
			u, e := c.API.Get(ctx, "user", b.ID)
			if e != nil {
				return e
			}
			args = []string{"admin", "group-member", verb, "--group-id", g.ID, "--username", u.Name}
			want = []string{a.ID}
		}
		if _, e = fullRun(ctx, c, args...); e != nil {
			return e
		}
		if e = wantMembers(ctx, c, g.ID, want); e != nil {
			return e
		}
		_, e = c.API.Get(ctx, "user", b.ID)
		return e
	}
}

func adminCreate(ctx context.Context, c *harness.Case, kind string, args []string, name string, expected map[string]any) error {
	before, e := c.API.List(ctx, kind, nil)
	if e != nil {
		return e
	}
	for _, o := range before {
		if o.Name == name {
			return fmt.Errorf("creation name already exists")
		}
	}
	if kind == "user" {
		login, err := c.IntentControlledUser("admin-user-create")
		if err != nil {
			return err
		}
		if login != name {
			return errors.New("controlled user name changed before its ownership intent")
		}
	} else if e = c.Intent(kind, name, ""); e != nil {
		return e
	}
	out, runErr := fullRun(ctx, c, args...)
	after, e := c.API.List(ctx, kind, nil)
	if e != nil {
		return e
	}
	var created []harness.Object
	for _, o := range after {
		if o.Name == name {
			created = append(created, o)
			if e = c.Track(o); e != nil {
				return e
			}
		}
	}
	if runErr != nil {
		return runErr
	}
	if len(created) != 1 {
		return fmt.Errorf("expected one created %s, found %d", kind, len(created))
	}
	o := created[0]
	if kind == "labelCategory" || kind == "labelValue" {
		_, e = recordByName(out, o.Name)
	} else if kind == "group" {
		receipt := asObject(out["result"])
		if e = field(receipt, "group_luid", o.ID); e == nil {
			e = field(receipt, "status", "created")
		}
	} else {
		_, e = record(out, o.ID)
	}
	if e != nil {
		return e
	}
	for k, v := range expected {
		if e = field(o.Fields, k, v); e != nil {
			return e
		}
	}
	for _, old := range before {
		fresh, e := c.API.Get(ctx, kind, old.ID)
		if e != nil {
			return e
		}
		if fresh.Name != old.Name {
			return fmt.Errorf("creation renamed existing object")
		}
		if kind == "user" && fresh.Fields["siteRole"] != old.Fields["siteRole"] {
			return fmt.Errorf("creation changed existing user role")
		}
	}
	return nil
}

func adminLabel(kind, verb string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		command := "label-category"
		if kind == "labelValue" {
			command = "label-value"
		}
		if verb == "create" {
			return adminCreate(ctx, c, kind, []string{"admin", command, "create", "--name", c.NamePrefix + "category", "--description", "Live suite category"}, c.NamePrefix+"category", map[string]any{"description": "Live suite category"})
		}
		if verb == "upsert" {
			category, e := c.Seed(ctx, "labelCategory", "parent")
			if e != nil {
				return e
			}
			name := c.NamePrefix + "value"
			args := []string{"admin", command, "update", "--name", name, "--category", category.Name, "--description", "Live value"}
			if e = adminCreate(ctx, c, kind, args, name, map[string]any{"category": category.Name, "description": "Live value"}); e != nil {
				return e
			}
			if _, e = fullRun(ctx, c, "admin", command, "update", "--name", name, "--description", "Updated live value"); e != nil {
				return e
			}
			o, e := named(ctx, c, kind, name)
			if e != nil {
				return e
			}
			if e = field(o.Fields, "description", "Updated live value"); e != nil {
				return e
			}
			return field(o.Fields, "category", category.Name)
		}
		o, e := c.Seed(ctx, kind, "subject")
		if e != nil {
			return e
		}
		sentinel, e := c.Seed(ctx, kind, "sentinel")
		if e != nil {
			return e
		}
		args := []string{"admin", command, verb, "--name", o.Name}
		if verb == "list" {
			args = []string{"admin", command, "list", "--all"}
		}
		newName := o.Name
		if verb == "update" {
			newName += "-renamed"
			args = append(args, "--new-name", newName, "--description", "Updated live category")
			if e = c.Intent(kind, newName, ""); e != nil {
				return e
			}
		}
		out, e := fullRun(ctx, c, args...)
		if verb == "update" {
			if changed, findErr := named(ctx, c, kind, newName); findErr == nil {
				if trackErr := c.Track(changed); trackErr != nil {
					return trackErr
				}
			}
		}
		if e != nil {
			return e
		}
		if _, e = named(ctx, c, kind, sentinel.Name); e != nil {
			return fmt.Errorf("sentinel label definition missing: %w", e)
		}
		if verb == "delete" {
			items, e := c.API.List(ctx, kind, nil)
			if e != nil {
				return e
			}
			for _, item := range items {
				if item.Name == o.Name {
					return fmt.Errorf("deleted label definition remains")
				}
			}
			return nil
		}
		fresh, e := named(ctx, c, kind, newName)
		if e != nil {
			return e
		}
		if verb == "update" {
			return field(fresh.Fields, "description", "Updated live category")
		}
		row, e := recordByName(out, o.Name)
		if e != nil {
			return e
		}
		if e = field(row, "description", fresh.Fields["description"]); e != nil {
			return e
		}
		if kind == "labelValue" {
			return field(row, "category", fresh.Fields["category"])
		}
		return nil
	}
}

func adminPermission(variant string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		kind := variant
		defaults := ""
		if strings.HasPrefix(variant, "defaults-") {
			kind = "project"
			defaults = strings.TrimPrefix(variant, "defaults-")
		}
		var resource harness.Object
		var e error
		if kind == "project" {
			resource, e = c.Project(ctx, "permissions")
		} else {
			resource, e = c.Content(ctx, kind, "native")
		}
		if e != nil {
			return e
		}
		group, e := c.Seed(ctx, "group", "principal")
		if e != nil {
			return e
		}
		sentinel, e := c.Seed(ctx, "group", "sentinel")
		if e != nil {
			return e
		}
		path := c.API.REST(kind + "s/" + url.PathEscape(resource.ID) + "/permissions")
		if defaults != "" {
			path = c.API.REST("projects/" + url.PathEscape(resource.ID) + "/default-permissions/" + defaults)
		}
		payload := map[string]any{"permissions": map[string]any{"granteeCapabilities": []any{map[string]any{"group": map[string]any{"id": sentinel.ID}, "capabilities": map[string]any{"capability": []any{map[string]any{"name": "Read", "mode": "Deny"}}}}}}}
		if _, e = c.API.JSON(ctx, http.MethodPut, path, payload); e != nil {
			return e
		}
		before, e := c.API.JSON(ctx, http.MethodGet, path, nil)
		if e != nil {
			return e
		}
		if !permissionPresent(before, sentinel.ID, "Read", "Deny") {
			return fmt.Errorf("independent sentinel permission seed failed")
		}
		if permissionPresent(before, group.ID, "Read", "Allow") {
			return fmt.Errorf("target rule already exists")
		}
		base := []string{"--kind", kind, "--id", resource.ID, "--principal-type", "group", "--principal-id", group.ID, "--capability", "Read"}
		if defaults != "" {
			base = append(base, "--default-for", defaults)
		}
		for _, verb := range []string{"create", "inspect", "delete"} {
			args := append([]string{"admin", "permission", verb}, base...)
			if verb != "inspect" {
				args = append(args, "--mode", "Allow")
			}
			out, e := fullRun(ctx, c, args...)
			if e != nil {
				return e
			}
			fresh, e := c.API.JSON(ctx, http.MethodGet, path, nil)
			if e != nil {
				return e
			}
			if permissionPresent(fresh, group.ID, "Read", "Allow") != (verb != "delete") {
				return fmt.Errorf("permission %s did not persist exact rule outcome", verb)
			}
			if !permissionPresent(fresh, sentinel.ID, "Read", "Deny") {
				return fmt.Errorf("permission operation changed sentinel")
			}
			if verb == "inspect" {
				row, e := recordField(out, "principal_luid", group.ID)
				if e != nil {
					return e
				}
				if e = field(row, "capability", "Read"); e != nil {
					return e
				}
				if e = field(row, "mode", "Allow"); e != nil {
					return e
				}
			}
		}
		return nil
	}
}

func permissionPresent(data any, principal, capability, mode string) bool {
	for _, r := range allMaps(data) {
		g, _ := r["group"].(map[string]any)
		if g["id"] == principal {
			for _, v := range allMaps(r["capabilities"]) {
				if v["name"] == capability && v["mode"] == mode {
					return true
				}
			}
		}
	}
	return false
}
func addMember(ctx context.Context, c *harness.Case, group, user string) error {
	_, e := c.API.JSON(ctx, http.MethodPost, c.API.REST("groups/"+url.PathEscape(group)+"/users"), map[string]any{"user": map[string]any{"id": user}})
	return e
}
func members(ctx context.Context, c *harness.Case, group string) ([]string, error) {
	items, e := c.API.List(ctx, "groups/"+url.PathEscape(group)+"/users", nil)
	if e != nil {
		return nil, e
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	slices.Sort(ids)
	return ids, nil
}
func wantMembers(ctx context.Context, c *harness.Case, group string, want []string) error {
	got, e := members(ctx, c, group)
	if e != nil {
		return e
	}
	want = slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		return fmt.Errorf("direct member set %v, want %v", got, want)
	}
	return nil
}
func named(ctx context.Context, c *harness.Case, kind, name string) (harness.Object, error) {
	items, e := c.API.List(ctx, kind, nil)
	if e != nil {
		return harness.Object{}, e
	}
	var matches []harness.Object
	for _, o := range items {
		if o.Name == name {
			matches = append(matches, o)
		}
	}
	if len(matches) != 1 {
		return harness.Object{}, fmt.Errorf("expected one %s named %q, got %d", kind, name, len(matches))
	}
	return matches[0], nil
}
func absent(ctx context.Context, c *harness.Case, kind, id string) error {
	items, e := c.API.List(ctx, kind, nil)
	if e != nil {
		return e
	}
	for _, o := range items {
		if o.ID == id {
			return fmt.Errorf("deleted %s remains in independent inventory", kind)
		}
	}
	return nil
}
func field(row map[string]any, key string, want any) error {
	got, ok := row[key]
	if !ok || !jsonEqual(got, want) {
		return fmt.Errorf("field %s = %v, want %v", key, got, want)
	}
	return nil
}
func jsonEqual(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}
func allMaps(v any) []map[string]any {
	var out []map[string]any
	switch x := v.(type) {
	case map[string]any:
		out = append(out, x)
		for _, child := range x {
			out = append(out, allMaps(child)...)
		}
	case []any:
		for _, child := range x {
			out = append(out, allMaps(child)...)
		}
	case []map[string]any:
		for _, child := range x {
			out = append(out, allMaps(child)...)
		}
	}
	return out
}
func record(data any, id string) (map[string]any, error) {
	for _, key := range []string{"luid", "id", "metadata_id", "definition_luid", "metric_luid", "subscription_luid"} {
		for _, r := range allMaps(data) {
			if r[key] == id {
				return r, nil
			}
		}
	}
	return nil, fmt.Errorf("missing expected record %q", id)
}
func recordField(data any, key string, value any) (map[string]any, error) {
	for _, r := range allMaps(data) {
		if reflect.DeepEqual(r[key], value) {
			return r, nil
		}
	}
	return nil, fmt.Errorf("missing expected %s=%v", key, value)
}
func recordByName(data any, name string) (map[string]any, error) {
	return recordField(data, "name", name)
}
func array(v any) []any { x, _ := v.([]any); return x }
func equalIDs(rows []any, want []string) error {
	got := make([]string, 0, len(rows))
	for _, v := range rows {
		r, _ := v.(map[string]any)
		id, _ := r["luid"].(string)
		if id == "" {
			id, _ = r["id"].(string)
		}
		got = append(got, id)
	}
	slices.Sort(got)
	want = slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		return fmt.Errorf("identity set %v, want %v", got, want)
	}
	return nil
}

func fullRun(ctx context.Context, c *harness.Case, args ...string) (map[string]any, error) {
	if !slices.Contains(args, "--environment") {
		args = append(slices.Clone(args), "--environment", c.Environment)
	}
	return c.Run(ctx, append(args, "--full")...)
}
