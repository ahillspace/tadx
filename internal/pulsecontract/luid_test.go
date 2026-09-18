package pulsecontract_test

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/pulsecontract"
)

func TestValidateLUIDShapePreservesOpaqueValuesAndRejectsMalformedTokens(t *testing.T) {
	for _, value := range []string{"metric-1", "4f9a6d2e-opaque", "value_with.provider"} {
		if err := pulsecontract.ValidateLUIDShape("metric", value); err != nil {
			t.Fatalf("value %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "metric with spaces", strings.Repeat("x", 256)} {
		if err := pulsecontract.ValidateLUIDShape("metric", value); err == nil || !strings.Contains(err.Error(), value) {
			t.Fatalf("value %q error=%v", value, err)
		}
	}
}
