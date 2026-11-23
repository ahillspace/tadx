package twbschema_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/twbschema"
)

// validWorkbook conforms to the 2026.1 schema with no suppression; the
// plugin validator reports it valid with --no-ignore-known-gaps.
const validWorkbook = `<?xml version='1.0' encoding='utf-8' ?>
<workbook source-build='2026.1.0 (20261.26.0101.0000)' version='18.1' xmlns:user='http://www.tableausoftware.com/xml/user'>
  <document-format-change-manifest>
    <SheetIdentifierTracking />
  </document-format-change-manifest>
  <style />
  <worksheets>
    <worksheet name='Sheet 1'>
      <table>
        <view>
          <datasources />
          <aggregation value='true' />
        </view>
        <style />
        <panes>
          <pane>
            <view>
              <breakdown value='auto' />
            </view>
            <mark class='Automatic' />
          </pane>
        </panes>
        <rows />
        <cols />
      </table>
      <simple-id uuid='{11111111-2222-3333-4444-555555555555}' />
    </worksheet>
  </worksheets>
  <windows>
    <window class='worksheet' name='Sheet 1'>
      <cards />
      <simple-id uuid='{11111111-2222-3333-4444-666666666666}' />
    </window>
  </windows>
  <explain-data enabled-for-viewer='true' extreme-values-enabled-for-all='true'>
    <explanation-types>
      <explanation-type type='number-of-records' enabled='true' />
    </explanation-types>
  </explain-data>
</workbook>
`

func validate(t *testing.T, version, doc string) []twbschema.Issue {
	t.Helper()
	schema, err := twbschema.Load(version)
	if err != nil {
		t.Fatal(err)
	}
	result, err := schema.Validate(context.Background(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return result.Issues
}

func edit(t *testing.T, doc, old, replacement string) string {
	t.Helper()
	if !strings.Contains(doc, old) {
		t.Fatalf("fixture has no %q", old)
	}
	return strings.Replace(doc, old, replacement, 1)
}

func TestAValidWorkbookHasNoIssues(t *testing.T) {
	if issues := validate(t, "2026.1", validWorkbook); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

// Desktop refuses a workbook whose table has cols before rows (error
// D2E8DA72), so the issue names the element, where it is, and the order.
func TestAnElementOutOfOrderIsReported(t *testing.T) {
	doc := edit(t, validWorkbook, "<rows />\n        <cols />", "<cols />\n        <rows />")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	got := issues[0]
	if got.Line != 23 || got.Path != `worksheet "Sheet 1" > table > cols` {
		t.Fatalf("issue at line %d path %q", got.Line, got.Path)
	}
	want := "<cols> is not allowed here; expected <mark-layout> or <rows>. <table> children go in this order: (view, style, panes, mark-layout?, rows, cols,"
	if !strings.HasPrefix(got.Message, want) {
		t.Fatalf("message = %q\nwant prefix %q", got.Message, want)
	}
}

func TestEverySchemaFromTableau2018Point1Loads(t *testing.T) {
	versions := twbschema.Versions()
	if len(versions) != 31 || versions[0] != "2018.1" || versions[30] != "2026.3" {
		t.Fatalf("versions = %v", versions)
	}
	for _, version := range versions {
		if _, err := twbschema.Load(version); err != nil {
			t.Errorf("%s: %v", version, err)
		}
	}
}

// Desktop refuses a workbook whose top-level children are out of order the
// same way, so the issue path starts at the root.
func TestWorkbookChildrenOutOfOrderAreReportedFromTheRoot(t *testing.T) {
	start := strings.Index(validWorkbook, "  <worksheets>")
	end := strings.Index(validWorkbook, "  <windows>")
	worksheets := validWorkbook[start:end]
	doc := edit(t, strings.Replace(validWorkbook, worksheets, "", 1), "  </windows>\n", "  </windows>\n"+worksheets)
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	got := issues[0]
	if got.Line != 13 || got.Path != "workbook > worksheets" {
		t.Fatalf("issue at line %d path %q", got.Line, got.Path)
	}
	want := "<worksheets> is not allowed here; expected <datagraph>, <thumbnails>, <external>, <referenced-extensions>, or <explain-data>. " +
		"<workbook> children go in this order: (document-format-change-manifest?, repository-location?, preferences?, style-theme?, style?,"
	if !strings.HasPrefix(got.Message, want) {
		t.Fatalf("message = %q\nwant prefix %q", got.Message, want)
	}
}

// A missing child is reported at the parent's opening tag, as libxml2, which
// the plugin validator runs, reports it; matching it keeps the two comparable.
func TestAMissingRequiredChildNamesWhatIsMissing(t *testing.T) {
	doc := edit(t, validWorkbook, "          <aggregation value='true' />\n", "")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	got := issues[0]
	if got.Line != 10 || got.Path != `worksheet "Sheet 1" > table > view` {
		t.Fatalf("issue at line %d path %q", got.Line, got.Path)
	}
	want := "<view> is missing <aggregation>. <view> children go in this order: (datasources?, mapsources?, datasource-dependencies*,"
	if !strings.HasPrefix(got.Message, want) {
		t.Fatalf("message = %q\nwant prefix %q", got.Message, want)
	}
}

func TestAnAttributeValueOutsideItsEnumerationNamesTheAllowedValues(t *testing.T) {
	doc := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Bars' />")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	got := issues[0]
	if got.Line != 20 || got.Path != `worksheet "Sheet 1" > table > panes > pane > mark` {
		t.Fatalf("issue at line %d path %q", got.Line, got.Path)
	}
	want := "class='Bars' is not allowed; expected one of: Automatic, Text, Icon, Shape, Rectangle, Bar, GanttBar, Square, Circle, Heatmap, PolyLine, Line, Polygon, Area, Pie, Multipolygon, VizExtension"
	if got.Message != want {
		t.Fatalf("message = %q\nwant      %q", got.Message, want)
	}
}

func TestAnUnknownAttributeIsReported(t *testing.T) {
	doc := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Automatic' colour='red' />")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Message != "<mark> has no attribute colour; it takes class, orientation" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAMissingRequiredAttributeIsReported(t *testing.T) {
	doc := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark />")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Message != "<mark> is missing the required attribute class" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAnAttributeOfTheWrongTypeIsReported(t *testing.T) {
	doc := edit(t, validWorkbook, "<aggregation value='true' />", "<aggregation value='yes' />")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Message != "value='yes' is not allowed; expected a boolean (true, false, 1, 0)" {
		t.Fatalf("issues = %#v", issues)
	}
}

// The schemas end worksheets, dashboards, and windows with a required
// simple-id, but Tableau only writes one under SheetIdentifierTracking, so the
// plugin drops the schema's complaint and the manifest rule reports it.
func TestASheetWithoutSimpleIDIsReportedOnlyUnderSheetIdentifierTracking(t *testing.T) {
	withoutIDs := edit(t, validWorkbook, "      <simple-id uuid='{11111111-2222-3333-4444-555555555555}' />\n", "")
	withoutIDs = edit(t, withoutIDs, "      <simple-id uuid='{11111111-2222-3333-4444-666666666666}' />\n", "")
	untracked := edit(t, withoutIDs, "    <SheetIdentifierTracking />\n", "")
	if issues := validate(t, "2026.1", untracked); len(issues) != 0 {
		t.Fatalf("untracked issues = %#v", issues)
	}
	issues := validate(t, "2026.1", withoutIDs)
	if len(issues) != 2 {
		t.Fatalf("tracked issues = %#v", issues)
	}
	if issues[0].Line != 8 || issues[0].Path != `worksheet "Sheet 1"` ||
		issues[0].Message != "<worksheet> needs a <simple-id uuid='{...}' /> after <table> when <document-format-change-manifest> has <SheetIdentifierTracking />" {
		t.Fatalf("worksheet issue = %#v", issues[0])
	}
	if issues[1].Path != `window "Sheet 1"` || !strings.Contains(issues[1].Message, "as its last child") {
		t.Fatalf("window issue = %#v", issues[1])
	}
}

// A dashboard ends with a simple-id too, so under SheetIdentifierTracking a
// dashboard without one is reported, and without the flag it is not.
func TestADashboardWithoutSimpleIDIsReportedOnlyUnderSheetIdentifierTracking(t *testing.T) {
	withoutID := edit(t, validWorkbook, "  <windows>\n", "  <dashboards>\n    <dashboard name='Dashboard 1'>\n      <zones />\n    </dashboard>\n  </dashboards>\n  <windows>\n")
	issues := validate(t, "2026.1", withoutID)
	if len(issues) != 1 || issues[0].Line != 30 || issues[0].Path != `dashboard "Dashboard 1"` ||
		issues[0].Message != "<dashboard> needs a <simple-id uuid='{...}' /> as its last child when <document-format-change-manifest> has <SheetIdentifierTracking />" {
		t.Fatalf("tracked issues = %#v", issues)
	}
	withID := edit(t, withoutID, "      <zones />\n", "      <zones />\n      <simple-id uuid='{11111111-2222-3333-4444-777777777777}' />\n")
	if issues := validate(t, "2026.1", withID); len(issues) != 0 {
		t.Fatalf("issues with a simple-id = %#v", issues)
	}
	untracked := edit(t, withoutID, "    <SheetIdentifierTracking />\n", "")
	untracked = edit(t, untracked, "      <simple-id uuid='{11111111-2222-3333-4444-555555555555}' />\n", "")
	untracked = edit(t, untracked, "      <simple-id uuid='{11111111-2222-3333-4444-666666666666}' />\n", "")
	if issues := validate(t, "2026.1", untracked); len(issues) != 0 {
		t.Fatalf("untracked issues = %#v", issues)
	}
}

func TestASimpleIDWithoutSheetIdentifierTrackingIsReported(t *testing.T) {
	doc := edit(t, validWorkbook, "    <SheetIdentifierTracking />\n", "")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 2 || issues[0].Line != 25 ||
		issues[0].Message != "Tableau rejects <simple-id> unless <document-format-change-manifest> has <SheetIdentifierTracking />; remove it" {
		t.Fatalf("issues = %#v", issues)
	}
}

// Tableau still writes <sort class='computed'>, which the schemas only know
// as <computed-sort>; the newer form needs SortTagCleanup.
func TestLegacySortsAreCheckedAsTheirNewerForm(t *testing.T) {
	legacy := edit(t, validWorkbook, "          <aggregation value='true' />\n", "          <computed-sort column='[ds].[x]' direction='ASC' using='[ds].[y]' />\n          <aggregation value='true' />\n")
	issues := validate(t, "2026.1", legacy)
	if len(issues) != 1 || issues[0].Message != "Tableau rejects <computed-sort> unless <document-format-change-manifest> has <SortTagCleanup />; use <sort class='computed'> instead" {
		t.Fatalf("issues = %#v", issues)
	}
	old := edit(t, legacy, "<computed-sort column", "<sort class='computed' column")
	if issues := validate(t, "2026.1", old); len(issues) != 0 {
		t.Fatalf("legacy sort issues = %#v", issues)
	}
}

// Feature capability markers are Tableau internals the schemas never list.
func TestFeatureCapabilityMarkersAreNotReported(t *testing.T) {
	doc := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Automatic' _.fcp.Marks.true...size='1' />")
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

// The schemas require explain-data, which many workbooks omit.
func TestAWorkbookWithoutExplainDataHasNoIssue(t *testing.T) {
	start := strings.Index(validWorkbook, "  <explain-data")
	end := strings.Index(validWorkbook, "</workbook>")
	doc := validWorkbook[:start] + validWorkbook[end:]
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestADuplicateWindowNameIsReported(t *testing.T) {
	window := "    <window class='worksheet' name='Sheet 1'>\n      <cards />\n      <simple-id uuid='{11111111-2222-3333-4444-777777777777}' />\n    </window>\n"
	doc := edit(t, validWorkbook, "  </windows>\n", window+"  </windows>\n")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Line != 34 || issues[0].Message != "<window> name='Sheet 1' repeats; each <window> in <windows> needs a unique name" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestTextOutsideItsTypeIsReported(t *testing.T) {
	doc := edit(t, validWorkbook, "<rows />", "<rows>stray</rows>")
	issues := validate(t, "2026.1", doc)
	if len(issues) != 0 {
		// rows has simple string content, so any text is allowed.
		t.Fatalf("rows issues = %#v", issues)
	}
	doc = edit(t, validWorkbook, "<cards />", "<cards>stray</cards>")
	issues = validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Message != "<cards> takes no text; remove 'stray'" {
		t.Fatalf("cards issues = %#v", issues)
	}
}

func TestTheSchemaFollowsTheDocumentsTableauRelease(t *testing.T) {
	tests := []struct {
		name, doc, version, warning string
	}{
		{"source build", validWorkbook, "2026.1", ""},
		{"build comment", "<?xml version='1.0' encoding='utf-8' ?>\n<!-- build 20253.25.1007.1312 -->\n<datasource version='18.1'></datasource>", "2025.3", ""},
		{"source build first", "<!-- build 20253.25.1007.1312 -->\n<workbook source-build='2024.2.0 (20242.24.0101.0000)'></workbook>", "2024.2", ""},
		{"placeholder build", "<!-- build 20251.25.0101.0000 -->\n<workbook source-build='0.0.0 (00000.0.0.0)'></workbook>", "2025.1", ""},
		{"newer", "<workbook source-build='2027.1.0 (20271.27.0101.0000)'></workbook>", "2026.3",
			"Tableau 2027.1 is newer than the newest schema, so it is checked against 2026.3 and newer features may show as issues"},
		{"between", "<workbook source-build='2023.4.0 (20234.23.0101.0000)'></workbook>", "2023.3",
			"no schema matches Tableau 2023.4, so it is checked against 2023.3, the nearest older release"},
		{"older", "<workbook source-build='2010.1.0 (20101.10.0101.0000)'></workbook>", "2018.1",
			"Tableau 2010.1 is older than the oldest schema, so it is checked against 2018.1"},
		{"unknown", "<workbook version='18.1'></workbook>", "2026.3",
			"the document does not name its Tableau release, so it is checked against the newest schema, 2026.3"},
	}
	for _, test := range tests {
		schema, warning, err := twbschema.Select([]byte(test.doc))
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if schema.Version() != test.version || warning != test.warning {
			t.Errorf("%s: version %s warning %q", test.name, schema.Version(), warning)
		}
	}
}

// An edit is judged by what it introduced: issues the original already had,
// counted by message since lines and names shift, are dropped.
func TestIntroducedDropsIssuesTheOriginalHad(t *testing.T) {
	original := []twbschema.Issue{{Line: 5, Message: "a"}, {Line: 9, Message: "b"}}
	current := []twbschema.Issue{{Line: 6, Message: "a"}, {Line: 7, Message: "a"}, {Line: 12, Message: "c"}}
	got := twbschema.Introduced(current, original)
	if len(got) != 2 || got[0].Line != 7 || got[1].Message != "c" {
		t.Fatalf("introduced = %#v", got)
	}
}

// A repoint renames a datasource, which changes every issue path under it,
// so an inherited issue still matches by message alone.
func TestIntroducedIgnoresRenamedPaths(t *testing.T) {
	original := []twbschema.Issue{{Line: 5, Path: `datasource "Sales" > connection`, Message: "a"}}
	current := []twbschema.Issue{{Line: 9, Path: `datasource "sqlproxy.0abc" > connection`, Message: "a"}}
	if got := twbschema.Introduced(current, original); len(got) != 0 {
		t.Fatalf("introduced = %#v", got)
	}
}

// encoding/xml accepts a repeated attribute, but the document is not
// well-formed XML and Tableau refuses it.
func TestARepeatedAttributeIsAnIssue(t *testing.T) {
	doc := "<datasource inline='true' version='18.1'>\n  <aliases enabled='yes' enabled='no' />\n</datasource>\n"
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Line != 2 || issues[0].Message != "<aliases> repeats the attribute enabled; XML allows each attribute once" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestADatasourceFileIsCheckedAsAWorkbookDatasource(t *testing.T) {
	doc := "<?xml version='1.0' encoding='utf-8' ?>\n<datasource inline='true' version='18.1' xmlns:user='http://www.tableausoftware.com/xml/user'>\n  <connection class='federated' />\n  <aliases enabled='maybe' />\n</datasource>\n"
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Line != 4 || issues[0].Message != "enabled='maybe' is not allowed; expected one of: yes, no" {
		t.Fatalf("issues = %#v", issues)
	}
}

// The schemas leave a connection's content open, since each connector
// defines its own.
func TestConnectionContentIsNotChecked(t *testing.T) {
	doc := "<datasource inline='true' version='18.1'>\n  <connection class='federated'><anything-at-all x='1'><nested /></anything-at-all></connection>\n</datasource>\n"
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func lookup(t *testing.T, root, element, attribute string) []twbschema.Element {
	t.Helper()
	schema, err := twbschema.Load("2026.1")
	if err != nil {
		t.Fatal(err)
	}
	elements, err := schema.Lookup(root, element, attribute)
	if err != nil {
		t.Fatal(err)
	}
	return elements
}

func TestLookupDescribesAnElementsChildrenAndAttributes(t *testing.T) {
	elements := lookup(t, "workbook", "mark", "")
	if len(elements) == 0 {
		t.Fatal("no mark")
	}
	var mark *twbschema.Element
	for i := range elements {
		for _, p := range elements[i].Paths {
			if strings.HasSuffix(p, "table > panes > pane > mark") {
				mark = &elements[i]
			}
		}
	}
	if mark == nil {
		t.Fatalf("no table pane mark in %#v", elements)
	}
	if len(mark.Attributes) != 2 || mark.Attributes[0].Name != "class" || !mark.Attributes[0].Required ||
		!strings.HasPrefix(mark.Attributes[0].Type, "one of: Automatic, Text, Icon,") {
		t.Fatalf("attributes = %#v", mark.Attributes)
	}
}

func TestLookupByPathNarrowsToOneElement(t *testing.T) {
	elements := lookup(t, "workbook", "worksheet > table", "")
	if len(elements) != 1 {
		t.Fatalf("elements = %d", len(elements))
	}
	if !strings.HasPrefix(elements[0].Children, "(view, style, panes, mark-layout?, rows, cols,") {
		t.Fatalf("children = %q", elements[0].Children)
	}
}

func TestLookupOfAnAttributeKeepsOnlyThatAttribute(t *testing.T) {
	elements := lookup(t, "workbook", "worksheet > table > panes > pane > mark", "class")
	if len(elements) != 1 || len(elements[0].Attributes) != 1 || elements[0].Attributes[0].Name != "class" {
		t.Fatalf("elements = %#v", elements)
	}
}

func TestLookupNamesUniqueAndOpenContent(t *testing.T) {
	windows := lookup(t, "workbook", "workbook > windows", "")
	if len(windows) != 1 || !slices.Contains(windows[0].Unique, "each <window> needs a unique name") {
		t.Fatalf("windows = %#v", windows)
	}
	connection := lookup(t, "datasource", "datasource > connection", "")
	if len(connection) == 0 || !connection[0].OpenContent {
		t.Fatalf("connection = %#v", connection)
	}
}

// A datasource document is the datasource root and what it holds, so paths
// start there and workbook elements are not found.
func TestLookupInADatasourceStartsAtTheDatasourceRoot(t *testing.T) {
	aliases := lookup(t, "datasource", "aliases", "")
	if len(aliases) != 3 || aliases[0].Paths[0] != "datasource > aliases" || aliases[1].Paths[0] != "datasource > column > aliases" {
		t.Fatalf("aliases = %#v", aliases)
	}
	schema, _ := twbschema.Load("2026.1")
	if _, err := schema.Lookup("datasource", "worksheet", ""); err == nil || err.Error() != "a Tableau 2026.1 datasource has no element <worksheet>" {
		t.Fatalf("err = %v", err)
	}
}

func TestLookupOfAnUnknownElementFails(t *testing.T) {
	schema, _ := twbschema.Load("2026.1")
	if _, err := schema.Lookup("workbook", "no-such-element", ""); err == nil || err.Error() != "a Tableau 2026.1 workbook has no element <no-such-element>" {
		t.Fatalf("err = %v", err)
	}
	if _, err := schema.Lookup("workbook", "mark", "colour"); err == nil || !strings.Contains(err.Error(), "no attribute colour") {
		t.Fatalf("err = %v", err)
	}
}

// Nesting is bounded, so a hostile document cannot grow the validator's stack
// without limit.
func TestValidateRefusesNestingBeyondTheBound(t *testing.T) {
	schema, err := twbschema.Load("2026.1")
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.Repeat("<a>", 1025) + strings.Repeat("</a>", 1025)
	if _, err := schema.Validate(context.Background(), []byte(doc)); err == nil || err.Error() != "document nests elements beyond 1024 levels" {
		t.Fatalf("err = %v", err)
	}
	doc = strings.Repeat("<a>", 1024) + strings.Repeat("</a>", 1024)
	if _, err := schema.Validate(context.Background(), []byte(doc)); err != nil {
		t.Fatalf("1024 levels: %v", err)
	}
}

// A known gap is not a content error, so the children after it are still
// checked.
func TestAKnownGapDoesNotHideLaterIssues(t *testing.T) {
	capability := edit(t, validWorkbook, "      <simple-id uuid='{11111111-2222-3333-4444-555555555555}' />\n    </worksheet>",
		"      <_.fcp.Test.true...marker />\n      <bogus />\n      <simple-id uuid='{11111111-2222-3333-4444-555555555555}' />\n    </worksheet>")
	issues := validate(t, "2026.1", capability)
	if len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "<bogus> is not allowed here") {
		t.Fatalf("feature capability: issues = %#v", issues)
	}
	explain := validWorkbook[strings.Index(validWorkbook, "  <explain-data"):strings.Index(validWorkbook, "</workbook>")]
	accelerator := edit(t, validWorkbook, explain, "  <accelerator-details>\n    <exchange-product-id>1</exchange-product-id>\n  </accelerator-details>\n  <style />\n")
	issues = validate(t, "2026.1", accelerator)
	if len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "<style> is not allowed here") {
		t.Fatalf("accelerator-details: issues = %#v", issues)
	}
}

// Values compare as their type reads them, so a fixed boolean accepts 1 for
// true and integer zone ids that differ only in leading zeros repeat.
func TestValuesCompareByType(t *testing.T) {
	doc := "<datasource inline='1' version='18.1'>\n</datasource>\n"
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("fixed boolean: issues = %#v", issues)
	}
	doc = "<datasource inline='true' version='18.1'>\n  <layout dim-ordering='alphabetic' measure-ordering='alphabetic' show-structure='true' rowDisplayCount='0000000000000000000000000000000000000000' />\n</datasource>\n"
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || issues[0].Message != "rowDisplayCount='0000000000000000000000000000000000000000' is not allowed; expected a positive integer" {
		t.Fatalf("positive integer: issues = %#v", issues)
	}
	dashboard := edit(t, validWorkbook, "  <windows>", "  <dashboards>\n    <dashboard name='D'>\n      <zones>\n        <zone id='1' x='0' y='0' w='1' h='1' />\n        <zone id='01' x='0' y='0' w='1' h='1' />\n      </zones>\n      <simple-id uuid='{11111111-2222-3333-4444-777777777777}' />\n    </dashboard>\n  </dashboards>\n  <windows>")
	issues = validate(t, "2026.1", dashboard)
	if len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "<zone> id='01' repeats") {
		t.Fatalf("unique integer: issues = %#v", issues)
	}
}

// An integer bound is compared exactly, however long the value.
func TestIntegerBoundsCompareExactly(t *testing.T) {
	doc := validWorkbook[:strings.Index(validWorkbook, "</workbook>")] + "  <workbook-optimizer>\n    <rule-config>\n      <rule ruleID='%s' />\n    </rule-config>\n  </workbook-optimizer>\n</workbook>\n"
	for value, want := range map[string]int{"0001": 0, "-1" + strings.Repeat("0", 400): 1, "0": 1} {
		if issues := validate(t, "2026.1", strings.Replace(doc, "%s", value, 1)); len(issues) != want {
			t.Errorf("ruleID=%s: issues = %#v", value, issues)
		}
	}
}

// A document with more issues than the bound keeps the first ones found and
// counts the rest, and long values are clipped.
func TestIssuesAreBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("<datasource inline='true' version='18.1'>\n")
	for i := 0; i < 1500; i++ {
		b.WriteString("  <aliases enabled='" + strings.Repeat("x", 100) + "' />\n")
	}
	b.WriteString("</datasource>\n")
	schema, err := twbschema.Load("2026.1")
	if err != nil {
		t.Fatal(err)
	}
	result, err := schema.Validate(context.Background(), []byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	// Each value is an issue, and the second <aliases> is one more.
	if len(result.Issues) != 1000 || result.Omitted != 501 {
		t.Fatalf("issues = %d, omitted = %d", len(result.Issues), result.Omitted)
	}
	if want := "enabled='" + strings.Repeat("x", 40) + "...' is not allowed; expected one of: yes, no"; result.Issues[0].Message != want {
		t.Fatalf("message = %q", result.Issues[0].Message)
	}
}

// xmlns=” declares the default namespace and is not an attribute, and an
// element in a namespace is not the schema's element of the same local name.
func TestNamespacesAreNotSchemaNames(t *testing.T) {
	doc := "<datasource inline='true' version='18.1' xmlns=''>\n</datasource>\n"
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("xmlns: issues = %#v", issues)
	}
	doc = "<datasource inline='true' version='18.1' xmlns:x='urn:x'>\n  <x:aliases enabled='yes' />\n</datasource>\n"
	issues := validate(t, "2026.1", doc)
	if len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "<urn:x:aliases> is not allowed here") {
		t.Fatalf("namespaced element: issues = %#v", issues)
	}
}

// An attribute in a namespace is checked like any other: an element that
// declares no wildcard rejects it, ##local wildcards accept only unqualified
// attributes, and the user attribute group is Tableau's open extension point.
func TestNamespacedAttributesAreChecked(t *testing.T) {
	closed := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Automatic' xmlns:x='urn:x' x:colour='red' />")
	issues := validate(t, "2026.1", closed)
	if len(issues) != 1 || issues[0].Line != 20 || !strings.HasPrefix(issues[0].Message, "<mark> has no attribute urn:x:colour; it takes class") {
		t.Fatalf("closed element: issues = %#v", issues)
	}
	user := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Automatic' user:colour='red' />")
	if issues := validate(t, "2026.1", user); len(issues) != 1 || !strings.Contains(issues[0].Message, "has no attribute http://www.tableausoftware.com/xml/user:colour") {
		t.Fatalf("user attribute on a closed element: issues = %#v", issues)
	}
	long := "urn:" + strings.Repeat("n", 100000)
	clipped := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Automatic' xmlns:x='"+long+"' x:colour='red' />")
	if issues := validate(t, "2026.1", clipped); len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "<mark> has no attribute urn:"+strings.Repeat("n", 36)+"...:colour;") {
		t.Fatalf("long namespace: issues = %#v", issues)
	}
	// Two namespaces that differ past the clip are still two attributes.
	twin := edit(t, validWorkbook, "<mark class='Automatic' />", "<mark class='Automatic' xmlns:a='"+long+"a' xmlns:b='"+long+"b' a:colour='red' b:colour='red' />")
	if issues := validate(t, "2026.1", twin); len(issues) != 2 {
		t.Fatalf("namespaces alike up to the clip: issues = %#v", issues)
	}
	doc := "<datasource inline='true' version='18.1' xmlns:user='http://www.tableausoftware.com/xml/user'>\n  <column datatype='string' name='[a]' role='dimension' type='nominal' user:auto-column='numrec' />\n</datasource>\n"
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("user attribute group: issues = %#v", issues)
	}
}

// An enumeration of integers matches any spelling of the same integer, as
// libxml2 compares values in their type's value space.
func TestIntegerEnumerationsCompareValues(t *testing.T) {
	for value, want := range map[string]int{"1": 0, "+01": 0, "3": 1} {
		doc := edit(t, validWorkbook, "      <cards />\n", "      <cards />\n      <viewpoint>\n        <default-map-tool-selection tool='"+value+"' />\n      </viewpoint>\n")
		if issues := validate(t, "2026.1", doc); len(issues) != want {
			t.Fatalf("tool=%s: issues = %#v", value, issues)
		}
	}
}

// XSD's \d matches any Unicode decimal digit.
func TestPatternDigitsAreUnicode(t *testing.T) {
	doc := "<datasource inline='true' version='١٨.١'>\n</datasource>\n"
	if issues := validate(t, "2026.1", doc); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

// A chain's paths run through the chain, and a name reached from several
// parents keeps a path through each.
func TestLookupPathsFollowTheQuery(t *testing.T) {
	window := lookup(t, "workbook", "window > simple-id", "")
	if len(window) != 1 || len(window[0].Paths) == 0 {
		t.Fatalf("window > simple-id = %#v", window)
	}
	for _, path := range window[0].Paths {
		if path != "workbook > windows > window > simple-id" {
			t.Fatalf("paths = %q", window[0].Paths)
		}
	}
	var paths []string
	for _, element := range lookup(t, "workbook", "simple-id", "") {
		paths = append(paths, element.Paths...)
	}
	for _, want := range []string{"workbook > worksheets > worksheet > simple-id", "workbook > windows > window > simple-id"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("simple-id paths = %q, want %q", paths, want)
		}
	}
}

// An element that accepts undeclared attributes answers for any attribute
// name, with no declared type to show.
func TestLookupOfAnUndeclaredAttributeOnAnOpenElement(t *testing.T) {
	connection := lookup(t, "datasource", "datasource > connection", "class")
	if len(connection) == 0 || !connection[0].OpenAttributes || len(connection[0].Attributes) != 0 {
		t.Fatalf("connection = %#v", connection)
	}
}

// A sheet name is clipped in issue paths as values are in messages, so a
// long name cannot multiply into every issue.
func TestIssuePathsClipSheetNames(t *testing.T) {
	name := strings.Repeat("n", 5000)
	doc := edit(t, validWorkbook, "<worksheet name='Sheet 1'>", "<worksheet name='"+name+"'>")
	doc = edit(t, doc, "        <rows />\n", "        <bogus />\n        <rows />\n")
	issues := validate(t, "2026.1", doc)
	if want := `worksheet "` + strings.Repeat("n", 40) + `..." > table > bogus`; len(issues) != 1 || issues[0].Path != want {
		t.Fatalf("issues = %+v", issues)
	}
}

// The manifest rules are checked at the end of the document, after every
// other issue, so gated elements past the bound are counted, not kept.
func TestGatedElementsAreBounded(t *testing.T) {
	sorts := strings.Repeat("          <computed-sort column='[ds].[x]' direction='ASC' using='[ds].[y]' />\n", 1500)
	doc := edit(t, validWorkbook, "          <aggregation value='true' />\n", sorts+"          <aggregation value='true' />\n")
	schema, err := twbschema.Load("2026.1")
	if err != nil {
		t.Fatal(err)
	}
	result, err := schema.Validate(context.Background(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 1000 || result.Omitted != 500 {
		t.Fatalf("issues = %d, omitted = %d", len(result.Issues), result.Omitted)
	}
}

// An element name the schema does not know is clipped in the paths of the
// issues under it too.
func TestIssuePathsClipElementNames(t *testing.T) {
	name := strings.Repeat("e", 5000)
	doc := "<datasource inline='true' version='18.1'>\n  <" + name + " a='1' a='2'>\n    <x a='1' a='2' />\n  </" + name + ">\n</datasource>\n"
	issues := validate(t, "2026.1", doc)
	clipped := strings.Repeat("e", 40) + "..."
	if len(issues) != 3 || issues[1].Message != "<"+clipped+"> repeats the attribute a; XML allows each attribute once" || issues[2].Path != "datasource > "+clipped+" > x" {
		t.Fatalf("issues = %+v", issues)
	}
}
