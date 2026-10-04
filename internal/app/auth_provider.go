package app

import (
	"context"

	authops "github.com/ahillspace/tadx/actions/auth"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
)

// authCredentialResolver binds the invocation's configuration snapshot.
type authCredentialResolver struct{ runtime *runtimeDependencies }

func (r authCredentialResolver) Resolve(_ context.Context, alias string) (authops.LoginTarget, error) {
	_, environment, err := r.runtime.environment(alias, true)
	if err != nil {
		return authops.LoginTarget{}, err
	}
	return authops.LoginTarget{
		Environment: environment.Alias, ServerURL: environment.URL,
		SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion,
		PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv,
	}, nil
}

// loginAuthenticator binds command-scoped native authentication mechanics.
type loginAuthenticator struct{ runtime *runtimeDependencies }

func (a loginAuthenticator) Authenticate(ctx context.Context, target authops.LoginTarget, credential authops.LoginCredential) (authops.LoginAuthentication, error) {
	transport := a.runtime.transport(target.APIVersion)
	session, err := a.runtime.commandSessions().AuthenticateCredentials(ctx, coreauth.Target{
		Environment: target.Environment, ServerURL: target.ServerURL,
		SiteContentURL: target.SiteContentURL, Operation: "auth.login",
	}, coreauth.PATCredentials{Name: credential.PATName, Secret: credential.PATSecret}, tableauauth.NewClient(transport))
	if err != nil {
		return authops.LoginAuthentication{}, err
	}
	return authops.LoginAuthentication{SiteLUID: session.SiteLUID(), UserLUID: session.UserLUID()}, nil
}
