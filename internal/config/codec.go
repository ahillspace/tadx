package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var environmentFieldPaths = []string{"url", "site_content_url", "auth.type", "auth.pat_name_env", "auth.pat_secret_env", "auth.credential_ref", "default_workspace", "cache_max_concurrency"}

func decodeConfiguration(data []byte) (Config, bool, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return Config{}, false, fmt.Errorf("decode configuration: %s", yamlScalarExcerpt.ReplaceAllString(err.Error(), "[value redacted]"))
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, false, fmt.Errorf("decode configuration: %s", yamlScalarExcerpt.ReplaceAllString(err.Error(), "[value redacted]"))
		}
		return Config{}, false, errors.New("decode configuration: multiple YAML documents are not supported")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Config{}, false, errors.New("configuration must be a YAML mapping")
	}
	root, err := expandAliases(document.Content[0], make(map[*yaml.Node]bool), new(0), 0)
	if err != nil {
		return Config{}, false, err
	}
	fields, err := mappingFields(root)
	if err != nil {
		return Config{}, false, err
	}
	if _, exists := fields["workspace"]; exists {
		return Config{}, false, errors.New("workspace manifest supplied as CLI settings; use --config for CLI settings and --workspace for a registered workspace")
	}
	c := Config{Environments: make(map[string]Environment), Workspaces: make(map[string]WorkspaceRegistration), InvalidEnvironments: make(map[string]InvalidEntry), InvalidWorkspaces: make(map[string]InvalidEntry), unknownFields: make(map[string]*yaml.Node)}
	for _, field := range slices.Sorted(maps.Keys(fields)) {
		node := fields[field]
		switch field {
		case "version":
			if node.Tag != "!!int" || node.Decode(&c.Version) != nil {
				return Config{}, false, errors.New("configuration version must be an integer")
			}
		case "default_environment", "default_workspace":
			if node.Tag != "!!str" {
				return Config{}, false, fmt.Errorf("configuration %s must be a string", field)
			}
			if field == "default_environment" {
				c.DefaultEnvironment = node.Value
			} else {
				c.DefaultWorkspace = node.Value
			}
		case "environments", "workspaces":
			entries, err := mappingFields(node)
			if err != nil {
				return Config{}, false, fmt.Errorf("configuration %s must be a mapping with unique string keys", field)
			}
			for name, entry := range entries {
				if field == "environments" {
					c.InvalidEnvironments[name] = InvalidEntry{node: entry}
				} else {
					c.InvalidWorkspaces[name] = InvalidEntry{node: entry}
				}
			}
		case "site_mutations":
			if node.Kind != yaml.SequenceNode {
				return Config{}, false, errors.New("site_mutations must be a sequence")
			}
			for _, entry := range node.Content {
				values, err := mappingFields(entry)
				if err != nil {
					return Config{}, false, errors.New("site_mutations entries must be mappings with unique fields")
				}
				for key, value := range values {
					if !slices.Contains([]string{"server_url", "site_content_url", "enabled"}, key) {
						return Config{}, false, errors.New("site_mutations contains an unknown field")
					}
					if (key == "enabled" && value.Tag != "!!bool") || (key != "enabled" && value.Tag != "!!str") {
						return Config{}, false, fmt.Errorf("site_mutations %s has an invalid type", key)
					}
				}
				var mutation SiteMutation
				if entry.Decode(&mutation) != nil {
					return Config{}, false, errors.New("site_mutations contains a malformed entry")
				}
				c.SiteMutations = append(c.SiteMutations, mutation)
			}
		case "mutations_enabled": // Legacy values never authorize mutations.
		default:
			c.unknownFields[field] = node
		}
	}
	if err := validateFile(c); err != nil {
		return Config{}, false, err
	}
	// Decode known fields before migration, without allowing one bad default to stop load.
	for name, invalid := range c.InvalidEnvironments {
		invalid.environment, _ = decodeEnvironment(invalid.node)
		c.InvalidEnvironments[name] = invalid
		c.Environments[name] = invalid.environment
	}
	for name, invalid := range c.InvalidWorkspaces {
		invalid.workspace, _ = decodeWorkspace(invalid.node)
		c.InvalidWorkspaces[name] = invalid
		c.Workspaces[name] = invalid.workspace
	}
	diagnosticCandidate := c
	diagnosticCandidate.Environments = nil
	diagnosticCandidate.Workspaces = nil
	initial, err := classifyConfiguration(diagnosticCandidate)
	if err != nil {
		return Config{}, false, err
	}
	migrated := false
	// Migrate each reference separately. Failed migrations retain their original
	// values and become deferred defaults or entry violations.
	if looksLikePath(c.DefaultWorkspace) {
		probe := c
		probe.Environments = nil
		probe.Workspaces = maps.Clone(c.Workspaces)
		if next, changed, err := migrateLegacyWorkspaceDefaults(probe); err == nil && changed {
			c.DefaultWorkspace, c.Workspaces = next.DefaultWorkspace, next.Workspaces
			migrated = true
		}
	}
	for _, alias := range slices.Sorted(maps.Keys(c.Environments)) {
		environment := c.Environments[alias]
		if !looksLikePath(environment.DefaultWorkspace) {
			continue
		}
		// Do not migrate another field inside an already quarantined entry.
		if invalid, exists := initial.InvalidEnvironments[alias]; exists {
			if len(invalid.Violations) != 1 || invalid.Violations[0].Field != "default_workspace" {
				continue
			}
		}
		probe := c
		probe.DefaultWorkspace = ""
		probe.Environments = map[string]Environment{alias: environment}
		probe.Workspaces = maps.Clone(c.Workspaces)
		if next, changed, err := migrateLegacyWorkspaceDefaults(probe); err == nil && changed {
			c.Environments[alias], c.Workspaces = next.Environments[alias], next.Workspaces
			invalid := c.InvalidEnvironments[alias]
			_ = setNodeField(invalid.node, "default_workspace", next.Environments[alias].DefaultWorkspace)
			c.InvalidEnvironments[alias] = invalid
			migrated = true
		}
	}
	// Raw entries are authoritative for pre-existing mappings; generated migration
	// registrations have no original node and are encoded from their typed values.
	for alias := range c.InvalidEnvironments {
		delete(c.Environments, alias)
	}
	for name := range c.InvalidWorkspaces {
		delete(c.Workspaces, name)
	}
	c, err = classifyConfiguration(c)
	if err != nil {
		return Config{}, false, err
	}
	c.sourceInvalidEnvironments = cloneInvalidEntries(c.InvalidEnvironments)
	c.sourceInvalidWorkspaces = cloneInvalidEntries(c.InvalidWorkspaces)
	c.sourceDefaultEnvironment, c.sourceDefaultWorkspace = c.DefaultEnvironment, c.DefaultWorkspace
	return c, migrated, nil
}

func validateFile(c Config) error {
	if c.Version != CurrentVersion {
		return &ValidationError{Violations: []string{fmt.Sprintf("version must be %d", CurrentVersion)}}
	}
	if err := validateSiteMutations(c.SiteMutations); err != nil {
		return errors.New("site_mutations is malformed: server identity or consent entries are invalid")
	}
	return nil
}

func mappingFields(node *yaml.Node) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, errors.New("expected a mapping")
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, errors.New("mapping keys must be strings")
		}
		if _, exists := fields[key.Value]; exists {
			return nil, errors.New("mapping fields must be unique")
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, nil
}

// entryFields isolates ambiguous fields without discarding independent known
// identities needed for credential repair and filesystem protection.
// File controls continue to use the strict mappingFields decoder.
func entryFields(node *yaml.Node) (map[string]*yaml.Node, []EntryViolation) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, []EntryViolation{{Field: "entry", Rule: "must be a mapping with unique string fields"}}
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	duplicates := make(map[string]bool)
	var violations []EntryViolation
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			violations = append(violations, EntryViolation{Field: "entry", Rule: "field names must be strings"})
			continue
		}
		if duplicates[key.Value] {
			continue
		}
		if _, exists := fields[key.Value]; exists {
			delete(fields, key.Value)
			duplicates[key.Value] = true
			violations = append(violations, EntryViolation{Field: key.Value, Rule: "must not be repeated"})
			continue
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, violations
}

func decodeEnvironment(node *yaml.Node) (Environment, []EntryViolation) {
	var environment Environment
	fields, violations := entryFields(node)
	violations = slices.DeleteFunc(violations, func(violation EntryViolation) bool { return violation.Field == "api_version" })
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		value := fields[name]
		if name == "api_version" {
			continue
		}
		if name == "auth" {
			authFields, authViolations := entryFields(value)
			for _, violation := range authViolations {
				if violation.Field == "entry" {
					violation.Field = "auth"
				}
				if violation.Field == "pat_name_env" || violation.Field == "pat_secret_env" {
					violation.SecretRisk = true
				}
				violations = append(violations, violation)
			}
			for _, field := range slices.Sorted(maps.Keys(authFields)) {
				node := authFields[field]
				var target *string
				switch field {
				case "type":
					target = &environment.Auth.Type
				case "pat_name_env":
					target = &environment.Auth.PATNameEnv
				case "pat_secret_env":
					target = &environment.Auth.PATSecretEnv
				case "credential_ref":
					target = &environment.Auth.CredentialRef
				}
				if target == nil {
					violations = append(violations, EntryViolation{Field: field, Rule: "is unknown", SecretRisk: field == "pat_name" || field == "pat_secret"})
					continue
				}
				if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
					violations = append(violations, EntryViolation{Field: field, Rule: "must be a string", SecretRisk: field == "pat_name_env" || field == "pat_secret_env"})
					continue
				}
				*target = node.Value
			}
			continue
		}
		if name == "cache_max_concurrency" {
			if value.Tag != "!!int" || value.Decode(&environment.CacheMaxConcurrency) != nil {
				violations = append(violations, EntryViolation{Field: name, Rule: "must be an integer"})
			}
			continue
		}
		var target *string
		switch name {
		case "url":
			target = &environment.URL
		case "site_content_url":
			target = &environment.SiteContentURL
		case "default_workspace":
			target = &environment.DefaultWorkspace
		}
		if target == nil {
			violations = append(violations, EntryViolation{Field: name, Rule: "is unknown"})
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			violations = append(violations, EntryViolation{Field: name, Rule: "must be a string"})
			continue
		}
		*target = value.Value
	}
	return environment, violations
}

func decodeWorkspace(node *yaml.Node) (WorkspaceRegistration, []EntryViolation) {
	var registration WorkspaceRegistration
	fields, violations := entryFields(node)
	for _, field := range slices.Sorted(maps.Keys(fields)) {
		node := fields[field]
		var target *string
		switch field {
		case "id":
			target = &registration.ID
		case "path":
			target = &registration.Path
		}
		if target == nil {
			violations = append(violations, EntryViolation{Field: field, Rule: "is unknown"})
			continue
		}
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			violations = append(violations, EntryViolation{Field: field, Rule: "must be a string"})
			continue
		}
		*target = node.Value
	}
	return registration, violations
}

func valueNode(value any) (*yaml.Node, error) {
	var node yaml.Node
	err := node.Encode(value)
	return &node, err
}
func (c Config) environmentNode(alias string) (*yaml.Node, error) {
	if invalid, ok := c.InvalidEnvironments[alias]; ok {
		if invalid.node == nil {
			return nil, errors.New("invalid environment has no original YAML")
		}
		return cloneNode(invalid.node), nil
	}
	return valueNode(c.Environments[alias])
}

func setNodeField(node *yaml.Node, path string, value any) error {
	parts := strings.Split(path, ".")
	for _, part := range parts[:len(parts)-1] {
		if node == nil || node.Kind != yaml.MappingNode {
			return errors.New("environment field parent must be a mapping")
		}
		fields, violations := entryFields(node)
		for _, violation := range violations {
			if violation.Field == part {
				return errors.New("environment field parent is ambiguous")
			}
		}
		if child, ok := fields[part]; ok {
			node = child
		} else {
			child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part}, child)
			node = child
		}
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return errors.New("environment field parent must be a mapping")
	}
	encoded, err := valueNode(value)
	if err != nil {
		return errors.New("environment field could not be encoded")
	}
	field := parts[len(parts)-1]
	contents := make([]*yaml.Node, 0, len(node.Content)+2)
	replaced := false
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && key.Value == field {
			if !replaced {
				contents = append(contents, key, encoded)
				replaced = true
			}
			continue
		}
		contents = append(contents, key, node.Content[i+1])
	}
	if !replaced {
		contents = append(contents, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}, encoded)
	}
	node.Content = contents
	return nil
}

func removeNodeField(node *yaml.Node, path string) {
	parent, field, found := strings.Cut(path, ".")
	if found {
		fields, _ := entryFields(node)
		removeNodeField(fields[parent], field)
		return
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	contents := make([]*yaml.Node, 0, len(node.Content))
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && key.Value == path {
			continue
		}
		contents = append(contents, key, node.Content[i+1])
	}
	node.Content = contents
}

func expandAliases(node *yaml.Node, active map[*yaml.Node]bool, count *int, depth int) (*yaml.Node, error) {
	if node == nil {
		return nil, nil
	}
	*count++
	if depth > 64 || *count > 100000 || active[node] {
		return nil, errors.New("configuration YAML aliases or nesting exceed the safe bound")
	}
	active[node] = true
	defer delete(active, node)
	if node.Kind == yaml.AliasNode {
		return expandAliases(node.Alias, active, count, depth+1)
	}
	clone := *node
	clone.Anchor = ""
	clone.Alias = nil
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		next, err := expandAliases(child, active, count, depth+1)
		if err != nil {
			return nil, err
		}
		clone.Content[i] = next
	}
	normalizeMappingMerge(&clone)
	return &clone, nil
}

// normalizeMappingMerge applies YAML's explicit-key override and first-merge
// precedence after alias expansion. Malformed merges remain visible to the
// entry or file decoder; normalization must not discard ambiguous fields.
func normalizeMappingMerge(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		return
	}
	mergeIndex := -1
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode && key.Tag == "!!merge" && key.Value == "<<" {
			if mergeIndex >= 0 {
				return
			}
			mergeIndex = i
		}
	}
	if mergeIndex < 0 {
		return
	}
	merge := node.Content[mergeIndex+1]
	sources := []*yaml.Node{merge}
	if merge.Kind == yaml.SequenceNode {
		sources = merge.Content
	}
	for _, source := range sources {
		if _, err := mappingFields(source); err != nil {
			return
		}
	}
	content := make([]*yaml.Node, 0, len(node.Content))
	seen := make(map[string]bool)
	for i := 0; i < len(node.Content); i += 2 {
		if i != mergeIndex {
			content = append(content, node.Content[i], node.Content[i+1])
			seen[node.Content[i].Value] = true
		}
	}
	for _, source := range sources {
		for i := 0; i < len(source.Content); i += 2 {
			key := source.Content[i]
			if !seen[key.Value] {
				content = append(content, key, source.Content[i+1])
				seen[key.Value] = true
			}
		}
	}
	node.Content = content
}

func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	clone := *node
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		clone.Content[i] = cloneNode(child)
	}
	return &clone
}
func cloneNodes(nodes map[string]*yaml.Node) map[string]*yaml.Node {
	if nodes == nil {
		return nil
	}
	clone := make(map[string]*yaml.Node, len(nodes))
	for name, node := range nodes {
		clone[name] = cloneNode(node)
	}
	return clone
}
func cloneInvalidEntries(entries map[string]InvalidEntry) map[string]InvalidEntry {
	if entries == nil {
		return nil
	}
	clone := maps.Clone(entries)
	for name, entry := range clone {
		entry.node = cloneNode(entry.node)
		entry.Violations = slices.Clone(entry.Violations)
		clone[name] = entry
	}
	return clone
}

// nodeMeaning excludes YAML formatting and deleted legacy API versions.
func nodeMeaning(node *yaml.Node, environment bool) any {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		fields := make(map[string]any)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if environment && key == "api_version" {
				continue
			}
			fields[key] = nodeMeaning(node.Content[i+1], false)
		}
		return fields
	}
	children := make([]any, len(node.Content))
	for i, child := range node.Content {
		children[i] = nodeMeaning(child, false)
	}
	return struct {
		Kind       yaml.Kind
		Tag, Value string
		Children   []any
	}{node.Kind, node.Tag, node.Value, children}
}

func sameNodeMeaning(a, b *yaml.Node, environment bool) bool {
	return reflect.DeepEqual(nodeMeaning(a, environment), nodeMeaning(b, environment))
}

func prepareWrite(candidate Config) (Config, []byte, error) {
	if err := validateFile(candidate); err != nil {
		return Config{}, nil, err
	}
	next, err := classifyConfiguration(candidate)
	if err != nil {
		return Config{}, nil, err
	}
	var violations []string
	for _, alias := range slices.Sorted(maps.Keys(next.InvalidEnvironments)) {
		invalid := next.InvalidEnvironments[alias]
		source, exists := candidate.sourceInvalidEnvironments[alias]
		_, replaced := candidate.Environments[alias]
		allowed := exists && !replaced && sameNodeMeaning(invalid.node, source.node, true)
		if !allowed && exists && !replaced && candidate.clearedCredentials[alias] && credentialRefPattern.MatchString(source.CredentialRef) {
			cleared := cloneNode(source.node)
			removeNodeField(cleared, "auth.credential_ref")
			allowed = sameNodeMeaning(invalid.node, cleared, true)
		}
		if allowed {
			for _, v := range invalid.Violations {
				if !slices.Contains(source.Violations, v) {
					allowed = false
					break
				}
			}
		}
		if !allowed {
			violations = append(violations, violationSummary("environment", alias, invalid.Violations))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(next.InvalidWorkspaces)) {
		invalid := next.InvalidWorkspaces[name]
		source, exists := candidate.sourceInvalidWorkspaces[name]
		_, replaced := candidate.Workspaces[name]
		allowed := exists && !replaced && sameNodeMeaning(invalid.node, source.node, false)
		if allowed {
			for _, v := range invalid.Violations {
				if !slices.Contains(source.Violations, v) {
					allowed = false
					break
				}
			}
		}
		if !allowed {
			violations = append(violations, violationSummary("workspace", name, invalid.Violations))
		}
	}
	if candidate.DefaultEnvironment != "" && candidate.DefaultEnvironment != candidate.sourceDefaultEnvironment {
		if _, err := next.ResolveEnvironment(candidate.DefaultEnvironment); err != nil {
			violations = append(violations, err.Error())
		}
	}
	if candidate.DefaultWorkspace != "" && candidate.DefaultWorkspace != candidate.sourceDefaultWorkspace {
		if _, _, err := next.ResolveWorkspace(candidate.DefaultWorkspace); err != nil {
			violations = append(violations, err.Error())
		}
	}
	if len(violations) > 0 {
		return Config{}, nil, &ValidationError{Violations: violations}
	}
	root, err := valueNode(struct {
		Version            int            `yaml:"version"`
		DefaultEnvironment string         `yaml:"default_environment,omitempty"`
		DefaultWorkspace   string         `yaml:"default_workspace,omitempty"`
		SiteMutations      []SiteMutation `yaml:"site_mutations,omitempty"`
	}{next.Version, next.DefaultEnvironment, next.DefaultWorkspace, next.SiteMutations})
	if err != nil {
		return Config{}, nil, errors.New("encode configuration fields")
	}
	appendEntries := func(field string, nodes map[string]*yaml.Node) {
		if len(nodes) == 0 {
			return
		}
		mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, name := range slices.Sorted(maps.Keys(nodes)) {
			mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, nodes[name])
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}, mapping)
	}
	environments := make(map[string]*yaml.Node)
	for _, alias := range next.EnvironmentAliases() {
		node, err := next.environmentNode(alias)
		if err != nil {
			return Config{}, nil, err
		}
		removeNodeField(node, "api_version")
		environments[alias] = node
	}
	appendEntries("environments", environments)
	workspaces := make(map[string]*yaml.Node)
	for name, registration := range next.Workspaces {
		node, err := valueNode(registration)
		if err != nil {
			return Config{}, nil, err
		}
		workspaces[name] = node
	}
	for name, invalid := range next.InvalidWorkspaces {
		workspaces[name] = cloneNode(invalid.node)
	}
	appendEntries("workspaces", workspaces)
	for _, name := range slices.Sorted(maps.Keys(next.unknownFields)) {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, cloneNode(next.unknownFields[name]))
	}
	data, err := yaml.Marshal(root)
	if err != nil {
		return Config{}, nil, errors.New("encode configuration")
	}
	if len(data) > maxConfigBytes {
		return Config{}, nil, fmt.Errorf("configuration exceeds %d bytes", maxConfigBytes)
	}
	return next, data, nil
}
