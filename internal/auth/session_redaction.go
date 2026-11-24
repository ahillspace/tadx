package auth

import "net/http"

// SessionRedactor captures the PAT and token used for a request without
// exposing their values to consumers. A response processed after renewal may
// refer to the token used before the request was sent or the current token.
func SessionRedactor(s Session) func(string) string {
	if live, ok := s.(*session); ok {
		original := live.currentToken()
		return func(value string) string {
			return Redact(value, live.credentials.Name, live.credentials.Secret, original, live.currentToken())
		}
	}
	// Other session implementations may supply token authorization without a PAT.
	// Observe the same authorization boundary used to send a request.
	var token string
	if s != nil {
		req := &http.Request{Header: make(http.Header)}
		s.Authorize(req)
		token = req.Header.Get(TableauAuthHeader)
	}
	return func(value string) string { return Redact(value, token) }
}
