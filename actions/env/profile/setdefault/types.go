package setdefault

type Input struct {
	Alias string `json:"alias"`
}
type Output struct {
	Status             string   `json:"status"`
	DefaultEnvironment string   `json:"default_environment"`
	Help               []string `json:"help"`
}
