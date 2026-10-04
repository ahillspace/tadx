package value

// DatasourceNativeDownload carries one authoritative datasource and unchanged native package.
type DatasourceNativeDownload struct {
	LUID             string
	Name             string
	ProjectLUID      string
	ProjectPath      string
	Filename         string
	Content          []byte
	TableauRequestID string
}
