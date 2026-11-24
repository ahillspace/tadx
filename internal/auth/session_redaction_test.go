package auth

import (
	"fmt"
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
			redact := SessionRedactor(session)
			if err = session.Renew(t.Context(), rejectedWith(session)); err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"agent-name", "highly-secret", "token-1", "token-2"} {
				if redact("prefix "+secret+" suffix") != Redact("prefix "+secret+" suffix", secret) {
					t.Fatal("secret not redacted")
				}
			}
			if v := "  Expected value\n"; redact(v) != v {
				t.Fatal("altered an ordinary metadata value")
			}
		}()
	}
}
