package app

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCategoryHelpExplainsDescendantCommandsWithoutSetup(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"admin", "group", "--help"}, []string{"member add", "member remove", "--group-id", "--username", "--minimum-site-role", "--batch-file", "batch:"}},
		{[]string{"admin", "--help"}, []string{"group create", "user create", "permission create", "--site-role", "--principal-type"}},
		{[]string{"content", "workbook", "--help"}, []string{"publish", "pull", "--include-pds", "--file", "--destination-project-id", "--preview"}},
		{[]string{"pulse", "--help"}, []string{"definition create", "metric fork", "--aggregation", "COUNT_DISTINCT", "--filter", "repeatable", "CUSTOM_N_DAYS"}},
		{[]string{"catalog", "--help"}, []string{"column update", "database inspect", "--table-id", "--metadata-id", "--add-tag"}},
		{[]string{"workspace", "--help"}, []string{"artifact", "--workspace", "--path"}},
		{[]string{"help", "admin", "group"}, []string{"member add", "--minimum-site-role"}},
		{[]string{"adm", "grp", "-h"}, []string{"member add", "--minimum-site-role"}},
	} {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			root := t.TempDir()
			options := overviewOptions(t, root)
			// Help must work even when configured local state is unreadable as YAML.
			if err := os.WriteFile(options.ConfigPath, []byte("invalid: ["), 0600); err != nil {
				t.Fatal(err)
			}
			code, output := runOverview(t, root, tc.args, options)
			if code != 0 {
				t.Fatalf("help failed: %s", output)
			}
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Errorf("category help missing %q", want)
				}
			}
			if strings.Contains(output, options.ConfigPath) {
				t.Error("help exposed configured filesystem location")
			}
		})
	}
}

func TestCategoryHelpDoesNotRenderSuppliedValuesOrChangeWithFull(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	var outputs []string
	for _, extra := range [][]string{nil, {"--full"}, {"--json"}} {
		args := append([]string{"admin", "group", "create", "--name", "private-input-never-render", "--env", "private-environment-never-render", "--help"}, extra...)
		before := overviewSnapshot(t, root)
		var out strings.Builder
		if code := Run(context.Background(), args, &out, options); code != 0 {
			t.Fatalf("help failed: %s", out.String())
		}
		if strings.Contains(out.String(), "never-render") {
			t.Fatal("help exposed supplied arguments")
		}
		if after := overviewSnapshot(t, root); !reflect.DeepEqual(before, after) {
			t.Fatal("help changed local state")
		}
		outputs = append(outputs, out.String())
	}
	if outputs[0] != outputs[1] || outputs[0] != outputs[2] {
		t.Fatal("presentation flags altered syntax help")
	}
}
