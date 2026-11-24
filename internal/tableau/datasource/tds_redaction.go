package datasource

import "github.com/ahillspace/tadx/internal/auth"

// TDSRedactor protects metadata used in work-copy exports and verification.
// A change to an exact selector during redaction makes it unsafe to use.
func (c *Client) TDSRedactor() func(string) string {
	if c == nil {
		return auth.SessionRedactor(nil)
	}
	return auth.SessionRedactor(c.session)
}
