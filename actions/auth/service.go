package auth

// Ports declares the exact dependencies used by each authentication operation.
// Construction has no credential-store or network effect.
type Ports struct {
	CheckResolver      CheckEnvironmentResolver
	CheckAuthenticator CheckAuthenticator
	LoginResolver      LoginResolver
	LoginAuthenticator LoginAuthenticator
	LoginStore         LoginStore
	LogoutResolver     LogoutResolver
	LogoutStore        LogoutStore
	StatusResolver     StatusResolver
	StatusLookup       StatusLookupEnv
}

// Service owns the named authentication workflows for one command invocation.
type Service struct{ Ports }

// New constructs the authentication service without resolving a target.
func New(ports Ports) *Service { return &Service{Ports: ports} }
