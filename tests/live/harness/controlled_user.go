package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
)

const controlledUserPlaceholder = "{prefix}"

// ValidateControlledUserTemplate checks that a profile supplies one complete
// generated-login slot and that the resulting approved login can fit safely.
func ValidateControlledUserTemplate(template string) error {
	if strings.Count(template, controlledUserPlaceholder) != 1 || strings.ContainsAny(strings.Replace(template, controlledUserPlaceholder, "", 1), "{}\r\n\x00 \t") {
		return errors.New("controlled user name must contain exactly one safe {prefix} placeholder")
	}
	probe := strings.Repeat("a", 32)
	_, err := renderControlledUser(template, probe)
	return err
}

// ControlledUserLogin renders the only disposable user name accepted by this
// case from its approved template, cryptographic run identity, and role.
func (c *Case) ControlledUserLogin(role string) (string, error) {
	if c == nil || c.Journal == nil || c.NamePrefix == "" {
		return "", Blocked("controlled user requires its exact case and ownership journal")
	}
	binding, err := c.Require("controlled_user")
	if err != nil {
		return "", err
	}
	if role == "" || len(role) > 32 {
		return "", Blocked("controlled user requires a bounded role label")
	}
	if !validControlledUserRole(role) {
		return "", Blocked("controlled user role label contains unsupported characters")
	}
	if len(c.Journal.RunID) != 20 {
		return "", Blocked("controlled user journal lacks its cryptographic run identity")
	}
	if _, err := hex.DecodeString(c.Journal.RunID); err != nil {
		return "", Blocked("controlled user journal lacks its cryptographic run identity")
	}
	if err := ValidateControlledUserTemplate(binding.Name); err != nil {
		return "", Blocked("controlled_user name template is invalid")
	}
	token := controlledUserToken(c.NamePrefix, c.Journal.RunID, role)
	login, err := renderControlledUser(binding.Name, token)
	if err != nil {
		return "", Blocked("controlled_user name template cannot render a safe unique login")
	}
	return login, nil
}

// IntentControlledUser stores a prefix-owned journal key and the exact remote
// login rendered from the approved template for uncertain-write reconciliation.
func (c *Case) IntentControlledUser(role string) (string, error) {
	login, err := c.ControlledUserLogin(role)
	if err != nil {
		return "", err
	}
	binding, _ := c.Require("controlled_user")
	ownerKey := c.NamePrefix + "-user-" + role
	token := controlledUserToken(c.NamePrefix, c.Journal.RunID, role)
	if err := c.Journal.AddControlledUserIntent(ownerKey, login, binding.Name, token); err != nil {
		return "", err
	}
	return login, nil
}

func (c *Case) ConfirmControlledUserReceipt(role, login string, returned Object) error {
	binding, err := c.Require("controlled_user")
	if err != nil {
		return err
	}
	expected, err := c.ControlledUserLogin(role)
	if err != nil {
		return err
	}
	if login != expected {
		return errors.New("controlled user receipt does not match its exact approved login")
	}
	token := controlledUserToken(c.NamePrefix, c.Journal.RunID, role)
	return c.Journal.ConfirmControlledUserReceipt(c.NamePrefix+"-user-"+role, login, binding.Name, token, returned)
}

func renderControlledUser(template, token string) (string, error) {
	if token == "" || strings.ContainsAny(token, "{}@ \t\r\n\x00") || strings.Count(template, controlledUserPlaceholder) != 1 {
		return "", errors.New("invalid controlled user render")
	}
	login := strings.Replace(template, controlledUserPlaceholder, token, 1)
	if strings.ContainsAny(login, "{}\r\n\x00 \t") || strings.Count(login, token) != 1 || len(login) > 254 {
		return "", errors.New("controlled user render is not a bounded exact login")
	}
	if strings.Count(login, "@") != 1 {
		return "", errors.New("controlled user render is not one email address")
	}
	parts := strings.SplitN(login, "@", 2)
	local, domain := parts[0], parts[1]
	if len(local) > 64 || !validEmailLocal(local) || !validEmailDomain(domain) {
		return "", errors.New("controlled user email is not canonical or exceeds its length limits")
	}
	address, err := mail.ParseAddress(login)
	if err != nil || address.Address != login || address.Name != "" {
		return "", errors.New("controlled user render is not an exact email address")
	}
	return login, nil
}

func validEmailLocal(local string) bool {
	if local == "" || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return false
	}
	for _, character := range local {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune(".!#$%&'*+-/=?^_`{|}~", character) {
			continue
		}
		return false
	}
	return true
}

func validEmailDomain(domain string) bool {
	if domain == "" || len(domain) > 253 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func controlledUserToken(prefix, runID, role string) string {
	roleHash := sha256.Sum256([]byte(prefix + "\x00" + role))
	return runID + hex.EncodeToString(roleHash[:6])
}

func validControlledUserRole(role string) bool {
	if role == "" || len(role) > 32 {
		return false
	}
	for _, character := range role {
		if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
			return false
		}
	}
	return true
}
