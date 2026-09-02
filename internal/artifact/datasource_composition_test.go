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
	entry, err := writer.Create("Data/Sales.tds")
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
