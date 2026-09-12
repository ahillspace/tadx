package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PreviewUpdate applies the normal configuration mutator to an isolated copy.
// It performs the same load, migration, validation, and encoding checks without
// acquiring a lock, creating directories, saving migrations, or writing a file.
func PreviewUpdate(path string, createIfMissing bool, mutate func(Config) (Config, error)) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, errors.New("configuration path is required")
	}
	current, _, _, err := load(path)
	if err != nil {
		if !(createIfMissing && errors.Is(err, os.ErrNotExist)) {
			return Config{}, err
		}
		current = Config{Version: CurrentVersion}
	}
	next, err := mutate(cloneConfig(current))
	if errors.Is(err, ErrNoChange) {
		return current, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := next.Validate(); err != nil {
		return Config{}, err
	}
	data, err := yaml.Marshal(next)
	if err != nil {
		return Config{}, fmt.Errorf("encode configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return Config{}, fmt.Errorf("configuration exceeds %d bytes", maxConfigBytes)
	}
	return next, nil
}
