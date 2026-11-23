package twbschema

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
)

var (
	sourceBuild  = regexp.MustCompile(`^\s*(\d+)\.(\d+)`)
	buildComment = regexp.MustCompile(`^\s*build\s+(\d{4})(\d)\.\d{2}\.`)
)

// Select returns the schema for the Tableau release a workbook or datasource
// was saved with, read from the root's source-build attribute and then the
// build comment before it. When no schema matches, Select uses the nearest
// one and returns a warning that says so.
func Select(doc []byte) (*Schema, string, error) {
	release, named := documentRelease(doc)
	versions := Versions()
	oldest, newest := versions[0], versions[len(versions)-1]
	var version, warning string
	switch {
	case !named:
		version = newest
		warning = "the document does not name its Tableau release, so it is checked against the newest schema, " + newest
	case releaseLess(newest, release):
		version = newest
		warning = fmt.Sprintf("Tableau %s is newer than the newest schema, so it is checked against %s and newer features may show as issues", release, newest)
	case releaseLess(release, oldest):
		version = oldest
		warning = fmt.Sprintf("Tableau %s is older than the oldest schema, so it is checked against %s", release, oldest)
	default:
		for _, v := range versions {
			if !releaseLess(release, v) {
				version = v
			}
		}
		if version != release {
			warning = fmt.Sprintf("no schema matches Tableau %s, so it is checked against %s, the nearest older release", release, version)
		}
	}
	schema, err := Load(version)
	return schema, warning, err
}

// documentRelease reads the Tableau release, such as 2026.1, from the root
// element's source-build attribute or the build comment before the root. A
// placeholder build such as 0.0.0 names no release.
func documentRelease(doc []byte) (string, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(doc))
	var fromComment string
	for {
		token, err := decoder.Token()
		if err != nil {
			return fromComment, fromComment != ""
		}
		switch t := token.(type) {
		case xml.Comment:
			if m := buildComment.FindSubmatch(t); m != nil && fromComment == "" {
				fromComment = release(string(m[1]), string(m[2]))
			}
		case xml.StartElement:
			for _, a := range t.Attr {
				if a.Name.Space == "" && a.Name.Local == "source-build" {
					if m := sourceBuild.FindStringSubmatch(a.Value); m != nil {
						if r := release(m[1], m[2]); r != "" {
							return r, true
						}
					}
				}
			}
			return fromComment, fromComment != ""
		}
	}
}

func release(year, minor string) string {
	y, err := strconv.Atoi(year)
	if err != nil || y < 2000 {
		return ""
	}
	r, err := strconv.Atoi(minor)
	if err != nil {
		return ""
	}
	return strconv.Itoa(y) + "." + strconv.Itoa(r)
}

// Introduced returns the issues in current that original does not have.
// Issues match by message, counted, since an edit shifts lines and a repoint
// renames the datasources in issue paths: an edit that repeats an inherited
// issue is still reported.
func Introduced(current, original []Issue) []Issue {
	inherited := map[string]int{}
	for _, issue := range original {
		inherited[issue.Message]++
	}
	var out []Issue
	for _, issue := range current {
		if inherited[issue.Message] > 0 {
			inherited[issue.Message]--
			continue
		}
		out = append(out, issue)
	}
	return out
}
