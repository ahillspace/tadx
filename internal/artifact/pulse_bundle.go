package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const MaxPulseBundleBytes = 32 * 1024 * 1024

// PulseBundle is a portable saved definition and its complete metric inventory.
// It intentionally excludes users, subscriptions, values, and generated insights.
type PulseBundle struct {
	Version              int                 `json:"version"`
	SourceServerOrigin   string              `json:"source_server_origin"`
	SourceSiteLUID       string              `json:"source_site_luid"`
	DefinitionLUID       string              `json:"definition_luid"`
	DatasourceReferences []string            `json:"datasource_references"`
	Definition           json.RawMessage     `json:"definition"`
	Metrics              []PulseBundleMetric `json:"metrics"`
}

type PulseBundleMetric struct {
	LUID           string          `json:"luid"`
	DefinitionLUID string          `json:"definition_luid"`
	IsDefault      bool            `json:"is_default"`
	Specification  json.RawMessage `json:"specification"`
}

func DecodePulseBundle(data []byte) (PulseBundle, error) {
	var bundle PulseBundle
	if len(data) == 0 || len(data) > MaxPulseBundleBytes {
		return bundle, errors.New("Pulse bundle exceeds its 32 MiB bound or is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return bundle, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return bundle, errors.New("Pulse bundle requires one JSON document")
	}
	if bundle.Version != 1 {
		return bundle, errors.New("unsupported Pulse bundle version; pull a current complete bundle")
	}
	if origin, err := NormalizeServerOrigin(bundle.SourceServerOrigin); err != nil || origin != bundle.SourceServerOrigin || bundle.SourceSiteLUID == "" {
		return bundle, errors.New("Pulse bundle requires canonical source identity")
	}
	_, identity, err := canonicalPulseDefinition(bundle.Definition)
	if err != nil {
		return bundle, err
	}
	if bundle.DefinitionLUID != identity.ID || len(bundle.DatasourceReferences) != 1 || bundle.DatasourceReferences[0] != identity.DatasourceID {
		return bundle, errors.New("Pulse bundle definition and datasource identities do not match")
	}
	if len(bundle.Metrics) == 0 || len(bundle.Metrics) > 10000 {
		return bundle, errors.New("Pulse bundle requires a complete metric inventory from 1 through 10000 records")
	}
	seen := map[string]bool{}
	for _, metric := range bundle.Metrics {
		var spec map[string]json.RawMessage
		if metric.LUID == "" || seen[metric.LUID] || metric.DefinitionLUID != identity.ID || json.Unmarshal(metric.Specification, &spec) != nil || spec == nil || len(spec["measurement_period"]) == 0 || len(spec["filters"]) == 0 {
			return bundle, errors.New("Pulse bundle contains a duplicate, mismatched, or incomplete metric specification")
		}
		var period map[string]json.RawMessage
		var filters []json.RawMessage
		if json.Unmarshal(spec["measurement_period"], &period) != nil || len(period) == 0 || json.Unmarshal(spec["filters"], &filters) != nil || filters == nil {
			return bundle, errors.New("Pulse bundle metric requires a complete period object and explicit filter array")
		}
		seen[metric.LUID] = true
	}
	return bundle, nil
}

func readPulseBundleFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Pulse bundle must be a regular file, not a symbolic link")
	}
	return readBoundedFile(path, MaxPulseBundleBytes)
}

// ReadPulseBundle rejects old definition-only snapshots and mismatched documents.
func ReadPulseBundle(ctx context.Context, path string) (PulseBundle, error) {
	artifact, err := NewPulseDefinitionManager(nil).Read(ctx, path)
	if err != nil {
		return PulseBundle{}, err
	}
	data, err := readPulseBundleFile(filepath.Join(path, "bundle.json"))
	if err != nil {
		return PulseBundle{}, errors.New("artifact has no readable portable Pulse bundle; pull the definition again")
	}
	bundle, err := DecodePulseBundle(data)
	if err != nil {
		return bundle, err
	}
	if bundle.DefinitionLUID != artifact.Metadata.TableauID || bundle.SourceServerOrigin != artifact.Metadata.SourceServerOrigin || bundle.SourceSiteLUID != artifact.Metadata.SourceSiteLUID {
		return bundle, errors.New("Pulse bundle identity differs from artifact provenance")
	}
	canonical, _, err := canonicalPulseDefinition(bundle.Definition)
	if err != nil {
		return bundle, err
	}
	resource, _, err := canonicalPulseDefinition(artifact.Configuration)
	if err != nil || !bytes.Equal(canonical, resource) {
		return bundle, errors.New("Pulse bundle and resource.json differ; update both consistently or pull again")
	}
	return bundle, nil
}
