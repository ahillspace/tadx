package twbschema

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
)

// documentLimit and depthLimit bound the size and nesting of one document.
// issueLimit bounds the issues one document reports.
const (
	documentLimit = 256 << 20
	depthLimit    = 1024
	issueLimit    = 1000
)

// namedScopes start an issue path, so a path reads from the sheet or
// datasource an agent recognises.
var namedScopes = map[string]bool{"worksheet": true, "dashboard": true, "window": true, "datasource": true}

// frame is one open element during validation.
type frame struct {
	name  string
	label string
	line  int
	decl  *elementDecl
	set   stateSet
	text  strings.Builder
	// seen holds the values each of the element's unique constraints has met.
	seen map[*uniqueDecl]map[string]bool
	// simpleID records whether the element has a simple-id child.
	simpleID bool
	// failed stops content checks after the first content error, so one
	// misplaced child is one issue.
	failed bool
	// skip marks content the schema leaves unchecked.
	skip bool
}

// Validate checks a workbook or datasource document and returns its issues in
// document order. An error means the document could not be read as XML.
func (s *Schema) Validate(ctx context.Context, doc []byte) (Result, error) {
	if len(doc) > documentLimit {
		return Result{}, errors.New("document exceeds the 256 MiB bound")
	}
	v := &validation{lines: lineCounter{doc: doc, line: 1}, flags: map[string]bool{}, unkept: map[string]int{}}
	decoder := xml.NewDecoder(bytes.NewReader(doc))
	var stack []*frame
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Result{}, fmt.Errorf("read the document as XML: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) == depthLimit {
				return Result{}, errors.New("document nests elements beyond 1024 levels")
			}
			child := &frame{name: elementName(t.Name), line: v.lines.at(offset), label: label(elementName(t.Name), t.Attr)}
			v.gate(stack, child)
			attrs := normaliseLegacySort(child, t.Attr)
			if len(stack) == 0 {
				child.decl = s.elements[child.name]
				if child.decl == nil {
					v.add(child.line, []*frame{child}, fmt.Sprintf("<%s> is not a Tableau workbook or datasource root", child.name))
					child.skip = true
				}
			} else {
				v.child(stack, child)
				v.unique(stack, child, attrs)
			}
			stack = append(stack, child)
			v.attributes(stack, attrs)
			if child.decl != nil && child.decl.ctype != nil && child.decl.ctype.model != nil {
				child.set = child.decl.ctype.model.begin()
			}
		case xml.CharData:
			if n := len(stack); n > 0 && !stack[n-1].skip && stack[n-1].decl != nil && stack[n-1].decl.ctype != nil {
				stack[n-1].text.Write(t)
			}
		case xml.EndElement:
			v.end(stack)
			stack = stack[:len(stack)-1]
		}
	}
	v.manifest()
	slices.SortStableFunc(v.issues, func(a, b Issue) int { return a.Line - b.Line })
	return Result{Issues: v.issues, Omitted: v.omitted}, nil
}

type validation struct {
	lines   lineCounter
	issues  []Issue
	omitted int
	// flags are the children of the root's document-format-change-manifest.
	flags map[string]bool
	// gated are elements Tableau accepts only under a manifest flag, and
	// sheets are the top-level worksheets and windows that flag constrains.
	// unkept counts the gated elements past issueLimit by name, since no
	// more than that many can be reported.
	gated  []gatedElement
	sheets []*sheetRecord
	unkept map[string]int
}

type gatedElement struct {
	line int
	path string
	name string
}

type sheetRecord struct {
	frame *frame
	path  string
}

func (v *validation) add(line int, stack []*frame, message string) {
	if len(v.issues) == issueLimit {
		v.omitted++
		return
	}
	v.record(Issue{Line: line, Path: issuePath(stack), Message: message})
}

// record keeps an issue until the bound and counts the rest.
func (v *validation) record(issue Issue) {
	if len(v.issues) == issueLimit {
		v.omitted++
		return
	}
	v.issues = append(v.issues, issue)
}

// child checks one child element against its parent's content model and
// resolves the child's declaration.
func (v *validation) child(stack []*frame, child *frame) {
	line := child.line
	parent := stack[len(stack)-1]
	if parent.skip {
		child.skip = true
		return
	}
	if parent.decl.ctype == nil {
		// An element with no declared type accepts any content.
		child.skip = true
		return
	}
	model := parent.decl.ctype.model
	if model == nil {
		if !parent.failed {
			parent.failed = true
			v.add(line, append(stack, child), fmt.Sprintf("<%s> allows no child elements; remove <%s>", parent.name, child.name))
		}
		child.skip = true
		return
	}
	if featureCapability(child.name) {
		// Tableau writes feature capability markers anywhere; they are not
		// content, so the model does not step past them.
		child.skip = true
		return
	}
	if !parent.failed {
		next, matched := model.step(parent.set, child.name)
		if omitted := omittedBefore(child.name, model.expected(parent.set)); matched == nil && omitted != "" {
			if past, edge := model.step(parent.set, omitted); edge != nil {
				next, matched = model.step(past, child.name)
			}
		}
		if matched != nil {
			parent.set = next
			if matched.wildcard {
				child.skip = true
				return
			}
			child.decl = matched.decl
			return
		}
		parent.failed = true
		v.add(line, append(stack, child), notAllowed(parent.name, child.name, model, parent.set))
	}
	if child.decl = model.declFor(child.name); child.decl == nil {
		child.skip = true
	}
}

// attributes checks an element's attributes against its type.
func (v *validation) attributes(stack []*frame, attrs []xml.Attr) {
	f := stack[len(stack)-1]
	line := f.line
	repeated := map[xml.Name]bool{}
	for _, a := range attrs {
		if repeated[a.Name] {
			v.add(line, stack, fmt.Sprintf("<%s> repeats the attribute %s; XML allows each attribute once", clip(f.name), a.Name.Local))
		}
		repeated[a.Name] = true
	}
	if f.skip || f.decl == nil || f.decl.ctype == nil {
		return
	}
	ctype := f.decl.ctype
	seen := map[string]bool{}
	foreign := map[xml.Name]bool{}
	for _, a := range attrs {
		if foreignAttribute(a.Name) {
			// No declaration is in a namespace, so only an ##any wildcard
			// admits it. Identity is the full name; only the message clips it.
			if !ctype.anyAttrs && !foreign[a.Name] {
				v.add(line, stack, unknownAttribute(f.name, clip(a.Name.Space)+":"+clip(a.Name.Local), ctype))
			}
			foreign[a.Name] = true
			continue
		}
		name := attrName(a.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		decl := ctype.attr(name)
		if decl == nil {
			if !ctype.anyAttrs && !(ctype.localAttrs && a.Name.Space == "") && !featureCapability(name) {
				v.add(line, stack, unknownAttribute(f.name, name, ctype))
			}
			continue
		}
		if decl.hasFixed && decl.stype.canonical(a.Value) != decl.stype.canonical(decl.fixed) {
			v.add(line, stack, fmt.Sprintf("%s='%s' is not allowed; it must be '%s'", name, clip(a.Value), decl.fixed))
		} else if miss := decl.stype.check(a.Value); miss != "" {
			v.add(line, stack, fmt.Sprintf("%s='%s' is not allowed; expected %s", name, clip(a.Value), miss))
		}
	}
	for _, decl := range ctype.attrs {
		if decl.required && !seen[decl.name] {
			v.add(line, stack, fmt.Sprintf("<%s> is missing the required attribute %s", f.name, decl.name))
		}
	}
}

// elementName returns the schema name of an element. The Tableau schemas
// declare no target namespace, so an element in a namespace keeps it and
// matches no declaration.
func elementName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return clip(name.Space) + ":" + clip(name.Local)
}

// attrName returns the schema name of an unqualified or xml: attribute, or
// "" for a namespace declaration.
func attrName(name xml.Name) string {
	switch name.Space {
	case "":
		if name.Local == "xmlns" {
			return ""
		}
		return name.Local
	case "xml", "http://www.w3.org/XML/1998/namespace":
		return "xml:" + name.Local
	}
	return ""
}

// foreignAttribute reports an attribute in a namespace other than xml, which
// the Tableau schemas declare no attributes in.
func foreignAttribute(name xml.Name) bool {
	switch name.Space {
	case "", "xmlns", "xml", "http://www.w3.org/XML/1998/namespace":
		return false
	}
	return true
}

func unknownAttribute(element, name string, ctype *complexType) string {
	if len(ctype.attrs) == 0 {
		return fmt.Sprintf("<%s> has no attribute %s; it takes no attributes", element, name)
	}
	names := make([]string, len(ctype.attrs))
	for i, a := range ctype.attrs {
		names[i] = a.name
	}
	return fmt.Sprintf("<%s> has no attribute %s; it takes %s", element, name, strings.Join(names, ", "))
}

// end checks an element's text and reports required children it is
// missing, at its opening tag as libxml2 does.
func (v *validation) end(stack []*frame) {
	closing := stack[len(stack)-1]
	if len(stack) > 1 && closing.name == "simple-id" {
		stack[len(stack)-2].simpleID = true
	}
	if closing.skip || closing.decl == nil || closing.decl.ctype == nil {
		return
	}
	ctype := closing.decl.ctype
	v.text(stack, ctype)
	if closing.failed || ctype.model == nil {
		return
	}
	model := ctype.model
	if model.accepts(closing.set) || missingGap(closing.name, model.expected(closing.set)) {
		return
	}
	v.add(closing.line, stack, fmt.Sprintf("<%s> is missing %s. <%s> children go in this order: %s",
		closing.name, alternatives(model.required(closing.set)), closing.name, model.order()))
}

// text checks the element's text against its simple content, or reports
// text in an element that takes none.
func (v *validation) text(stack []*frame, ctype *complexType) {
	f := stack[len(stack)-1]
	value := f.text.String()
	if ctype.text == nil {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			v.add(f.line, stack, fmt.Sprintf("<%s> takes no text; remove '%s'", f.name, clip(trimmed)))
		}
		return
	}
	if ctype.model != nil {
		return
	}
	if miss := ctype.text.check(value); miss != "" {
		v.add(f.line, stack, fmt.Sprintf("<%s> text '%s' is not allowed; expected %s", f.name, clip(strings.TrimSpace(value)), miss))
	}
}

func clip(s string) string {
	if utf8.RuneCountInString(s) <= 40 {
		return s
	}
	return string([]rune(s)[:40]) + "..."
}

// unique checks the child against the unique constraints of its parent and
// grandparent, the only depths the Tableau schemas select at.
func (v *validation) unique(stack []*frame, child *frame, attrs []xml.Attr) {
	for depth := 1; depth <= 2 && depth <= len(stack); depth++ {
		scope := stack[len(stack)-depth]
		if scope.skip || scope.decl == nil {
			continue
		}
		for _, u := range scope.decl.uniques {
			if len(u.selector) != depth || u.selector[depth-1] != child.name || (depth == 2 && stack[len(stack)-1].name != u.selector[0]) {
				continue
			}
			value, ok := attrValue(attrs, u.field)
			if !ok {
				continue
			}
			key := value
			if child.decl != nil && child.decl.ctype != nil {
				if decl := child.decl.ctype.attr(u.field); decl != nil {
					key = decl.stype.canonical(value)
				}
			}
			if scope.seen == nil {
				scope.seen = map[*uniqueDecl]map[string]bool{}
			}
			if scope.seen[u] == nil {
				scope.seen[u] = map[string]bool{}
			}
			if scope.seen[u][key] {
				each := "<" + child.name + ">"
				if depth == 2 {
					each = fmt.Sprintf("<%s> under <%s>", child.name, u.selector[0])
				}
				v.add(child.line, append(stack, child), fmt.Sprintf("<%s> %s='%s' repeats; each %s in <%s> needs a unique %s",
					child.name, u.field, clip(value), each, scope.name, u.field))
				continue
			}
			scope.seen[u][key] = true
		}
	}
}

func attrValue(attrs []xml.Attr, name string) (string, bool) {
	for _, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func notAllowed(parent, child string, model *contentModel, set stateSet) string {
	expected := model.expected(set)
	if len(expected) == 0 {
		return fmt.Sprintf("<%s> is not allowed here; <%s> takes no more children. <%s> children go in this order: %s",
			child, parent, parent, model.order())
	}
	return fmt.Sprintf("<%s> is not allowed here; expected %s. <%s> children go in this order: %s",
		child, alternatives(expected), parent, model.order())
}

func alternatives(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		if name == "*" {
			quoted[i] = "any element"
		} else {
			quoted[i] = "<" + name + ">"
		}
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " or " + quoted[1]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + ", or " + quoted[len(quoted)-1]
}

// label names an element in issue paths, clipped as values are in messages.
func label(name string, attrs []xml.Attr) string {
	if !namedScopes[name] {
		return clip(name)
	}
	for _, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == "name" {
			return fmt.Sprintf("%s %q", name, clip(a.Value))
		}
	}
	return name
}

// issuePath renders the element chain from the innermost named sheet or
// datasource, or from the root when there is none.
func issuePath(stack []*frame) string {
	start := 0
	for i := len(stack) - 1; i >= 0; i-- {
		if namedScopes[stack[i].name] && stack[i].label != stack[i].name {
			start = i
			break
		}
	}
	parts := make([]string, 0, len(stack)-start)
	for _, f := range stack[start:] {
		parts = append(parts, f.label)
	}
	return strings.Join(parts, " > ")
}

// lineCounter maps the increasing byte offsets of the decoder's tokens to
// 1-based line numbers, counting newlines once as the offsets advance.
type lineCounter struct {
	doc    []byte
	offset int
	line   int
}

func (l *lineCounter) at(offset int64) int {
	end := min(int(offset), len(l.doc))
	if end > l.offset {
		l.line += bytes.Count(l.doc[l.offset:end], []byte{'\n'})
		l.offset = end
	}
	return l.line
}
