package toon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
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
	typ reflect.Type
	ptr uintptr
}

func validateStrings(value reflect.Value, seen map[visit]bool) error {
	if !value.IsValid() {
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
			if err := validateStrings(iter.Key(), seen); err != nil {
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
		key := visit{typ: value.Type(), ptr: value.Pointer()}
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
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() {
				if err := validateStrings(value.Field(i), seen); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func replaceNonFinite(value reflect.Value, seen map[visit]bool) any {
	if !value.IsValid() {
		return nil
	}
	if value.CanInterface() {
		if _, ok := value.Interface().(json.Marshaler); ok {
			return value.Interface()
		}
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
		result := make(orderedJSONObject, 0, value.NumField())
		for index := 0; index < value.NumField(); index++ {
			fieldType := value.Type().Field(index)
			if !fieldType.IsExported() {
				continue
			}
			tag := fieldType.Tag.Get("json")
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			name := parts[0]
			if name == "" {
				name = fieldType.Name
			}
			omitEmpty := false
			for _, option := range parts[1:] {
				if option == "omitempty" || option == "omitzero" {
					omitEmpty = true
				}
			}
			fieldValue := value.Field(index)
			if omitEmpty && fieldValue.IsZero() {
				continue
			}
			result = append(result, orderedJSONField{key: name, value: replaceNonFinite(fieldValue, seen)})
		}
		return result
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice {
			key := visit{typ: value.Type(), ptr: value.Pointer()}
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

type orderedJSONField struct {
	key   string
	value any
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
