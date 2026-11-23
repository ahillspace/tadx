package twbschema

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// The Tableau schemas and the files Tableau writes disagree in a few known
// ways. These rules follow the Tableau plugin's workbook validator, which
// confirmed each one against workbooks from a live site.

// featureCapability reports a Tableau feature capability marker, an internal
// name such as _.fcp.Marks.true...size that the schemas never list.
func featureCapability(name string) bool { return strings.HasPrefix(name, "_.fcp.") }

// omittedBefore names the required element the schemas expect before child
// although Tableau leaves it out, given what the model expects next:
// explain-data, before the elements that follow it in a workbook. The
// conditions are the plugin validator's.
func omittedBefore(child string, expected []string) string {
	switch {
	case child == "data-orientation" && slices.Equal(expected, []string{"explain-data"}),
		child == "accelerator-details" && slices.Contains(expected, "explain-data"):
		return "explain-data"
	}
	return ""
}

// missingGap reports a missing child the schemas require although Tableau
// leaves it out: explain-data on a workbook, and simple-id on sheets, which
// the SheetIdentifierTracking manifest rule reports instead.
func missingGap(element string, expected []string) bool {
	switch element {
	case "workbook":
		return slices.Contains(expected, "explain-data")
	case "worksheet", "dashboard", "window":
		return slices.Contains(expected, "simple-id")
	}
	return false
}

// legacySorts are the classes of <sort class='...'>, which Tableau still
// writes but the schemas know only as <{class}-sort>.
var legacySorts = map[string]bool{"computed": true, "manual": true, "natural": true, "alphabetic": true}

// normaliseLegacySort checks a legacy sort as its newer form, without its
// class attribute.
func normaliseLegacySort(f *frame, attrs []xml.Attr) []xml.Attr {
	if f.name != "sort" {
		return attrs
	}
	for i, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == "class" && legacySorts[a.Value] {
			f.name = a.Value + "-sort"
			return slices.Delete(slices.Clone(attrs), i, i+1)
		}
	}
	return attrs
}

// manifestGated maps an element Tableau rejects without a manifest flag to
// the flag and the fix.
var manifestGated = map[string]struct{ flag, fix string }{
	"computed-sort":   {"SortTagCleanup", "use <sort class='computed'> instead"},
	"manual-sort":     {"SortTagCleanup", "use <sort class='manual'> instead"},
	"natural-sort":    {"SortTagCleanup", "use <sort class='natural'> instead"},
	"alphabetic-sort": {"SortTagCleanup", "use <sort class='alphabetic'> instead"},
	"simple-id":       {"SheetIdentifierTracking", "remove it"},
}

// topLevelSheets maps each workbook list to the sheet kind it holds, the
// sheets that end with a simple-id.
var topLevelSheets = map[string]string{"worksheets": "worksheet", "dashboards": "dashboard", "windows": "window"}

// gate records what the manifest rules need from an element before a legacy
// sort is renamed: the manifest's flags, elements a flag allows, and the
// top-level sheets the identifier flag constrains.
func (v *validation) gate(stack []*frame, child *frame) {
	if len(stack) == 2 && stack[1].name == "document-format-change-manifest" {
		v.flags[child.name] = true
	}
	if _, ok := manifestGated[child.name]; ok {
		if len(v.gated) == issueLimit {
			v.unkept[child.name]++
		} else {
			v.gated = append(v.gated, gatedElement{line: child.line, path: issuePath(append(stack, child)), name: child.name})
		}
	}
	if len(stack) == 2 && topLevelSheets[stack[1].name] == child.name {
		v.sheets = append(v.sheets, &sheetRecord{frame: child, path: issuePath(append(stack, child))})
	}
}

// manifest reports elements whose manifest flag is missing, and sheets the
// identifier flag requires a simple-id on. The schemas cannot express either
// rule, since the manifest's content is open.
func (v *validation) manifest() {
	for _, g := range v.gated {
		rule := manifestGated[g.name]
		if !v.flags[rule.flag] {
			v.record(Issue{Line: g.line, Path: g.path, Message: fmt.Sprintf(
				"Tableau rejects <%s> unless <document-format-change-manifest> has <%s />; %s", g.name, rule.flag, rule.fix)})
		}
	}
	for name, count := range v.unkept {
		if !v.flags[manifestGated[name].flag] {
			v.omitted += count
		}
	}
	if !v.flags["SheetIdentifierTracking"] {
		return
	}
	for _, sheet := range v.sheets {
		if sheet.frame.simpleID {
			continue
		}
		where := "as its last child"
		if sheet.frame.name == "worksheet" {
			where = "after <table>"
		}
		v.record(Issue{Line: sheet.frame.line, Path: sheet.path, Message: fmt.Sprintf(
			"<%s> needs a <simple-id uuid='{...}' /> %s when <document-format-change-manifest> has <SheetIdentifierTracking />", sheet.frame.name, where)})
	}
}
