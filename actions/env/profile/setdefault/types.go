package setdefault

type Input struct {
	Preview bool   `json:"preview,omitempty"`
	Alias   string `json:"alias"`
}
type Output struct {
	WouldChange        *bool    `json:"would_change,omitempty"`
	Status             string   `json:"status"`
	DefaultEnvironment string   `json:"default_environment"`
	Help               []string `json:"help"`
}
