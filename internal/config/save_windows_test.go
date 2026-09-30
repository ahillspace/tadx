package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/config"
)

// Every tadx command reads the configuration, so a parallel command can hold
// it open while another command saves it.
func TestSaveReplacesConfigurationAfterTransientReaderCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	done := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = reader.Close()
		close(done)
	}()
	defer func() { <-done }()

	if err := config.Save(path, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatalf("Save() while a reader held the configuration error = %v", err)
	}
}
