package readsource

import "github.com/ahillspace/tadx/internal/value"

// SearchPage applies observed source coverage without changing page contents.
func SearchPage(page value.SearchPage, source *Metadata) value.SearchPage {
	if source != nil {
		switch source.Mode {
		case Tableau:
			page.Source = "live"
		case Cache:
			page.Source = "cache"
		}
		if source.CacheWarning != "" {
			page.Warnings = []string{source.CacheWarning}
		}
	}
	return page
}
