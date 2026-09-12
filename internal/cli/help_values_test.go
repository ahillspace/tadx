package cli

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	permissioncreate "github.com/ahillspace/tadx/actions/admin/permission/create"
	usercreate "github.com/ahillspace/tadx/actions/admin/user/create"
	cacherefresh "github.com/ahillspace/tadx/actions/cache/refresh"
	catalogaudit "github.com/ahillspace/tadx/actions/catalog/audit"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	datasourceschema "github.com/ahillspace/tadx/actions/datasource/schema"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/agenttarget"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	updatecli "github.com/ahillspace/tadx/internal/cli/update"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func helpValuesTree() *cobra.Command {
	root := &cobra.Command{Use: "tadx"}
	admin := admincli.New(admincli.Dependencies{PermissionCapabilities: tableauadmin.PermissionCapabilities})
	admin.AddCommand(admincli.NewLabels(admincli.LabelDependencies{}))
	root.AddCommand(admin, catalogcli.New(catalogcli.Dependencies{}), pulsecli.New(pulsecli.Dependencies{}), envcli.New(envcli.Dependencies{}), workspacecli.New(workspacecli.Dependencies{}), newSearch(nil, nil))
	content := &cobra.Command{Use: "content"}
	content.AddCommand(contentcli.NewLabels(contentcli.LabelDependencies{}))
	root.AddCommand(content)
	return root
}

func helpValueFlag(t *testing.T, root *cobra.Command, path, flag string) *pflag.Flag {
	t.Helper()
	command, remaining, err := root.Find(strings.Fields(path))
	if err != nil || len(remaining) > 0 {
		t.Fatalf("command %q: remaining=%v err=%v", path, remaining, err)
	}
	value := command.Flags().Lookup(flag)
	if value == nil {
		t.Fatalf("%s missing --%s", path, flag)
	}
	return value
}

func TestHelpChoicesMatchLocalValidators(t *testing.T) {
	root := helpValuesTree()
	applyHelpValues(root)
	cases := []struct {
		path, flag string
		count      int
		validate   func(string) error
	}{
		{"search", "type", 11, func(v string) error { _, err := searchaction.Types(v); return err }},
		{"catalog search", "type", 3, func(v string) error {
			return catalogsearch.ValidateInput(catalogsearch.Input{Query: "sales", Types: []string{v}, TableID: "table-id"})
		}},
		{"catalog audit", "type", 3, func(v string) error { return catalogaudit.ValidateInput(catalogaudit.Input{Type: v, ID: "scope-id"}) }},
		{"catalog audit", "check", 2, func(v string) error {
			return catalogaudit.ValidateInput(catalogaudit.Input{Type: "table", ID: "scope-id", Checks: []string{v}})
		}},
		{"admin user create", "auth-setting", 4, func(v string) error {
			return usercreate.ValidateInput(usercreate.Input{Environment: "dev", Name: "analyst", SiteRole: "Viewer", AuthSetting: v})
		}},
		{"pulse metric fork", "period", 16, func(v string) error {
			in := metricfork.Input{MetricLUID: "metric-id", Timeframe: v}
			if v == "CUSTOM_N_DAYS" {
				in.CustomDays = 30
			}
			return metricfork.ValidateInput(in)
		}},
	}
	for flag, count := range map[string]int{"kind": 4, "principal-type": 2, "mode": 2, "default-for": 3} {
		cases = append(cases, struct {
			path, flag string
			count      int
			validate   func(string) error
		}{"admin permission create", flag, count, func(v string) error {
			in := permissioncreate.Input{Environment: "dev", ResourceKind: "project", ResourceLUID: "project-id", PrincipalType: "user", PrincipalLUID: "user-id", Capability: "Read", Mode: "Allow"}
			switch flag {
			case "kind":
				in.ResourceKind = v
			case "principal-type":
				in.PrincipalType = v
			case "mode":
				in.Mode = v
			case "default-for":
				in.DefaultFor = v
			}
			return permissioncreate.ValidateInput(in)
		}})
	}
	for _, tc := range cases {
		t.Run(tc.path+"/"+tc.flag, func(t *testing.T) {
			values := helpValueFlag(t, root, tc.path, tc.flag).Annotations["tadx.help.choices"]
			if len(values) != tc.count {
				t.Fatalf("incomplete choices: %v", values)
			}
			unique := map[string]bool{}
			for _, value := range values {
				if unique[value] {
					t.Errorf("duplicate choice %q", value)
				}
				unique[value] = true
				if err := tc.validate(value); err != nil {
					t.Errorf("advertised %q rejected: %v", value, err)
				}
			}
		})
	}
	for flag, count := range map[string]int{"aggregation": 7, "minimum-granularity": 5, "number-format": 3, "sentiment": 3, "temporality": 2} {
		values := helpValueFlag(t, root, "pulse definition create", flag).Annotations["tadx.help.choices"]
		if len(values) != count {
			t.Fatalf("incomplete %s choices: %v", flag, values)
		}
		for _, value := range values {
			intent := definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-id", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: []string{"Region"}}
			switch flag {
			case "aggregation":
				intent.Aggregation = value
			case "minimum-granularity":
				intent.MinimumGranularity = value
			case "number-format":
				intent.NumberFormat = value
			case "sentiment":
				intent.Sentiment = value
			case "temporality":
				intent.Temporality = value
			}
			if err := definitioncreate.ValidateInput(definitioncreate.Input{Intent: intent}); err != nil {
				t.Errorf("%s=%s rejected: %v", flag, value, err)
			}
		}
	}
}

func TestHelpValuesAreScopedAndPreserveFlagBehavior(t *testing.T) {
	root := helpValuesTree()
	before := map[*pflag.Flag][3]string{}
	var record func(*cobra.Command)
	record = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) { before[f] = [3]string{f.DefValue, f.Value.String(), f.Value.Type()} })
		for _, child := range c.Commands() {
			record(child)
		}
	}
	record(root)
	applyHelpValues(root)
	first := helpValueFlag(t, root, "pulse definition create", "name").Usage
	applyHelpValues(root)
	if got := helpValueFlag(t, root, "pulse definition create", "name").Usage; got != first {
		t.Fatal("help metadata is not idempotent")
	}
	for f, want := range before {
		if got := [3]string{f.DefValue, f.Value.String(), f.Value.Type()}; got != want || f.Changed {
			t.Errorf("--%s behavior changed: %v -> %v", f.Name, want, got)
		}
	}
	for _, path := range []string{"catalog search", "catalog audit", "content label list"} {
		if slices.Contains(helpValueFlag(t, root, path, "type").Annotations["tadx.help.choices"], "workbook") {
			t.Errorf("search family leaked into %s", path)
		}
	}
	if got := helpValueFlag(t, root, "admin user create", "site-role"); len(got.Annotations["tadx.help.choices"]) != 0 || !strings.Contains(got.Usage, "site and version") {
		t.Fatal("site roles claim an unsupported finite list")
	}
	for path, want := range helpLimitDefaults {
		command, rest, err := root.Find(strings.Fields(path))
		if err != nil || len(rest) > 0 {
			continue
		}
		if f := command.Flags().Lookup("limit"); f != nil && !reflect.DeepEqual(f.Annotations["tadx.help.default"], []string{want}) {
			t.Errorf("%s missing action default %s", path, want)
		}
	}
	for _, kind := range []string{"project", "workbook", "datasource", "flow"} {
		command, _, _ := root.Find([]string{"admin", "permission", "create"})
		if !strings.Contains(command.Long, kind+": "+strings.Join(tableauadmin.PermissionCapabilities(kind), ", ")) {
			t.Errorf("missing authoritative %s capabilities", kind)
		}
	}
}

func TestHelpValuesForIsolatedCommandPaths(t *testing.T) {
	for _, tc := range []struct {
		path, flag string
		count      int
		validate   func(string) error
	}{
		{"content datasource schema", "role", 4, func(v string) error {
			_, err := datasourceschema.NormalizeInput(datasourceschema.Input{DatasourceLUID: "datasource-id", Role: v})
			return err
		}},
		{"cache refresh", "scope", 8, func(v string) error { return cacherefresh.ValidateInput(cacherefresh.Input{Scopes: []string{v}}) }},
		{"content project create", "content-permissions", 3, func(v string) error {
			return projectcreate.ValidateInput(projectcreate.Input{Environment: "dev", Name: "Project", ContentPermissions: v})
		}},
		{"content lineage pull", "kind", 4, func(v string) error {
			in := lineagepull.Input{Kind: v}
			in.Selector.LUID = "resource-id"
			return lineagepull.ValidateInput(in)
		}},
	} {
		root := &cobra.Command{Use: "tadx"}
		command := root
		for _, name := range strings.Fields(tc.path) {
			child := &cobra.Command{Use: name}
			command.AddCommand(child)
			command = child
		}
		command.Flags().String(tc.flag, "", "value")
		applyHelpValues(root)
		values := command.Flags().Lookup(tc.flag).Annotations["tadx.help.choices"]
		if len(values) != tc.count {
			t.Fatalf("%s choices: %v", tc.path, values)
		}
		for _, v := range values {
			if err := tc.validate(v); err != nil {
				t.Errorf("%s %q rejected: %v", tc.path, v, err)
			}
		}
	}
	for _, path := range []string{"agent install", "agent uninstall", "update"} {
		root := &cobra.Command{Use: "tadx"}
		root.AddCommand(agentcli.New(agentcli.Dependencies{}), updatecli.New(updatecli.Dependencies{}))
		command, _, err := root.Find(strings.Fields(path))
		if err != nil {
			t.Fatal(err)
		}
		applyHelpValues(root)
		want := agenttarget.SupportedTargets()
		if path != "agent uninstall" {
			want = append([]string{"auto"}, want...)
		}
		if got := command.Flags().Lookup("target").Annotations["tadx.help.choices"]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s targets=%v want=%v", path, got, want)
		}
	}
}
