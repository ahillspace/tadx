package update

import (
	"reflect"
	"strings"
)

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.UserLUID) == "" {
		return usage("selector", "admin user update requires --environment and --id; the environment selects the site")
	}
	if in.AuthSetting != nil && in.IdPConfigurationID != nil {
		return usage("auth_setting", "admin user update cannot set auth setting and IdP configuration ID together")
	}
	if in.AuthSetting != nil {
		switch *in.AuthSetting {
		case "ServerDefault", "SAML", "OpenID", "TableauIDWithMFA":
		default:
			return usage("auth_setting", "Supported --auth-setting values: ServerDefault, SAML, OpenID, TableauIDWithMFA. Use --idp-configuration-id for an exact authentication configuration.")
		}
	}
	req := Request{in.FullName, in.Email, in.SiteRole, in.AuthSetting, in.IdentityPoolName, in.IdPConfigurationID, in.Language, in.Locale}
	if reflect.DeepEqual(req, Request{}) {
		return usage("fields", "admin user update requires at least one explicit field")
	}
	return nil
}
