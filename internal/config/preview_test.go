package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewUpdateDoesNotCreateConfigurationDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	result, err := PreviewUpdate(filepath.Join(root, "config.yaml"), true, func(cfg Config) (Config, error) {
		cfg.Environments = map[string]Environment{"test": {URL: "https://tableau.example.test", Auth: Auth{Type: AuthTypePAT}}}
		return cfg, nil
	})
	if err != nil || len(result.Environments) != 1 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("preview created directory: %v", err)
	}
}

func TestPreviewUpdateRejectsInvalidEffectiveConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	_, err := PreviewUpdate(path, true, func(cfg Config) (Config, error) {
		cfg.Environments = map[string]Environment{"test": {URL: "https://tableau.example.test", Auth: Auth{Type: AuthTypePAT, PATNameEnv: "SAME", PATSecretEnv: "SAME"}}}
		return cfg, nil
	})
	if err == nil {
		t.Fatal("invalid effective configuration accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed preview wrote configuration: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("failed preview created lock: %v", err)
	}
}
