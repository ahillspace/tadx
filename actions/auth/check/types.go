package check

// Input selects one configured Tableau environment.
type Input struct {
	Environment string
}

// Target contains non-secret sign-in context.
type Target struct {
	Environment       string
	ServerURL         string
	SiteContentURL    string
	APIVersion        string
	PATNameVariable   string
	PATSecretVariable string
}

// Authentication is the non-secret result of PAT sign-in.
type Authentication struct {
	SiteLUID string
	UserLUID string
}

// Output is the stable authenticated target result.
type Output struct {
	Status         string   `json:"status"`
	Environment    string   `json:"environment"`
	ServerURL      string   `json:"server_url"`
	SiteContentURL string   `json:"site_content_url"`
	SiteLUID       string   `json:"site_luid"`
	UserLUID       string   `json:"user_luid,omitempty"`
	Help           []string `json:"help"`
}
