package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type inspectionReader func(kind, id, typ, name string) (value.TDSRead, error)

func (read inspectionReader) ReadTDSElements(_ context.Context, kind, id, typ, name string) (value.TDSRead, error) {
	return read(kind, id, typ, name)
}

func TestTDSInspectionBoundedCollectionsAndExactReads(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000003"
	var queries []string
	read := inspectionReader(func(kind, gotID, typ, name string) (value.TDSRead, error) {
		if kind != "datasource" || gotID != id {
			t.Fatalf("target %s %s", kind, gotID)
		}
		queries = append(queries, typ+":"+name)
		if name != "" {
			return value.TDSRead{Absent: true}, nil
		}
		switch typ {
		case "Column":
			var rows []value.TDSElement
			for i := range maxTDSElements + 2 {
				rows = append(rows, value.TDSElement{Name: fmt.Sprint(i), Column: &value.TDSColumn{}})
			}
			return value.TDSRead{Elements: rows}, nil
		case "FieldDescription":
			return value.TDSRead{Elements: []value.TDSElement{{Name: "Broken", Error: &value.TDSStatus{Code: 7}}, {Name: "Visible", Description: new("units")}}}, errors.New("partial response")
		default:
			return value.TDSRead{}, nil
		}
	})
	target := editTarget
	target.DatasourceLUID = id
	target.WorkCopyLUID = ""
	out, err := InspectTDSElements(t.Context(), read, "datasource", target, "demo", nil, nil)
	if err == nil || out.Complete || len(out.Observations) != 3 || out.Observations[0].Omitted != 2 || len(out.Observations[0].Elements) != maxTDSElements || len(out.Observations[1].Elements) != 2 {
		t.Fatal(out, err)
	}
	compact, _ := json.Marshal(out.CompactOutput())
	full, _ := json.Marshal(out.FullOutput())
	if len(compact) >= len(full) || !strings.Contains(string(compact), `"details":"--full"`) {
		t.Fatal("unbounded compact projection")
	}
	queries = nil
	named, err := InspectTDSElements(t.Context(), read, "datasource", target, "demo", []string{"out of page"}, []string{"T"})
	if err != nil || !named.Complete || strings.Join(queries, ",") != "Column:out of page,FieldDescription:out of page,TableDescription:T" || !named.Observations[0].Absent {
		t.Fatal(named, queries, err)
	}
}

func TestTDSAuditRedactsCredentialsAndContainsMarkup(t *testing.T) {
	const credential = "private-fixture-marker"
	root := t.TempDir()
	inspect := CopyInspection{EditTarget: editTarget, Observations: []EditObservation{{ElementType: "Column", Elements: []value.TDSElement{{Name: credential, Column: &value.TDSColumn{Caption: new("<img>\n# injected")}}}}}, MetadataWarning: "upstream supplied " + credential}
	inspect.Observations = append(inspect.Observations, EditObservation{ElementType: "FieldDescription", Complete: true})
	out, err := ExportEditCopy(root, "audit.md", "edits.yaml", inspect, nil, func(v string) string { return strings.ReplaceAll(v, credential, "[REDACTED]") })
	if err == nil || out.AuditFile != "audit.md" || out.ChangesFile != "" {
		t.Fatal(out, err)
	}
	data, err := os.ReadFile(filepath.Join(root, out.AuditFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), credential) || strings.Contains(string(data), "\n# injected") || strings.Contains(string(data), "<img>") {
		t.Fatalf("unsafe audit %q", data)
	}
}

func TestTDSAuditBoundsIndexedSourceFieldsAndLineage(t *testing.T) {
	fields := make([]value.FieldDescription, maxTDSElements+1)
	fields[0].UpstreamColumns = make([]value.MetadataColumn, maxTDSElements+2)
	fields[0].ReferencedFields = make([]value.MetadataIdentity, maxTDSElements+3)
	fields[0].Sheets = make([]value.MetadataIdentity, maxTDSElements+4)
	fields[0].ReferencedFields[0].Name = "visible-reference"
	fields[0].ReferencedFields[maxTDSElements].Name = "omitted-reference"
	fields[0].Sheets[0].Name = "visible-sheet"
	fields[0].Sheets[maxTDSElements].Name = "omitted-sheet"
	fields[0].UpstreamColumns[0].Name = "visible-column"
	fields[0].UpstreamColumns[maxTDSElements].Name = "omitted-column"
	fields[maxTDSElements].Name = "omitted-field"
	metadata := value.MetadataDatasourceDescriptions{Complete: true, Fields: fields}
	inspect := CopyInspection{EditTarget: editTarget, SourceMetadata: &metadata}
	audit, err := editAudit(inspect, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"visible-reference", "visible-sheet", "visible-column", `"referenced_fields_omitted": 3`, `"sheets_omitted": 4`, `"fields_omitted": 1`, `"upstream_columns_omitted": 2`, `"provider_complete": true`} {
		if !strings.Contains(string(audit), want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, omit := range []string{"omitted-reference", "omitted-sheet", "omitted-column", "omitted-field"} {
		if strings.Contains(string(audit), omit) {
			t.Errorf("included %q", omit)
		}
	}
}
