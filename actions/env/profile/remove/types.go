package remove

type Input struct {
	Alias string `json:"alias"`
}
type Output struct {
	Status      string   `json:"status"`
	Environment string   `json:"environment"`
	Help        []string `json:"help"`
}
