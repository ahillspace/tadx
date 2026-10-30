package artifact

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// workbookDatasourceFixture mirrors the structure Tableau writes for a workbook
// connected to published datasources: a workbook-level repository location, a
// parameters datasource, live sqlproxy datasources with their own repository
// locations, a cached datasource copy inside a CDATA metadata attribute, an
// embedded federated connection, and worksheet-level datasource references.
const workbookDatasourceFixture = `<?xml version='1.0' encoding='utf-8' ?>

<!-- build 20251.25.0101.0000 -->
<workbook original-version='18.1' source-build='2025.1.0' version='18.1' xmlns:user='http://www.tableausoftware.com/xml/user'>
  <repository-location derived-from='https://tableau.example.com/t/source-site/workbooks/Sales?rev=1.0' id='Sales' path='/t/source-site/workbooks' revision='1.0' site='source-site' />
  <datasources>
    <datasource hasconnection='false' inline='true' name='Parameters' version='18.1'>
      <aliases enabled='yes' />
    </datasource>
    <datasource caption='Sales &amp; Margin' inline='true' name='sqlproxy.0abc' version='18.1'>
      <repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' />
      <connection channel='https' class='sqlproxy' dbname='SalesMargin' directory='dataserver' port='443' server='tableau.example.com' server-ds-friendly-name='Sales &amp; Margin'>
        <relation name='sqlproxy' table='[sqlproxy]' type='table' />
      </connection>
      <metadata-records>
        <metadata-record class='capability'>
          <attributes>
            <attribute datatype='string' name='datasource'>&quot;<![CDATA[<?xml version='1.0' encoding='utf-8' ?>

<datasource :source-version='18.1' formatted-name='Extract' inline='true' version='18.1' xml:base='http://localhost:8080' xmlns:user='http://www.tableausoftware.com/xml/user'>
  <repository-location derived-from='http://localhost:8080/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' />
  <connection :user-groups='local\\All Users\#' channel='http' class='sqlproxy' dbname='SalesMargin' directory='dataserver' port='8080' server='localhost'>
  </connection>
</datasource>
]]>&quot;</attribute>
            <attribute datatype='string' name='dialect-definition'>&quot;<![CDATA[<dialect-definition><primary-dialect class='sqlproxy' dbname='SalesMargin' /></dialect-definition>]]>&quot;</attribute>
          </attributes>
        </metadata-record>
      </metadata-records>
    </datasource>
    <datasource caption='Orders' inline='true' name='sqlproxy.1def' version='18.1'>
      <repository-location derived-from='https://tableau.example.com/t/source-site/datasources/Orders?rev=1.0' id='Orders' path='/t/source-site/datasources' revision='1.0' site='source-site' />
      <connection channel='https' class='sqlproxy' dbname='Orders' directory='dataserver' port='443' server='tableau.example.com'>
      </connection>
    </datasource>
    <datasource caption='Local' inline='true' name='federated.2ghi' version='18.1'>
      <connection class='federated'>
        <named-connections>
          <named-connection caption='db' name='postgres.1'>
            <connection class='postgres' dbname='SalesMargin' server='db.example.com' />
          </named-connection>
        </named-connections>
      </connection>
    </datasource>
  </datasources>
  <worksheets>
    <worksheet name='Sheet 1'>
      <table>
        <view>
          <datasources>
            <datasource caption='Sales &amp; Margin' name='sqlproxy.0abc' />
          </datasources>
        </view>
      </table>
    </worksheet>
  </worksheets>
</workbook>
`

const (
	fixtureLiveLocation     = `<repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' />`
	fixtureLiveConnection   = `<connection channel='https' class='sqlproxy' dbname='SalesMargin' directory='dataserver' port='443' server='tableau.example.com' server-ds-friendly-name='Sales &amp; Margin'>`
	fixtureCachedConnection = `<connection :user-groups='local\\All Users\#' channel='http' class='sqlproxy' dbname='SalesMargin' directory='dataserver' port='8080' server='localhost'>`
)

func TestReadWorkbookDatasourcesReturnsLivePublishedConnections(t *testing.T) {
	want := []WorkbookDatasource{{Caption: "Sales & Margin", ContentURL: "SalesMargin"}, {Caption: "Orders", ContentURL: "Orders"}}
	got, err := ReadWorkbookDatasources(context.Background(), "Sales.twb", PayloadFile(t, []byte(workbookDatasourceFixture)))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("twb datasources=%#v err=%v", got, err)
	}
	packaged := workbookPackageFixture(t, workbookDatasourceFixture)
	got, err = ReadWorkbookDatasources(context.Background(), "Sales.twbx", PayloadFile(t, packaged))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("twbx datasources=%#v err=%v", got, err)
	}
	got, err = ReadWorkbookDatasources(context.Background(), "Embedded.twb", PayloadFile(t, []byte("<workbook><datasources><datasource name='Parameters'/></datasources></workbook>")))
	if err != nil || len(got) != 0 {
		t.Fatalf("embedded datasources=%#v err=%v", got, err)
	}
	for name, content := range map[string][]byte{"Broken.twb": []byte("<workbook><datasources>"), "Broken.twbx": []byte("not a zip"), "Book.tds": []byte("<workbook/>")} {
		if _, err := ReadWorkbookDatasources(context.Background(), name, PayloadFile(t, content)); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestReadWorkbookDatasourcesStopsWhenCancelled(t *testing.T) {
	large := "<workbook><datasources>" + strings.Repeat("<datasource name='Parameters'/>", 5000) + "</datasources></workbook>"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, content := range map[string][]byte{"Large.twb": []byte(large), "Large.twbx": workbookPackageFixture(t, large)} {
		if _, err := ReadWorkbookDatasources(ctx, name, PayloadFile(t, content)); err != context.Canceled {
			t.Fatalf("%s cancel=%v", name, err)
		}
	}
}

func TestReadNativeWorkbookReportsDatasources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Sales.twb")
	if err := os.WriteFile(path, []byte(workbookDatasourceFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	native, err := ReadNative(context.Background(), path, "workbook")
	if err != nil || len(native.Datasources) != 2 || native.Datasources[0].ContentURL != "SalesMargin" {
		t.Fatalf("native=%#v err=%v", native, err)
	}
}

func TestRewriteWorkbookDatasourcesRebindsOnlyTheUploadCopy(t *testing.T) {
	for _, test := range []struct {
		name, origin, site string
		source, want       []string
	}{
		{
			name: "named site on another host", origin: "https://target.example.com", site: "target-site",
			want: []string{
				`<repository-location derived-from='https://target.example.com/t/target-site/datasources/SalesMargin_1?rev=1.0' id='SalesMargin_1' path='/t/target-site/datasources' revision='1.0' site='target-site' />`,
				`<connection channel='https' class='sqlproxy' dbname='SalesMargin_1' directory='dataserver' port='443' server='target.example.com' server-ds-friendly-name='Sales &amp; Margin'>`,
			},
		},
		{
			name: "default site on a nonstandard port", origin: "https://target.example.com:8443", site: "",
			want: []string{
				`<repository-location derived-from='https://target.example.com:8443/datasources/SalesMargin_1?rev=1.0' id='SalesMargin_1' path='/datasources' revision='1.0' />`,
				`<connection channel='https' class='sqlproxy' dbname='SalesMargin_1' directory='dataserver' port='8443' server='target.example.com' server-ds-friendly-name='Sales &amp; Margin'>`,
			},
		},
		{
			name: "same host keeps connection endpoint", origin: "https://TABLEAU.example.com", site: "target-site",
			want: []string{
				`<repository-location derived-from='https://tableau.example.com/t/target-site/datasources/SalesMargin_1?rev=1.0' id='SalesMargin_1' path='/t/target-site/datasources' revision='1.0' site='target-site' />`,
				`<connection channel='https' class='sqlproxy' dbname='SalesMargin_1' directory='dataserver' port='443' server='tableau.example.com' server-ds-friendly-name='Sales &amp; Margin'>`,
			},
		},
		{
			name: "default-site source gains a site", origin: "https://target.example.com", site: "target-site",
			source: []string{`<repository-location derived-from='https://tableau.example.com/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/datasources' revision='1.0' />`},
			want: []string{
				`<repository-location derived-from='https://target.example.com/t/target-site/datasources/SalesMargin_1?rev=1.0' id='SalesMargin_1' path='/t/target-site/datasources' revision='1.0' site='target-site' />`,
				`<connection channel='https' class='sqlproxy' dbname='SalesMargin_1' directory='dataserver' port='443' server='target.example.com' server-ds-friendly-name='Sales &amp; Margin'>`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := workbookDatasourceFixture
			if test.source != nil {
				source = strings.Replace(source, fixtureLiveLocation, test.source[0], 1)
			}
			directory := t.TempDir()
			path := filepath.Join(directory, "Sales.twb")
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			rewritten, err := RewriteWorkbookDatasources(context.Background(), workbookRewriteRequest(t, path, test.origin, test.site))
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(rewritten.Path)
			expected := strings.Replace(workbookDatasourceFixture, fixtureLiveLocation, test.want[0], 1)
			expected = strings.Replace(expected, fixtureLiveConnection, test.want[1], 1)
			expected = strings.Replace(expected, fixtureCachedConnection, strings.Replace(fixtureCachedConnection, "dbname='SalesMargin'", "dbname='SalesMargin_1'", 1), 1)
			assertRewrittenFile(t, rewritten, directory, expected)
			if after, _ := os.ReadFile(path); string(after) != source {
				t.Fatal("rewrite changed the source workbook")
			}
		})
	}
}

func TestRewriteWorkbookDatasourcesPreservesPackageEntries(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Sales.twbx")
	source := workbookPackageFixture(t, workbookDatasourceFixture)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	rewritten, err := RewriteWorkbookDatasources(context.Background(), workbookRewriteRequest(t, path, "https://tableau.example.com", "source-site"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(rewritten.Path)
	if after, _ := os.ReadFile(path); !bytes.Equal(after, source) {
		t.Fatal("rewrite changed the source package")
	}
	if rewritten.Path == path || filepath.Dir(rewritten.Path) != directory || !strings.HasPrefix(filepath.Base(rewritten.Path), ".tadx-publish-rewrite-") || filepath.Ext(rewritten.Path) != ".twbx" {
		t.Fatalf("rewritten path=%q", rewritten.Path)
	}
	before, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(rewritten.Path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if after.Comment != before.Comment || len(after.File) != len(before.File) {
		t.Fatalf("package comment=%q entries=%d", after.Comment, len(after.File))
	}
	for index, entry := range after.File {
		original := before.File[index]
		if entry.Name != original.Name || entry.Method != original.Method || !entry.Modified.Equal(original.Modified) || entry.Comment != original.Comment {
			t.Fatalf("entry %d header=%#v original=%#v", index, entry.FileHeader, original.FileHeader)
		}
		if entry.Name == "Sales.twb" {
			definition := readZipEntry(t, entry)
			if !strings.Contains(definition, "dbname='SalesMargin_1'") || strings.Contains(definition, " dbname='SalesMargin' directory='dataserver' port='443'") {
				t.Fatalf("definition was not rebound:\n%s", definition)
			}
			continue
		}
		if entry.CRC32 != original.CRC32 || !bytes.Equal(rawZipEntry(t, entry), rawZipEntry(t, original)) {
			t.Fatalf("entry %q was not copied byte-for-byte", entry.Name)
		}
	}
}

func TestRewriteWorkbookDatasourcesRejectsChangedOrMismatchedSources(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Sales.twb")
	if err := os.WriteFile(path, []byte(workbookDatasourceFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	request := workbookRewriteRequest(t, path, "https://target.example.com", "target-site")
	for name, change := range map[string]func(*WorkbookDatasourceRewrite){
		"fingerprint":     func(r *WorkbookDatasourceRewrite) { r.Fingerprint = "sha256:" + strings.Repeat("0", 64) },
		"size":            func(r *WorkbookDatasourceRewrite) { r.Size++ },
		"absent source":   func(r *WorkbookDatasourceRewrite) { r.ContentURLs = map[string]string{"Missing": "Missing_1"} },
		"empty target":    func(r *WorkbookDatasourceRewrite) { r.ContentURLs = map[string]string{"SalesMargin": ""} },
		"cdata delimiter": func(r *WorkbookDatasourceRewrite) { r.ContentURLs = map[string]string{"SalesMargin": "a]]>b"} },
		"no bindings":     func(r *WorkbookDatasourceRewrite) { r.ContentURLs = nil },
		"origin":          func(r *WorkbookDatasourceRewrite) { r.ServerOrigin = "target.example.com" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			change(&changed)
			if rewritten, err := RewriteWorkbookDatasources(context.Background(), changed); err == nil {
				t.Fatalf("rewrite accepted %s: %#v", name, rewritten)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("rewrite left files behind: %v %v", entries, err)
			}
		})
	}
}

func workbookRewriteRequest(t *testing.T, path, origin, site string) WorkbookDatasourceRewrite {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return WorkbookDatasourceRewrite{Path: path, Fingerprint: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(content)), ServerOrigin: origin, Site: site, ContentURLs: map[string]string{"SalesMargin": "SalesMargin_1"}}
}

func assertRewrittenFile(t *testing.T, rewritten RewrittenWorkbook, directory, expected string) {
	t.Helper()
	if filepath.Dir(rewritten.Path) != directory || !strings.HasPrefix(filepath.Base(rewritten.Path), ".tadx-publish-rewrite-") || filepath.Ext(rewritten.Path) != ".twb" {
		t.Fatalf("rewritten path=%q", rewritten.Path)
	}
	content, err := os.ReadFile(rewritten.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("rewritten workbook:\n%s\nwant:\n%s", content, expected)
	}
	sum := sha256.Sum256(content)
	if rewritten.Size != int64(len(content)) || rewritten.Fingerprint != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("rewritten=%#v size=%d", rewritten, len(content))
	}
}

func workbookPackageFixture(t *testing.T, definition string) []byte {
	t.Helper()
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	modified := time.Date(2025, 1, 2, 3, 4, 6, 0, time.UTC)
	for _, entry := range []struct {
		name    string
		method  uint16
		content string
	}{
		{"Sales.twb", zip.Deflate, definition},
		{"Data/Extracts/sales.hyper", zip.Store, "hyper bytes"},
		{"Image/logo.png", zip.Deflate, strings.Repeat("png", 64)},
		{"Data/nested.twb", zip.Deflate, "<workbook/>"},
	} {
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: entry.name, Method: entry.method, Modified: modified, Comment: "entry " + entry.name})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.SetComment("packaged workbook"); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func readZipEntry(t *testing.T, entry *zip.File) string {
	t.Helper()
	stream, err := entry.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	content, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func rawZipEntry(t *testing.T, entry *zip.File) []byte {
	t.Helper()
	stream, err := entry.OpenRaw()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
