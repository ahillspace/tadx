package read

import "strings"

func listLimitReason(limit int, all bool) string {
	if limit < 0 || limit > 10000 {
		return "limit must be between 1 and 10000"
	}
	if all && limit != 0 {
		return "--all cannot be combined with --limit"
	}
	return ""
}

func listLimit(limit int, all bool) int {
	if all {
		return 10000
	}
	if limit == 0 {
		return 25
	}
	return limit
}

func identityReason(ids ...string) string {
	for _, id := range ids {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return "identities must be exact and contain no surrounding whitespace or control characters"
		}
	}
	return ""
}

func selectorReason(id, metadataID string) string {
	if (id == "") == (metadataID == "") {
		return "select exactly one --id or --metadata-id"
	}
	return ""
}
