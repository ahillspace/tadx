package tableau

import (
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// DispositionFilename returns the filename parameter of a content download's
// Content-Disposition header, or "" when the header has none.
//
// Tableau sends the plain filename parameter form-encoded, as Java's URLEncoder
// does: a content named "Sales & Pipeline" downloads with
// filename="Sales+%26+Pipeline.tdsx". A plain value is therefore decoded with
// url.QueryUnescape. An RFC 5987 filename* value is already decoded by
// mime.ParseMediaType and is returned as is. A plain value that is not valid
// form encoding, is not UTF-8 once decoded, or would gain a path separator is
// returned exactly as sent, so decoding never turns an accepted name into a
// rejected one. Callers still validate the result with ValidFilename.
func DispositionFilename(value string) string {
	for _, candidate := range []string{value, "attachment; " + value} {
		_, parameters, err := mime.ParseMediaType(candidate)
		if err != nil || parameters["filename"] == "" {
			continue
		}
		if extendedFilename(value) {
			return parameters["filename"]
		}
		return formDecodedFilename(parameters["filename"])
	}
	for _, part := range strings.Split(value, ";") {
		name, filename, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(name), "filename") {
			return formDecodedFilename(strings.Trim(strings.TrimSpace(filename), `"`))
		}
	}
	return ""
}

// ValidFilename reports whether filename is a single local path component.
func ValidFilename(filename string) bool {
	return filename != "." && filename != ".." &&
		!strings.ContainsAny(filename, `/\`) && filepath.Base(filename) == filename
}

// extendedFilename reports whether the header carries an RFC 2231 or RFC 5987
// filename* parameter. Form-encoded values never contain a literal ";".
func extendedFilename(value string) bool {
	for _, part := range strings.Split(value, ";") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(part)), "filename*") {
			return true
		}
	}
	return false
}

func formDecodedFilename(raw string) string {
	decoded, err := url.QueryUnescape(raw)
	if err != nil || !utf8.ValidString(decoded) ||
		strings.ContainsAny(decoded, `/\`) && !strings.ContainsAny(raw, `/\`) {
		return raw
	}
	return decoded
}
