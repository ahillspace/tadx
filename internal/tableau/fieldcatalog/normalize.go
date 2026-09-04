package fieldcatalog

import (
	"regexp"
	"sort"
	"strings"
)

var tableIDSuffix = regexp.MustCompile(`(?i)^(.*)_[0-9a-f]{8,}$`)

func normalizeSchema(datasourceLUID, datasourceName string, tableNames map[string]string, raw []rawField, provenance string) Schema {
	nameCounts := make(map[string]int)
	tableCounts := make(map[string]int)
	fields := make([]Field, 0, len(raw))
	for _, value := range raw {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		caption := strings.TrimSpace(value.Caption)
		if caption == "" {
			caption = name
		}
		table := strings.TrimSpace(tableNames[value.LogicalTableID])
		if table == "" && value.LogicalTableID != "" {
			table = tableCaption(value.LogicalTableID)
		}
		nameCounts[name]++
		label := caption
		if nameCounts[name] > 1 {
			label = caption + " (" + fallback(table, "calculation") + ")"
		}

		exclusion := strings.TrimSpace(value.ExclusionHint)
		if exclusion == "" {
			switch strings.ToUpper(strings.TrimSpace(value.ColumnClass)) {
			case "TABLE_CALCULATION":
				exclusion = "table_calc"
			case "BIN":
				exclusion = "bin"
			default:
				if value.Hidden || value.Internal {
					exclusion = "internal"
				}
			}
		}
		requiresUserAggregation := value.RequiresUserAggregation
		if exclusion == "" {
			aggregation := strings.ToUpper(strings.TrimSpace(value.DefaultAggregation))
			requiresUserAggregation = requiresUserAggregation || aggregation == "AGG" || aggregation == "USER"
		}
		dataType := normalizeType(firstNonempty(value.DataType, value.PhysicalType))
		timeType := normalizedTimeType(value.DataType, value.PhysicalType)
		role := "dimension"
		if strings.EqualFold(value.Role, "MEASURE") {
			role = "measure"
		} else if timeType != "" {
			role = "date"
		}
		if exclusion != "" {
			role = "excluded"
		}
		fieldProvenance := strings.TrimSpace(value.Provenance)
		if fieldProvenance == "" {
			fieldProvenance = provenance
		}
		fields = append(fields, Field{
			ID: name, Name: name, Caption: caption, Label: label, Role: role, DataType: dataType, TimeType: timeType,
			Table: table, LogicalTableID: strings.TrimSpace(value.LogicalTableID), DefaultAggregation: strings.ToUpper(strings.TrimSpace(value.DefaultAggregation)),
			Formula: value.Formula, RequiresUserAggregation: requiresUserAggregation, Excluded: exclusion != "", ExclusionReason: exclusion, Provenance: fieldProvenance,
		})
		if table != "" {
			tableCounts[value.LogicalTableID]++
			if _, exists := tableNames[value.LogicalTableID]; !exists {
				tableNames[value.LogicalTableID] = table
			}
		}
	}
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Table != fields[j].Table {
			return fields[i].Table < fields[j].Table
		}
		if fields[i].Label != fields[j].Label {
			return fields[i].Label < fields[j].Label
		}
		return fields[i].ID < fields[j].ID
	})
	tables := make([]Table, 0, len(tableNames))
	for id, name := range tableNames {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
			continue
		}
		tables = append(tables, Table{ID: id, Name: name, FieldCount: tableCounts[id]})
	}
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Name != tables[j].Name {
			return tables[i].Name < tables[j].Name
		}
		return tables[i].ID < tables[j].ID
	})
	warnings := []string{}
	hasMeasure, hasDate := false, false
	for _, field := range fields {
		hasMeasure = hasMeasure || field.Role == "measure"
		hasDate = hasDate || field.Role == "date"
	}
	if !hasMeasure {
		warnings = append(warnings, "No usable measures were discovered in this datasource.")
	}
	if !hasDate {
		warnings = append(warnings, "No supported date fields were discovered in this datasource.")
	}
	return Schema{DatasourceLUID: strings.TrimSpace(datasourceLUID), DatasourceName: strings.TrimSpace(datasourceName), Tables: tables, Fields: fields, Warnings: warnings}
}

func tableCaption(value string) string {
	match := tableIDSuffix.FindStringSubmatch(value)
	if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
		return match[1]
	}
	return value
}

func normalizeType(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "_", "-", "_", "__", "_").Replace(value)
	if value == "" {
		return "STRING"
	}
	return value
}

func normalizedTimeType(values ...string) string {
	for _, value := range values {
		switch normalizeType(value) {
		case "DATE":
			return "DATE"
		case "DATETIME", "DATE_TIME", "TIMESTAMP":
			return "DATETIME"
		}
	}
	return ""
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
