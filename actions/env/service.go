package env

// Store isolates environment profile configuration reads and guarded updates.
type Store interface {
	ListReader
	GetReader
	Adder
	Updater
	Remover
	DefaultSetter
}

// Service owns named environment profile operations.
type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }
