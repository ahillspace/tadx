package version_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	version "github.com/ahillspace/tadx/internal/version"
)

func TestCheckerReadsBoundedRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://github.com/ahillspace/tadx/releases/tag/v1.2.3","published_at":"2026-09-05T00:00:00Z","extra":true}`))
	}))
	defer server.Close()
	release, err := (version.Checker{Client: server.Client(), URL: server.URL}).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.2.3" {
		t.Fatalf("release = %#v", release)
	}
}

func TestCheckerRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for range 70 * 1024 {
			_, _ = writer.Write([]byte("x"))
		}
	}))
	defer server.Close()
	if _, err := (version.Checker{Client: server.Client(), URL: server.URL}).Latest(context.Background()); err == nil {
		t.Fatal("accepted oversized release response")
	}
}
