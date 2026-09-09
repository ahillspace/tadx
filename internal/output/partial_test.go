package output_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/output"
)

type outputCarrier struct {
	value any
	cause error
}

func (e outputCarrier) Error() string        { return e.cause.Error() }
func (e outputCarrier) Unwrap() error        { return e.cause }
func (e outputCarrier) OperationOutput() any { return e.value }

func TestPartialErrorProjectionRetainsBoundsAndRedaction(t *testing.T) {
	value := projectableResult{Status: "created", Secret: "secret-value"}
	err := fmt.Errorf("operation: %w", outputCarrier{value, errors.New("inspection failed: secret-value")})
	for _, full := range []bool{false, true} {
		var buffer bytes.Buffer
		if err := output.RenderError(&buffer, err, output.Options{Full: full, Secrets: []string{"secret-value"}}); err != nil {
			t.Fatal(err)
		}
		text := buffer.String()
		if strings.Contains(text, "secret-value") || !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, "status: created") {
			t.Fatalf("lost result or redaction: %s", text)
		}
		if strings.Count(text, "error:") != 1 || !strings.Contains(text, "output:") {
			t.Fatalf("not one combined document: %s", text)
		}
		if strings.Contains(text, "secret:") != full {
			t.Fatalf("projection wrong: %s", text)
		}
	}
}
