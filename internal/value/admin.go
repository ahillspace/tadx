package value

// AdminUser is the normalized user observation shared by Tableau reads and
// administration actions. Public list and mutation projections stay separate.
type AdminUser struct {
	LUID               string `json:"luid"`
	Name               string `json:"name"`
	FullName           string `json:"full_name,omitempty"`
	Email              string `json:"email,omitempty"`
	SiteRole           string `json:"site_role,omitempty"`
	LastLogin          string `json:"last_login,omitempty"`
	ExternalAuthUserID string `json:"external_auth_user_id,omitempty"`
	AuthSetting        string `json:"auth_setting,omitempty"`
	IdentityPoolName   string `json:"identity_pool_name,omitempty"`
	IdPConfigurationID string `json:"idp_configuration_id,omitempty"`
	Language           string `json:"language,omitempty"`
	Locale             string `json:"locale,omitempty"`
	Domain             string `json:"domain,omitempty"`
	RequestID          string `json:"-"`
	MutationStatus     string `json:"-"`
}

// AdminGroup is the normalized Tableau group observation. Direct membership
// and display-only fields belong to the action's enriched record.
type AdminGroup struct {
	LUID                string `json:"luid"`
	Name                string `json:"name"`
	Domain              string `json:"domain,omitempty"`
	MinimumSiteRole     string `json:"minimum_site_role,omitempty"`
	GrantLicenseMode    string `json:"grant_license_mode,omitempty"`
	ExternalUserEnabled *bool  `json:"external_user_enabled,omitempty"`
	RequestID           string `json:"-"`
	MutationStatus      string `json:"-"`
}

type AdminCreateUserRequest struct {
	Name, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID string
	Email, Language, Locale                                           string
}

type AdminUpdateUserRequest struct {
	FullName, Email, SiteRole, AuthSetting, IdentityPoolName *string
	IdPConfigurationID, Language, Locale                     *string
}

type AdminCreateGroupRequest struct {
	Name, MinimumSiteRole string
	ExternalUserEnabled   *bool
}

type AdminUpdateGroupRequest struct {
	Name                *string `json:"name,omitempty"`
	MinimumSiteRole     *string `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool   `json:"external_user_enabled,omitempty"`
}
