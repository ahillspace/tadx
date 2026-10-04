package value

// PublicationDestination identifies the saved exact target of an accepted write.
type PublicationDestination struct {
	Operation, ResourceID, Name, ProjectID string
}

// ResourceDestination is native evidence, not a claim about mutation success.
type ResourceDestination struct {
	ResourceID, Name, ProjectID string
}
