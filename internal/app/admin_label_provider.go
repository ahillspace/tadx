package app

import (
	"context"

	labelcategory "github.com/ahillspace/tadx/actions/admin/labelcategory"
	labelvalue "github.com/ahillspace/tadx/actions/admin/labelvalue"
)

type adminLabelCategoryProvider struct{ runtime *runtimeDependencies }

func (p adminLabelCategoryProvider) Open(ctx context.Context, alias, operation string, explicit bool) (labelcategory.LiveSession, error) {
	connection, err := p.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return labelcategory.LiveSession{}, capabilitySetupError(operation+".setup", operation, alias, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	return labelcategory.LiveSession{Target: labelcategory.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}, Ports: p.runtime.clients(connection).metadataAssets}, nil
}

type adminLabelValueProvider struct{ runtime *runtimeDependencies }

func (p adminLabelValueProvider) Open(ctx context.Context, alias, operation string, explicit bool) (labelvalue.LiveSession, error) {
	connection, err := p.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return labelvalue.LiveSession{}, capabilitySetupError(operation+".setup", operation, alias, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	return labelvalue.LiveSession{Target: labelvalue.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}, Ports: p.runtime.clients(connection).metadataAssets}, nil
}
