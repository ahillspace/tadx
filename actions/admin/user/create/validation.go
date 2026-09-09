package create

import (
	"strings"
)

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.SiteRole) == "" {
		return usage("selector", "admin user create requires --environment, --name, and --site-role; the environment selects the site")
	}
	if (in.AuthSetting == "") == (in.IdPConfigurationID == "") {
		return usage("auth_setting", "admin user create requires exactly one explicit auth setting or IdP configuration ID")
	}
	if in.AuthSetting != "" {
		switch in.AuthSetting {
		case "ServerDefault", "SAML", "OpenID", "TableauIDWithMFA":
		default:
			return usage("auth_setting", "Supported --auth-setting values: ServerDefault, SAML, OpenID, TableauIDWithMFA. Use --idp-configuration-id for an exact authentication configuration.")
		}
	}
	return nil
}
