// Package value contains dependency-free, shared identity and normalized metadata values.
// Action inputs, outputs, behavior, and provider payloads remain in their owning packages.
package value

// ContentIdentity identifies exact REST content and its containing project.
type ContentIdentity struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
}

// OwnedContentIdentity adds the owner required by move and update actions.
type OwnedContentIdentity struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
}

// ProjectIdentity identifies one authoritative project destination.
type ProjectIdentity struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
	Path string `json:"path"`
}
