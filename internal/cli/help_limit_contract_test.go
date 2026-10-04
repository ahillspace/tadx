package cli

// These baseline expectations are independent of command-owner help metadata.
var helpLimitDefaults = map[string]string{
	"content workbook list": "25", "content datasource list": "25", "content flow list": "25", "content project list": "25",
	"content datasource schema": "20", "catalog label list": "20",
	"admin user list": "25", "admin group list": "25", "admin label-value list": "20", "admin label-category list": "20",
	"catalog database list": "25", "catalog table list": "25", "catalog column list": "25", "catalog search": "25", "catalog audit": "1000",
	"pulse definition list": "25", "pulse metric list": "25",
	"env list": "20", "workspace list": "20", "workspace status": "20", "capability list": "20",
}
