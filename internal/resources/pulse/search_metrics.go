package pulse

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
	"github.com/ahillspace/tadx/internal/value"
)

// MetricSearchClient supplies native pages without consumer workflow dependencies.
type MetricSearchClient interface {
	ListDefinitions(context.Context, tableaupulse.PageRequest) (tableaupulse.DefinitionPage, error)
	ListMetrics(context.Context, string, tableaupulse.PageRequest) (tableaupulse.MetricPage, error)
}

// MetricSearch retains definition pages for one command-scoped search.
type MetricSearch struct {
	client          MetricSearchClient
	definitionPages map[string]tableaupulse.DefinitionPage
}

func NewMetricSearch(client MetricSearchClient) *MetricSearch {
	return &MetricSearch{client: client, definitionPages: make(map[string]tableaupulse.DefinitionPage)}
}

type metricSearchCursor struct {
	Version             int    `json:"v"`
	DefinitionPageToken string `json:"d,omitempty"`
	DefinitionIndex     int    `json:"i,omitzero"`
	MetricPageToken     string `json:"m,omitempty"`
	DefinitionDigest    string `json:"s,omitempty"`
}

func (s *MetricSearch) List(ctx context.Context, encoded string, limit int) (value.SearchPage, error) {
	state := metricSearchCursor{Version: 1}
	if encoded != "" {
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.DefinitionIndex < 0 || (state.DefinitionDigest == "" && (state.DefinitionIndex != 0 || state.MetricPageToken != "")) {
			return value.SearchPage{}, errors.New("invalid Pulse metric search cursor")
		}
	}
	definitions, ok := s.definitionPages[state.DefinitionPageToken]
	if !ok {
		var err error
		definitions, err = s.client.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: 100, PageToken: state.DefinitionPageToken})
		if err != nil {
			return value.SearchPage{}, err
		}
		s.definitionPages[state.DefinitionPageToken] = definitions
	}
	digest := definitionPageDigest(definitions)
	if state.DefinitionDigest != "" && state.DefinitionDigest != digest {
		return value.SearchPage{}, errors.New("Pulse definition inventory changed during metric search")
	}
	state.DefinitionDigest = digest
	if state.DefinitionIndex > len(definitions.Definitions) {
		return value.SearchPage{}, errors.New("invalid Pulse metric search cursor")
	}
	if state.DefinitionIndex == len(definitions.Definitions) {
		if definitions.NextPageToken == "" {
			return value.SearchPage{Items: []value.SearchItem{}}, nil
		}
		state.DefinitionPageToken = definitions.NextPageToken
		state.DefinitionIndex = 0
		state.DefinitionDigest = ""
		return value.SearchPage{Items: []value.SearchItem{}, NextCursor: encodeMetricSearchCursor(state)}, nil
	}
	definition := definitions.Definitions[state.DefinitionIndex]
	metrics, err := s.client.ListMetrics(ctx, definition.LUID, tableaupulse.PageRequest{PageSize: limit, PageToken: state.MetricPageToken})
	if err != nil {
		return value.SearchPage{}, err
	}
	result := value.SearchPage{Items: make([]value.SearchItem, len(metrics.Metrics))}
	for index, metric := range metrics.Metrics {
		name := metric.Name
		if strings.TrimSpace(name) == "" {
			name = definition.Name
		}
		result.Items[index] = value.SearchItem{LUID: metric.LUID, Type: "metric", Name: name}
	}
	if metrics.NextPageToken != "" {
		state.MetricPageToken = metrics.NextPageToken
		result.NextCursor = encodeMetricSearchCursor(state)
		return result, nil
	}
	state.DefinitionIndex++
	state.MetricPageToken = ""
	if state.DefinitionIndex < len(definitions.Definitions) {
		result.NextCursor = encodeMetricSearchCursor(state)
	} else if definitions.NextPageToken != "" {
		state.DefinitionPageToken = definitions.NextPageToken
		state.DefinitionIndex = 0
		state.DefinitionDigest = ""
		result.NextCursor = encodeMetricSearchCursor(state)
	}
	return result, nil
}

func definitionPageDigest(page tableaupulse.DefinitionPage) string {
	identities := make([]string, len(page.Definitions))
	for i, item := range page.Definitions {
		identities[i] = item.LUID
	}
	data, _ := json.Marshal(struct {
		Items []string `json:"items"`
		Next  string   `json:"next"`
	}{identities, page.NextPageToken})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeMetricSearchCursor(cursor metricSearchCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}
