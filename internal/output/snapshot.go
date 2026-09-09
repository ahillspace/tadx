package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
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
	redactSnapshot(normalized)
	return json.Marshal(normalized)
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
