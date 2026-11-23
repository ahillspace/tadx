package twbschema

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Element describes what the schema allows for one element declaration.
type Element struct {
	Name string
	// Paths are element chains from the document root that reach the
	// declaration, the shortest through each parent and each chain the query
	// named, shortest first and at most maxPaths of them.
	Paths []string
	// PathsOmitted counts the further chains that reach it.
	PathsOmitted int
	Doc          string
	// Children is the content model in Tableau's order notation, such as
	// (view, style, panes, mark-layout?, rows, cols, ...); empty when the
	// element takes no child elements.
	Children string
	// Text describes the text the element takes; empty when it takes none.
	Text       string
	Attributes []Attribute
	// OpenAttributes means the element also accepts attributes the schema
	// does not declare.
	OpenAttributes bool
	// OpenContent means the schema leaves the element's content unchecked.
	OpenContent bool
	// Unique lists the constraints the element places on its descendants.
	Unique []string
}

// Attribute describes one declared attribute.
type Attribute struct {
	Name     string
	Type     string
	Required bool
	Fixed    string
	Doc      string
}

const maxPaths = 5

// NotFoundError reports a lookup the schema has no element or attribute for,
// as opposed to a query that is not well formed.
type NotFoundError struct{ message string }

func (e *NotFoundError) Error() string { return e.message }

// graph indexes the element declarations reachable from a document root.
type graph struct {
	// order lists declarations breadth first from the roots, so the first
	// path recorded for each is a shortest one.
	order    []*elementDecl
	byName   map[string][]*elementDecl
	children map[*elementDecl][]*elementDecl
	// routes holds, for each declaration, the shortest chain through each
	// parent that reaches it, shortest first.
	routes map[*elementDecl]routes
	// shortest is each declaration's shortest chain, so a route through a
	// parent never passes through the declaration itself.
	shortest map[*elementDecl][]*elementDecl
}

// routes are element chains to one declaration, at most maxPaths of them,
// and a count of the rest.
type routes struct {
	paths   []string
	omitted int
}

func (r *routes) add(path string) {
	if slices.Contains(r.paths, path) {
		return
	}
	if len(r.paths) < maxPaths {
		r.paths = append(r.paths, path)
		return
	}
	r.omitted++
}

type graphKey struct {
	schema *Schema
	root   string
}

var graphs sync.Map // graphKey -> *graph

// graph indexes the declarations reachable from one document root.
func (s *Schema) graph(root *elementDecl) *graph {
	key := graphKey{s, root.name}
	if g, ok := graphs.Load(key); ok {
		return g.(*graph)
	}
	g := &graph{byName: map[string][]*elementDecl{}, children: map[*elementDecl][]*elementDecl{},
		routes: map[*elementDecl]routes{root: {paths: []string{root.name}}}, shortest: map[*elementDecl][]*elementDecl{root: {root}}}
	queue := []*elementDecl{root}
	for len(queue) > 0 {
		decl := queue[0]
		queue = queue[1:]
		g.order = append(g.order, decl)
		g.byName[decl.name] = append(g.byName[decl.name], decl)
		if decl.ctype == nil || decl.ctype.model == nil {
			continue
		}
		seen := map[*elementDecl]bool{}
		for _, state := range decl.ctype.model.states {
			e := state.edge
			if e == nil || e.wildcard || seen[e.decl] {
				continue
			}
			seen[e.decl] = true
			g.children[decl] = append(g.children[decl], e.decl)
			if _, ok := g.shortest[e.decl]; !ok {
				g.shortest[e.decl] = append(slices.Clip(g.shortest[decl]), e.decl)
				queue = append(queue, e.decl)
			}
			if slices.Contains(g.shortest[decl], e.decl) {
				// A recursive element such as zone > zone adds no route.
				continue
			}
			r := g.routes[e.decl]
			r.add(g.routes[decl].paths[0] + " > " + e.decl.name)
			g.routes[e.decl] = r
		}
	}
	actual, _ := graphs.LoadOrStore(key, g)
	return actual.(*graph)
}

// Lookup describes the elements a workbook or datasource document, named by
// root, can hold at element: an element name, or a chain of names ending at
// the element, such as "worksheet > table". A non-empty attribute keeps only
// that attribute. A name declared in several places returns each distinct
// declaration.
func (s *Schema) Lookup(root, element, attribute string) ([]Element, error) {
	rootDecl := s.elements[root]
	if rootDecl == nil || (root != "workbook" && root != "datasource") {
		return nil, fmt.Errorf("%q is not a workbook or datasource document root", root)
	}
	chain, err := parseChain(element)
	if err != nil {
		return nil, err
	}
	attribute = strings.TrimSpace(attribute)
	g := s.graph(rootDecl)
	// Each match carries the routes the query reached it by: every route to
	// the chain's first element, extended by each further name.
	matches := map[*elementDecl]routes{}
	for _, decl := range g.byName[chain[0]] {
		matches[decl] = g.routes[decl]
	}
	for _, name := range chain[1:] {
		next := map[*elementDecl]routes{}
		for _, decl := range g.ordered(matches) {
			from := matches[decl]
			for _, child := range g.children[decl] {
				if child.name != name {
					continue
				}
				r := next[child]
				for _, path := range from.paths {
					r.add(path + " > " + name)
				}
				r.omitted += from.omitted
				next[child] = r
			}
		}
		matches = next
	}
	if len(matches) == 0 {
		return nil, &NotFoundError{fmt.Sprintf("a Tableau %s %s has no element <%s>", s.version, root, strings.Join(chain, "> <"))}
	}
	// Declarations of one type describe the same element, so they are one
	// result with every path that reaches them.
	var out []Element
	index := map[*complexType]int{}
	for _, decl := range g.ordered(matches) {
		i, ok := index[decl.ctype]
		if !ok || decl.ctype == nil {
			i = len(out)
			index[decl.ctype] = i
			out = append(out, describe(decl))
		}
		for _, path := range matches[decl].paths {
			out[i].addPath(path)
		}
		out[i].PathsOmitted += matches[decl].omitted
	}
	if attribute == "" {
		return out, nil
	}
	var kept []Element
	for _, element := range out {
		declared := slices.IndexFunc(element.Attributes, func(a Attribute) bool { return a.Name == attribute })
		switch {
		case declared >= 0:
			element.Attributes = []Attribute{element.Attributes[declared]}
		case element.OpenAttributes:
			// The element accepts the attribute without declaring a type.
			element.Attributes = nil
		default:
			continue
		}
		kept = append(kept, element)
	}
	if len(kept) == 0 {
		first := out[0]
		names := make([]string, len(first.Attributes))
		for i, a := range first.Attributes {
			names[i] = a.Name
		}
		takes := "no attributes"
		if len(names) > 0 {
			takes = strings.Join(names, ", ")
		}
		return nil, &NotFoundError{fmt.Sprintf("<%s> in the Tableau %s schema has no attribute %s; it takes %s", first.Name, s.version, attribute, takes)}
	}
	return kept, nil
}

// ordered returns the matched declarations in breadth-first order, shortest
// path first.
func (g *graph) ordered(matches map[*elementDecl]routes) []*elementDecl {
	out := make([]*elementDecl, 0, len(matches))
	for _, decl := range g.order {
		if _, ok := matches[decl]; ok {
			out = append(out, decl)
		}
	}
	return out
}

func (e *Element) addPath(path string) {
	if slices.Contains(e.Paths, path) {
		return
	}
	if len(e.Paths) < maxPaths {
		e.Paths = append(e.Paths, path)
		return
	}
	e.PathsOmitted++
}

func parseChain(element string) ([]string, error) {
	var chain []string
	for _, part := range strings.FieldsFunc(element, func(r rune) bool { return r == '>' || r == '/' }) {
		name := strings.Trim(strings.TrimSpace(part), "<>")
		if name == "" || strings.ContainsAny(name, " \t@") {
			return nil, fmt.Errorf("%q is not an element name or a chain such as worksheet > table", element)
		}
		chain = append(chain, name)
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("%q names no element", element)
	}
	return chain, nil
}

func describe(decl *elementDecl) Element {
	element := Element{Name: decl.name, Doc: decl.doc}
	for _, u := range decl.uniques {
		if len(u.selector) == 1 {
			element.Unique = append(element.Unique, fmt.Sprintf("each <%s> needs a unique %s", u.selector[0], u.field))
		} else {
			element.Unique = append(element.Unique, fmt.Sprintf("each <%s> under <%s> needs a unique %s", u.selector[1], u.selector[0], u.field))
		}
	}
	ctype := decl.ctype
	if ctype == nil {
		element.OpenContent, element.OpenAttributes = true, true
		return element
	}
	element.OpenAttributes = ctype.localAttrs || ctype.anyAttrs
	if ctype.model != nil {
		element.Children = ctype.model.order()
		element.OpenContent = ctype.model.open()
	}
	if ctype.text != nil {
		element.Text = ctype.text.describe()
	}
	for _, a := range ctype.attrs {
		element.Attributes = append(element.Attributes, Attribute{Name: a.name, Type: a.stype.describe(), Required: a.required, Fixed: a.fixed, Doc: a.doc})
	}
	return element
}

// open reports a model whose only children are unchecked wildcards.
func (m *contentModel) open() bool {
	wildcard := false
	for _, state := range m.states {
		if state.edge == nil {
			continue
		}
		if !state.edge.wildcard {
			return false
		}
		wildcard = true
	}
	return wildcard
}
