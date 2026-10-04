package app

import (
	"context"

	"github.com/ahillspace/tadx/actions/contentlabel"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
)

type contentLabelProvider struct{ runtime *runtimeDependencies }

func (p contentLabelProvider) Open(ctx context.Context, environment, operation string, explicit bool) (contentlabel.Session, error) {
	connection, err := p.runtime.tableauConnection(ctx, environment, explicit)
	if err != nil {
		return contentlabel.Session{}, capabilitySetupError(operation+".setup", operation, environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	return contentlabel.Session{
		Environment: connection.environment.Alias,
		Site:        connection.environment.SiteContentURL,
		Ports:       p.runtime.clients(connection).metadataAssets,
	}, nil
}

func contentLabelDependencies(r *runtimeDependencies) *catalogcli.LabelDependencies {
	service := contentlabel.New(contentLabelProvider{runtime: r}, func() error {
		return r.checkManagedCapability("admin.label.value.inspect")
	})
	return &catalogcli.LabelDependencies{Lister: service, Inspector: service, Updater: service, Deleter: service}
}
