package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestCacheConcurrencyConfigurationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "version: 1\nenvironments:\n  staging:\n    url: https://tableau.example.com\n    auth:\n      type: pat\n    cache_max_concurrency: 12\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, value); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(roundtrip), "cache_max_concurrency: 12") {
		t.Fatalf("roundtrip=%s", roundtrip)
	}
}

func TestCacheConcurrencyConfigurationBounds(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 32, 256, 257} {
		cfg := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{"staging": {URL: "https://tableau.example.com", Auth: config.Auth{Type: config.AuthTypePAT}, CacheMaxConcurrency: limit}}}
		err := cfg.Validate()
		if valid := limit >= 0 && limit <= 256; (err == nil) != valid {
			t.Fatalf("limit=%d error=%v", limit, err)
		}
	}
}
