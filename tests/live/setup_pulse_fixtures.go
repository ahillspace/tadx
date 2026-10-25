package live

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

const pulseNativeBinding = "pulse-native"

// PreparePulseFixtures publishes the authored portable Pulse datasource pack.
// It checkpoints exact datasource identities in pulse_fixture_source and
// pulse_fixture_destination before schema readiness is attempted.
func PreparePulseFixtures(ctx context.Context, c *harness.Case, withDestination bool) error {
	if c == nil || c.API == nil || c.Journal == nil || c.Root == "" {
		return harness.Blocked("Pulse native setup requires its shared root and ownership journal")
	}
	input, ok := c.Profile.Bindings[pulseNativeBinding]
	if !ok {
		return harness.Blocked("Pulse native setup requires pulse-native")
	}
	if err := validatePulseNativeInput(input); err != nil {
		return err
	}
	native, err := os.ReadFile(input.Path)
	if err != nil {
		return harness.Blocked("Pulse native fixture is unavailable")
	}
	if err := harness.CheckNative(ctx, "datasource", native, input); err != nil {
		return fmt.Errorf("verify authored Pulse datasource package: %w", err)
	}
	if c.Profile.Bindings == nil {
		c.Profile.Bindings = map[string]harness.Binding{}
	}
	roles := []string{"source"}
	if withDestination {
		roles = append(roles, "destination")
	}
	for _, role := range roles {
		projectName := c.NamePrefix + "-pulse-" + role + "-project"
		project, found, err := pulseOwnedReceipt(c, "project", projectName, c.Root)
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
		project, err = awaitPulseObject(ctx, c, "project", project, projectName, c.Root, nil, true)
		if err != nil {
			return err
		}
		datasourceName := c.NamePrefix + "-pulse-" + role + "-datasource"
		datasource, found, err := pulseOwnedReceipt(c, "datasource", datasourceName, project.ID)
		if err != nil {
			return err
		}
		if !found {
			if err := c.Intent("datasource", datasourceName, project.ID); err != nil {
				return err
			}
			datasource, err = c.API.Publish(ctx, "datasource", input.Path, project.ID, datasourceName, false)
			if err != nil {
				return err
			}
			if err := c.ConfirmReceipt("datasource", datasourceName, project.ID, datasource); err != nil {
				return err
			}
		}
		binding := harness.Binding{ID: datasource.ID, Name: datasourceName, Path: input.Path, Params: map[string]string{"project_id": project.ID}, Expected: maps.Clone(input.Expected)}
		key := "pulse_fixture_" + role
		c.Profile.Bindings[key] = binding
	}
	return nil
}

// AwaitPulseFixtures verifies immutable owned datasource packages and reads
// target field identities only after setup has checkpointed their LUIDs.
func AwaitPulseFixtures(ctx context.Context, c *harness.Case, withDestination bool) error {
	if c == nil || c.API == nil || c.Journal == nil || c.Root == "" {
		return harness.Blocked("Pulse readiness requires its retained setup context")
	}
	input := c.Profile.Bindings[pulseNativeBinding]
	if err := validatePulseNativeInput(input); err != nil {
		return err
	}
	source, err := awaitPulseBinding(ctx, c, "source", input, true)
	if err != nil {
		return err
	}
	params, err := resolvePulseFields(input, source)
	if err != nil {
		return err
	}
	params["project_id"] = c.Profile.Bindings["pulse_fixture_source"].Params["project_id"]
	c.Profile.Bindings["pulse_datasource"] = harness.Binding{ID: c.Profile.Bindings["pulse_fixture_source"].ID, Name: c.Profile.Bindings["pulse_fixture_source"].Name, Path: input.Path, Params: params, Expected: maps.Clone(input.Expected)}
	if withDestination {
		destination, err := awaitPulseBinding(ctx, c, "destination", input, true)
		if err != nil {
			return err
		}
		destinationParams, err := resolvePulseFields(input, destination)
		if err != nil {
			return err
		}
		if err := pulseFieldsCompatible(params, destinationParams); err != nil {
			return err
		}
		binding := c.Profile.Bindings["pulse_fixture_destination"]
		destinationParams["project_id"] = binding.Params["project_id"]
		binding.Params = destinationParams
		binding.Expected = maps.Clone(input.Expected)
		c.Profile.Bindings["pulse_destination_datasource"] = binding
	}
	return nil
}

// VerifyPulseFixtures is read-only and never repairs or recreates owned content.
func VerifyPulseFixtures(ctx context.Context, c *harness.Case, withDestination bool) error {
	if c == nil || c.API == nil || c.Journal == nil || c.Root == "" {
		return harness.Blocked("Pulse verification requires its retained setup context")
	}
	if _, ok := c.Profile.Bindings[pulseNativeBinding]; !ok {
		return nil
	}
	input := c.Profile.Bindings[pulseNativeBinding]
	source, err := awaitPulseBinding(ctx, c, "source", input, false)
	if err != nil {
		return err
	}
	want, err := resolvePulseFields(input, source)
	if err != nil {
		return err
	}
	if err := matchSavedPulseBinding(c.Profile.Bindings["pulse_fixture_source"], c.Profile.Bindings["pulse_datasource"], want); err != nil {
		return err
	}
	if withDestination {
		destination, err := awaitPulseBinding(ctx, c, "destination", input, false)
		if err != nil {
			return err
		}
		mapped, err := resolvePulseFields(input, destination)
		if err != nil {
			return err
		}
		if err := pulseFieldsCompatible(want, mapped); err != nil {
			return err
		}
		if err := matchSavedPulseBinding(c.Profile.Bindings["pulse_fixture_destination"], c.Profile.Bindings["pulse_destination_datasource"], mapped); err != nil {
			return err
		}
	}
	return nil
}

func matchSavedPulseBinding(fixture, ready harness.Binding, expected map[string]string) error {
	if ready.ID == "" || fixture.ID != ready.ID || fixture.Params["project_id"] != ready.Params["project_id"] {
		return harness.Blocked("saved Pulse binding differs from its exact owned datasource")
	}
	for _, key := range []string{"measure_field", "date_field", "dimension_field", "category"} {
		if ready.Params[key] != expected[key] {
			return harness.Blocked("saved Pulse binding differs from fresh target schema mapping")
		}
	}
	return nil
}

func pulseFieldsCompatible(source, destination map[string]string) error {
	for _, key := range []string{"measure_field", "date_field", "dimension_field"} {
		if source[key] == "" || destination[key] == "" || source[key] != destination[key] {
			return harness.Blocked("Pulse destination datasource fields are incompatible with the source and cannot be safely remapped")
		}
	}
	return nil
}

func awaitPulseBinding(ctx context.Context, c *harness.Case, role string, input harness.Binding, track bool) ([]harness.PulseSchemaField, error) {
	binding, ok := c.Profile.Bindings["pulse_fixture_"+role]
	if !ok || binding.ID == "" || binding.Params["project_id"] == "" {
		return nil, harness.Blocked("Pulse fixture setup identity checkpoint is missing")
	}
	projectName := c.NamePrefix + "-pulse-" + role + "-project"
	project, found, err := pulseOwnedReceipt(c, "project", projectName, c.Root)
	if err != nil {
		return nil, err
	}
	if !found || project.ID != binding.Params["project_id"] {
		return nil, harness.Blocked("Pulse fixture project is not owned by its exact setup journal")
	}
	datasourceName := c.NamePrefix + "-pulse-" + role + "-datasource"
	datasource, found, err := pulseOwnedReceipt(c, "datasource", datasourceName, project.ID)
	if err != nil {
		return nil, err
	}
	if !found || datasource.ID != binding.ID {
		return nil, harness.Blocked("Pulse fixture datasource is not owned by its exact setup journal")
	}
	ready, err := awaitPulseObject(ctx, c, "datasource", datasource, datasourceName, project.ID, &input, track)
	if err != nil {
		return nil, err
	}
	if ready.ID != binding.ID {
		return nil, harness.Blocked("Pulse fixture datasource identity changed")
	}
	return awaitPulseSchema(ctx, c, ready.ID, input)
}

func awaitPulseSchema(ctx context.Context, c *harness.Case, datasourceID string, input harness.Binding) ([]harness.PulseSchemaField, error) {
	seconds := c.Profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 300
	}
	budget := time.Duration(seconds) * time.Second
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var fields []harness.PulseSchemaField
	var lastCheckErr error
	err := harness.Observe(bounded, budget, func() error {
		schema, err := c.API.ReadPulseSchema(bounded, datasourceID)
		if err != nil {
			lastCheckErr = err
			return err
		}
		if _, err := resolvePulseFields(input, schema.Fields); err != nil {
			lastCheckErr = err
			return err
		}
		fields = schema.Fields
		lastCheckErr = nil
		return nil
	})
	return fields, preservePulseReadinessCause(err, lastCheckErr)
}

func preservePulseReadinessCause(observationErr, lastCheckErr error) error {
	if observationErr == nil || lastCheckErr == nil {
		return observationErr
	}
	var cause string
	if prerequisite, ok := errors.AsType[*harness.PrerequisiteError](lastCheckErr); ok {
		cause = prerequisite.Reason
	} else if httpErr, ok := errors.AsType[*harness.HTTPError](lastCheckErr); ok {
		cause = httpErr.Error()
	} else {
		cause = "Pulse schema read failed"
	}
	return fmt.Errorf("%w: last Pulse readiness check: %s", observationErr, cause)
}

func awaitPulseObject(ctx context.Context, c *harness.Case, kind string, receipt harness.Object, name, parent string, native *harness.Binding, track bool) (harness.Object, error) {
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
			return harness.Blocked("owned Pulse fixture identity or parent changed; cleanup and rebuild the immutable pack")
		}
		if native != nil {
			download, err := c.API.Download(bounded, "datasource", receipt.ID)
			if err != nil {
				return err
			}
			if err := harness.CheckNative(bounded, "datasource", download.Bytes, *native); err != nil {
				return harness.Blocked("owned Pulse datasource payload differs from its authored package")
			}
			evidence, err := harness.ValidateNative("datasource", download)
			if err != nil {
				return err
			}
			want, ok := native.Expected["extract_count"].(float64)
			if ok && int(want) != len(evidence.Extracts) {
				return harness.Blocked("owned Pulse datasource extract count differs from authored native facts")
			}
		}
		observed = object
		return nil
	})
	if err != nil {
		return receipt, err
	}
	if track {
		if err := c.Track(observed); err != nil {
			return receipt, err
		}
	}
	return observed, nil
}

func pulseOwnedReceipt(c *harness.Case, kind, name, parent string) (harness.Object, bool, error) {
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
		return harness.Object{}, false, harness.Inconclusive("Pulse fixture has multiple ownership records; retain setup state for cleanup")
	}
	entry := matches[0]
	if entry.Done {
		return harness.Object{}, false, harness.Blocked("Pulse fixture was removed; clean up and rebuild setup")
	}
	if entry.Object == nil || entry.Object.ID == "" {
		return harness.Object{}, false, harness.Inconclusive("Pulse fixture creation is uncertain; reconcile cleanup before rebuilding")
	}
	if entry.Object.Kind != kind {
		return harness.Object{}, false, errors.New("Pulse fixture receipt has a different resource kind")
	}
	return *entry.Object, true, nil
}

func validatePulseNativeInput(binding harness.Binding) error {
	if binding.Path == "" {
		return harness.Blocked("pulse-native requires a portable datasource package path")
	}
	for _, key := range []string{"measure_name", "date_name", "dimension_name", "category"} {
		if strings.TrimSpace(binding.Params[key]) == "" {
			return harness.Blocked("pulse-native requires authored " + key)
		}
	}
	fields, ok := binding.Expected["pulse_fields"].([]any)
	if !ok || len(fields) < 3 {
		return harness.Blocked("pulse-native requires authored pulse_fields with name, type, and role")
	}
	if !nativeSingleExtract(binding.Expected["extract_count"]) {
		return harness.Blocked("pulse-native requires one authored Hyper extract")
	}
	columns := nativeStrings(binding.Expected["columns"])
	if len(columns) == 0 {
		return harness.Blocked("pulse-native requires authored native column names")
	}
	seen := map[string]bool{}
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok || nativeText(field["name"]) == "" || nativeText(field["type"]) == "" || nativeText(field["role"]) == "" {
			return harness.Blocked("pulse-native authored fields require exact name, type, and role")
		}
		name := nativeText(field["name"])
		if seen[name] {
			return harness.Blocked("pulse-native authored field names must be unique")
		}
		seen[name] = true
		if !nativeColumnMatchesName(columns, name) {
			return harness.Blocked("pulse-native selected schema field is absent from its native column manifest")
		}
	}
	for _, key := range []string{"measure_name", "date_name", "dimension_name"} {
		if !seen[binding.Params[key]] {
			return harness.Blocked("pulse-native selected " + key + " is absent from authored pulse_fields")
		}
	}
	return nil
}

func nativeColumnMatchesName(columns []string, name string) bool {
	for _, column := range columns {
		if column == name || column == "["+name+"]" {
			return true
		}
	}
	return false
}

func resolvePulseFields(input harness.Binding, fields []harness.PulseSchemaField) (map[string]string, error) {
	authored, _ := input.Expected["pulse_fields"].([]any)
	byName := make(map[string]map[string]any, len(authored))
	for _, raw := range authored {
		field := raw.(map[string]any)
		byName[nativeText(field["name"])] = field
	}
	result := map[string]string{"category": input.Params["category"]}
	for _, selection := range []struct {
		nameKey, outputKey, role string
	}{
		{"measure_name", "measure_field", "measure"},
		{"date_name", "date_field", "dimension"},
		{"dimension_name", "dimension_field", "dimension"},
	} {
		name := input.Params[selection.nameKey]
		want := byName[name]
		if nativeText(want["role"]) != selection.role {
			return nil, harness.Blocked("authored Pulse field role is incompatible with " + selection.nameKey)
		}
		var matches []harness.PulseSchemaField
		for _, field := range fields {
			if field.ID == name || field.Name == name {
				matches = append(matches, field)
			}
		}
		if len(matches) != 1 {
			if len(matches) == 0 {
				return nil, harness.Inconclusive("target datasource schema is not yet ready for " + selection.nameKey)
			}
			return nil, harness.Blocked("target datasource field mapping is ambiguous")
		}
		field := matches[0]
		if field.Hidden || field.Internal || field.Excluded || field.ColumnClass != "column" || field.Role != selection.role || !pulseSchemaTypeMatches(field.DataType, nativeText(want["type"])) {
			return nil, harness.Blocked("target datasource field for " + selection.nameKey + " differs from authored identity, type, role, or eligibility")
		}
		result[selection.outputKey] = field.ID
	}
	return result, nil
}

func pulseSchemaTypeMatches(actual, authored string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	authored = strings.ToLower(strings.TrimSpace(authored))
	if authored == "text" {
		authored = "string"
	}
	return actual != "" && actual == authored
}

func nativeText(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func nativeSingleExtract(value any) bool {
	switch value := value.(type) {
	case int:
		return value == 1
	case int64:
		return value == 1
	case float64:
		return value == 1
	default:
		return false
	}
}

func nativeStrings(value any) []string {
	switch values := value.(type) {
	case []string:
		return slices.Clone(values)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil
			}
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

func pulseNativeSelected(specs []Spec, p harness.Profile) bool {
	_, ok := p.Bindings[pulseNativeBinding]
	return ok && pulseCasesSelected(specs)
}

func pulseCasesSelected(specs []Spec) bool {
	return slices.ContainsFunc(specs, func(spec Spec) bool {
		return strings.HasPrefix(spec.ID, "live.pulse-") || spec.ID == "live.search.pulse-discovery"
	})
}

func pulseMigrationSelected(specs []Spec) bool {
	return slices.ContainsFunc(specs, func(spec Spec) bool {
		return spec.ID == "live.pulse-definition.publish"
	})
}
