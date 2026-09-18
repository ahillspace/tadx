package app

import (
	"strings"
	"testing"
)

func TestNoWaitIsScopedToNativePublishAndDownload(t *testing.T) {
	for _, test := range []struct {
		path []string
		want bool
	}{
		{[]string{"content", "workbook", "publish"}, true},
		{[]string{"content", "datasource", "publish"}, true},
		{[]string{"content", "flow", "publish"}, true},
		{[]string{"search"}, false},
		{[]string{"pulse", "definition", "publish"}, false},
		{[]string{"content", "workbook", "pull"}, true},
		{[]string{"content", "datasource", "pull"}, true},
		{[]string{"content", "flow", "pull"}, true},
		{[]string{"pulse", "definition", "pull"}, false},
	} {
		var out strings.Builder
		args := append(test.path, "--help")
		if code := Run(t.Context(), args, &out, Options{}); code != 0 {
			t.Fatalf("%v: code=%d output=%s", test.path, code, out.String())
		}
		if got := strings.Contains(out.String(), "--no-wait"); got != test.want {
			t.Errorf("%v no-wait=%v want=%v", test.path, got, test.want)
		}
	}
}
