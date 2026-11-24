package auth

import (
	"net/http"
	"slices"
)

// SessionRedactor protects metadata observed across requests and session renewal.
func SessionRedactor(s Session) func(string) string {
	var header http.Header
	// A native session already tracks its tokens. Other implementations
	// expose their token only when authorizing a request.
	if _, native := s.(*session); s != nil && !native {
		header = make(http.Header)
		s.Authorize(&http.Request{Header: header})
	}
	return RequestRedactor(s, header)
}

// RequestRedactor protects diagnostics with the credentials actually sent, the
// session's PAT, and every token installed on that session during the command.
func RequestRedactor(s Session, sent http.Header, additional ...string) func(string) string {
	secrets := append(slices.Clone(additional), sent.Values(TableauAuthHeader)...)
	if live, ok := s.(*session); ok {
		return func(value string) string {
			live.mu.RLock()
			credentials := slices.Concat(secrets, live.retiredTokens, []string{live.credentials.Name, live.credentials.Secret, live.token})
			live.mu.RUnlock()
			return Redact(value, credentials...)
		}
	}
	return func(value string) string { return Redact(value, secrets...) }
}
