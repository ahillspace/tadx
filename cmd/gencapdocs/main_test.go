package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateMapPreservesAuthoredPage(t *testing.T) {
	const prefix = "<!doctype html>\r\n<style>authored</style>\r\n<script id=\"capability-data\" type=\"application/json\">"
	const suffix = "</script>\r\n<script>authoredBehavior()</script>"
	path := filepath.Join(t.TempDir(), "map.html")
	if err := os.WriteFile(path, []byte(prefix+"\n[{}]\n"+suffix), 0o600); err != nil {
		t.Fatal(err)
	}
	data := []byte("[{\"id\":\"catalog.audit\"}]\n")
	for range 2 {
		if err := updateMap(path, data); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := prefix + "\n" + strings.TrimSpace(string(data)) + "\n" + suffix
		if string(got) != want {
			t.Fatalf("page outside snapshot changed or update was not idempotent: %s", got)
		}
	}
}

func TestUpdateMapRejectsMalformedPageWithoutWriting(t *testing.T) {
	const marker = `<script id="capability-data" type="application/json">`
	for _, content := range []string{"<html></html>", marker + "[]", marker + "[]</script>" + marker + "[]</script>"} {
		path := filepath.Join(t.TempDir(), "map.html")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := updateMap(path, []byte("[]")); err == nil {
			t.Fatal("expected malformed page rejection")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("malformed page changed: %q, %v", got, err)
		}
	}
}
