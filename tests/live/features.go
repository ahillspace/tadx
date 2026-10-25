package live

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

var restFeatureRequirement = regexp.MustCompile(`(?i)REST API\s*>=\s*([0-9]+\.[0-9]+)`)

// VerifyTargetFeatures checks only service prerequisites of the selected cases.
// A successful probe proves service access, not content indexing or mutation rights.
func VerifyTargetFeatures(ctx context.Context, api *harness.Client, profile harness.Profile, specs []Spec) error {
	metadata, pulse, labels := false, false, false
	minimum := ""
	for _, spec := range specs {
		for _, requirement := range spec.Requires {
			for _, match := range restFeatureRequirement.FindAllStringSubmatch(requirement, -1) {
				if minimum == "" {
					minimum = match[1]
				} else {
					newer, err := featureVersionAtLeast(match[1], minimum)
					if err != nil {
						return err
					}
					if newer {
						minimum = match[1]
					}
				}
			}
			value := strings.ToLower(requirement)
			if strings.Contains(value, "metadata") || value == "catalog-read" || value == "catalog-write" {
				metadata = true
			}
			if value == "pulse" {
				pulse = true
			}
			if value == "labels-api" || value == "labels" {
				labels = true
			}
		}
	}
	if minimum == "" && !metadata && !pulse && !labels {
		return nil
	}
	if api == nil {
		return errors.New("selected target feature checks require an authenticated independent API client")
	}
	if minimum != "" {
		supported, err := featureVersionAtLeast(api.Version, minimum)
		if err != nil {
			return err
		}
		if !supported {
			return harness.Unsupported("selected tests require REST API " + minimum + " or later; target negotiated " + api.Version)
		}
	}
	seconds := profile.Budgets.ObservationSeconds
	if seconds <= 0 {
		seconds = 30
	}
	seconds = min(seconds, 30)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	if metadata {
		document, err := api.JSON(ctx, http.MethodPost, "/api/metadata/graphql", map[string]any{"query": "query TadxLiveFeatureProbe { __typename }"})
		if err != nil {
			return featureProbeError("Metadata API", err)
		}
		if problems, exists := document["errors"]; exists && problems != nil {
			list, ok := problems.([]any)
			if !ok || len(list) > 0 {
				return errors.New("Metadata API feature probe returned GraphQL errors")
			}
		}
		data, ok := document["data"].(map[string]any)
		if !ok {
			return errors.New("Metadata API feature probe omitted GraphQL data")
		}
		typename, ok := data["__typename"].(string)
		if !ok || typename == "" {
			return errors.New("Metadata API feature probe omitted its root type")
		}
	}
	if pulse {
		document, err := api.JSON(ctx, http.MethodGet, "/api/-/pulse/definitions?page_size=1", nil)
		if err != nil {
			return featureProbeError("Tableau Pulse", err)
		}
		if _, ok := document["definitions"].([]any); !ok {
			return errors.New("Tableau Pulse feature probe omitted the definitions collection")
		}
	}
	if labels {
		document, err := api.JSON(ctx, http.MethodGet, api.REST("labelCategories"), nil)
		if err != nil {
			return featureProbeError("label categories API", err)
		}
		if _, ok := document["labelCategoryList"].(map[string]any); !ok {
			if _, ok := document["labelCategories"].(map[string]any); !ok {
				return errors.New("label categories feature probe omitted its collection")
			}
		}
	}
	return nil
}

func featureProbeError(service string, err error) error {
	if harness.IsNotFound(err) {
		return harness.Unsupported("selected tests require " + service + "; the authenticated target does not expose its endpoint")
	}
	return fmt.Errorf("verify selected %s prerequisite: %w", service, err)
}

func featureVersionAtLeast(actual, minimum string) (bool, error) {
	parse := func(value string) (int, int, error) {
		major, minor, ok := strings.Cut(value, ".")
		if !ok {
			return 0, 0, errors.New("target feature check requires a numeric REST major.minor version")
		}
		a, errA := strconv.Atoi(major)
		b, errB := strconv.Atoi(minor)
		if errA != nil || errB != nil || a < 0 || b < 0 {
			return 0, 0, errors.New("target feature check requires a numeric REST major.minor version")
		}
		return a, b, nil
	}
	major, minor, err := parse(actual)
	if err != nil {
		return false, err
	}
	wantMajor, wantMinor, err := parse(minimum)
	if err != nil {
		return false, err
	}
	return major > wantMajor || (major == wantMajor && minor >= wantMinor), nil
}
