package catalog

import "context"

// Direct port fixtures exercise the private mutation runners without a target provider.
func newDatabaseRunner(r DatabaseReader, w DatabaseWriter) *databaseUpdate {
	return &databaseUpdate{reader: r, writer: w}
}
func newTableRunner(r TableReader, w TableWriter) *tableUpdate {
	return &tableUpdate{reader: r, writer: w}
}
func newColumnRunner(r ColumnReader, w ColumnWriter) *columnUpdate {
	return &columnUpdate{reader: r, writer: w}
}
func (a *databaseUpdate) Execute(ctx context.Context, in DatabaseInput, preview bool) (DatabaseOutput, error) {
	if err := ValidateDatabaseInput(in); err != nil {
		return DatabaseOutput{}, err
	}
	return a.executeValidated(ctx, in, preview)
}
func (a *tableUpdate) Execute(ctx context.Context, in TableInput, preview bool) (TableOutput, error) {
	if err := ValidateTableInput(in); err != nil {
		return TableOutput{}, err
	}
	return a.executeValidated(ctx, in, preview)
}
func (a *columnUpdate) Execute(ctx context.Context, in ColumnInput, preview bool) (ColumnOutput, error) {
	if err := ValidateColumnInput(in); err != nil {
		return ColumnOutput{}, err
	}
	return a.executeValidated(ctx, in, preview)
}
