// Package output renders redacted TADX results through the TOON output layer.
package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/toon"
)

const (
	// Redacted replaces every configured secret before rendering.
	Redacted = "[REDACTED]"
	// DefaultMaxStringLength bounds individual string fields in compact output.
	DefaultMaxStringLength = 4096
)

// Options configures result rendering.
type Options struct {
	Full            bool
	Raw             bool
	RawCapable      bool
	MaxStringLength int
	Secrets         []string
	TOON            toon.EncodeOptions
	ConfigPath      string
}

// CompactProjector provides an explicit token-bounded default view.
type CompactProjector interface {
	CompactOutput() any
}

// FullProjector provides an explicit bounded expanded view.
type FullProjector interface {
	FullOutput() any
}

// Render writes compact TOON with the default bounds.
func Render(writer io.Writer, value any) error {
	return RenderWithOptions(writer, value, Options{})
}

// RenderWithOptions redacts, bounds, and renders one result document.
func RenderWithOptions(writer io.Writer, value any, options Options) error {
	if writer == nil {
		return fmt.Errorf("output writer is nil")
	}
	if options.Raw && !options.RawCapable {
		return errs.New(errs.KindUsage, "raw output is not supported for this capability")
	}
	if options.Raw {
		return renderRaw(writer, value, options)
	}
	if options.Full {
		if projector, ok := value.(FullProjector); ok {
			value = projector.FullOutput()
		}
	} else {
		if projector, ok := value.(CompactProjector); ok {
			value = projector.CompactOutput()
		}
	}
	limit := options.MaxStringLength
	if limit == 0 {
		limit = DefaultMaxStringLength
	}
	if options.ConfigPath != "" {
		value = bindHintValue(value, options.ConfigPath)
	}
	if len(options.Secrets) == 0 && (options.Full || !hasLongString(reflect.ValueOf(value), limit, make(map[visit]bool))) {
		encoded, err := toon.EncodeWithOptions(value, options.TOON)
		if err != nil {
			return fmt.Errorf("render TOON: %w", err)
		}
		return writeDocument(writer, encoded)
	}
	normalized, err := normalize(value)
	if err != nil {
		return err
	}
	redactor := newRedactor(options.Secrets)
	normalized = transform(normalized, redactor, limit, options.Full)
	encoded, err := toon.EncodeWithOptions(normalized, options.TOON)
	if err != nil {
		return fmt.Errorf("render TOON: %w", err)
	}
	return writeDocument(writer, encoded)
}

func bindHintValue(value any, configPath string) any {
	if value == nil || configPath == "" {
		return value
	}
	cloned := bindHintReflect(reflect.ValueOf(value), configPath, make(map[visit]bool), 0)
	if cloned.IsValid() && cloned.CanInterface() {
		return cloned.Interface()
	}
	return value
}

const maxHintDepth = 64

func bindHintReflect(value reflect.Value, configPath string, seen map[visit]bool, depth int) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if depth >= maxHintDepth {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		cloned := bindHintReflect(value.Elem(), configPath, seen, depth+1)
		result := reflect.New(value.Type()).Elem()
		result.Set(cloned)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return value
		}
		seen[key] = true
		defer delete(seen, key)
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(bindHintReflect(value.Elem(), configPath, seen, depth+1))
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for index := 0; index < value.NumField(); index++ {
			field := result.Field(index)
			if !field.CanSet() || value.Type().Field(index).PkgPath != "" {
				continue
			}
			name := value.Type().Field(index).Name
			jsonName := strings.Split(value.Type().Field(index).Tag.Get("json"), ",")[0]
			if name == "CorrectiveAction" || jsonName == "corrective_action" {
				if field.Kind() == reflect.String {
					field.SetString(commandhint.BindConfig(field.String(), configPath))
					continue
				}
			}
			if name == "Help" || jsonName == "help" {
				if field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String {
					bound := reflect.MakeSlice(field.Type(), field.Len(), field.Len())
					reflect.Copy(bound, field)
					field.Set(bound)
					for item := 0; item < field.Len(); item++ {
						field.Index(item).SetString(commandhint.BindConfig(field.Index(item).String(), configPath))
					}
					continue
				}
			}
			field.Set(bindHintReflect(field, configPath, seen, depth+1))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		if value.Type().Elem().Kind() != reflect.Struct && value.Type().Elem().Kind() != reflect.Interface && value.Type().Elem().Kind() != reflect.Pointer && value.Type().Elem().Kind() != reflect.Slice {
			return value
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return value
		}
		seen[key] = true
		defer delete(seen, key)
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(bindHintReflect(value.Index(index), configPath, seen, depth+1))
		}
		return result
	default:
		return value
	}
}

// RenderError writes a structured error document through the same renderer.
func RenderError(writer io.Writer, err error, options Options) error {
	var carrier interface{ OperationOutput() any }
	if errors.As(err, &carrier) {
		value := carrier.OperationOutput()
		if options.Full {
			if projector, ok := value.(FullProjector); ok {
				value = projector.FullOutput()
			}
		} else if projector, ok := value.(CompactProjector); ok {
			value = projector.CompactOutput()
		}
		return RenderWithOptions(writer, struct {
			Output any          `json:"output"`
			Error  errs.Payload `json:"error"`
		}{value, errs.Structure(err).Error}, options)
	}
	return RenderWithOptions(writer, errs.Structure(err), options)
}

func renderRaw(writer io.Writer, value any, options Options) error {
	redactor := newRedactor(options.Secrets)
	limit := options.MaxStringLength
	if limit == 0 {
		limit = DefaultMaxStringLength
	}
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = append([]byte(nil), typed...)
	case string:
		raw = []byte(typed)
	default:
		normalized, err := normalize(value)
		if err != nil {
			return err
		}
		normalized = transform(normalized, redactor, limit, options.Full)
		raw, err = json.Marshal(normalized)
		if err != nil {
			return fmt.Errorf("render raw JSON: %w", err)
		}
		return writeDocument(writer, raw)
	}
	text := redactor(string(raw))
	if !options.Full && limit > 0 {
		text = truncate(text, limit)
	}
	return writeDocument(writer, []byte(text))
}

func writeDocument(writer io.Writer, document []byte) error {
	document = bytes.TrimRight(document, "\r\n")
	document = append(document, '\n')
	_, err := writer.Write(document)
	return err
}

func normalize(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize output: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("normalize output: %w", err)
	}
	return normalized, nil
}

func newRedactor(secrets []string) func(string) string {
	filtered := make([]string, 0, len(secrets))
	seen := make(map[string]bool)
	for _, secret := range secrets {
		if secret != "" && !seen[secret] {
			filtered = append(filtered, secret)
			seen[secret] = true
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return len(filtered[i]) > len(filtered[j]) })
	return func(value string) string {
		for _, secret := range filtered {
			value = strings.ReplaceAll(value, secret, Redacted)
		}
		return value
	}
}

func transform(value any, redact func(string) string, limit int, full bool) any {
	switch typed := value.(type) {
	case string:
		result := redact(typed)
		if !full && limit > 0 {
			result = truncate(result, limit)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = transform(item, redact, limit, full)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[redact(key)] = transform(item, redact, limit, full)
		}
		return result
	default:
		return value
	}
}

func truncate(value string, limit int) string {
	count := utf8.RuneCountInString(value)
	if count <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + fmt.Sprintf(" (truncated, %d chars total - use --full to see complete body)", count)
}

type visit struct {
	typ reflect.Type
	ptr uintptr
}

func hasLongString(value reflect.Value, limit int, seen map[visit]bool) bool {
	if limit <= 0 || !value.IsValid() {
		return false
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		if value.Kind() == reflect.Pointer {
			key := visit{typ: value.Type(), ptr: value.Pointer()}
			if seen[key] {
				return false
			}
			seen[key] = true
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		return utf8.RuneCountInString(value.String()) > limit
	case reflect.Map:
		if value.IsNil() {
			return false
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return false
		}
		seen[key] = true
		iterator := value.MapRange()
		for iterator.Next() {
			if hasLongString(iterator.Value(), limit, seen) {
				return true
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return false
		}
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if seen[key] {
			return false
		}
		seen[key] = true
		fallthrough
	case reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if hasLongString(value.Index(index), limit, seen) {
				return true
			}
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if value.Type().Field(index).IsExported() && hasLongString(value.Field(index), limit, seen) {
				return true
			}
		}
	}
	return false
}
