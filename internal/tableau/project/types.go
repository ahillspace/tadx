// Package project implements the released Tableau project REST client family.
package project

// Project is one authoritative project row from Tableau REST.
type Project struct {
	LUID                            string
	Name                            string
	Description                     string
	ParentLUID                      string
	OwnerLUID                       string
	TopLevel                        *bool
	ContentPermissions              string
	ControllingPermissionsProjectID string
	CreatedAt                       string
	UpdatedAt                       string
	ProjectCount                    *int
	WorkbookCount                   *int
	ViewCount                       *int
	DatasourceCount                 *int
}

// ListRequest selects one bounded REST page.
type ListRequest struct {
	PageNumber int
	PageSize   int
	Name       string
	ParentLUID string
	OwnerName  string
	TopLevel   *bool
}

// Page is one normalized classic REST page.
type Page struct {
	Number           int
	Size             int
	Total            int
	Items            []Project
	TableauRequestID string
}
