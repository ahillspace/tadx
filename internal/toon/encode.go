package toon

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	safeKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	numericLike    = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
)

type encoder struct {
	indentSize int
	delimiter  rune
}

type shapeField struct {
	name     string
	children []shapeField
}

// Encode returns canonical TOON using the default options.
func Encode(value any) ([]byte, error) {
	return EncodeWithOptions(value, EncodeOptions{})
}

// EncodeWithOptions returns canonical TOON for a JSON-normalizable value.
func EncodeWithOptions(value any, options EncodeOptions) ([]byte, error) {
	indentSize := options.IndentSize
	if indentSize == 0 {
		indentSize = 2
	}
	if indentSize < 1 {
		return nil, fmt.Errorf("indent size must be positive")
	}
	delimiter := options.Delimiter
	if delimiter == 0 {
		delimiter = ','
	}
	if delimiter != ',' && delimiter != '\t' && delimiter != '|' {
		return nil, fmt.Errorf("unsupported delimiter %q", delimiter)
	}
	root, err := normalize(value)
	if err != nil {
		return nil, err
	}
	enc := encoder{indentSize: indentSize, delimiter: delimiter}
	lines, err := enc.encodeRoot(root)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func (e encoder) encodeRoot(value *node) ([]string, error) {
	switch value.kind {
	case nodeObject:
		if shape, ok := keyedShape(value); ok {
			return e.encodeKeyed(nil, value, shape, 0), nil
		}
		return e.encodeObjectFields(value, 0), nil
	case nodeArray:
		return e.encodeArray(nil, value, 0, true), nil
	default:
		return []string{e.encodePrimitive(value, e.delimiter)}, nil
	}
}

func (e encoder) encodeObjectFields(value *node, depth int) []string {
	var lines []string
	for _, item := range value.object {
		lines = append(lines, e.encodeField(item.key, item.value, depth)...)
	}
	return lines
}

func (e encoder) encodeField(key string, value *node, depth int) []string {
	prefix := e.indent(depth) + encodeKey(key)
	switch value.kind {
	case nodeObject:
		if shape, ok := keyedShape(value); ok {
			return e.encodeKeyed(&key, value, shape, depth)
		}
		lines := []string{prefix + ":"}
		lines = append(lines, e.encodeObjectFields(value, depth+1)...)
		return lines
	case nodeArray:
		return e.encodeArray(&key, value, depth, true)
	default:
		return []string{prefix + ": " + e.encodePrimitive(value, e.delimiter)}
	}
}

func (e encoder) encodeArray(key *string, value *node, depth int, allowTable bool) []string {
	prefix := e.indent(depth)
	if key != nil {
		prefix += encodeKey(*key)
	}
	if len(value.array) == 0 {
		if key == nil && depth == 0 {
			return []string{"[]"}
		}
		if key != nil {
			return []string{prefix + ": []"}
		}
		return []string{prefix + "[0" + e.delimiterSymbol() + "]:"}
	}
	if allPrimitive(value.array) {
		cells := make([]string, len(value.array))
		for i, item := range value.array {
			cells[i] = e.encodePrimitive(item, e.delimiter)
		}
		return []string{fmt.Sprintf("%s[%d%s]: %s", prefix, len(cells), e.delimiterSymbol(), strings.Join(cells, string(e.delimiter)))}
	}
	if allowTable {
		if shape, ok := tabularShape(value); ok {
			lines := []string{fmt.Sprintf("%s[%d%s]{%s}:", prefix, len(value.array), e.delimiterSymbol(), e.encodeShape(shape))}
			for _, row := range value.array {
				cells := e.flatten(row, shape)
				lines = append(lines, e.indent(depth+1)+strings.Join(cells, string(e.delimiter)))
			}
			return lines
		}
	}
	lines := []string{fmt.Sprintf("%s[%d%s]:", prefix, len(value.array), e.delimiterSymbol())}
	for _, item := range value.array {
		lines = append(lines, e.encodeListItem(item, depth+1)...)
	}
	return lines
}

func (e encoder) encodeListItem(value *node, depth int) []string {
	prefix := e.indent(depth)
	switch value.kind {
	case nodeObject:
		if len(value.object) == 0 {
			return []string{prefix + "-"}
		}
		fieldLines := e.encodeObjectFields(value, depth+1)
		fieldLines[0] = prefix + "- " + strings.TrimPrefix(fieldLines[0], e.indent(depth+1))
		return fieldLines
	case nodeArray:
		lines := e.encodeArray(nil, value, depth, false)
		lines[0] = prefix + "- " + strings.TrimPrefix(lines[0], prefix)
		return lines
	default:
		return []string{prefix + "- " + e.encodePrimitive(value, e.delimiter)}
	}
}

func (e encoder) encodeKeyed(key *string, value *node, shape []shapeField, depth int) []string {
	prefix := e.indent(depth)
	if key != nil {
		prefix += encodeKey(*key)
	}
	lines := []string{fmt.Sprintf("%s[%d:%s]{%s}:", prefix, len(value.object), e.delimiterSymbol(), e.encodeShape(shape))}
	for _, entry := range value.object {
		cells := e.flatten(entry.value, shape)
		lines = append(lines, e.indent(depth+1)+encodeKey(entry.key)+": "+strings.Join(cells, string(e.delimiter)))
	}
	return lines
}

func (e encoder) encodeShape(shape []shapeField) string {
	items := make([]string, len(shape))
	for i, item := range shape {
		items[i] = encodeKey(item.name)
		if len(item.children) > 0 {
			items[i] += "{" + e.encodeShape(item.children) + "}"
		}
	}
	return strings.Join(items, string(e.delimiter))
}

func (e encoder) flatten(value *node, shape []shapeField) []string {
	var cells []string
	for _, item := range shape {
		child, _ := value.field(item.name)
		if len(item.children) == 0 {
			cells = append(cells, e.encodePrimitive(child, e.delimiter))
		} else {
			cells = append(cells, e.flatten(child, item.children)...)
		}
	}
	return cells
}

func (e encoder) encodePrimitive(value *node, delimiter rune) string {
	switch value.kind {
	case nodeNull:
		return "null"
	case nodeBool:
		return strconv.FormatBool(value.value.(bool))
	case nodeNumber:
		canonical, err := canonicalNumber(value.value.(json.Number).String())
		if err != nil {
			return "null"
		}
		return canonical
	case nodeString:
		text := value.value.(string)
		if requiresQuote(text, delimiter) {
			return quote(text)
		}
		return text
	default:
		panic("non-primitive TOON cell")
	}
}

func (e encoder) indent(depth int) string {
	return strings.Repeat(" ", depth*e.indentSize)
}

func (e encoder) delimiterSymbol() string {
	if e.delimiter == ',' {
		return ""
	}
	return string(e.delimiter)
}

func allPrimitive(values []*node) bool {
	for _, value := range values {
		if !value.primitive() {
			return false
		}
	}
	return true
}

func tabularShape(value *node) ([]shapeField, bool) {
	if value.kind != nodeArray || len(value.array) == 0 {
		return nil, false
	}
	objects := value.array
	return uniformObjectShape(objects)
}

func keyedShape(value *node) ([]shapeField, bool) {
	if value.kind != nodeObject || len(value.object) < 2 {
		return nil, false
	}
	objects := make([]*node, len(value.object))
	for i, item := range value.object {
		objects[i] = item.value
	}
	return uniformObjectShape(objects)
}

func uniformObjectShape(objects []*node) ([]shapeField, bool) {
	if len(objects) == 0 || objects[0].kind != nodeObject || len(objects[0].object) == 0 {
		return nil, false
	}
	shape := make([]shapeField, len(objects[0].object))
	for i, item := range objects[0].object {
		shape[i].name = item.key
	}
	for _, object := range objects {
		if object.kind != nodeObject || len(object.object) != len(shape) {
			return nil, false
		}
		for _, item := range shape {
			if _, ok := object.field(item.name); !ok {
				return nil, false
			}
		}
	}
	for i := range shape {
		column := make([]*node, len(objects))
		for j, object := range objects {
			column[j], _ = object.field(shape[i].name)
		}
		if allPrimitive(column) {
			continue
		}
		children, ok := uniformObjectShape(column)
		if !ok {
			return nil, false
		}
		shape[i].children = children
	}
	return shape, true
}

func encodeKey(key string) string {
	if safeKeyPattern.MatchString(key) {
		return key
	}
	return quote(key)
}

func requiresQuote(value string, delimiter rune) bool {
	if value == "" || value == "true" || value == "false" || value == "null" || numericLike.MatchString(value) {
		return true
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "#") {
		return true
	}
	if strings.HasPrefix(value, " ") || strings.HasPrefix(value, "\t") || strings.HasSuffix(value, " ") || strings.HasSuffix(value, "\t") {
		return true
	}
	for _, r := range value {
		if r <= 0x1f || r == ':' || r == '"' || r == '\\' || r == '[' || r == ']' || r == '{' || r == '}' || r == delimiter {
			return true
		}
	}
	return false
}

func quote(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&builder, `\u%04x`, r)
			} else {
				builder.WriteRune(r)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func canonicalNumber(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty number")
	}
	negative := input[0] == '-'
	if negative {
		input = input[1:]
	}
	exponent := new(big.Int)
	if index := strings.IndexAny(input, "eE"); index >= 0 {
		if _, ok := exponent.SetString(input[index+1:], 10); !ok {
			return "", fmt.Errorf("invalid number exponent")
		}
		input = input[:index]
	}
	integer := input
	fraction := ""
	if index := strings.IndexByte(input, '.'); index >= 0 {
		integer, fraction = input[:index], input[index+1:]
	}
	digits := integer + fraction
	leading := len(digits) - len(strings.TrimLeft(digits, "0"))
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "0", nil
	}
	digits = strings.TrimRight(digits, "0")
	magnitude := new(big.Int).Set(exponent)
	magnitude.Add(magnitude, big.NewInt(int64(len(integer)-leading-1)))
	var result string
	if magnitude.Cmp(big.NewInt(-6)) >= 0 && magnitude.Cmp(big.NewInt(21)) < 0 {
		magnitudeInt := int(magnitude.Int64())
		decimalPosition := magnitudeInt + 1
		switch {
		case decimalPosition <= 0:
			result = "0." + strings.Repeat("0", -decimalPosition) + digits
		case decimalPosition >= len(digits):
			result = digits + strings.Repeat("0", decimalPosition-len(digits))
		default:
			result = digits[:decimalPosition] + "." + digits[decimalPosition:]
		}
	} else {
		result = digits[:1]
		if len(digits) > 1 {
			result += "." + digits[1:]
		}
		if magnitude.Sign() >= 0 {
			result += "e+" + magnitude.String()
		} else {
			result += "e" + magnitude.String()
		}
	}
	if negative {
		result = "-" + result
	}
	return result, nil
}
