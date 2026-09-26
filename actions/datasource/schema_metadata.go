package datasource

import "github.com/ahillspace/tadx/internal/value"

func schemaProjectMetadata(fields []Field, input SchemaInput) []Field {
	result := append([]Field(nil), fields...)
	for i := range result {
		if !input.Descriptions && !input.Tags {
			result[i].Metadata, result[i].MetadataMatch = nil, ""
			continue
		}
		if result[i].Metadata == nil {
			continue
		}
		item := *result[i].Metadata
		item.UpstreamColumns = append([]value.MetadataColumn(nil), item.UpstreamColumns...)
		if !input.Descriptions {
			item.Description = nil
			item.Inherited = nil
			item.InheritedObserved = false
		}
		for n := range item.UpstreamColumns {
			if !input.Descriptions {
				item.UpstreamColumns[n].Description = nil
			}
			if !input.Tags {
				item.UpstreamColumns[n].Tags = nil
				item.UpstreamColumns[n].TagsObserved = false
			}
		}
		result[i].Metadata = &item
	}
	return result
}
