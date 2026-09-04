package fieldcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
)

const datasourceFieldsQuery = `query GetDatasourceFields($luid: String!, $first: Int!, $after: String) {
  publishedDatasources(filter: { luid: $luid }) {
    luid
    name
    fieldsConnection(first: $first, after: $after) {
      totalCount
      pageInfo { hasNextPage endCursor }
      nodes {
        __typename
        id
        name
        fullyQualifiedName
        description
        isHidden
        ... on ColumnField { role dataType aggregation }
        ... on CalculatedField { role dataType aggregation formula }
      }
    }
  }
}`

type graphQLNode struct {
	Type               string  `json:"__typename"`
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	FullyQualifiedName string  `json:"fullyQualifiedName"`
	Description        *string `json:"description"`
	Role               string  `json:"role"`
	DataType           string  `json:"dataType"`
	Aggregation        *string `json:"aggregation"`
	Formula            *string `json:"formula"`
	Hidden             bool    `json:"isHidden"`
}

type fieldsGraphQLEnvelope struct {
	Data struct {
		PublishedDatasources []struct {
			LUID             string `json:"luid"`
			Name             string `json:"name"`
			FieldsConnection struct {
				TotalCount int `json:"totalCount"`
				PageInfo   struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []graphQLNode `json:"nodes"`
			} `json:"fieldsConnection"`
		} `json:"publishedDatasources"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"errors"`
}

func (c *Client) readMetadataGraphQL(ctx context.Context, luid, datasourceName string) (Schema, error) {
	const pageSize = 1000
	after := any(nil)
	seenCursors := make(map[string]struct{})
	seenIDs := make(map[string]graphQLNode)
	nodes := make([]graphQLNode, 0)
	total := -1
	requestID := ""
	for page := 0; page < 1000; page++ {
		body, _ := json.Marshal(map[string]any{"query": datasourceFieldsQuery, "variables": map[string]any{"luid": luid, "first": pageSize, "after": after}})
		response, err := c.post(ctx, "/api/metadata/graphql", "datasource.schema.metadata", body)
		if err != nil {
			return Schema{}, err
		}
		requestID = response.TableauRequestID
		var envelope fieldsGraphQLEnvelope
		if err := json.Unmarshal(response.Body, &envelope); err != nil {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("decode Metadata API response: %w", err), true)
		}
		if len(envelope.Errors) > 0 {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API returned %d GraphQL errors: %s", len(envelope.Errors), strings.TrimSpace(envelope.Errors[0].Message)), false)
		}
		if len(envelope.Data.PublishedDatasources) != 1 {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("expected one published datasource for LUID %q, received %d", luid, len(envelope.Data.PublishedDatasources)), true)
		}
		datasource := envelope.Data.PublishedDatasources[0]
		if datasource.LUID != luid {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API returned datasource LUID %q, expected %q", datasource.LUID, luid), true)
		}
		if strings.TrimSpace(datasource.Name) != "" {
			datasourceName = datasource.Name
		}
		connection := datasource.FieldsConnection
		if connection.TotalCount < 0 {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API returned negative field total %d", connection.TotalCount), true)
		}
		if total < 0 {
			total = connection.TotalCount
		} else if total != connection.TotalCount {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API field total changed from %d to %d", total, connection.TotalCount), true)
		}
		for _, node := range connection.Nodes {
			identity := strings.TrimSpace(node.ID)
			if identity == "" {
				return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API returned a field without an ID"), true)
			}
			if previous, exists := seenIDs[identity]; exists {
				current, _ := json.Marshal(node)
				prior, _ := json.Marshal(previous)
				if string(current) != string(prior) {
					return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API returned conflicting field ID %q", identity), true)
				}
				continue
			}
			seenIDs[identity] = node
			nodes = append(nodes, node)
		}
		if len(nodes) > total {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API returned more fields than its total"), true)
		}
		if !connection.PageInfo.HasNextPage {
			if len(nodes) != total {
				return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API pagination ended at %d of %d fields", len(nodes), total), true)
			}
			fields := classifyGraphQLFields(nodes)
			result := normalizeSchema(luid, datasourceName, map[string]string{}, fields, "metadata_graphql")
			result.RequestID = requestID
			return result, nil
		}
		cursor := strings.TrimSpace(connection.PageInfo.EndCursor)
		if cursor == "" {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API field page omitted its continuation cursor"), true)
		}
		if _, exists := seenCursors[cursor]; exists {
			return Schema{}, tableau.NewProtocolError("datasource.schema.metadata", response, fmt.Errorf("Metadata API repeated field cursor %q", cursor), true)
		}
		seenCursors[cursor] = struct{}{}
		after = cursor
	}
	return Schema{}, fmt.Errorf("Metadata API field pagination exceeded 1000 pages")
}

var (
	tableCalcPattern = regexp.MustCompile(`(?i)\b(?:FIRST|INDEX|LAST|LOOKUP|MODEL_EXTENSION_BOOL|MODEL_EXTENSION_INT|MODEL_EXTENSION_REAL|MODEL_EXTENSION_STR|MODEL_PERCENTILE|MODEL_QUANTILE|PREVIOUS_VALUE|RANK|RANK_DENSE|RANK_MODIFIED|RANK_PERCENTILE|RANK_UNIQUE|RUNNING_AVG|RUNNING_COUNT|RUNNING_MAX|RUNNING_MIN|RUNNING_SUM|SCRIPT_BOOL|SCRIPT_INT|SCRIPT_REAL|SCRIPT_STR|SIZE|TOTAL|WINDOW_AVG|WINDOW_CORR|WINDOW_COUNT|WINDOW_COVAR|WINDOW_COVARP|WINDOW_MAX|WINDOW_MEDIAN|WINDOW_MIN|WINDOW_PERCENTILE|WINDOW_STDEV|WINDOW_STDEVP|WINDOW_SUM|WINDOW_VAR|WINDOW_VARP)\s*\(`)
	userAggPattern   = regexp.MustCompile(`(?i)\b(?:SUM|AVG|MIN|MAX|COUNT|COUNTD|MEDIAN|ATTR|STDEV|STDEVP|VAR|VARP)\s*\(`)
	calcIDPattern    = regexp.MustCompile(`(?i)^\[(Calculation_\d+)\]$`)
	singleFQNPattern = regexp.MustCompile(`^\[([^\]]+)\]$`)
	bracketPattern   = regexp.MustCompile(`\[([^\]]+)\]`)
)

type analyzedField struct {
	node          graphQLNode
	name          string
	references    []string
	exclusion     string
	requiresUser  bool
	directUserAgg bool
}

func classifyGraphQLFields(nodes []graphQLNode) []rawField {
	analysis := make([]*analyzedField, 0, len(nodes))
	byName := make(map[string][]*analyzedField)
	for _, node := range nodes {
		if node.Hidden {
			continue
		}
		name := canonicalFieldID(node)
		formula := value(node.Formula)
		prepared := prepareFormula(formula)
		entry := &analyzedField{node: node, name: name, references: formulaReferences(prepared), directUserAgg: userAggPattern.MatchString(stripCurlyBlocks(prepared))}
		fqn := strings.ToLower(strings.TrimSpace(node.FullyQualifiedName))
		switch {
		case strings.Contains(fqn, "__tableau_internal_object_id__") || strings.EqualFold(node.DataType, "TABLE") || strings.Contains(fqn, "[__"):
			entry.exclusion = "internal"
		case tableCalcPattern.MatchString(formula):
			entry.exclusion = "table_calc"
		case hasBlendedReference(prepared):
			entry.exclusion = "blend_ref"
		}
		analysis = append(analysis, entry)
		if strings.TrimSpace(node.Name) != "" {
			byName[node.Name] = append(byName[node.Name], entry)
		}
	}
	for pass := 0; pass <= len(analysis); pass++ {
		changed := false
		for _, entry := range analysis {
			if entry.exclusion != "" || value(entry.node.Formula) == "" {
				continue
			}
			for _, reference := range entry.references {
				for _, dependency := range byName[reference] {
					if dependency.exclusion != "" {
						entry.exclusion = dependency.exclusion
						changed = true
						break
					}
				}
				if entry.exclusion != "" {
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	for _, entry := range analysis {
		entry.requiresUser = entry.exclusion == "" && entry.directUserAgg
	}
	for pass := 0; pass <= len(analysis); pass++ {
		changed := false
		for _, entry := range analysis {
			if entry.exclusion != "" || entry.requiresUser || value(entry.node.Formula) == "" {
				continue
			}
			for _, reference := range entry.references {
				for _, dependency := range byName[reference] {
					if dependency.exclusion == "" && dependency.requiresUser {
						entry.requiresUser = true
						changed = true
						break
					}
				}
				if entry.requiresUser {
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	fields := make([]rawField, 0, len(analysis))
	for _, entry := range analysis {
		fields = append(fields, rawField{Name: entry.name, Caption: entry.node.Name, DataType: entry.node.DataType, PhysicalType: entry.node.DataType, ColumnClass: map[bool]string{true: "CALCULATION", false: "COLUMN"}[value(entry.node.Formula) != ""], DefaultAggregation: value(entry.node.Aggregation), Formula: value(entry.node.Formula), Role: entry.node.Role, ExclusionHint: entry.exclusion, RequiresUserAggregation: entry.requiresUser, Provenance: "metadata_graphql"})
	}
	return fields
}

func canonicalFieldID(node graphQLNode) string {
	fqn := strings.TrimSpace(node.FullyQualifiedName)
	if strings.EqualFold(node.Type, "CalculatedField") {
		if match := calcIDPattern.FindStringSubmatch(fqn); len(match) == 2 {
			return match[1]
		}
	}
	if match := singleFQNPattern.FindStringSubmatch(fqn); len(match) == 2 {
		return match[1]
	}
	if fqn != "" && !strings.Contains(fqn, "][") {
		return fqn
	}
	return strings.TrimSpace(node.Name)
}

func prepareFormula(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); {
		if index+1 < len(value) && value[index:index+2] == "//" {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			continue
		}
		if index+1 < len(value) && value[index:index+2] == "/*" {
			index += 2
			for index+1 < len(value) && value[index:index+2] != "*/" {
				index++
			}
			index = min(index+2, len(value))
			continue
		}
		if value[index] == '\'' || value[index] == '"' {
			quote := value[index]
			builder.WriteByte(' ')
			index++
			for index < len(value) {
				if value[index] == '\\' && index+1 < len(value) {
					builder.WriteString("  ")
					index += 2
					continue
				}
				builder.WriteByte(' ')
				if value[index] == quote {
					index++
					break
				}
				index++
			}
			continue
		}
		builder.WriteByte(value[index])
		index++
	}
	return builder.String()
}

func stripCurlyBlocks(value string) string {
	var builder strings.Builder
	depth := 0
	for _, character := range value {
		switch character {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				builder.WriteRune(character)
			} else {
				builder.WriteRune(' ')
			}
		}
	}
	return builder.String()
}

func formulaReferences(value string) []string {
	matches := bracketPattern.FindAllStringSubmatch(value, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			result = append(result, match[1])
		}
	}
	return result
}

func hasBlendedReference(value string) bool {
	return strings.Contains(value, "].[") || strings.Contains(value, "] . [")
}
