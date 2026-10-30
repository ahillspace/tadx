package artifact

import (
	"archive/zip"
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestClassifyDatasourcePackagePreservesDirectParentReferences(t *testing.T) {
	content := []byte(`<datasource><relation datasource-url="z-parent"/><relation datasource-url="a-parent"/><relation datasource-url="z-parent"/></datasource>`)
	definition := classifyTestDefinition("Sales.tds", content)
	status, parents := definition.CompositionStatus, definition.ParentDataSourceURLs
	if status != CompositionStatusComposed || !reflect.DeepEqual(parents, []string{"a-parent", "z-parent"}) {
		t.Fatalf("status = %q, parents = %#v", status, parents)
	}
	if string(content) != `<datasource><relation datasource-url="z-parent"/><relation datasource-url="a-parent"/><relation datasource-url="z-parent"/></datasource>` {
		t.Fatal("classification changed native bytes")
	}
}

func TestClassifyPackagedDatasourceReadsRootDefinitionWithoutRewriting(t *testing.T) {
	var content bytes.Buffer
	writer := zip.NewWriter(&content)
	entry, err := writer.Create("Sales.tds")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`<datasource><relation datasource-url="parent-sales"/></datasource>`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), content.Bytes()...)
	definition := classifyTestDefinition("Sales.tdsx", content.Bytes())
	status, parents := definition.CompositionStatus, definition.ParentDataSourceURLs
	if status != CompositionStatusComposed || !reflect.DeepEqual(parents, []string{"parent-sales"}) {
		t.Fatalf("status = %q, parents = %#v", status, parents)
	}
	if !bytes.Equal(content.Bytes(), original) {
		t.Fatal("classification changed packaged datasource bytes")
	}
}

func TestClassifyOrdinaryAndUnreadableDatasource(t *testing.T) {
	if definition := classifyTestDefinition("ordinary.tds", []byte(`<datasource/>`)); definition.CompositionStatus != CompositionStatusOrdinary || len(definition.ParentDataSourceURLs) != 0 || definition.ContentURL != "" {
		t.Fatalf("definition = %#v", definition)
	}
	if definition := classifyTestDefinition("bad.tdsx", []byte("not a zip")); definition.CompositionStatus != CompositionStatusUnknown {
		t.Fatalf("definition = %#v", definition)
	}
}

func TestClassifyPackagedDatasourceUsesOnlyTopLevelDefinition(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries [][2]string
		status  string
	}{
		{"nested definition only", [][2]string{{"Data/Sales.tds", `<datasource><relation datasource-url="parent"/></datasource>`}}, CompositionStatusUnknown},
		{"top definition with nested definition", [][2]string{{"real.tds", `<datasource/>`}, {"Data/sub/other.tds", `<datasource><relation datasource-url="parent"/></datasource>`}}, CompositionStatusOrdinary},
		{"two top definitions", [][2]string{{"one.tds", `<datasource/>`}, {"two.tds", `<datasource/>`}}, CompositionStatusUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			var content bytes.Buffer
			writer := zip.NewWriter(&content)
			for _, item := range test.entries {
				entry, err := writer.Create(item[0])
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write([]byte(item[1])); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if definition := classifyTestDefinition("Sales.tdsx", content.Bytes()); definition.CompositionStatus != test.status || len(definition.ParentDataSourceURLs) != 0 {
				t.Fatalf("definition = %#v", definition)
			}
		})
	}
}

func TestReadDatasourceDefinitionReportsItsOwnRepositoryLocation(t *testing.T) {
	definition := `<datasource formatted-name='Sales &amp; Margin' inline='true'>
  <repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' />
  <connection class='federated'>
    <relation datasource-url='parent-a' />
  </connection>
  <object-graph><datasource><repository-location id='NestedParent' /></datasource></object-graph>
</datasource>`
	got := classifyTestDefinition("Sales.tds", []byte(definition))
	if got.ContentURL != "SalesMargin" || got.CompositionStatus != CompositionStatusComposed {
		t.Fatalf("definition = %#v", got)
	}
	var content bytes.Buffer
	writer := zip.NewWriter(&content)
	entry, err := writer.Create("Sales.tds")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(definition)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := classifyTestDefinition("Sales.tdsx", content.Bytes()); got.ContentURL != "SalesMargin" {
		t.Fatalf("packaged definition = %#v", got)
	}
	if got := classifyTestDefinition("Local.tds", []byte(`<datasource><object-graph><datasource><repository-location id='NestedParent' /></datasource></object-graph></datasource>`)); got.ContentURL != "" {
		t.Fatalf("unpublished definition = %#v", got)
	}
}

func classifyTestDefinition(filename string, content []byte) datasourceDefinition {
	return readDatasourceDefinition(filename, bytes.NewReader(content), int64(len(content)))
}

func TestClassifyPackagedDatasourceReadsCompleteLargeDefinition(t *testing.T) {
	for _, test := range []struct {
		name  string
		valid bool
	}{{"complete XML", true}, {"invalid XML after metadata", false}} {
		t.Run(test.name, func(t *testing.T) {
			var content bytes.Buffer
			archive := zip.NewWriter(&content)
			definition, err := archive.Create("large.tds")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(definition, "<datasource>"); err != nil {
				t.Fatal(err)
			}
			block := strings.Repeat("<column/>", 4096)
			for range 17*1024*1024/len(block) + 1 {
				if _, err := io.WriteString(definition, block); err != nil {
					t.Fatal(err)
				}
			}
			footer := `<repository-location id="LateSource"/><relation datasource-url="late-parent"/></datasource>`
			if !test.valid {
				footer += "<broken>"
			}
			if _, err := io.WriteString(definition, footer); err != nil {
				t.Fatal(err)
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			got := classifyTestDefinition("large.tdsx", content.Bytes())
			if !test.valid {
				if got.CompositionStatus != CompositionStatusUnknown || got.ContentURL != "" || len(got.ParentDataSourceURLs) != 0 {
					t.Fatalf("invalid definition retained partial metadata: %#v", got)
				}
				return
			}
			if got.CompositionStatus != CompositionStatusComposed || got.ContentURL != "LateSource" || !reflect.DeepEqual(got.ParentDataSourceURLs, []string{"late-parent"}) {
				t.Fatalf("large definition metadata = %#v", got)
			}
		})
	}
}
