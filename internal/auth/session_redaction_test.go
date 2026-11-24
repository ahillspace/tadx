package auth

import (
	"fmt"
	"strings"
	"testing"
)

func TestSessionMetadataRedactionAcrossCredentialSourcesAndRenewal(t *testing.T) {
	for _, stored := range []bool{false, true} {
		fixture := newStoredPATFixture(t)
		target := fixture.target
		target.PATNameVariable = "NAME"
		target.PATSecretVariable = "SECRET"
		lookup := LookupEnvFunc(func(key string) (string, bool) {
			switch key {
			case "NAME":
				return "agent-name", true
			case "SECRET":
				return "highly-secret", true
			default:
				return "", false
			}
		})
		signer := &countingSigner{}
		if stored {
			lookup = nil
			// Prime the saved session and prove the next command adopts it.
			fixture.command(t, signer, fixture.target)
			target = fixture.target
		} else {
			target.CredentialReference = ""
		}
		func() {
			sessions := NewCommandSessions(lookup, fixture.store, fixture.directory)
			defer sessions.Close()
			session, err := sessions.Authenticate(t.Context(), target, signer)
			if err != nil {
				t.Fatal(err)
			}
			if signer.signIns() != 1 {
				t.Fatal("unexpected second sign-in")
			}
			if fmt.Sprintf("%#v", session) != session.String() {
				t.Fatal("debug formatting exposed session internals")
			}
			redactors := []func(string) string{SessionRedactor(session)}
			for range 2 {
				if err = session.Renew(t.Context(), rejectedWith(session)); err != nil {
					t.Fatal(err)
				}
				redactors = append(redactors, SessionRedactor(session))
			}
			for i, redact := range redactors {
				for _, secret := range []string{"agent-name", "highly-secret", "token-1", "token-2", "token-3"} {
					if redact("prefix "+secret+" suffix") != Redact("prefix "+secret+" suffix", secret) {
						t.Errorf("redactor created after %d renewals exposed a credential", i)
					}
				}
				if v := "  Expected value\n"; redact(v) != v {
					t.Fatal("altered an ordinary metadata value")
				}
			}
		}()
		for _, record := range fixture.backend.entries {
			if strings.Contains(record, "token-1") || strings.Contains(record, "token-2") {
				t.Fatal("retired session token persisted in the credential store")
			}
		}
	}
}
