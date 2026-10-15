package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// classifyConfiguration computes entry usability over one complete candidate.
// Cross-entry groups include partially decoded entries so quarantine never hides
// a credential identity, variable reference, or registered root from validation.
func classifyConfiguration(candidate Config) (Config, error) {
	c := cloneConfig(candidate)
	environmentNodes := make(map[string]*yaml.Node)
	for alias, invalid := range c.InvalidEnvironments {
		if invalid.node == nil {
			return Config{}, fmt.Errorf("environment %q has no original YAML", alias)
		}
		environmentNodes[alias] = invalid.node
	}
	for alias, environment := range c.Environments {
		node, err := valueNode(environment)
		if err != nil {
			return Config{}, err
		}
		environmentNodes[alias] = node
	}
	workspaceNodes := make(map[string]*yaml.Node)
	for name, invalid := range c.InvalidWorkspaces {
		if invalid.node == nil {
			return Config{}, fmt.Errorf("workspace %q has no original YAML", name)
		}
		workspaceNodes[name] = invalid.node
	}
	for name, registration := range c.Workspaces {
		node, err := valueNode(registration)
		if err != nil {
			return Config{}, err
		}
		workspaceNodes[name] = node
	}
	c.Environments = make(map[string]Environment)
	c.Workspaces = make(map[string]WorkspaceRegistration)
	c.InvalidEnvironments = make(map[string]InvalidEntry)
	c.InvalidWorkspaces = make(map[string]InvalidEntry)
	workspaceEntries := make(map[string]InvalidEntry)
	workspaceIDs := make(map[string][]string)
	workspaceRoots := make(map[string][]string)
	for _, name := range slices.Sorted(maps.Keys(workspaceNodes)) {
		registration, violations := decodeWorkspace(workspaceNodes[name])
		if err := ValidateWorkspaceName(name); err != nil {
			violations = append(violations, EntryViolation{Field: "name", Rule: err.Error()})
		}
		if len(workspaceNodes) > maxWorkspaces {
			violations = append(violations, EntryViolation{Field: "registry", Rule: fmt.Sprintf("must not exceed %d entries", maxWorkspaces)})
		}
		if !workspaceIDPattern.MatchString(registration.ID) {
			violations = append(violations, EntryViolation{Field: "id", Rule: "must match ws_<32 lowercase hex>"})
		} else {
			workspaceIDs[registration.ID] = append(workspaceIDs[registration.ID], name)
		}
		if root, err := canonicalWorkspaceRoot(registration.Path); err != nil {
			violations = append(violations, EntryViolation{Field: "path", Rule: "must identify a nonempty canonical root"})
		} else {
			key := workspaceRootKey(root)
			workspaceRoots[key] = append(workspaceRoots[key], name)
		}
		workspaceEntries[name] = InvalidEntry{node: workspaceNodes[name], workspace: registration, Violations: violations}
	}
	addWorkspaceCollision := func(field, rule string, names []string) {
		for _, name := range names {
			entry := workspaceEntries[name]
			for _, other := range names {
				if other != name {
					entry.Violations = append(entry.Violations, EntryViolation{Field: field, Rule: rule, OtherEntry: other})
				}
			}
			workspaceEntries[name] = entry
		}
	}
	for _, names := range workspaceIDs {
		if len(names) > 1 {
			addWorkspaceCollision("id", "must not be shared by registrations", names)
		}
	}
	for _, names := range workspaceRoots {
		if len(names) > 1 {
			addWorkspaceCollision("path", "canonical root must not be shared by registrations", names)
		}
	}
	workspaceNames := slices.Sorted(maps.Keys(workspaceEntries))
	for i, name := range workspaceNames {
		for _, other := range workspaceNames[i+1:] {
			if strings.EqualFold(name, other) {
				addWorkspaceCollision("name", "must be unique under case-insensitive matching", []string{name, other})
			}
		}
	}
	for name, entry := range workspaceEntries {
		sortViolations(entry.Violations)
		if len(entry.Violations) > 0 {
			c.InvalidWorkspaces[name] = entry
		} else {
			c.Workspaces[name] = entry.workspace
		}
	}
	type variableOwner struct {
		alias, field string
		defaulted    bool
	}
	variables := make(map[string][]variableOwner)
	credentials := make(map[string][]string)
	environmentEntries := make(map[string]InvalidEntry)
	for _, alias := range slices.Sorted(maps.Keys(environmentNodes)) {
		environment, violations := decodeEnvironment(environmentNodes[alias])
		if strings.TrimSpace(alias) == "" {
			violations = append(violations, EntryViolation{Field: "alias", Rule: "must not be empty"})
		}
		if validateServerURL(environment.URL) != nil {
			violations = append(violations, EntryViolation{Field: "url", Rule: "must be an HTTPS server URL without credentials, a query, or a fragment"})
		}
		if environment.CacheMaxConcurrency < 0 || environment.CacheMaxConcurrency > 256 {
			violations = append(violations, EntryViolation{Field: "cache_max_concurrency", Rule: "must be between 1 and 256, or omitted for the default"})
		}
		if environment.Auth.Type != AuthTypePAT {
			violations = append(violations, EntryViolation{Field: "auth.type", Rule: "must be pat"})
		}
		for _, reference := range []struct{ field, value string }{{"pat_name_env", environment.Auth.PATNameEnv}, {"pat_secret_env", environment.Auth.PATSecretEnv}} {
			if reference.value != "" && !ValidVariableReference(reference.value) {
				violations = append(violations, EntryViolation{Field: reference.field, Rule: VariableReferenceRule + "; a PAT saved there should be revoked", SecretRisk: true})
			}
		}
		reference := environment.Auth.CredentialRef
		if reference != "" {
			if !credentialRefPattern.MatchString(reference) {
				violations = append(violations, EntryViolation{Field: "credential_ref", Rule: "must match cred_<32 lowercase hex>"})
			} else {
				credentials[reference] = append(credentials[reference], alias)
			}
		}
		if environment.DefaultWorkspace != "" {
			if looksLikePath(environment.DefaultWorkspace) {
				violations = append(violations, EntryViolation{Field: "default_workspace", Rule: "must be a logical name, not a path"})
			}
		}
		defaultName, defaultSecret := DefaultPATVariableNames(alias)
		nameVariable, secretVariable := environment.Auth.PATNameEnv, environment.Auth.PATSecretEnv
		if nameVariable == "" {
			nameVariable = defaultName
		}
		if secretVariable == "" {
			secretVariable = defaultSecret
		}
		if strings.EqualFold(nameVariable, secretVariable) {
			violations = append(violations, EntryViolation{Field: "pat_name_env", Rule: "PAT name and secret must use different variables"}, EntryViolation{Field: "pat_secret_env", Rule: "PAT name and secret must use different variables"})
		}
		if ValidVariableReference(nameVariable) {
			key := strings.ToUpper(nameVariable)
			variables[key] = append(variables[key], variableOwner{alias, "pat_name_env", environment.Auth.PATNameEnv == ""})
		}
		if ValidVariableReference(secretVariable) {
			key := strings.ToUpper(secretVariable)
			variables[key] = append(variables[key], variableOwner{alias, "pat_secret_env", environment.Auth.PATSecretEnv == ""})
		}
		entry := InvalidEntry{node: environmentNodes[alias], environment: environment, Violations: violations}
		if credentialRefPattern.MatchString(reference) {
			entry.CredentialRef = reference
		}
		environmentEntries[alias] = entry
	}
	for _, aliases := range credentials {
		for _, alias := range aliases {
			entry := environmentEntries[alias]
			for _, other := range aliases {
				if other != alias {
					entry.Violations = append(entry.Violations, EntryViolation{Field: "credential_ref", Rule: "must not be shared by environments", OtherEntry: other})
				}
			}
			environmentEntries[alias] = entry
		}
	}
	for _, owners := range variables {
		for i, owner := range owners {
			for _, other := range owners[i+1:] {
				if owner.alias == other.alias || (!owner.defaulted && !other.defaulted) {
					continue
				}
				for _, pair := range [][2]variableOwner{{owner, other}, {other, owner}} {
					entry := environmentEntries[pair[0].alias]
					entry.Violations = append(entry.Violations, EntryViolation{Field: pair[0].field, Rule: "conflicts with another environment because at least one reference uses the default", OtherEntry: pair[1].alias})
					environmentEntries[pair[0].alias] = entry
				}
			}
		}
	}
	for alias, entry := range environmentEntries {
		sortViolations(entry.Violations)
		entry.Violations = slices.Compact(entry.Violations)
		if len(entry.Violations) > 0 {
			c.InvalidEnvironments[alias] = entry
		} else {
			c.Environments[alias] = entry.environment
		}
	}
	return c, nil
}

func sortViolations(violations []EntryViolation) {
	slices.SortFunc(violations, func(a, b EntryViolation) int {
		if n := strings.Compare(a.Field, b.Field); n != 0 {
			return n
		}
		if n := strings.Compare(a.Rule, b.Rule); n != 0 {
			return n
		}
		return strings.Compare(a.OtherEntry, b.OtherEntry)
	})
}
