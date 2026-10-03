package project

// Ports names the read and mutation contracts for each project operation.
// Callers provide only the ports used by the operation they invoke.
type Ports struct {
	Provider        Provider
	CreateResolver  CreateResolver
	Creator         CreateCreator
	DeleteResolver  DeleteResolver
	Deleter         Deleter
	InspectResolver InspectResolver
	ListReader      ListReader
	MoveResolver    MoveResolver
	Mover           Mover
	UpdateResolver  UpdateResolver
	Updater         Updater
}

// Service owns project lifecycle decisions and delegates native requests to ports.
type Service struct{ Ports }

// New constructs the project service without contacting Tableau.
func New(ports Ports) *Service {
	return &Service{Ports: ports}
}
