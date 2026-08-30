package toon

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type decodedLine struct {
	depth       int
	content     string
	blankBefore bool
	number      int
}

type header struct {
	key       string
	hasKey    bool
	length    int
	keyed     bool
	delimiter rune
	fields    []shapeField
	inline    string
}

type decoder struct {
	lines  []decodedLine
	strict bool
}

// Decode parses TOON in strict mode.
func Decode(data []byte) (any, error) {
	return decode(data, 2, true)
}

// DecodeWithOptions parses TOON with explicit strictness and indentation.
func DecodeWithOptions(data []byte, options DecodeOptions) (any, error) {
	indentSize := options.IndentSize
	if indentSize == 0 {
		indentSize = 2
	}
	return decode(data, indentSize, options.Strict)
}

func decode(data []byte, indentSize int, strict bool) (any, error) {
	if indentSize < 1 {
		return nil, fmt.Errorf("indent size must be positive")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("TOON input is not valid UTF-8")
	}
	text := string(data)
	text = strings.TrimPrefix(text, "\ufeff")
	lines, err := prepareLines(text, indentSize, strict)
	if err != nil {
		return nil, err
	}
	parser := decoder{lines: lines, strict: strict}
	return parser.parseRoot()
}

func prepareLines(text string, indentSize int, strict bool) ([]decodedLine, error) {
	rawLines := strings.Split(text, "\n")
	lines := make([]decodedLine, 0, len(rawLines))
	blankPending := false
	for index, raw := range rawLines {
		raw = strings.TrimSuffix(raw, "\r")
		raw = strings.TrimRight(raw, " ")
		spaces := 0
		for spaces < len(raw) && raw[spaces] == ' ' {
			spaces++
		}
		if spaces < len(raw) && raw[spaces] == '#' {
			continue
		}
		if strings.Trim(raw, " \t") == "" {
			blankPending = true
			continue
		}
		tabs := 0
		cursor := spaces
		for cursor < len(raw) && raw[cursor] == '\t' {
			tabs++
			cursor++
		}
		if strict && tabs > 0 {
			return nil, fmt.Errorf("line %d: tabs cannot be used for indentation", index+1)
		}
		if strict && spaces%indentSize != 0 {
			return nil, fmt.Errorf("line %d: indentation is not a multiple of %d", index+1, indentSize)
		}
		lines = append(lines, decodedLine{
			depth:       spaces/indentSize + tabs,
			content:     raw[cursor:],
			blankBefore: blankPending,
			number:      index + 1,
		})
		blankPending = false
	}
	return lines, nil
}

func (d decoder) parseRoot() (any, error) {
	if len(d.lines) == 0 {
		return map[string]any{}, nil
	}
	if d.lines[0].depth != 0 {
		return nil, d.lineError(0, "root content must have depth 0")
	}
	first := d.lines[0]
	if first.content == "[]" {
		if d.strict && len(d.lines) != 1 {
			return nil, d.lineError(1, "trailing content after root empty array")
		}
		return []any{}, nil
	}
	parsedHeader, isHeader, err := parseHeader(first.content, d.strict)
	if err != nil {
		return nil, d.lineError(0, err.Error())
	}
	if isHeader && !parsedHeader.hasKey {
		var value any
		var next int
		if parsedHeader.keyed {
			value, next, err = d.parseKeyed(parsedHeader, 0, 1)
		} else {
			value, next, err = d.parseArray(parsedHeader, 0, 1)
		}
		if err != nil {
			return nil, err
		}
		if d.strict && next != len(d.lines) {
			return nil, d.lineError(next, "trailing content after root value")
		}
		return value, nil
	}
	if len(d.lines) == 1 && firstUnquoted(first.content, ':') < 0 {
		return parsePrimitive(first.content)
	}
	value, next, err := d.parseObject(0, 0)
	if err != nil {
		return nil, err
	}
	if next != len(d.lines) {
		return nil, d.lineError(next, "content does not belong to the root object")
	}
	return value, nil
}

func (d decoder) parseObject(depth, start int) (map[string]any, int, error) {
	result := make(map[string]any)
	seen := make(map[string]bool)
	index := start
	for index < len(d.lines) {
		line := d.lines[index]
		if line.depth < depth {
			break
		}
		if line.depth > depth {
			return nil, index, d.lineError(index, "over-indented line does not belong to an open scope")
		}
		parsedHeader, isHeader, err := parseHeader(line.content, d.strict)
		if err != nil {
			return nil, index, d.lineError(index, err.Error())
		}
		if isHeader {
			if !parsedHeader.hasKey {
				return nil, index, d.lineError(index, "keyless header is not valid in an object")
			}
			var value any
			var next int
			if parsedHeader.keyed {
				value, next, err = d.parseKeyed(parsedHeader, depth, index+1)
			} else {
				value, next, err = d.parseArray(parsedHeader, depth, index+1)
			}
			if err != nil {
				return nil, index, err
			}
			if err := d.assign(result, seen, parsedHeader.key, value, index); err != nil {
				return nil, index, err
			}
			index = next
			continue
		}
		keyToken, valueToken, ok := splitKeyValue(line.content)
		if !ok {
			return nil, index, d.lineError(index, "expected a key followed by a colon")
		}
		key, err := parseKey(keyToken)
		if err != nil {
			return nil, index, d.lineError(index, err.Error())
		}
		valueToken = trimSpaces(valueToken)
		if valueToken == "" {
			if index+1 < len(d.lines) && d.lines[index+1].depth > depth {
				if d.strict && d.lines[index+1].depth != depth+1 {
					return nil, index, d.lineError(index+1, "nested scope jumps more than one depth")
				}
				value, next, nestedErr := d.parseObject(depth+1, index+1)
				if nestedErr != nil {
					return nil, index, nestedErr
				}
				if err := d.assign(result, seen, key, value, index); err != nil {
					return nil, index, err
				}
				index = next
				continue
			}
			if err := d.assign(result, seen, key, map[string]any{}, index); err != nil {
				return nil, index, err
			}
			index++
			continue
		}
		var value any
		if valueToken == "[]" {
			value = []any{}
		} else {
			value, err = parsePrimitive(valueToken)
			if err != nil {
				return nil, index, d.lineError(index, err.Error())
			}
		}
		if err := d.assign(result, seen, key, value, index); err != nil {
			return nil, index, err
		}
		index++
		if index < len(d.lines) && d.lines[index].depth > depth {
			if d.strict {
				return nil, index, d.lineError(index, "primitive field cannot open a nested scope")
			}
			for index < len(d.lines) && d.lines[index].depth > depth {
				index++
			}
		}
	}
	return result, index, nil
}

func (d decoder) assign(object map[string]any, seen map[string]bool, key string, value any, line int) error {
	if seen[key] && d.strict {
		return d.lineError(line, fmt.Sprintf("duplicate object key %q", key))
	}
	object[key] = value
	seen[key] = true
	return nil
}

func (d decoder) parseArray(item header, headerDepth, start int) ([]any, int, error) {
	if len(item.fields) > 0 {
		return d.parseTable(item, headerDepth, start)
	}
	if item.inline != "" {
		tokens, err := splitDelimited(item.inline, item.delimiter)
		if err != nil {
			return nil, start, d.lineError(start-1, err.Error())
		}
		values := make([]any, len(tokens))
		for i, token := range tokens {
			values[i], err = parsePrimitive(token)
			if err != nil {
				return nil, start, d.lineError(start-1, err.Error())
			}
		}
		if d.strict && len(values) != item.length {
			return nil, start, d.lineError(start-1, fmt.Sprintf("array declares %d values but contains %d", item.length, len(values)))
		}
		return values, start, nil
	}
	if item.length == 0 {
		return []any{}, start, nil
	}
	values := make([]any, 0, min(item.length, len(d.lines)-start))
	index := start
	itemDepth := headerDepth + 1
	for index < len(d.lines) {
		line := d.lines[index]
		if line.depth <= headerDepth {
			break
		}
		if line.depth != itemDepth || !isListItem(line.content) {
			break
		}
		if d.strict && len(values) > 0 && line.blankBefore {
			return nil, index, d.lineError(index, "blank line inside array scope")
		}
		value, next, err := d.parseListItem(index, itemDepth)
		if err != nil {
			return nil, index, err
		}
		values = append(values, value)
		index = next
	}
	if d.strict && len(values) != item.length {
		return nil, index, d.lineError(start-1, fmt.Sprintf("array declares %d items but contains %d", item.length, len(values)))
	}
	return values, index, nil
}

func (d decoder) parseListItem(index, depth int) (any, int, error) {
	content := d.lines[index].content
	remainder := ""
	if content != "-" {
		remainder = content[2:]
	}
	if remainder == "" {
		return map[string]any{}, index + 1, nil
	}
	if trimSpaces(remainder) == "[]" {
		return []any{}, index + 1, nil
	}
	parsedHeader, isHeader, err := parseHeader(remainder, d.strict)
	if err != nil {
		return nil, index, d.lineError(index, err.Error())
	}
	if isHeader && !parsedHeader.hasKey {
		if parsedHeader.keyed || len(parsedHeader.fields) > 0 {
			return nil, index, d.lineError(index, "keyless fields-bearing header is not valid as a list item")
		}
		return d.parseArray(parsedHeader, depth, index+1)
	}
	if isHeader || firstUnquoted(remainder, ':') >= 0 {
		end := index + 1
		for end < len(d.lines) && d.lines[end].depth > depth {
			end++
		}
		if d.strict {
			for nested := index + 1; nested < end; nested++ {
				if d.lines[nested].blankBefore {
					return nil, index, d.lineError(nested, "blank line inside array item scope")
				}
			}
		}
		subLines := make([]decodedLine, 0, end-index)
		subLines = append(subLines, decodedLine{depth: depth + 1, content: remainder, number: d.lines[index].number})
		subLines = append(subLines, d.lines[index+1:end]...)
		sub := decoder{lines: subLines, strict: d.strict}
		object, next, parseErr := sub.parseObject(depth+1, 0)
		if parseErr != nil {
			return nil, index, parseErr
		}
		if next != len(subLines) {
			return nil, index, fmt.Errorf("line %d: list-item object has unconsumed content", d.lines[index].number)
		}
		return object, end, nil
	}
	value, err := parsePrimitive(remainder)
	if err != nil {
		return nil, index, d.lineError(index, err.Error())
	}
	return value, index + 1, nil
}

func (d decoder) parseTable(item header, headerDepth, start int) ([]any, int, error) {
	leafCount := countLeaves(item.fields)
	rows := make([]any, 0, min(item.length, len(d.lines)-start))
	index := start
	rowDepth := headerDepth + 1
	for index < len(d.lines) {
		line := d.lines[index]
		if line.depth <= headerDepth {
			break
		}
		if line.depth != rowDepth {
			return nil, index, d.lineError(index, "tabular row has invalid indentation")
		}
		colon := firstUnquoted(line.content, ':')
		delim := firstUnquoted(line.content, item.delimiter)
		if colon >= 0 && (delim < 0 || colon < delim) {
			break
		}
		if d.strict && len(rows) > 0 && line.blankBefore {
			return nil, index, d.lineError(index, "blank line inside tabular scope")
		}
		tokens, err := splitDelimited(line.content, item.delimiter)
		if err != nil {
			return nil, index, d.lineError(index, err.Error())
		}
		if d.strict && len(tokens) != leafCount {
			return nil, index, d.lineError(index, fmt.Sprintf("row has %d cells, expected %d", len(tokens), leafCount))
		}
		cells := make([]any, len(tokens))
		for i, token := range tokens {
			cells[i], err = parsePrimitive(token)
			if err != nil {
				return nil, index, d.lineError(index, err.Error())
			}
		}
		rows = append(rows, materialize(item.fields, cells))
		index++
	}
	if d.strict && len(rows) != item.length {
		return nil, index, d.lineError(start-1, fmt.Sprintf("table declares %d rows but contains %d", item.length, len(rows)))
	}
	return rows, index, nil
}

func (d decoder) parseKeyed(item header, headerDepth, start int) (map[string]any, int, error) {
	result := make(map[string]any)
	seen := make(map[string]bool)
	leafCount := countLeaves(item.fields)
	index := start
	entries := 0
	entryDepth := headerDepth + 1
	for index < len(d.lines) {
		line := d.lines[index]
		if line.depth <= headerDepth {
			break
		}
		if line.depth != entryDepth {
			return nil, index, d.lineError(index, "keyed entry has invalid indentation")
		}
		keyToken, cellsToken, ok := splitKeyValue(line.content)
		if !ok {
			if d.strict {
				return nil, index, d.lineError(index, "keyed entry row requires a colon")
			}
			index++
			continue
		}
		if d.strict && entries > 0 && line.blankBefore {
			return nil, index, d.lineError(index, "blank line inside keyed table scope")
		}
		key, err := parseKey(keyToken)
		if err != nil {
			return nil, index, d.lineError(index, err.Error())
		}
		tokens, err := splitDelimited(cellsToken, item.delimiter)
		if err != nil {
			return nil, index, d.lineError(index, err.Error())
		}
		if d.strict && len(tokens) != leafCount {
			return nil, index, d.lineError(index, fmt.Sprintf("entry row has %d cells, expected %d", len(tokens), leafCount))
		}
		cells := make([]any, len(tokens))
		for i, token := range tokens {
			cells[i], err = parsePrimitive(token)
			if err != nil {
				return nil, index, d.lineError(index, err.Error())
			}
		}
		if err := d.assign(result, seen, key, materialize(item.fields, cells), index); err != nil {
			return nil, index, err
		}
		entries++
		index++
	}
	if d.strict && entries != item.length {
		return nil, index, d.lineError(start-1, fmt.Sprintf("keyed table declares %d entries but contains %d", item.length, entries))
	}
	return result, index, nil
}

func materialize(fields []shapeField, cells []any) map[string]any {
	index := 0
	var walk func([]shapeField) map[string]any
	walk = func(items []shapeField) map[string]any {
		result := make(map[string]any)
		for _, item := range items {
			if len(item.children) > 0 {
				result[item.name] = walk(item.children)
				continue
			}
			if index < len(cells) {
				result[item.name] = cells[index]
				index++
			}
		}
		return result
	}
	return walk(fields)
}

func countLeaves(fields []shapeField) int {
	count := 0
	for _, item := range fields {
		if len(item.children) == 0 {
			count++
		} else {
			count += countLeaves(item.children)
		}
	}
	return count
}

func parseHeader(content string, strict bool) (header, bool, error) {
	item, ok, err := parseHeaderSyntax(content, strict)
	if err != nil && !strict {
		return header{}, false, nil
	}
	return item, ok, err
}

func parseHeaderSyntax(content string, strict bool) (header, bool, error) {
	open := firstUnquoted(content, '[')
	if open < 0 {
		return header{}, false, nil
	}
	if colon := firstUnquoted(content, ':'); colon >= 0 && colon < open {
		return header{}, false, nil
	}
	close := firstUnquotedFrom(content, ']', open+1)
	if close < 0 {
		return header{}, false, fmt.Errorf("unterminated header bracket")
	}
	keyToken := content[:open]
	if keyToken != strings.TrimRight(keyToken, " \t") {
		return header{}, false, fmt.Errorf("whitespace between key and header bracket")
	}
	item := header{delimiter: ','}
	if keyToken != "" {
		key, err := parseKey(keyToken)
		if err != nil {
			return header{}, false, err
		}
		item.key, item.hasKey = key, true
	}
	body := content[open+1 : close]
	position := 0
	for position < len(body) && body[position] >= '0' && body[position] <= '9' {
		position++
	}
	if position == 0 || (position > 1 && body[0] == '0') {
		return header{}, false, fmt.Errorf("header length must be a canonical non-negative integer")
	}
	length, err := strconv.Atoi(body[:position])
	if err != nil {
		return header{}, false, fmt.Errorf("header length is out of range")
	}
	item.length = length
	if position < len(body) && body[position] == ':' {
		item.keyed = true
		position++
	}
	if position < len(body) && (body[position] == '\t' || body[position] == '|') {
		item.delimiter = rune(body[position])
		position++
	}
	if position != len(body) {
		return header{}, false, fmt.Errorf("malformed header bracket")
	}
	rest := content[close+1:]
	if strings.HasPrefix(rest, "{") {
		fields, consumed, fieldErr := parseFieldGroup(rest, item.delimiter, strict)
		if fieldErr != nil {
			return header{}, false, fieldErr
		}
		item.fields = fields
		rest = rest[consumed:]
	}
	if !strings.HasPrefix(rest, ":") {
		return header{}, false, fmt.Errorf("header requires a colon")
	}
	item.inline = trimSpaces(rest[1:])
	if len(item.fields) > 0 && item.inline != "" {
		return header{}, false, fmt.Errorf("fields-bearing header cannot contain inline values")
	}
	if item.keyed && len(item.fields) == 0 {
		return header{}, false, fmt.Errorf("keyed header requires a field list")
	}
	return item, true, nil
}

func parseFieldGroup(input string, delimiter rune, strict bool) ([]shapeField, int, error) {
	if input == "" || input[0] != '{' {
		return nil, 0, fmt.Errorf("field group must start with an opening brace")
	}
	end := matchingBrace(input, 0)
	if end < 0 {
		return nil, 0, fmt.Errorf("unmatched field-group brace")
	}
	inner := input[1:end]
	if inner == "" {
		return nil, 0, fmt.Errorf("field group cannot be empty")
	}
	parts, err := splitFieldEntries(inner, delimiter)
	if err != nil {
		return nil, 0, err
	}
	fields := make([]shapeField, 0, len(parts))
	seen := make(map[string]bool)
	for _, part := range parts {
		open := firstUnquoted(part, '{')
		nameToken := part
		var children []shapeField
		if open >= 0 {
			nameToken = part[:open]
			var consumed int
			children, consumed, err = parseFieldGroup(part[open:], delimiter, strict)
			if err != nil {
				return nil, 0, err
			}
			if open+consumed != len(part) {
				return nil, 0, fmt.Errorf("content follows a nested field group")
			}
		}
		name, keyErr := parseKey(nameToken)
		if keyErr != nil {
			return nil, 0, keyErr
		}
		if seen[name] && strict {
			return nil, 0, fmt.Errorf("duplicate field name %q", name)
		}
		seen[name] = true
		fields = append(fields, shapeField{name: name, children: children})
	}
	return fields, end + 1, nil
}

func splitFieldEntries(input string, delimiter rune) ([]string, error) {
	var parts []string
	start, depth := 0, 0
	inQuotes, escaped := false, false
	for index, r := range input {
		if inQuotes {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				inQuotes = false
			}
			continue
		}
		switch r {
		case '"':
			inQuotes = true
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unmatched field-group brace")
			}
		default:
			if depth == 0 && (r == ',' || r == '|' || r == '\t') {
				if r != delimiter {
					return nil, fmt.Errorf("field-list delimiter does not match header delimiter")
				}
				parts = append(parts, input[start:index])
				start = index + utf8.RuneLen(r)
			}
		}
	}
	if inQuotes || depth != 0 {
		return nil, fmt.Errorf("unterminated quoted field or nested group")
	}
	parts = append(parts, input[start:])
	for _, part := range parts {
		if trimSpaces(part) == "" {
			return nil, fmt.Errorf("field name cannot be empty")
		}
	}
	return parts, nil
}

func matchingBrace(input string, start int) int {
	depth := 0
	inQuotes, escaped := false, false
	for index := start; index < len(input); index++ {
		character := input[index]
		if inQuotes {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inQuotes = false
			}
			continue
		}
		switch character {
		case '"':
			inQuotes = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func parsePrimitive(token string) (any, error) {
	token = trimSpaces(token)
	if strings.HasPrefix(token, "\"") {
		return parseQuoted(token)
	}
	switch token {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if decodedNumberPattern.MatchString(token) && !forbiddenLeadingZero(token) {
		canonical, err := canonicalNumber(token)
		if err != nil {
			return nil, err
		}
		return json.Number(canonical), nil
	}
	return token, nil
}

var decodedNumberPattern = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func forbiddenLeadingZero(token string) bool {
	unsigned := strings.TrimPrefix(token, "-")
	integer := unsigned
	if index := strings.IndexAny(unsigned, ".eE"); index >= 0 {
		integer = unsigned[:index]
	}
	return len(integer) > 1 && integer[0] == '0'
}

func parseKey(token string) (string, error) {
	token = trimSpaces(token)
	if strings.HasPrefix(token, "\"") {
		return parseQuoted(token)
	}
	return token, nil
}

func parseQuoted(token string) (string, error) {
	if len(token) < 2 || token[0] != '"' {
		return "", fmt.Errorf("quoted token is unterminated")
	}
	var builder strings.Builder
	for index := 1; index < len(token); {
		character := token[index]
		if character == '"' {
			if index != len(token)-1 {
				return "", fmt.Errorf("characters follow a quoted token")
			}
			return builder.String(), nil
		}
		if character == '\\' {
			if index+1 >= len(token) {
				return "", fmt.Errorf("quoted token ends after a backslash")
			}
			escape := token[index+1]
			switch escape {
			case '\\', '"':
				builder.WriteByte(escape)
				index += 2
			case 'n':
				builder.WriteByte('\n')
				index += 2
			case 'r':
				builder.WriteByte('\r')
				index += 2
			case 't':
				builder.WriteByte('\t')
				index += 2
			case 'u':
				if index+6 > len(token) {
					return "", fmt.Errorf("short Unicode escape")
				}
				value, err := strconv.ParseUint(token[index+2:index+6], 16, 16)
				if err != nil {
					return "", fmt.Errorf("invalid Unicode escape")
				}
				r := rune(value)
				if r >= 0xd800 && r <= 0xdfff {
					return "", fmt.Errorf("surrogate Unicode escape is not valid TOON")
				}
				builder.WriteRune(r)
				index += 6
			default:
				return "", fmt.Errorf("unknown escape \\%c", escape)
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(token[index:])
		if r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("invalid UTF-8 in quoted token")
		}
		if r < 0x20 && r != '\t' {
			return "", fmt.Errorf("unescaped control character in quoted token")
		}
		builder.WriteRune(r)
		index += size
	}
	return "", fmt.Errorf("quoted token is unterminated")
}

func splitDelimited(input string, delimiter rune) ([]string, error) {
	input = trimSpaces(input)
	if input == "" {
		return nil, nil
	}
	var tokens []string
	start := 0
	inQuotes, escaped := false, false
	for index, r := range input {
		if inQuotes {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				inQuotes = false
			}
			continue
		}
		if r == '"' {
			inQuotes = true
		} else if r == delimiter {
			tokens = append(tokens, trimSpaces(input[start:index]))
			start = index + utf8.RuneLen(r)
		}
	}
	if inQuotes {
		return nil, fmt.Errorf("unterminated quoted token")
	}
	tokens = append(tokens, trimSpaces(input[start:]))
	return tokens, nil
}

func splitKeyValue(content string) (string, string, bool) {
	colon := firstUnquoted(content, ':')
	if colon < 0 {
		return "", "", false
	}
	return content[:colon], content[colon+1:], true
}

func isListItem(content string) bool {
	return content == "-" || strings.HasPrefix(content, "- ")
}

func firstUnquoted(input string, target rune) int {
	return firstUnquotedFrom(input, target, 0)
}

func firstUnquotedFrom(input string, target rune, start int) int {
	inQuotes, escaped := false, false
	for index := start; index < len(input); {
		r, size := utf8.DecodeRuneInString(input[index:])
		if inQuotes {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				inQuotes = false
			}
		} else if r == '"' {
			inQuotes = true
		} else if r == target {
			return index
		}
		index += size
	}
	return -1
}

func trimSpaces(value string) string {
	return strings.Trim(value, " ")
}

func (d decoder) lineError(index int, message string) error {
	if index >= 0 && index < len(d.lines) {
		return fmt.Errorf("line %d: %s", d.lines[index].number, message)
	}
	return fmt.Errorf("%s", message)
}
