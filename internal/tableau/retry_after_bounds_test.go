package tableau

import (
	"math"
	"net/http"
	"testing"
	"time"
)

func TestRetryAfterSecondsCannotOverflowDuration(t *testing.T) {
	for _, value := range []string{"9223372037", "9223372036854775807", "18446744073709551616", "999999999999999999999999999999999999999"} {
		header := make(http.Header)
		header.Set("Retry-After", value)
		got, ok := parseRetryAfter(header)
		if !ok || got != time.Duration(math.MaxInt64) {
			t.Fatalf("header=%q delay=%s valid=%v", value, got, ok)
		}
	}
}
