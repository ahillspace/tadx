package twbschema

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const unbounded = -1

// xsNode is one element of an XSD document.
type xsNode struct {
	local string
	attrs map[string]string
	kids  []*xsNode
	text  string
}

func (n *xsNode) attr(name string) string { return n.attrs[name] }

// documentation returns the node's xs:annotation text with its lines
// trimmed and joined.
func (n *xsNode) documentation() string {
	var lines []string
	for _, kid := range n.kids {
		if kid.local != "annotation" {
			continue
		}
		for _, doc := range kid.kids {
			if doc.local == "documentation" {
				lines = append(lines, strings.Fields(doc.text)...)
			}
		}
	}
	return strings.Join(lines, " ")
}

func parseXSD(src []byte) (*xsNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(src))
	var stack []*xsNode
	var root *xsNode
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			n := &xsNode{local: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				if a.Name.Space == "" {
					n.attrs[a.Name.Local] = a.Value
				}
			}
			if len(stack) == 0 {
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			}
			stack = append(stack, n)
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1].local == "documentation" {
				stack[len(stack)-1].text += string(t)
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil || root.local != "schema" {
		return nil, errors.New("not an XML schema")
	}
	return root, nil
}

// elementDecl is one declared element. A nil ctype with a nil stype means the
// element has no declared type and accepts any content.
type elementDecl struct {
	name  string
	ctype *complexType
	doc   string
	// uniques are the xs:unique constraints the element places on its
	// descendants.
	uniques []*uniqueDecl
}

// uniqueDecl is one xs:unique constraint: no two elements at selector, a
// path of one or two child names, share a value of field.
type uniqueDecl struct {
	name     string
	selector []string
	field    string
}

// complexType is a compiled element type.
type complexType struct {
	// model checks the child elements; nil means no child elements are allowed.
	model *contentModel
	// text checks the element's text when it has simple content.
	text  *simpleType
	attrs []*attributeDecl
	// localAttrs accepts undeclared unqualified attributes, as
	// xs:anyAttribute namespace="##local" does; anyAttrs accepts undeclared
	// attributes in any namespace.
	localAttrs, anyAttrs bool
}

// attributeDecl is one declared attribute.
type attributeDecl struct {
	name     string
	doc      string
	stype    *simpleType
	required bool
	fixed    string
	hasFixed bool
}

func (t *complexType) attr(name string) *attributeDecl {
	for _, a := range t.attrs {
		if a.name == name {
			return a
		}
	}
	return nil
}

// particle is one term of a content model before NFA construction.
type particle struct {
	kind     string // element, sequence, choice, all, any
	min, max int
	decl     *elementDecl
	kids     []*particle
}

type compiler struct {
	complexTypes    map[string]*xsNode
	simpleTypes     map[string]*xsNode
	groups          map[string]*xsNode
	attrGroups      map[string]*xsNode
	compiledTypes   map[*xsNode]*complexType
	simpleTypesDone map[*xsNode]*simpleType
	decls           map[*xsNode]*elementDecl
}

// compile turns an XSD document into the root element declarations a document
// can start with.
func compile(src []byte) (map[string]*elementDecl, error) {
	root, err := parseXSD(src)
	if err != nil {
		return nil, err
	}
	c := &compiler{
		complexTypes:    map[string]*xsNode{},
		simpleTypes:     map[string]*xsNode{},
		groups:          map[string]*xsNode{},
		attrGroups:      map[string]*xsNode{},
		compiledTypes:   map[*xsNode]*complexType{},
		simpleTypesDone: map[*xsNode]*simpleType{},
		decls:           map[*xsNode]*elementDecl{},
	}
	var globals []*xsNode
	for _, n := range root.kids {
		switch n.local {
		case "complexType":
			c.complexTypes[n.attr("name")] = n
		case "simpleType":
			c.simpleTypes[n.attr("name")] = n
		case "group":
			c.groups[n.attr("name")] = n
		case "attributeGroup":
			c.attrGroups[n.attr("name")] = n
		case "element":
			globals = append(globals, n)
		}
	}
	roots := map[string]*elementDecl{}
	for _, n := range globals {
		decl, err := c.element(n)
		if err != nil {
			return nil, err
		}
		roots[decl.name] = decl
	}
	// A datasource file's root has the type the workbook uses for each of its
	// datasources; the schema declares no global datasource element.
	if n, ok := c.complexTypes["DataSource-CT"]; ok {
		ctype, err := c.complex(n)
		if err != nil {
			return nil, err
		}
		roots["datasource"] = &elementDecl{name: "datasource", ctype: ctype}
	}
	return roots, nil
}

func (c *compiler) element(n *xsNode) (*elementDecl, error) {
	if decl, ok := c.decls[n]; ok {
		return decl, nil
	}
	if n.attr("ref") != "" {
		return nil, fmt.Errorf("element ref %q is not supported", n.attr("ref"))
	}
	decl := &elementDecl{name: n.attr("name"), doc: n.documentation()}
	c.decls[n] = decl
	var typeNode *xsNode
	for _, kid := range n.kids {
		switch kid.local {
		case "unique":
			u := &uniqueDecl{name: kid.attr("name")}
			for _, part := range kid.kids {
				switch part.local {
				case "selector":
					u.selector = strings.Split(part.attr("xpath"), "/")
				case "field":
					field, ok := strings.CutPrefix(part.attr("xpath"), "@")
					if !ok {
						return nil, fmt.Errorf("unique %q field %q is not an attribute", u.name, part.attr("xpath"))
					}
					u.field = field
				}
			}
			if len(u.selector) == 0 || len(u.selector) > 2 || u.field == "" {
				return nil, fmt.Errorf("unique %q selector is not supported", u.name)
			}
			decl.uniques = append(decl.uniques, u)
		case "complexType":
			typeNode = kid
		case "simpleType":
			text, err := c.simple(kid)
			if err != nil {
				return nil, err
			}
			decl.ctype = &complexType{text: text}
		}
	}
	if name := n.attr("type"); name != "" {
		if strings.HasPrefix(name, "xs:") || c.simpleTypes[name] != nil {
			text, err := c.simpleNamed(name)
			if err != nil {
				return nil, err
			}
			decl.ctype = &complexType{text: text}
		} else if typeNode = c.complexTypes[name]; typeNode == nil {
			return nil, fmt.Errorf("element %q has unknown type %q", decl.name, name)
		}
	}
	if typeNode != nil {
		ctype, err := c.complex(typeNode)
		if err != nil {
			return nil, err
		}
		decl.ctype = ctype
	}
	return decl, nil
}

func (c *compiler) complex(n *xsNode) (*complexType, error) {
	if ctype, ok := c.compiledTypes[n]; ok {
		return ctype, nil
	}
	ctype := &complexType{}
	c.compiledTypes[n] = ctype
	content, err := c.content(n, ctype)
	if err != nil {
		return nil, err
	}
	if len(content) > 0 {
		ctype.model = newContentModel(&particle{kind: "sequence", min: 1, max: 1, kids: content})
	}
	return ctype, nil
}

// content returns the particles of a type's element content and adds its
// attributes and simple content to ctype.
func (c *compiler) content(n *xsNode, ctype *complexType) ([]*particle, error) {
	var out []*particle
	for _, kid := range n.kids {
		switch kid.local {
		case "simpleContent":
			for _, ext := range kid.kids {
				if ext.local != "extension" {
					return nil, fmt.Errorf("simpleContent %s is not supported", ext.local)
				}
				text, err := c.textBase(ext.attr("base"), ctype)
				if err != nil {
					return nil, err
				}
				ctype.text = text
				if err := c.attributes(ext, ctype); err != nil {
					return nil, err
				}
			}
		case "complexContent":
			for _, ext := range kid.kids {
				if ext.local != "extension" {
					return nil, fmt.Errorf("complexContent %s is not supported", ext.local)
				}
				base := c.complexTypes[ext.attr("base")]
				if base == nil {
					return nil, fmt.Errorf("unknown base type %q", ext.attr("base"))
				}
				inherited, err := c.content(base, ctype)
				if err != nil {
					return nil, err
				}
				out = append(out, inherited...)
				own, err := c.content(ext, ctype)
				if err != nil {
					return nil, err
				}
				out = append(out, own...)
			}
		case "sequence", "choice", "all", "group", "any":
			p, err := c.particle(kid)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		case "attribute", "attributeGroup", "anyAttribute":
			if err := c.attribute(kid, ctype); err != nil {
				return nil, err
			}
		}
	}
	if n.attr("mixed") == "true" && ctype.text == nil {
		ctype.text = &simpleType{builtin: "string", minLen: -1, maxLen: -1}
	}
	return out, nil
}

// textBase resolves a simpleContent base, which is a simple type or a complex
// type with simple content whose attributes are inherited.
func (c *compiler) textBase(name string, ctype *complexType) (*simpleType, error) {
	if base := c.complexTypes[name]; base != nil {
		if _, err := c.content(base, ctype); err != nil {
			return nil, err
		}
		return ctype.text, nil
	}
	return c.simpleNamed(name)
}

func (c *compiler) attributes(n *xsNode, ctype *complexType) error {
	for _, kid := range n.kids {
		switch kid.local {
		case "attribute", "attributeGroup", "anyAttribute":
			if err := c.attribute(kid, ctype); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *compiler) attribute(n *xsNode, ctype *complexType) error {
	switch n.local {
	case "anyAttribute":
		if n.attr("namespace") == "##local" {
			ctype.localAttrs = true
		} else {
			ctype.anyAttrs = true
		}
		return nil
	case "attributeGroup":
		ref := n.attr("ref")
		if strings.HasPrefix(ref, "user:") {
			// The user namespace is Tableau's open extension point; the
			// plugin validator stubs this group with an ##any wildcard.
			ctype.anyAttrs = true
			return nil
		}
		group := c.attrGroups[ref]
		if group == nil {
			return fmt.Errorf("unknown attribute group %q", ref)
		}
		return c.attributes(group, ctype)
	}
	if ref := n.attr("ref"); ref != "" {
		if ref != "xml:base" {
			return fmt.Errorf("attribute ref %q is not supported", ref)
		}
		stype, _ := newBuiltin("anyURI")
		ctype.attrs = append(ctype.attrs, &attributeDecl{name: ref, stype: stype})
		return nil
	}
	decl := &attributeDecl{name: n.attr("name"), doc: n.documentation(), required: n.attr("use") == "required"}
	decl.fixed, decl.hasFixed = n.attrs["fixed"]
	var err error
	if name := n.attr("type"); name != "" {
		decl.stype, err = c.simpleNamed(name)
	} else {
		for _, kid := range n.kids {
			if kid.local == "simpleType" {
				decl.stype, err = c.simple(kid)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("attribute %q: %w", decl.name, err)
	}
	if decl.stype == nil {
		decl.stype, _ = newBuiltin("anySimpleType")
	}
	ctype.attrs = append(ctype.attrs, decl)
	return nil
}

func (c *compiler) particle(n *xsNode) (*particle, error) {
	min, max, err := occurs(n)
	if err != nil {
		return nil, err
	}
	p := &particle{kind: n.local, min: min, max: max}
	switch n.local {
	case "element":
		if p.decl, err = c.element(n); err != nil {
			return nil, err
		}
	case "any":
	case "group":
		def := c.groups[n.attr("ref")]
		if def == nil {
			return nil, fmt.Errorf("unknown group %q", n.attr("ref"))
		}
		p.kind = "sequence"
		for _, kid := range def.kids {
			if kid.local == "annotation" {
				continue
			}
			inner, err := c.particle(kid)
			if err != nil {
				return nil, err
			}
			p.kids = append(p.kids, inner)
		}
	case "sequence", "choice", "all":
		for _, kid := range n.kids {
			if kid.local == "annotation" {
				continue
			}
			inner, err := c.particle(kid)
			if err != nil {
				return nil, err
			}
			p.kids = append(p.kids, inner)
		}
	default:
		return nil, fmt.Errorf("particle %s is not supported", n.local)
	}
	return p, nil
}

func occurs(n *xsNode) (int, int, error) {
	min, max := 1, 1
	if v := n.attr("minOccurs"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, fmt.Errorf("minOccurs %q: %w", v, err)
		}
		min = parsed
	}
	if v := n.attr("maxOccurs"); v == "unbounded" {
		max = unbounded
	} else if v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, fmt.Errorf("maxOccurs %q: %w", v, err)
		}
		max = parsed
	}
	return min, max, nil
}
