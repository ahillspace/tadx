package toon

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type nodeKind uint8

const (
	nodeNull nodeKind = iota
	nodeBool
	nodeNumber
	nodeString
	nodeArray
	nodeObject
)

type field struct {
	key   string
	value *node
}

type node struct {
	kind   nodeKind
	value  any
	array  []*node
	object []field
}

func normalize(value any) (*node, error) {
	if err := validateStrings(reflect.ValueOf(value), make(map[visit]bool)); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		var unsupported *json.UnsupportedValueError
		if !errors.As(err, &unsupported) || (unsupported.Value.Kind() != reflect.Float32 && unsupported.Value.Kind() != reflect.Float64) {
			return nil, fmt.Errorf("normalize JSON value: %w", err)
		}
		value = replaceNonFinite(reflect.ValueOf(value), make(map[visit]bool))
		data, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("normalize JSON value: %w", err)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return readJSONNode(decoder)
}

type visit struct {
	typ    reflect.Type
	ptr    uintptr
	length int
}

func marshalerBoundary(value reflect.Value) (any, bool) {
	var direct any
	if value.CanInterface() {
		direct = value.Interface()
	}
	var address any
	if value.Kind() != reflect.Pointer && value.CanAddr() && value.Addr().CanInterface() {
		address = value.Addr().Interface()
	}
	if marshaler, ok := address.(json.Marshaler); ok {
		return marshaler, true
	}
	if marshaler, ok := direct.(json.Marshaler); ok {
		return marshaler, true
	}
	if marshaler, ok := address.(encoding.TextMarshaler); ok {
		return marshaler, true
	}
	if marshaler, ok := direct.(encoding.TextMarshaler); ok {
		return marshaler, true
	}
	return nil, false
}

func validateStrings(value reflect.Value, seen map[visit]bool) error {
	if !value.IsValid() {
		return nil
	}
	if _, ok := marshalerBoundary(value); ok {
		return nil
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return validateStrings(value.Elem(), seen)
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return fmt.Errorf("cyclic value")
		}
		seen[key] = true
		defer delete(seen, key)
		return validateStrings(value.Elem(), seen)
	}
	switch value.Kind() {
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("TOON strings must contain valid UTF-8")
		}
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return fmt.Errorf("cyclic value")
		}
		seen[key] = true
		defer delete(seen, key)
		iter := value.MapRange()
		for iter.Next() {
			if err := validateJSONMapKey(iter.Key()); err != nil {
				return err
			}
			if err := validateStrings(iter.Value(), seen); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		key := visit{typ: value.Type(), ptr: value.Pointer(), length: value.Len()}
		if seen[key] {
			return fmt.Errorf("cyclic value")
		}
		seen[key] = true
		defer delete(seen, key)
		for i := 0; i < value.Len(); i++ {
			if err := validateStrings(value.Index(i), seen); err != nil {
				return err
			}
		}
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := validateStrings(value.Index(i), seen); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for _, field := range jsonStructFields(value.Type()) {
			fieldValue, ok := jsonFieldValue(value, field.index)
			if !ok || jsonFieldOmitted(fieldValue, field) {
				continue
			}
			if err := validateStrings(fieldValue, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateJSONMapKey(value reflect.Value) error {
	if value.Kind() != reflect.String {
		return nil
	}
	if !utf8.ValidString(value.String()) {
		return fmt.Errorf("TOON strings must contain valid UTF-8")
	}
	return nil
}

func replaceNonFinite(value reflect.Value, seen map[visit]bool) any {
	if !value.IsValid() {
		return nil
	}
	if marshaler, ok := marshalerBoundary(value); ok {
		return marshaler
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return replaceNonFinite(value.Elem(), seen)
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return nil
		}
		seen[key] = true
		defer delete(seen, key)
		return replaceNonFinite(value.Elem(), seen)
	}
	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil
		}
		if number == 0 {
			return float64(0)
		}
		return number
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return value.Interface()
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return nil
		}
		seen[key] = true
		defer delete(seen, key)
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		result := make(orderedJSONObject, 0, value.Len())
		for _, key := range keys {
			result = append(result, orderedJSONField{key: key.String(), value: replaceNonFinite(value.MapIndex(key), seen)})
		}
		return result
	case reflect.Struct:
		fields := jsonStructFields(value.Type())
		result := make(orderedJSONObject, 0, len(fields))
		for _, field := range fields {
			fieldValue, ok := jsonFieldValue(value, field.index)
			if !ok || jsonFieldOmitted(fieldValue, field) {
				continue
			}
			replaced := replaceNonFinite(fieldValue, seen)
			if field.quoted && replaced != nil {
				if _, boundary := marshalerBoundary(fieldValue); !boundary {
					replaced = jsonQuotedValue{value: replaced}
				}
			}
			result = append(result, orderedJSONField{key: field.name, value: replaced})
		}
		return result
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice {
			key := visit{typ: value.Type(), ptr: value.Pointer(), length: value.Len()}
			if seen[key] {
				return nil
			}
			seen[key] = true
			defer delete(seen, key)
		}
		result := make([]any, value.Len())
		for i := range result {
			result[i] = replaceNonFinite(value.Index(i), seen)
		}
		return result
	default:
		return value.Interface()
	}
}

type jsonStructField struct {
	name      string
	tagged    bool
	index     []int
	typ       reflect.Type
	omitEmpty bool
	omitZero  bool
	quoted    bool
}

var jsonStructFieldCache sync.Map

func jsonStructFields(root reflect.Type) []jsonStructField {
	if cached, ok := jsonStructFieldCache.Load(root); ok {
		return cached.([]jsonStructField)
	}
	fields := discoverJSONStructFields(root)
	actual, _ := jsonStructFieldCache.LoadOrStore(root, fields)
	return actual.([]jsonStructField)
}

func discoverJSONStructFields(root reflect.Type) []jsonStructField {
	current := []jsonStructField{}
	next := []jsonStructField{{typ: root}}
	var count map[reflect.Type]int
	var nextCount map[reflect.Type]int
	visited := make(map[reflect.Type]bool)
	fields := make([]jsonStructField, 0, root.NumField())

	for len(next) > 0 {
		current, next = next, current[:0]
		count, nextCount = nextCount, make(map[reflect.Type]int)
		for _, parent := range current {
			if visited[parent.typ] {
				continue
			}
			visited[parent.typ] = true
			for i := 0; i < parent.typ.NumField(); i++ {
				structField := parent.typ.Field(i)
				if structField.Anonymous {
					fieldType := structField.Type
					if fieldType.Kind() == reflect.Pointer {
						fieldType = fieldType.Elem()
					}
					if !structField.IsExported() && fieldType.Kind() != reflect.Struct {
						continue
					}
				} else if !structField.IsExported() {
					continue
				}

				tag := structField.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, options, _ := strings.Cut(tag, ",")
				if !isValidJSONTag(name) {
					name = ""
				}
				index := make([]int, len(parent.index)+1)
				copy(index, parent.index)
				index[len(parent.index)] = i

				fieldType := structField.Type
				if fieldType.Name() == "" && fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}
				if name != "" || !structField.Anonymous || fieldType.Kind() != reflect.Struct {
					field := jsonStructField{
						name:      name,
						tagged:    name != "",
						index:     index,
						typ:       fieldType,
						omitEmpty: jsonTagOption(options, "omitempty"),
						omitZero:  jsonTagOption(options, "omitzero"),
						quoted:    jsonTagOption(options, "string") && isJSONQuotedKind(fieldType.Kind()),
					}
					if field.name == "" {
						field.name = structField.Name
					}
					fields = append(fields, field)
					if count[parent.typ] > 1 {
						fields = append(fields, field)
					}
					continue
				}

				nextCount[fieldType]++
				if nextCount[fieldType] == 1 {
					next = append(next, jsonStructField{name: fieldType.Name(), index: index, typ: fieldType})
				}
			}
		}
	}

	sort.Slice(fields, func(i, j int) bool {
		left, right := fields[i], fields[j]
		if left.name != right.name {
			return left.name < right.name
		}
		if len(left.index) != len(right.index) {
			return len(left.index) < len(right.index)
		}
		if left.tagged != right.tagged {
			return left.tagged
		}
		return compareFieldIndex(left.index, right.index) < 0
	})

	selected := fields[:0]
	for i := 0; i < len(fields); {
		end := i + 1
		for end < len(fields) && fields[end].name == fields[i].name {
			end++
		}
		if end-i == 1 || len(fields[i].index) != len(fields[i+1].index) || fields[i].tagged != fields[i+1].tagged {
			selected = append(selected, fields[i])
		}
		i = end
	}

	sort.Slice(selected, func(i, j int) bool {
		return compareFieldIndex(selected[i].index, selected[j].index) < 0
	})
	return selected
}

func isJSONQuotedKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	default:
		return false
	}
}

func compareFieldIndex(left, right []int) int {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func isValidJSONTag(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", character) {
			continue
		}
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func jsonTagOption(options, target string) bool {
	for options != "" {
		var option string
		option, options, _ = strings.Cut(options, ",")
		if option == target {
			return true
		}
	}
	return false
}

func jsonFieldValue(value reflect.Value, index []int) (reflect.Value, bool) {
	for _, fieldIndex := range index {
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		value = value.Field(fieldIndex)
	}
	return value, true
}

func jsonFieldOmitted(value reflect.Value, field jsonStructField) bool {
	return field.omitEmpty && isEmptyJSONValue(value) || field.omitZero && isZeroJSONValue(value)
}

func isEmptyJSONValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return value.IsZero()
	}
	return false
}

type jsonIsZeroer interface {
	IsZero() bool
}

var jsonIsZeroerType = reflect.TypeFor[jsonIsZeroer]()

func isZeroJSONValue(value reflect.Value) bool {
	valueType := value.Type()
	switch {
	case valueType.Kind() == reflect.Interface && valueType.Implements(jsonIsZeroerType):
		return value.IsNil() || value.Elem().Kind() == reflect.Pointer && value.Elem().IsNil() || value.Interface().(jsonIsZeroer).IsZero()
	case valueType.Kind() == reflect.Pointer && valueType.Implements(jsonIsZeroerType):
		return value.IsNil() || value.Interface().(jsonIsZeroer).IsZero()
	case valueType.Implements(jsonIsZeroerType):
		return value.Interface().(jsonIsZeroer).IsZero()
	case reflect.PointerTo(valueType).Implements(jsonIsZeroerType):
		if !value.CanAddr() {
			addressable := reflect.New(valueType).Elem()
			addressable.Set(value)
			value = addressable
		}
		return value.Addr().Interface().(jsonIsZeroer).IsZero()
	default:
		return value.IsZero()
	}
}

type orderedJSONField struct {
	key   string
	value any
}

type jsonQuotedValue struct {
	value any
}

func (value jsonQuotedValue) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(value.value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(encoded))
}

type orderedJSONObject []orderedJSONField

func (object orderedJSONObject) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, field := range object {
		if index > 0 {
			buffer.WriteByte(',')
		}
		key, err := json.Marshal(field.key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
		buffer.Write(key)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func readJSONNode(decoder *json.Decoder) (*node, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case nil:
		return &node{kind: nodeNull}, nil
	case bool:
		return &node{kind: nodeBool, value: value}, nil
	case string:
		return &node{kind: nodeString, value: value}, nil
	case json.Number:
		return &node{kind: nodeNumber, value: value}, nil
	case json.Delim:
		switch value {
		case '[':
			result := &node{kind: nodeArray}
			for decoder.More() {
				child, childErr := readJSONNode(decoder)
				if childErr != nil {
					return nil, childErr
				}
				result.array = append(result.array, child)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return result, nil
		case '{':
			result := &node{kind: nodeObject}
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return nil, keyErr
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				child, childErr := readJSONNode(decoder)
				if childErr != nil {
					return nil, childErr
				}
				result.object = append(result.object, field{key: key, value: child})
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("unsupported JSON token %T", token)
}

func (n *node) primitive() bool {
	return n.kind <= nodeString
}

func (n *node) field(key string) (*node, bool) {
	for _, item := range n.object {
		if item.key == key {
			return item.value, true
		}
	}
	return nil, false
}
