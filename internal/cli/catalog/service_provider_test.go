package catalog

import (
	"context"

	catalogaction "github.com/ahillspace/tadx/actions/catalog"
)

type staticCatalogProvider struct{ assets catalogaction.Assets }

func (p staticCatalogProvider) Open(context.Context, string, string, bool) (catalogaction.Target, error) {
	return catalogaction.Target{Environment: "fixture", Site: "site", Assets: p.assets}, nil
}
