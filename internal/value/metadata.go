package value

import "strings"

// CanonicalContentType maps documented native label target spellings to the
// stable command names while leaving unknown values unchanged.
func CanonicalContentType(s string) string {
	switch strings.ToLower(s) {
	case "database":
		return "database"
	case "table":
		return "table"
	case "column":
		return "column"
	case "datasource", "data_source", "data-source":
		return "datasource"
	case "flow":
		return "flow"
	default:
		return s
	}
}

type MetadataQuery struct {
	LUID, MetadataID, Name, Text, ParentLUID, Cursor string
	Limit                                            int
}

type MetadataUpdate struct{ Description, ContactLUID *string }

type MetadataDatasourceDescriptions struct {
	LUID             string
	Identity         MetadataIdentity
	Description      *string
	Tags             []string
	TagsObserved     bool
	Fields           []FieldDescription
	Complete         bool
	ObservedAt       string
	TableauRequestID string
}

type MetadataPage[T any] struct {
	Items            []T
	NextCursor       string
	Total            int
	Complete         bool
	ObservedAt       string
	TableauRequestID string
}

// MetadataIdentity preserves the distinct GraphQL and REST namespaces.
type MetadataIdentity struct {
	MetadataID string `json:"metadata_id,omitempty"`
	LUID       string `json:"luid,omitempty"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

type MetadataDatabase struct {
	MetadataIdentity
	Description    *string  `json:"description,omitempty"`
	ContactLUID    string   `json:"contact_luid,omitempty"`
	ConnectionType string   `json:"connection_type,omitempty"`
	FilePath       string   `json:"file_path,omitempty"`
	Embedded       bool     `json:"embedded"`
	Tags           []string `json:"tags,omitempty"`
	TagsObserved   bool     `json:"tags_observed"`
}

type MetadataTable struct {
	MetadataIdentity
	Description  *string          `json:"description,omitempty"`
	ContactLUID  string           `json:"contact_luid,omitempty"`
	Database     MetadataIdentity `json:"database"`
	FullName     string           `json:"full_name,omitempty"`
	Schema       string           `json:"schema,omitempty"`
	Tags         []string         `json:"tags,omitempty"`
	TagsObserved bool             `json:"tags_observed"`
}

type MetadataColumn struct {
	MetadataIdentity
	Description  *string          `json:"description,omitempty"`
	Table        MetadataIdentity `json:"table"`
	RemoteType   string           `json:"remote_type,omitempty"`
	Nullable     *bool            `json:"nullable,omitempty"`
	Tags         []string         `json:"tags,omitempty"`
	TagsObserved bool             `json:"tags_observed"`
}

type DescriptionObservation struct {
	Value            *string  `json:"value,omitempty"`
	SourceMetadataID string   `json:"source_metadata_id"`
	Attribute        string   `json:"attribute"`
	Distance         *int     `json:"distance,omitempty"`
	Edges            []string `json:"edges,omitempty"`
}

type FieldDescription struct {
	MetadataID         string                   `json:"metadata_id"`
	Name               string                   `json:"name"`
	FullyQualifiedName string                   `json:"fully_qualified_name"`
	Description        *string                  `json:"description,omitempty"`
	Inherited          []DescriptionObservation `json:"inherited,omitempty"`
	InheritedObserved  bool                     `json:"inherited_observed"`
	UpstreamColumns    []MetadataColumn         `json:"upstream_columns,omitempty"`
}

type LabelTarget struct {
	Type string `json:"type"`
	LUID string `json:"luid"`
}
type LabelUpdate struct {
	Value    string `json:"value"`
	Message  string `json:"message"`
	Active   bool   `json:"active"`
	Elevated bool   `json:"elevated"`
}

type ContentLabel struct {
	LUID       string `json:"luid"`
	TargetLUID string `json:"target_luid"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	Message    string `json:"message,omitempty"`
	Active     bool   `json:"active"`
	Elevated   bool   `json:"elevated"`
	OwnerLUID  string `json:"owner_luid,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type LabelValue struct {
	Name            string `json:"name"`
	Category        string `json:"category"`
	Description     string `json:"description"`
	Internal        bool   `json:"internal"`
	ElevatedDefault bool   `json:"elevated_default"`
	BuiltIn         bool   `json:"built_in"`
}

type LabelCategory struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
