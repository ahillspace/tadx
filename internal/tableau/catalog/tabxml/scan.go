// Package tabxml provides strict, streaming helpers for Tableau REST XML.
package tabxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Pagination is the required classic REST pagination envelope.
type Pagination struct {
	Number int
	Size   int
	Total  int
}

// Element contains one item's attributes and direct-child values.
type Element struct {
	attrs           map[string]string
	children        map[string]string
	childAttrs      map[string]map[string]string
	grandchildAttrs map[string]map[string][]map[string]string
}

// Attr returns an attribute by local XML name.
func (e Element) Attr(name string) string { return e.attrs[name] }

// ChildText returns the trimmed text of a direct child.
func (e Element) ChildText(name string) string { return strings.TrimSpace(e.children[name]) }

// ChildAttr returns an attribute from the first matching direct child.
func (e Element) ChildAttr(child, attribute string) string {
	return e.childAttrs[child][attribute]
}

// GrandchildAttrValues returns one attribute from each matching grandchild.
func (e Element) GrandchildAttrValues(parent, child, attribute string) []string {
	children := e.grandchildAttrs[parent][child]
	values := make([]string, 0, len(children))
	for _, attributes := range children {
		values = append(values, attributes[attribute])
	}
	return values
}

// DecodeList validates and streams one classic REST list response.
func DecodeList(body []byte, container, item string, visit func(Element) error) (Pagination, int, error) {
	if strings.TrimSpace(container) == "" || strings.TrimSpace(item) == "" || visit == nil {
		return Pagination{}, 0, errors.New("Tableau XML list decoder is not configured")
	}
	decoder := newDecoder(body)
	root, err := nextStart(decoder)
	if err != nil {
		return Pagination{}, 0, xmlError(err)
	}
	if root.Name.Local != "tsResponse" {
		return Pagination{}, 0, fmt.Errorf("Tableau XML root is %q; expected tsResponse", root.Name.Local)
	}

	var page Pagination
	paginationCount := 0
	containerCount := 0
	itemCount := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return Pagination{}, 0, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "pagination":
				paginationCount++
				if paginationCount > 1 {
					return Pagination{}, 0, errors.New("Tableau XML contained multiple pagination elements")
				}
				page, err = decodePagination(decoder, value)
				if err != nil {
					return Pagination{}, 0, err
				}
			case container:
				containerCount++
				if containerCount > 1 {
					return Pagination{}, 0, fmt.Errorf("Tableau XML contained multiple %s elements", container)
				}
				count, err := decodeContainer(decoder, value, item, visit)
				if err != nil {
					return Pagination{}, 0, err
				}
				itemCount += count
			default:
				if err := decoder.Skip(); err != nil {
					return Pagination{}, 0, xmlError(err)
				}
			}
		case xml.EndElement:
			if value.Name.Local == root.Name.Local {
				if paginationCount != 1 {
					return Pagination{}, 0, fmt.Errorf("Tableau XML contained %d pagination elements; expected 1", paginationCount)
				}
				if containerCount != 1 {
					return Pagination{}, 0, fmt.Errorf("Tableau XML contained %d %s elements; expected 1", containerCount, container)
				}
				if err := requireEOF(decoder); err != nil {
					return Pagination{}, 0, err
				}
				return page, itemCount, nil
			}
		}
	}
}

func decodeContainer(decoder *xml.Decoder, start xml.StartElement, item string, visit func(Element) error) (int, error) {
	count := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return 0, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local != item {
				if err := decoder.Skip(); err != nil {
					return 0, xmlError(err)
				}
				continue
			}
			element, err := decodeElement(decoder, value)
			if err != nil {
				return 0, err
			}
			if err := visit(element); err != nil {
				return 0, err
			}
			count++
		case xml.EndElement:
			if value.Name.Local == start.Name.Local {
				return count, nil
			}
		}
	}
}

func decodeElement(decoder *xml.Decoder, start xml.StartElement) (Element, error) {
	element := Element{
		attrs:           attributes(start.Attr),
		children:        make(map[string]string),
		childAttrs:      make(map[string]map[string]string),
		grandchildAttrs: make(map[string]map[string][]map[string]string),
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			return Element{}, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			name := value.Name.Local
			if _, exists := element.childAttrs[name]; !exists {
				element.childAttrs[name] = attributes(value.Attr)
			}
			text, grandchildren, err := decodeChild(decoder, value)
			if err != nil {
				return Element{}, err
			}
			if _, exists := element.grandchildAttrs[name]; !exists {
				element.grandchildAttrs[name] = grandchildren
			}
			if _, exists := element.children[name]; !exists {
				element.children[name] = text
			}
		case xml.EndElement:
			if value.Name.Local == start.Name.Local {
				return element, nil
			}
		}
	}
}

func decodeChild(decoder *xml.Decoder, start xml.StartElement) (string, map[string][]map[string]string, error) {
	var text strings.Builder
	grandchildren := make(map[string][]map[string]string)
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return "", nil, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if depth == 1 {
				grandchildren[value.Name.Local] = append(grandchildren[value.Name.Local], attributes(value.Attr))
			}
			depth++
		case xml.CharData:
			if depth == 1 {
				text.Write(value)
			}
		case xml.EndElement:
			depth--
			if depth == 0 && value.Name.Local != start.Name.Local {
				return "", nil, fmt.Errorf("malformed Tableau XML child %q", start.Name.Local)
			}
		}
	}
	return text.String(), grandchildren, nil
}

func decodePagination(decoder *xml.Decoder, start xml.StartElement) (Pagination, error) {
	values := attributes(start.Attr)
	parse := func(name string) (int, error) {
		raw, ok := values[name]
		if !ok {
			return 0, fmt.Errorf("Tableau pagination omitted %s", name)
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("Tableau pagination %s %q is not an integer", name, raw)
		}
		return value, nil
	}
	number, err := parse("pageNumber")
	if err != nil {
		return Pagination{}, err
	}
	size, err := parse("pageSize")
	if err != nil {
		return Pagination{}, err
	}
	total, err := parse("totalAvailable")
	if err != nil {
		return Pagination{}, err
	}
	if err := decoder.Skip(); err != nil {
		return Pagination{}, xmlError(err)
	}
	return Pagination{Number: number, Size: size, Total: total}, nil
}

// Capability is one permission capability and mode pair.
type Capability struct {
	Name string
	Mode string
}

// Grantee is one user or group and its capabilities.
type Grantee struct {
	Type         string
	ID           string
	Capabilities []Capability
}

// PermissionDocument is one exact content permissions response.
type PermissionDocument struct {
	ContentType string
	ContentID   string
	Grantees    []Grantee
}

// DecodePermissions validates and decodes one workbook permissions response.
func DecodePermissions(body []byte) ([]Grantee, error) {
	document, err := DecodePermissionDocument(body)
	return document.Grantees, err
}

// DecodePermissionDocument validates content identity and permission rows.
func DecodePermissionDocument(body []byte) (PermissionDocument, error) {
	decoder := newDecoder(body)
	root, err := nextStart(decoder)
	if err != nil {
		return PermissionDocument{}, xmlError(err)
	}
	if root.Name.Local != "tsResponse" {
		return PermissionDocument{}, fmt.Errorf("Tableau XML root is %q; expected tsResponse", root.Name.Local)
	}
	permissionsCount := 0
	var document PermissionDocument
	for {
		token, err := decoder.Token()
		if err != nil {
			return PermissionDocument{}, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local != "permissions" {
				if err := decoder.Skip(); err != nil {
					return PermissionDocument{}, xmlError(err)
				}
				continue
			}
			permissionsCount++
			if permissionsCount > 1 {
				return PermissionDocument{}, errors.New("Tableau XML contained multiple permissions elements")
			}
			parsed, err := decodePermissionsContainer(decoder, value)
			if err != nil {
				return PermissionDocument{}, err
			}
			document = parsed
		case xml.EndElement:
			if value.Name.Local == root.Name.Local {
				if permissionsCount != 1 {
					return PermissionDocument{}, fmt.Errorf("Tableau XML contained %d permissions elements; expected 1", permissionsCount)
				}
				if err := requireEOF(decoder); err != nil {
					return PermissionDocument{}, err
				}
				return document, nil
			}
		}
	}
}

func decodePermissionsContainer(decoder *xml.Decoder, start xml.StartElement) (PermissionDocument, error) {
	result := PermissionDocument{}
	for {
		token, err := decoder.Token()
		if err != nil {
			return PermissionDocument{}, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "workbook" {
				if result.ContentID != "" {
					return PermissionDocument{}, errors.New("Tableau permission response contained multiple content identities")
				}
				result.ContentType = "workbook"
				result.ContentID = strings.TrimSpace(attributes(value.Attr)["id"])
				if result.ContentID == "" {
					return PermissionDocument{}, errors.New("Tableau permission response contained an incomplete content identity")
				}
				if err := decoder.Skip(); err != nil {
					return PermissionDocument{}, xmlError(err)
				}
				continue
			}
			if value.Name.Local != "granteeCapabilities" {
				if err := decoder.Skip(); err != nil {
					return PermissionDocument{}, xmlError(err)
				}
				continue
			}
			grantee, err := decodeGrantee(decoder, value)
			if err != nil {
				return PermissionDocument{}, err
			}
			result.Grantees = append(result.Grantees, grantee)
		case xml.EndElement:
			if value.Name.Local == start.Name.Local {
				if result.ContentType == "" || result.ContentID == "" {
					return PermissionDocument{}, errors.New("Tableau permission response omitted content identity")
				}
				return result, nil
			}
		}
	}
}

func decodeGrantee(decoder *xml.Decoder, start xml.StartElement) (Grantee, error) {
	var grantee Grantee
	for {
		token, err := decoder.Token()
		if err != nil {
			return Grantee{}, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "user", "group":
				if grantee.ID != "" {
					return Grantee{}, errors.New("Tableau permission block contained multiple grantees")
				}
				grantee.Type = value.Name.Local
				grantee.ID = strings.TrimSpace(attributes(value.Attr)["id"])
				if err := decoder.Skip(); err != nil {
					return Grantee{}, xmlError(err)
				}
			case "capabilities":
				capabilities, err := decodeCapabilities(decoder, value)
				if err != nil {
					return Grantee{}, err
				}
				grantee.Capabilities = append(grantee.Capabilities, capabilities...)
			default:
				if err := decoder.Skip(); err != nil {
					return Grantee{}, xmlError(err)
				}
			}
		case xml.EndElement:
			if value.Name.Local == start.Name.Local {
				if grantee.ID == "" || (grantee.Type != "user" && grantee.Type != "group") {
					return Grantee{}, errors.New("Tableau permission block contained an incomplete grantee identity")
				}
				return grantee, nil
			}
		}
	}
}

func decodeCapabilities(decoder *xml.Decoder, start xml.StartElement) ([]Capability, error) {
	var result []Capability
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, xmlError(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local != "capability" {
				if err := decoder.Skip(); err != nil {
					return nil, xmlError(err)
				}
				continue
			}
			attrs := attributes(value.Attr)
			capability := Capability{Name: strings.TrimSpace(attrs["name"]), Mode: strings.TrimSpace(attrs["mode"])}
			if capability.Name == "" || capability.Mode == "" {
				return nil, errors.New("Tableau permission response contained an incomplete capability")
			}
			result = append(result, capability)
			if err := decoder.Skip(); err != nil {
				return nil, xmlError(err)
			}
		case xml.EndElement:
			if value.Name.Local == start.Name.Local {
				return result, nil
			}
		}
	}
}

func newDecoder(body []byte) *xml.Decoder {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Strict = true
	return decoder
}

func nextStart(decoder *xml.Decoder) (xml.StartElement, error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if start, ok := token.(xml.StartElement); ok {
			return start, nil
		}
	}
}

func attributes(input []xml.Attr) map[string]string {
	result := make(map[string]string, len(input))
	for _, attribute := range input {
		result[attribute.Name.Local] = attribute.Value
	}
	return result
}

func requireEOF(decoder *xml.Decoder) error {
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return xmlError(err)
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return errors.New("Tableau XML contained data after tsResponse")
			}
		case xml.StartElement:
			return errors.New("Tableau XML contained an element after tsResponse")
		}
	}
}

func xmlError(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("malformed Tableau XML: unexpected end of document")
	}
	return fmt.Errorf("malformed Tableau XML: %w", err)
}
