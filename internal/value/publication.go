package value

// PublishDestination identifies the exact state selected by read-only publish
// planning. A retained pointer with an empty ExistingLUID requires absence;
// a nil pointer means no earlier destination was selected.
type PublishDestination struct {
	Environment  string `json:"environment"`
	Site         string `json:"site"`
	ProjectLUID  string `json:"project_luid"`
	Name         string `json:"name"`
	ExistingLUID string `json:"existing_luid"`
}
