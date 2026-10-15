// Package workspace implements named workspace and local artifact workflows.
package workspace

// Service supplies cohesive workspace operations directly to the command layer.
// Each dependency is scoped to the operation that consumes it.
type Service struct {
	Creator        Creator
	Registrar      Registrar
	Cloner         Cloner
	Lister         Lister
	Reader         StatusReader
	Setter         DefaultSetter
	Registry       Registry
	WorkspaceStore WorkspaceStore
	Mover          Mover
	ArtifactStore  ArtifactStore
	Cleaner        CleanStore
}

// Workspace is the stable identity shared by workspace operations.
type Workspace struct {
	Name       string   `json:"name"`
	ID         string   `json:"id,omitempty"`
	Root       string   `json:"root,omitempty"`
	Status     string   `json:"status,omitempty"`
	Violations []string `json:"violations,omitempty"`
}

// Registration is the manifest and registration result for create, register and clone.
type Registration struct {
	Workspace
	ManifestVersion int  `json:"manifest_version"`
	Registered      bool `json:"registered"`
}

// CreatedWorkspace adds the entries created by create and clone.
// Registration output deliberately does not include this field.
type CreatedWorkspace struct {
	Registration
	CreatedEntries []string `json:"created_entries"`
}

func createdWorkspace(registration Registration) CreatedWorkspace {
	return CreatedWorkspace{Registration: registration, CreatedEntries: []string{"tadx.yaml", "artifacts", ".tadx"}}
}

type compactWorkspaceIdentity struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

const maxMutationWarnings = 20

func boundMutationWarnings(input []string) ([]string, int) {
	unique := make([]string, 0, min(len(input), maxMutationWarnings))
	seen := make(map[string]bool, len(input))
	for _, warning := range input {
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		unique = append(unique, warning)
	}
	if len(unique) <= maxMutationWarnings {
		return unique, 0
	}
	return unique[:maxMutationWarnings], len(unique) - maxMutationWarnings
}
