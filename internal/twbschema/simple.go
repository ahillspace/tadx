package twbschema

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// simpleType checks one attribute or text value. A type is either a
// built-in, a restriction of a base type, or a union of member types.
type simpleType struct {
	builtin  string
	base     *simpleType
	members  []*simpleType
	enums    []string
	patterns []*regexp.Regexp
	sources  []string
	minLen   int
	maxLen   int
	minIncl  string
	minExcl  string
	// collapse trims and collapses whitespace before checking, as XSD does
	// for every built-in type except string.
	collapse bool
}

var builtinDescriptions = map[string]string{
	"string":             "",
	"anyURI":             "",
	"anySimpleType":      "",
	"base64Binary":       "base64 data",
	"boolean":            "a boolean (true, false, 1, 0)",
	"int":                "a 32-bit integer",
	"integer":            "an integer",
	"long":               "a 64-bit integer",
	"unsignedInt":        "a non-negative 32-bit integer",
	"unsignedShort":      "a non-negative 16-bit integer",
	"nonNegativeInteger": "a non-negative integer",
	"positiveInteger":    "a positive integer",
	"float":              "a number",
	"double":             "a number",
}

var (
	integerLexical = regexp.MustCompile(`^[+-]?[0-9]+$`)
	floatLexical   = regexp.MustCompile(`^([+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?|-?INF|NaN)$`)
)

func newBuiltin(name string) (*simpleType, error) {
	if _, ok := builtinDescriptions[name]; !ok {
		return nil, fmt.Errorf("built-in type xs:%s is not supported", name)
	}
	return &simpleType{builtin: name, minLen: -1, maxLen: -1, collapse: name != "string" && name != "anySimpleType"}, nil
}

// check returns what the value should have been, or "" when it is valid.
func (t *simpleType) check(value string) string {
	if t.collapse {
		value = strings.Join(strings.Fields(value), " ")
	}
	if t.members != nil {
		var expected []string
		for _, member := range t.members {
			miss := member.check(value)
			if miss == "" {
				return ""
			}
			expected = append(expected, miss)
		}
		return strings.Join(expected, " or ")
	}
	if t.builtin != "" {
		if validBuiltin(t.builtin, value) {
			return ""
		}
		return builtinDescriptions[t.builtin]
	}
	if miss := t.base.check(value); miss != "" {
		return miss
	}
	if t.enums != nil && !t.enumerates(value) {
		return "one of: " + strings.Join(t.enums, ", ")
	}
	for i, pattern := range t.patterns {
		if !pattern.MatchString(value) {
			return "a value matching " + t.sources[i]
		}
	}
	if n := utf8.RuneCountInString(value); t.minLen >= 0 && n < t.minLen {
		return fmt.Sprintf("at least %d characters", t.minLen)
	} else if t.maxLen >= 0 && n > t.maxLen {
		return fmt.Sprintf("at most %d characters", t.maxLen)
	}
	if t.minIncl != "" && compareNumbers(value, t.minIncl) < 0 {
		return "at least " + t.minIncl
	}
	if t.minExcl != "" && compareNumbers(value, t.minExcl) <= 0 {
		return "more than " + t.minExcl
	}
	return ""
}

func validBuiltin(name, value string) bool {
	switch name {
	case "boolean":
		return value == "true" || value == "false" || value == "1" || value == "0"
	case "int":
		_, err := strconv.ParseInt(value, 10, 32)
		return err == nil
	case "long":
		_, err := strconv.ParseInt(value, 10, 64)
		return err == nil
	case "unsignedInt":
		_, err := strconv.ParseUint(strings.TrimPrefix(value, "+"), 10, 32)
		return err == nil
	case "unsignedShort":
		_, err := strconv.ParseUint(strings.TrimPrefix(value, "+"), 10, 16)
		return err == nil
	case "integer":
		return integerLexical.MatchString(value)
	case "nonNegativeInteger", "positiveInteger":
		if !integerLexical.MatchString(value) {
			return false
		}
		zero := canonicalInteger(value) == "0"
		if name == "positiveInteger" {
			return !zero && value[0] != '-'
		}
		return zero || value[0] != '-'
	case "float", "double":
		return floatLexical.MatchString(value)
	case "base64Binary":
		_, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(value, " ", ""))
		return err == nil
	}
	return true
}

// compareNumbers orders two numeric values, exactly when both are integers,
// as every bound in the Tableau schemas is. A value that is not a number
// compares as equal, since its base type already reported it.
func compareNumbers(a, b string) int {
	if integerLexical.MatchString(a) && integerLexical.MatchString(b) {
		return compareIntegers(canonicalInteger(a), canonicalInteger(b))
	}
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	switch {
	case errA != nil || errB != nil || x == y:
		return 0
	case x < y:
		return -1
	}
	return 1
}

// canonicalInteger writes an integer without a plus sign or leading zeros,
// and zero without a sign, in time linear in its length.
func canonicalInteger(value string) string {
	negative := value[0] == '-'
	digits := strings.TrimLeft(strings.TrimLeft(value, "+-"), "0")
	switch {
	case digits == "":
		return "0"
	case negative:
		return "-" + digits
	}
	return digits
}

// compareIntegers orders two canonical integers.
func compareIntegers(a, b string) int {
	negative := a[0] == '-'
	if negative != (b[0] == '-') {
		if negative {
			return -1
		}
		return 1
	}
	order := len(a) - len(b)
	if order == 0 {
		order = strings.Compare(a, b)
	}
	switch {
	case order == 0:
		return 0
	case (order < 0) != negative:
		return -1
	}
	return 1
}

// canonical returns the value as its type reads it, so values that mean the
// same compare equal: 1 and true for a boolean, 01 and 1 for an integer.
func (t *simpleType) canonical(value string) string {
	if t.collapse {
		value = strings.Join(strings.Fields(value), " ")
	}
	builtin := t
	for builtin.base != nil {
		builtin = builtin.base
	}
	switch builtin.builtin {
	case "boolean":
		switch value {
		case "1":
			return "true"
		case "0":
			return "false"
		}
	case "int", "long", "integer", "unsignedInt", "unsignedShort", "nonNegativeInteger", "positiveInteger":
		if integerLexical.MatchString(value) {
			return canonicalInteger(value)
		}
	}
	return value
}

// enumerates reports whether the value equals an enumerated value as the
// type reads both, so +01 matches an xs:int enumeration of 1.
func (t *simpleType) enumerates(value string) bool {
	value = t.canonical(value)
	for _, v := range t.enums {
		if t.canonical(v) == value {
			return true
		}
	}
	return false
}

// simple compiles a named or inline xs:simpleType.
func (c *compiler) simple(n *xsNode) (*simpleType, error) {
	if t, ok := c.simpleTypesDone[n]; ok {
		return t, nil
	}
	t := &simpleType{minLen: -1, maxLen: -1}
	c.simpleTypesDone[n] = t
	for _, kid := range n.kids {
		switch kid.local {
		case "restriction":
			if err := c.restriction(kid, t); err != nil {
				return nil, err
			}
		case "union":
			for _, name := range strings.Fields(kid.attr("memberTypes")) {
				member, err := c.simpleNamed(name)
				if err != nil {
					return nil, err
				}
				t.members = append(t.members, member)
			}
			for _, inline := range kid.kids {
				if inline.local != "simpleType" {
					continue
				}
				member, err := c.simple(inline)
				if err != nil {
					return nil, err
				}
				t.members = append(t.members, member)
			}
		case "list":
			return nil, fmt.Errorf("simpleType list is not supported")
		}
	}
	if t.base == nil && t.members == nil {
		return nil, fmt.Errorf("simpleType %q has no restriction or union", n.attr("name"))
	}
	return t, nil
}

func (c *compiler) restriction(n *xsNode, t *simpleType) error {
	var err error
	if base := n.attr("base"); base != "" {
		t.base, err = c.simpleNamed(base)
	} else {
		for _, kid := range n.kids {
			if kid.local == "simpleType" {
				t.base, err = c.simple(kid)
			}
		}
	}
	if err != nil {
		return err
	}
	if t.base == nil {
		return fmt.Errorf("restriction has no base type")
	}
	t.collapse = t.base.collapse
	for _, facet := range n.kids {
		value := facet.attr("value")
		switch facet.local {
		case "enumeration":
			t.enums = append(t.enums, value)
		case "pattern":
			pattern, err := regexp.Compile(`^(?:` + translatePattern(value) + `)$`)
			if err != nil {
				return fmt.Errorf("pattern %q: %w", value, err)
			}
			t.patterns = append(t.patterns, pattern)
			t.sources = append(t.sources, value)
		case "minLength", "maxLength":
			bound, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s %q: %w", facet.local, value, err)
			}
			if facet.local == "minLength" {
				t.minLen = bound
			} else {
				t.maxLen = bound
			}
		case "minInclusive":
			t.minIncl = value
		case "minExclusive":
			t.minExcl = value
		case "simpleType", "annotation":
		default:
			return fmt.Errorf("facet %s is not supported", facet.local)
		}
	}
	return nil
}

// translatePattern rewrites an XSD pattern in Go's syntax. The two differ
// only in \d, which is any Unicode decimal digit in XSD and ASCII in Go;
// the Tableau schemas use no other escape the two read differently.
func translatePattern(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			if pattern[i+1] == 'd' {
				b.WriteString(`\p{Nd}`)
			} else {
				b.WriteString(pattern[i : i+2])
			}
			i++
			continue
		}
		b.WriteByte(pattern[i])
	}
	return b.String()
}

func (c *compiler) simpleNamed(name string) (*simpleType, error) {
	if builtin, ok := strings.CutPrefix(name, "xs:"); ok {
		return newBuiltin(builtin)
	}
	n := c.simpleTypes[name]
	if n == nil {
		return nil, fmt.Errorf("unknown simple type %q", name)
	}
	return c.simple(n)
}

// describe says what values the type takes, in the words check uses.
func (t *simpleType) describe() string {
	if t.members != nil {
		parts := make([]string, len(t.members))
		for i, member := range t.members {
			parts[i] = member.describe()
		}
		return strings.Join(parts, " or ")
	}
	if t.builtin != "" {
		if d := builtinDescriptions[t.builtin]; d != "" {
			return d
		}
		if t.builtin == "anyURI" {
			return "a URI"
		}
		return "text"
	}
	if t.enums != nil {
		return "one of: " + strings.Join(t.enums, ", ")
	}
	if len(t.sources) > 0 {
		return "a value matching " + strings.Join(t.sources, " and ")
	}
	base := t.base.describe()
	if t.minLen >= 0 {
		base += fmt.Sprintf(", at least %d characters", t.minLen)
	}
	if t.maxLen >= 0 {
		base += fmt.Sprintf(", at most %d characters", t.maxLen)
	}
	if t.minIncl != "" {
		base += ", at least " + t.minIncl
	}
	if t.minExcl != "" {
		base += ", more than " + t.minExcl
	}
	return base
}
