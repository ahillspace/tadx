package datasource

func schemaProjectMetadata(fields []Field, input SchemaInput) []Field {
	result := append([]Field(nil), fields...)
	for i := range result {
		result[i] = result[i].ProjectMetadata(input.Descriptions, input.Tags, false)
	}
	return result
}
