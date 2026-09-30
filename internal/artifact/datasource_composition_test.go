package artifact

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"
)

func TestClassifyDatasourcePackagePreservesDirectParentReferences(t *testing.T) {
	content := []byte(`<datasource><relation datasource-url="z-parent"/><relation datasource-url="a-parent"/><relation datasource-url="z-parent"/></datasource>`)
	status, parents := classifyDatasourcePackage("Sales.tds", content)
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
	status, parents := classifyDatasourcePackage("Sales.tdsx", content.Bytes())
	if status != CompositionStatusComposed || !reflect.DeepEqual(parents, []string{"parent-sales"}) {
		t.Fatalf("status = %q, parents = %#v", status, parents)
	}
	if !bytes.Equal(content.Bytes(), original) {
		t.Fatal("classification changed packaged datasource bytes")
	}
}

func TestClassifyOrdinaryAndUnreadableDatasource(t *testing.T) {
	if status, parents := classifyDatasourcePackage("ordinary.tds", []byte(`<datasource/>`)); status != CompositionStatusOrdinary || len(parents) != 0 {
		t.Fatalf("status = %q, parents = %#v", status, parents)
	}
	if status, _ := classifyDatasourcePackage("bad.tdsx", []byte("not a zip")); status != CompositionStatusUnknown {
		t.Fatalf("status = %q", status)
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
			if status, parents := classifyDatasourcePackage("Sales.tdsx", content.Bytes()); status != test.status || len(parents) != 0 {
				t.Fatalf("status = %q, parents = %#v", status, parents)
			}
		})
	}
}
