package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type boundedSnapshot struct {
	bytes.Buffer
	max int
}

func (b *boundedSnapshot) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, errors.New("expanded result exceeds saved-output size limit")
	}
	return b.Buffer.Write(p)
}

// Snapshot expands without invoking the action, redacts credential-bearing keys,
// and bounds serialization before allocating an additional normalized copy.
func Snapshot(value any, maxBytes int) (json.RawMessage, error) {
	return SnapshotWithConfig(value, maxBytes, "")
}

// SnapshotWithConfig preserves the command context used when a result is saved.
// Only recovery hints receive the config binding; result fields and raw diagnostics remain unchanged.
func SnapshotWithConfig(value any, maxBytes int, configPath string) (json.RawMessage, error) {
	return SnapshotWithOptions(value, maxBytes, Options{ConfigPath: configPath})
}

// SnapshotWithOptions captures expanded result metadata and redacts secrets.
// The serialized size bound includes the result and its shared diagnostics.
func SnapshotWithOptions(value any, maxBytes int, options Options) (json.RawMessage, error) {
	if err, ok := value.(error); ok {
		var carrier interface{ OperationOutput() any }
		if errors.As(err, &carrier) {
			v := carrier.OperationOutput()
			if p, ok := v.(FullProjector); ok {
				v = p.FullOutput()
			}
			value = struct {
				Output any          `json:"output"`
				Error  errs.Payload `json:"error"`
			}{v, errs.Structure(err).Error}
		} else {
			value = errs.Structure(err)
		}
	} else if p, ok := value.(FullProjector); ok {
		value = p.FullOutput()
	}
	value = bindHintValue(value, options.ConfigPath)
	b := &boundedSnapshot{max: maxBytes}
	if err := json.NewEncoder(b).Encode(value); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b.Bytes()))
	dec.UseNumber()
	var normalized any
	if err := dec.Decode(&normalized); err != nil {
		return nil, err
	}
	if len(options.Metadata) > 0 {
		b.Reset()
		if err := json.NewEncoder(b).Encode(options.Metadata); err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(b.Bytes()))
		decoder.UseNumber()
		var metadata map[string]any
		if err := decoder.Decode(&metadata); err != nil {
			return nil, err
		}
		normalized = mergeMetadata(normalized, metadata)
	}
	redactSnapshot(normalized)
	if len(options.Secrets) > 0 {
		normalized = transform(normalized, newRedactor(options.Secrets), 0, true)
	}
	b.Reset()
	if err := json.NewEncoder(b).Encode(normalized); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(bytes.Clone(b.Bytes()), []byte("\n")), nil
}

func redactSnapshot(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, item := range x {
			key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(k, "_", ""), "-", ""))
			switch key {
			case "patsecret", "patname", "password", "token", "sessiontoken", "accesstoken", "refreshtoken", "authorization", "credentials":
				x[k] = Redacted
			default:
				redactSnapshot(item)
			}
		}
	case []any:
		for _, item := range x {
			redactSnapshot(item)
		}
	}
}
