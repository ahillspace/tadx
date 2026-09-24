// Package version resolves local build identity and bounded release metadata.
package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

const (
	defaultReleaseURL = "https://api.github.com/repos/ahillspace/tadx/releases/latest"
	maxReleaseBytes   = 64 * 1024
)

// BuildVersion can be set through -ldflags for release builds.
var BuildVersion = ""

// Current returns the release version or a stable development marker.
func Current() string {
	if value := normalize(BuildVersion); value != "" {
		return value
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if value := normalize(info.Main.Version); value != "" && value != "(devel)" {
			return value
		}
	}
	return "dev"
}

// Modified reports whether Go marked the executable's source tree dirty.
// Release binaries without VCS build settings are treated as clean.
func Modified() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.modified" {
			return setting.Value == "true"
		}
	}
	return false
}

type Release struct {
	Version     string
	URL         string
	PublishedAt time.Time
}

type Checker struct {
	Client *http.Client
	URL    string
}

func (c Checker) Latest(ctx context.Context) (Release, error) {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := c.URL
	if endpoint == "" {
		endpoint = defaultReleaseURL
	}
	requestContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseBytes+1))
	if err != nil {
		return Release{}, err
	}
	if len(body) > maxReleaseBytes {
		return Release{}, errors.New("release response exceeds its byte limit")
	}
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("release request returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		// GitHub adds fields over time, so decode a bounded response permissively.
		if err := json.Unmarshal(body, &payload); err != nil {
			return Release{}, errors.New("release response is invalid")
		}
	}
	version := normalize(payload.TagName)
	if version == "" || !strings.HasPrefix(payload.HTMLURL, "https://github.com/ahillspace/tadx/releases/") {
		return Release{}, errors.New("release response omitted trusted identity")
	}
	return Release{Version: version, URL: payload.HTMLURL, PublishedAt: payload.PublishedAt}, nil
}

func normalize(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "v")
}
