package datasource

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/pathspec"
	"github.com/ahillspace/tadx/internal/value"
	"gopkg.in/yaml.v3"
)

type EditExports struct {
	AuditFile   string `json:"audit_file,omitempty"`
	ChangesFile string `json:"changes_file,omitempty"`
}

// editParent pins each checked directory before opening the next component.
// Callers operate on one leaf in its parent, including the final no-replace link.
func editParent(workspace, rel string) (*os.Root, string, error) {
	if workspace == "" || !fs.ValidPath(rel) || rel == "." || pathspec.IsAbs(rel) || strings.ContainsAny(rel, "\\:") {
		return nil, "", errors.New("expected a workspace-relative path with forward slashes")
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") || !filepath.IsLocal(part) || strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") {
			return nil, "", errors.New("workspace file paths cannot contain hidden or aliased components")
		}
	}
	info, err := os.Lstat(workspace)
	if err != nil || !editDirectory(info) {
		return nil, "", errors.New("the selected workspace root is unavailable or linked")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, "", err
	}
	pinned, err := root.Stat(".")
	if err != nil || !os.SameFile(info, pinned) {
		root.Close()
		return nil, "", errors.New("workspace root changed while opening")
	}
	for _, dir := range parts[:len(parts)-1] {
		info, err := root.Lstat(dir)
		if err != nil || !editDirectory(info) {
			root.Close()
			return nil, "", fmt.Errorf("workspace subdirectory %q is unavailable or linked", dir)
		}
		next, err := root.OpenRoot(dir)
		root.Close()
		if err != nil {
			return nil, "", err
		}
		pinned, err := next.Stat(".")
		if err != nil || !os.SameFile(info, pinned) {
			next.Close()
			return nil, "", errors.New("workspace subdirectory changed while opening")
		}
		root = next
	}
	return root, path.Base(rel), nil
}

func readEditFile(workspace, rel string) ([]byte, error) {
	dir, name, err := editParent(workspace, rel)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	info, err := dir.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxEditBytes {
		return nil, errors.New("changes file must be a regular file of at most 256 KiB")
	}
	f, err := openEditInput(dir, name, info)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxEditBytes+1))
	if len(data) > maxEditBytes {
		return nil, errors.New("changes file exceeds 256 KiB")
	}
	return data, err
}

func ValidateEditExports(workspace, audit, changes string) error {
	if audit != "" && strings.EqualFold(audit, changes) {
		return editUsage("Choose two distinct workspace-relative output paths.")
	}
	for _, p := range []string{audit, changes} {
		if p == "" {
			continue
		}
		dir, name, err := editParent(workspace, p)
		if err != nil {
			return editUsage(errs.WithWorkspaceRelativePaths(err, workspace).Error())
		}
		_, err = dir.Lstat(name)
		dir.Close()
		if err == nil {
			return editUsage("Output already exists: " + p)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return editUsage(errs.WithWorkspaceRelativePaths(err, workspace).Error())
		}
	}
	return nil
}

func createEditFile(workspace, rel string, body []byte) error {
	dir, name, err := editParent(workspace, rel)
	if err != nil {
		return err
	}
	defer dir.Close()
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := ".tadx-tds-" + hex.EncodeToString(nonce[:])
	f, err := dir.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(tmp)
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return dir.Link(tmp, name)
}

func editableDescriptionFields(inspect CopyInspection) map[string]bool {
	// An exact column name can supply an example if its description was
	// observable. An explicit named observation supersedes the earlier collection.
	eligible := map[string]bool{}
	descriptions := map[string]bool{}
	completeDescriptions := false
	for _, q := range inspect.Observations {
		switch q.ElementType {
		case "Column":
			if q.Name != "" {
				delete(eligible, q.Name)
			}
			for _, e := range q.Elements {
				if e.Name != "" && e.Error == nil && e.Column != nil {
					eligible[e.Name] = true
				}
			}
		case "FieldDescription":
			if q.Name == "" {
				completeDescriptions = q.Complete
			} else {
				descriptions[q.Name] = q.Complete && q.Absent
			}
			for _, e := range q.Elements {
				descriptions[e.Name] = e.Error == nil && e.Description != nil
			}
		}
	}
	for name := range eligible {
		observed, known := descriptions[name]
		if known && !observed || !known && !completeDescriptions {
			delete(eligible, name)
		}
	}
	return eligible
}

func editTemplate(inspect CopyInspection, fields []string, redact func(string) string) ([]byte, error) {
	eligible := editableDescriptionFields(inspect)
	if len(fields) == 0 {
		for name := range eligible {
			fields = append(fields, name)
		}
		slices.Sort(fields)
		fields = fields[:min(len(fields), 5)]
	}
	examples := make([]EditChange, 0, len(fields))
	for _, name := range fields {
		if redact(name) != name {
			return nil, errors.New("an observed template binding contains protected credential material")
		}
		if !eligible[name] {
			return nil, fmt.Errorf("template field %q lacks an exact readable column or description observation", name)
		}
		examples = append(examples, EditChange{Op: "set-field-description", Field: name, Description: "Describe the meaning of this field."})
	}
	if len(examples) > 0 {
		if err := ValidateEditChanges(examples); err != nil {
			return nil, err
		}
	}
	for _, binding := range []string{inspect.Server, inspect.SiteLUID, inspect.WorkCopyLUID, inspect.DatasourceLUID} {
		if redact(binding) != binding {
			return nil, errors.New("an observed target binding contains protected credential material")
		}
	}
	base, err := yaml.Marshal(EditDocument{Target: inspect.EditTarget, Changes: []EditChange{}})
	if err != nil {
		return nil, err
	}

	base = append(base, []byte("# Use field + description for set-field-description, or table + description for set-table-description.\n"+
		"# To remove only a description, use remove-field-description or remove-table-description with its exact name.\n"+
		"# Column edits use field and one setting: set-column-caption (caption), set-column-type (type), set-column-role (role).\n"+
		"# delete-column with field removes the entire column from the work copy.\n")...)
	if len(examples) == 0 {
		return base, nil
	}
	list, err := yaml.Marshal(struct {
		Changes []EditChange `yaml:"changes"`
	}{examples})
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Write(base)
	out.WriteString("# Replace the active changes: [] line with the entries to apply.\n")
	for _, line := range strings.Split(strings.TrimSuffix(string(list), "\n"), "\n") {
		out.WriteString("# " + line + "\n")
	}
	if out.Len() > maxEditBytes {
		return nil, errors.New("template exceeds the 256 KiB changes-file limit")
	}
	return out.Bytes(), nil
}

func editAudit(inspect CopyInspection, redact func(string) string) ([]byte, error) {
	var doc bytes.Buffer
	doc.WriteString("# Datasource work-copy audit\n\n")
	section := func(title string, data any) error {
		raw, err := output.SnapshotRedacted(data, 8<<20, redact)
		if err != nil {
			return err
		}
		var pretty bytes.Buffer
		if err = json.Indent(&pretty, raw, "", "  "); err != nil {
			return err
		}
		doc.WriteString("## " + title + "\n\n")
		for _, line := range strings.Split(pretty.String(), "\n") {
			doc.WriteString("    " + line + "\n")
		}
		doc.WriteByte('\n')
		return nil
	}
	if err := section("Target", inspect.EditTarget); err != nil {
		return nil, err
	}
	switch {
	case inspect.Checkout != nil:
		if err := section("Source at checkout", inspect.Checkout); err != nil {
			return nil, err
		}
		doc.WriteString("The local receipt records the published source identity at checkout and when Tableau acknowledged the work copy. Subsequent source renames and moves may change the current published identity details.\n\n")
	case inspect.DatasourceLUID != "":
		doc.WriteString("The checkout receipt identifies the source LUID. Historical source name, project and acknowledgement time were unavailable.\n\n")
	default:
		doc.WriteString("Source provenance is unknown.\n\n")
	}
	doc.WriteString("The observations below come from the work copy at " + inspect.ObservedAt + ". Each group reports its own read eligibility and coverage. Write eligibility requires evidence from the corresponding write operation.\n\n")
	if err := section("Tableau TDS observations", inspect.Observations); err != nil {
		return nil, err
	}
	doc.WriteString("## Edit support\n\nUse exact TDS column names for field descriptions and column edits. Table descriptions use exact TDS table names. The template contains inactive field-description examples. Review capability availability before applying changes.\n\n")
	names := []string{}
	for name := range editableDescriptionFields(inspect) {
		names = append(names, name)
	}
	slices.Sort(names)
	if err := section("Observed field-description candidates", names); err != nil {
		return nil, err
	}
	doc.WriteString("These selectors have readable TDS columns and description observations in this audit. Missing, omitted or failed observations leave other fields unqualified. Tableau checks permissions, entitlement and source eligibility independently for each request.\n\n")
	reason := inspect.MetadataWarning
	if metadata := inspect.SourceMetadata; metadata != nil {
		// Keep indexed fields independent of the TDS selectors and report any
		// omissions from the audit's own projection of provider observations.
		type auditField struct {
			value.FieldDescription
			UpstreamOmitted int `json:"upstream_columns_omitted,omitempty"`
		}
		limit := min(len(metadata.Fields), maxTDSElements)
		fields := make([]auditField, 0, limit)
		for _, field := range metadata.Fields[:limit] {
			count := min(len(field.UpstreamColumns), maxTDSElements)
			omitted := len(field.UpstreamColumns) - count
			field.UpstreamColumns = field.UpstreamColumns[:count]
			fields = append(fields, auditField{field, omitted})
		}
		view := struct {
			Identity         value.MetadataIdentity `json:"datasource"`
			Description      *string                `json:"description,omitempty"`
			Tags             []string               `json:"tags,omitempty"`
			TagsObserved     bool                   `json:"tags_observed"`
			Fields           []auditField           `json:"fields"`
			FieldsOmitted    int                    `json:"fields_omitted"`
			ProviderComplete bool                   `json:"provider_complete"`
			ObservedAt       string                 `json:"observed_at"`
			TableauRequestID string                 `json:"tableau_request_id,omitempty"`
		}{
			Identity: metadata.Identity, Description: metadata.Description, Tags: metadata.Tags,
			TagsObserved: metadata.TagsObserved, Fields: fields, FieldsOmitted: len(metadata.Fields) - limit,
			ProviderComplete: metadata.Complete, ObservedAt: metadata.ObservedAt, TableauRequestID: metadata.TableauRequestID,
		}
		if err := section("Published datasource Metadata API context", view); err != nil {
			reason = "The indexed metadata could not be included in the bounded audit."
		}
	} else if reason == "" {
		reason = "Published datasource metadata was unavailable for this inspection."
	}
	if reason != "" {
		if err := section("Published datasource Metadata API coverage", reason); err != nil {
			return nil, err
		}
	}
	if err := section("Inspect again", inspect.Help); err != nil {
		return nil, err
	}
	return doc.Bytes(), nil
}

// ExportEditCopy commits each output independently and returns committed paths
// if a sibling export fails. A late name collision never replaces another file.
func ExportEditCopy(workspace, audit, changes string, inspect CopyInspection, templateFields []string, redact func(string) string) (EditExports, error) {
	var out EditExports
	var failures []error
	if audit != "" && strings.EqualFold(audit, changes) {
		return out, editUsage("Choose distinct audit and changes paths.")
	}
	if audit != "" {
		data, err := editAudit(inspect, redact)
		if err == nil {
			err = createEditFile(workspace, audit, data)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("audit %q: %w", audit, err))
		} else {
			out.AuditFile = audit
		}
	}
	if changes != "" {
		data, err := editTemplate(inspect, templateFields, redact)
		if err == nil {
			err = createEditFile(workspace, changes, data)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("changes %q: %w", changes, err))
		} else {
			out.ChangesFile = changes
		}
	}
	if cause := errors.Join(failures...); cause != nil {
		return out, &errs.Error{ID: "datasource.tds.export_failed", Kind: errs.KindOperation, Summary: "One or more requested files could not be created.", Resource: inspect.WorkCopyLUID, Phase: errs.PhasePersistence, Cause: errs.WithWorkspaceRelativePaths(cause, workspace), Retryable: new(false)}
	}
	return out, nil
}
