package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	nativefixtures "github.com/ahillspace/tadx/tests/live/fixtures"
)

// Project creates a fresh, journal-owned child under the run's fixture root.
func (c *Case) Project(ctx context.Context, suffix string) (Object, error) {
	key := "project:" + suffix
	if existing, ok := c.Fixtures[key]; ok {
		return existing, nil
	}
	name := c.NamePrefix + "-" + suffix
	fields := map[string]any{"name": name, "description": "Disposable TADX live-suite fixture"}
	fields["contentPermissions"] = "ManagedByOwner"
	if c.Root != "" {
		fields["parentProjectId"] = c.Root
	}
	if err := c.Intent("project", name, c.Root); err != nil {
		return Object{}, err
	}
	object, err := c.API.Create(ctx, "project", fields)
	if err != nil {
		return Object{}, err
	}
	if err := c.ConfirmReceipt("project", name, c.Root, object); err != nil {
		return object, err
	}
	object, err = c.observeFixture(ctx, object, name, c.Root)
	if err != nil {
		return object, err
	}
	if c.Fixtures == nil {
		c.Fixtures = map[string]Object{}
	}
	c.Fixtures[key] = object
	return object, nil
}

// Content independently publishes one native fixture into a disposable project.
// Advanced variants require an explicit portable binding, never arbitrary site content.
func (c *Case) Content(ctx context.Context, kind, variant string) (Object, error) {
	if variant == "" || variant == "default" {
		variant = "native"
	}
	cacheKey := kind + ":" + variant
	if existing, ok := c.Fixtures[cacheKey]; ok {
		return existing, nil
	}
	artifactVariant := variant
	switch variant {
	case "base", "source", "sentinel", "target", "replacement":
		artifactVariant = "native"
	}
	key := kind + "-" + artifactVariant
	if artifactVariant != "native" && !strings.HasPrefix(artifactVariant, "native-") {
		key = kind + "-native-" + artifactVariant
	}
	filename := c.Profile.Artifacts[key]
	if binding, ok := c.Profile.Bindings[key]; ok && binding.Path != "" {
		filename = binding.Path
	}
	if filename == "" {
		return Object{}, Blocked(fmt.Sprintf("fixture %s requires a native artifact binding", key))
	}
	object, err := c.publishFixture(ctx, kind, filename, variant)
	if err != nil {
		return object, err
	}
	if c.Fixtures == nil {
		c.Fixtures = map[string]Object{}
	}
	c.Fixtures[cacheKey] = object
	return object, nil
}

func (c *Case) publishFixture(ctx context.Context, kind, filename, variant string) (Object, error) {
	filename = filepath.Clean(filename)
	project, err := c.Project(ctx, kind+"-"+variant)
	if err != nil {
		return Object{}, err
	}
	name := c.NamePrefix + "-" + kind + "-" + variant
	if err := c.Intent(kind, name, project.ID); err != nil {
		return Object{}, err
	}
	object, err := c.API.Publish(ctx, kind, filename, project.ID, name, false)
	if err != nil {
		return Object{}, err
	}
	if err := c.ConfirmReceipt(kind, name, project.ID, object); err != nil {
		return object, err
	}
	object, err = c.observeFixture(ctx, object, name, project.ID)
	if err != nil {
		return object, err
	}
	if c.Fixtures == nil {
		c.Fixtures = map[string]Object{}
	}
	return object, nil
}

// Seed creates fresh disposable resources and never uses a bootstrap user as a mutation subject.
func (c *Case) Seed(ctx context.Context, kind, suffix string) (Object, error) {
	if kind == "project" {
		return c.Project(ctx, suffix)
	}
	if kind == "workbook" || kind == "datasource" || kind == "flow" {
		key := kind + "-native"
		filename := c.Profile.Artifacts[key]
		if binding, ok := c.Profile.Bindings[key]; ok && binding.Path != "" {
			filename = binding.Path
		}
		if filename == "" {
			return Object{}, Blocked("fixture " + key + " requires a native artifact binding")
		}
		namespace := fmt.Sprintf("%s-seed-%d", suffix, len(c.Fixtures)+1)
		object, err := c.publishFixture(ctx, kind, filename, namespace)
		if err != nil {
			return object, err
		}
		c.Fixtures["seed:"+kind+":"+namespace] = object
		return object, nil
	}
	name := c.NamePrefix + "-" + suffix
	fields := map[string]any{"name": name}
	parent := ""
	switch kind {
	case "group":
	case "user":
		binding, err := c.Require("controlled_user")
		if err != nil {
			return Object{}, err
		}
		login, err := c.ControlledUserLogin(suffix)
		if err != nil {
			return Object{}, err
		}
		users, err := c.API.List(ctx, "user", nil)
		if err != nil {
			return Object{}, err
		}
		for _, existing := range users {
			if existing.Name == login {
				return Object{}, Blocked("controlled user login already exists; refusing to adopt it")
			}
		}
		fields["name"] = login
		name = login
		fields["siteRole"] = "Unlicensed"
		if binding.Params["siteRole"] != "" {
			fields["siteRole"] = binding.Params["siteRole"]
		}
		if setting := binding.Params["authSetting"]; setting != "" {
			fields["authSetting"] = setting
		}
		if setting := binding.Params["auth_setting"]; setting != "" {
			fields["authSetting"] = setting
		}
	case "labelCategory", "label-category":
		fields["description"] = "Disposable live-suite label category"
	case "labelValue", "label-value":
		category, err := c.Seed(ctx, "labelCategory", suffix+"-category")
		if err != nil {
			return Object{}, err
		}
		parent = category.ID
		fields["category"] = category.Name
		fields["description"] = "Disposable live-suite label value"
	case "pulseDefinition", "pulse.definition", "definition":
		binding, err := c.Require("pulse_datasource")
		if err != nil {
			return Object{}, err
		}
		if binding.ID == "" || binding.Params["measure_field"] == "" || binding.Params["date_field"] == "" {
			return Object{}, errors.New("Pulse fixture requires datasource, measure_field, and date_field bindings")
		}
		parent = binding.ID
		fields = maps.Clone(binding.Expected)
		if fields == nil {
			fields = map[string]any{}
		}
		fields["name"] = name
		if _, ok := fields["specification"]; !ok {
			fields["specification"] = map[string]any{"datasource": map[string]any{"id": binding.ID}, "basic_specification": map[string]any{"measure": map[string]any{"field": binding.Params["measure_field"], "aggregation": "AGGREGATION_SUM"}, "time_dimension": map[string]any{"field": binding.Params["date_field"]}, "filters": []any{}}, "is_running_total": false, "temporality": "TEMPORALITY_OVER_TIME"}
		}
		if _, ok := fields["extension_options"]; !ok {
			dimensions := []string{}
			if dimension := binding.Params["dimension_field"]; dimension != "" {
				dimensions = append(dimensions, dimension)
			}
			fields["extension_options"] = map[string]any{"allowed_dimensions": dimensions, "allowed_granularities": []string{"GRANULARITY_BY_DAY", "GRANULARITY_BY_WEEK", "GRANULARITY_BY_MONTH", "GRANULARITY_BY_QUARTER", "GRANULARITY_BY_YEAR"}, "offset_from_today": 0, "use_dynamic_offset": false}
		}
		defaults := map[string]any{"description": "Disposable TADX live-suite definition", "representation_options": map[string]any{"type": "NUMBER_FORMAT_TYPE_NUMBER", "sentiment_type": "SENTIMENT_TYPE_NONE"}, "insights_options": map[string]any{"show_insights": true, "settings": []any{}}, "comparisons": map[string]any{"comparisons": []any{}}, "datasource_goals": []any{}, "related_links": []any{}, "certification": map[string]any{"is_certified": false}}
		for key, value := range defaults {
			if _, ok := fields[key]; !ok {
				fields[key] = value
			}
		}
	default:
		return Object{}, Blocked(fmt.Sprintf("fixture seed is unsupported for resource kind %s", kind))
	}
	if kind == "user" {
		login, err := c.IntentControlledUser(suffix)
		if err != nil {
			return Object{}, err
		}
		if login != name {
			return Object{}, errors.New("controlled user identity changed before intent")
		}
	} else if err := c.Intent(kind, name, parent); err != nil {
		return Object{}, err
	}
	object, err := c.API.Create(ctx, kind, fields)
	if err != nil {
		return Object{}, err
	}
	if object.ID == c.API.UserID && kind == "user" {
		return Object{}, errors.New("Tableau returned the bootstrap user for a disposable user fixture")
	}
	if kind == "user" {
		if err := c.ConfirmControlledUserReceipt(suffix, name, object); err != nil {
			return object, err
		}
	} else if err := c.ConfirmReceipt(kind, name, parent, object); err != nil {
		return object, err
	}
	object, err = c.observeFixture(ctx, object, name, parent)
	if err != nil {
		return object, err
	}
	return object, nil
}

// observeFixture waits only for read visibility after its receipt is durable.
// A timeout returns the confirmed receipt so cleanup never loses its identity.
func (c *Case) observeFixture(ctx context.Context, receipt Object, requestedName, parent string) (Object, error) {
	switch receipt.Kind {
	case "project", "group", "user", "workbook", "datasource", "flow", "labelCategory", "label-category", "labelValue", "label-value", "pulseDefinition", "pulse.definition", "definition":
	default:
		return receipt, errors.New("fixture has no supported independent readiness observation")
	}
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	observationCtx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	var observed Object
	err := Observe(observationCtx, time.Duration(seconds)*time.Second, func() error {
		value, err := c.API.Get(observationCtx, receipt.Kind, receipt.ID)
		if err != nil {
			return err
		}
		if value.ID != receipt.ID || value.Name != requestedName {
			return errors.New("fixture identity or name is not yet independently visible")
		}
		if parent != "" && value.ProjectID != parent {
			return errors.New("fixture parent is not yet independently visible")
		}
		if receipt.OwnerID != "" && value.OwnerID != receipt.OwnerID {
			return errors.New("fixture owner is not yet independently visible")
		}
		if receipt.Description != "" && value.Description != receipt.Description {
			return errors.New("fixture description is not yet independently visible")
		}
		for _, key := range []string{"siteRole", "contentPermissions", "category"} {
			if expected, ok := receipt.Fields[key]; ok {
				if fmt.Sprint(value.Fields[key]) != fmt.Sprint(expected) {
					return fmt.Errorf("fixture %s is not yet independently visible", key)
				}
			}
		}
		observed = value
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

// GenerateFixtures creates portable native fixtures without remote data or credentials.
// The caller chooses an ignored output directory and saves returned bindings in its profile.
func GenerateFixtures(ctx context.Context, root string) (map[string]string, map[string]Binding, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	selected := os.Getenv("TADX_LIVECLI_PYTHON")
	if selected == "" {
		selected = "python"
	}
	python, err := exec.LookPath(selected)
	if err != nil {
		return nil, nil, Blocked("native fixture generation requires Python with tableauhyperapi installed")
	}
	probe := exec.CommandContext(ctx, python, "-c", "import tableauhyperapi")
	if output, err := probe.CombinedOutput(); err != nil {
		if bytes.Contains(output, []byte("ModuleNotFoundError")) {
			return nil, nil, Blocked("native fixture generation requires tableauhyperapi in the selected Python environment")
		}
		return nil, nil, errors.New("native fixture SDK probe failed")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, nil, err
	}
	staged, err := os.CreateTemp(root, ".fixture-generator-*.py")
	if err != nil {
		return nil, nil, err
	}
	script := staged.Name()
	defer os.Remove(script)
	if err := staged.Chmod(0600); err != nil {
		_ = staged.Close()
		return nil, nil, err
	}
	_, writeErr := staged.WriteString(nativefixtures.Generator)
	if err := errors.Join(writeErr, staged.Close()); err != nil {
		return nil, nil, err
	}
	command := exec.CommandContext(ctx, python, script, root)
	command.Dir = root
	if err := command.Run(); err != nil {
		return nil, nil, errors.New("native fixture generation failed after the Hyper SDK prerequisite check")
	}
	artifacts := map[string]string{}
	bindings := map[string]Binding{}
	for _, variant := range []string{"a", "b"} {
		manifestData, err := os.ReadFile(filepath.Join(root, "fixture-"+variant+".json"))
		if err != nil {
			return nil, nil, err
		}
		var expected map[string]any
		if err := json.Unmarshal(manifestData, &expected); err != nil {
			return nil, nil, err
		}
		for kind, ext := range map[string]string{"datasource": "tdsx", "workbook": "twbx"} {
			key := kind + "-native"
			if variant == "b" {
				key += "-b"
			}
			filename := filepath.Join(root, "fixture-"+variant+"."+ext)
			artifacts[key] = filename
			nativeExpected := maps.Clone(expected)
			if kind == "datasource" {
				delete(nativeExpected, "worksheets")
			}
			bindings[key] = Binding{Path: filename, Expected: nativeExpected, Params: map[string]string{"hyper_table": `"Extract"."Extract"`}}
		}
		key := "datasource-hyper"
		if variant == "b" {
			key += "-b"
		}
		filename := filepath.Join(root, "fixture-"+variant+".hyper")
		artifacts[key] = filename
		hyperExpected := maps.Clone(expected)
		delete(hyperExpected, "columns")
		delete(hyperExpected, "worksheets")
		bindings[key] = Binding{Path: filename, Expected: hyperExpected, Params: map[string]string{"hyper_table": `"Extract"."Extract"`}}
		alias := "datasource-native-hyper"
		if variant == "b" {
			alias += "-b"
		}
		artifacts[alias] = filename
		bindings[alias] = bindings[key]
	}
	aHyper := bindings["datasource-native-hyper"]
	bHyper := bindings["datasource-native-hyper-b"]
	aRows, _ := aHyper.Expected["rows"].([]any)
	bRows, _ := bHyper.Expected["rows"].([]any)
	combined := maps.Clone(aHyper.Expected)
	combined["rows"] = append(append([]any(nil), aRows...), bRows...)
	bindings["datasource-append-result"] = Binding{Expected: combined, Params: maps.Clone(aHyper.Params)}
	return artifacts, bindings, nil
}
