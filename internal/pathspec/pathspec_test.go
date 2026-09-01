package pathspec_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/pathspec"
)

func TestIsAbs(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"artifacts/workbook/Finance", false},
		{"./relative", false},
		{"/etc/passwd", true},      // Unix absolute
		{`\\server\share`, true},   // Windows UNC
		{`\windows`, true},         // Windows root-relative
		{`C:\workspace`, true},     // Windows drive absolute
		{"C:/workspace", true},     // Windows drive, forward slashes
		{"c:relative", true},       // Windows drive-relative carries a volume
		{"C-notdrive/path", false}, // not a drive letter
	} {
		if got := pathspec.IsAbs(tc.in); got != tc.want {
			t.Errorf("IsAbs(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestEscapes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"artifacts/workbook/Finance", false},
		{"a/b/../c", false},
		{"..", true},
		{"../secret", true},
		{`..\secret`, true}, // backslash convention normalized
		{"/abs", true},      // absolute escapes a relative base
		{`C:\abs`, true},
	} {
		if got := pathspec.Escapes(tc.in); got != tc.want {
			t.Errorf("Escapes(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
