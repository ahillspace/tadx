package auth

import (
	"net/http"
	"slices"
)

// SessionRedactor protects metadata observed across requests and session renewal.
func SessionRedactor(s Session) func(string) string {
	var header http.Header
	if s != nil {
		header = make(http.Header)
		s.Authorize(&http.Request{Header: header})
	}
	return RequestRedactor(s, header)
}

// RequestRedactor protects diagnostics with the credentials actually sent, the
// session's PAT, and its current token if another request has since renewed it.
func RequestRedactor(s Session, sent http.Header, additional ...string) func(string) string {
	secrets := append(slices.Clone(additional), sent.Values(TableauAuthHeader)...)
	if live, ok := s.(*session); ok {
		return func(value string) string {
			credentials := []string{live.credentials.Name, live.credentials.Secret, live.currentToken()}
			return Redact(value, append(credentials, secrets...)...)
		}
	}
	return func(value string) string { return Redact(value, secrets...) }
}
