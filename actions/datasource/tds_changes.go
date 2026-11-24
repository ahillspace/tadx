package datasource

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"gopkg.in/yaml.v3"
)

const maxEditCount = 50
const maxEditBytes = 256 << 10

type EditTarget struct {
	Server         string `yaml:"server" json:"server"`
	SiteLUID       string `yaml:"site_luid" json:"site_luid"`
	DatasourceLUID string `yaml:"datasource_luid,omitempty" json:"datasource_luid,omitempty"`
	WorkCopyLUID   string `yaml:"work_copy_luid" json:"work_copy_luid,omitempty"`
}

func (expected EditTarget) Match(actual EditTarget) error {
	server, err := config.CanonicalMutationServer(actual.Server)
	if err != nil {
		return editUsage("Invalid authenticated server.")
	}
	if expected.Server != server || !strings.EqualFold(expected.SiteLUID, actual.SiteLUID) || !strings.EqualFold(expected.WorkCopyLUID, actual.WorkCopyLUID) || (expected.DatasourceLUID != "" && !strings.EqualFold(actual.DatasourceLUID, expected.DatasourceLUID)) {
		return editUsage("The changes file target does not match the authenticated server, site, work copy, or verified source.")
	}
	return nil
}

type EditDocument struct {
	Target  EditTarget   `yaml:"target" json:"target"`
	Changes []EditChange `yaml:"changes" json:"changes"`
}

type EditChange struct {
	Op          string `yaml:"op" json:"op"`
	Field       string `yaml:"field,omitempty" json:"field,omitempty"`
	Table       string `yaml:"table,omitempty" json:"table,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Caption     string `yaml:"caption,omitempty" json:"caption,omitempty"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Role        string `yaml:"role,omitempty" json:"role,omitempty"`
}

type EditSelector struct {
	ElementType string `json:"element_type"`
	Name        string `json:"name"`
	Property    string `json:"property"`
}
type EditState struct {
	Presence string `json:"presence"`
	Value    string `json:"value"`
}

func (c EditChange) CapabilityID() string { return "datasource." + c.Op }

func (c EditChange) Selector() EditSelector {
	switch c.Op {
	case "set-field-description", "remove-field-description":
		return EditSelector{"FieldDescription", c.Field, "description"}
	case "set-table-description", "remove-table-description":
		return EditSelector{"TableDescription", c.Table, "description"}
	case "set-column-caption":
		return EditSelector{"Column", c.Field, "caption"}
	case "set-column-type":
		return EditSelector{"Column", c.Field, "type"}
	case "set-column-role":
		return EditSelector{"Column", c.Field, "role"}
	case "delete-column":
		return EditSelector{"Column", c.Field, "element"}
	default:
		return EditSelector{}
	}
}

func (c EditChange) Intended() EditState {
	switch c.Op {
	case "set-field-description", "set-table-description":
		return EditState{"present", c.Description}
	case "set-column-caption":
		return EditState{"present", c.Caption}
	case "set-column-role":
		return EditState{"present", c.Role}
	case "set-column-type":
		return EditState{"present", c.Type}
	default:
		return EditState{Presence: "absent"}
	}
}

func ValidateEditChanges(changes []EditChange) error {
	if len(changes) == 0 || len(changes) > maxEditCount {
		return editUsage("Activate one to 50 entries in the changes list.")
	}
	seen := map[EditSelector]bool{}
	columnEdits := map[string]bool{}
	deleted := map[string]bool{}
	for i, c := range changes {
		s := c.Selector()
		if s.ElementType == "" {
			return editUsage(fmt.Sprintf("Entry %d has an unsupported operation.", i+1))
		}
		expected := EditChange{Op: c.Op}
		if s.ElementType == "TableDescription" {
			expected.Table = c.Table
		} else {
			expected.Field = c.Field
		}
		switch c.Op {
		case "set-field-description", "set-table-description":
			expected.Description = c.Description
		case "set-column-caption":
			expected.Caption = c.Caption
		case "set-column-role":
			expected.Role = c.Role
		case "set-column-type":
			expected.Type = c.Type
		}
		wanted := c.Intended()
		if expected != c || strings.TrimSpace(s.Name) == "" || !utf8.ValidString(s.Name) || (wanted.Presence == "present" && (!utf8.ValidString(wanted.Value) || strings.TrimSpace(wanted.Value) == "")) {
			return editUsage(fmt.Sprintf("Entry %d requires an exact name and the properties for %s.", i+1, c.Op))
		}
		if c.Type == "FIELD_TYPE_UNKNOWN" || c.Role == "FIELD_ROLE_UNKNOWN" || c.Type == "UNRECOGNIZED" || c.Role == "UNRECOGNIZED" {
			return editUsage(fmt.Sprintf("Entry %d uses a rejected column type or role.", i+1))
		}
		if seen[s] || s.ElementType != "TableDescription" && (deleted[s.Name] || c.Op == "delete-column" && columnEdits[s.Name]) {
			return editUsage(fmt.Sprintf("Entry %d conflicts with another edit to the same target.", i+1))
		}
		seen[s] = true
		if s.ElementType != "TableDescription" {
			columnEdits[s.Name] = true
		}
		if c.Op == "delete-column" {
			deleted[s.Name] = true
		}
	}
	return nil
}

func editUsage(message string) error {
	return &errs.Error{ID: "datasource.tds.usage", Kind: errs.KindUsage, Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Summary: message, Retryable: new(false)}
}

// LoadEditDocument reads and validates a single workspace-bound intent before a
// Tableau session is selected. Its target values never choose a destination.
func LoadEditDocument(workspace, path string) (EditDocument, error) {
	data, err := readEditFile(workspace, path)
	if err != nil {
		return EditDocument{}, editFileUsage("--from-changes", err)
	}
	return parseEditDocument(data)
}

func parseEditDocument(data []byte) (EditDocument, error) {
	var doc EditDocument
	if len(data) > maxEditBytes || !utf8.Valid(data) {
		return doc, editUsage("The --from-changes file must be UTF-8 and at most 256 KiB.")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var tree yaml.Node
	if err := decoder.Decode(&tree); err != nil {
		return doc, editUsage("The --from-changes file must contain one YAML mapping.")
	}
	if err := decoder.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return doc, editUsage("The --from-changes file must contain one YAML document.")
	}
	if err := checkEditNode(&tree, 0); err != nil {
		return doc, err
	}
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return EditDocument{}, editUsage("The --from-changes file contains an unknown or duplicate key, or an invalid structure.")
	}
	server, err := config.CanonicalMutationServer(doc.Target.Server)
	if err != nil || !value.ValidTDSLUID(doc.Target.SiteLUID) || !value.ValidTDSLUID(doc.Target.WorkCopyLUID) || (doc.Target.DatasourceLUID != "" && !value.ValidTDSLUID(doc.Target.DatasourceLUID)) {
		return EditDocument{}, editUsage("The --from-changes target requires an HTTPS server and valid site and work-copy LUIDs, plus a valid source LUID if specified.")
	}
	doc.Target.Server = server
	if err := ValidateEditChanges(doc.Changes); err != nil {
		return EditDocument{}, err
	}
	return doc, nil
}

func checkEditNode(n *yaml.Node, depth int) error {
	if depth > 6 || n.Anchor != "" || n.Kind == yaml.AliasNode || n.Style&yaml.TaggedStyle != 0 || n.Kind == yaml.ScalarNode && (n.ShortTag() != "!!str" || n.Value == "") {
		return editUsage("The --from-changes document must use plain mappings, lists and nonempty text. Explicit tags, aliases, anchors and typed scalar values are unsupported.")
	}
	for _, child := range n.Content {
		if err := checkEditNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}
