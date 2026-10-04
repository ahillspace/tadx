package app

import (
	"context"

	subscription "github.com/ahillspace/tadx/actions/pulse/subscription"
	resourcepulse "github.com/ahillspace/tadx/internal/resources/pulse"
)

type subscriptionProvider struct{ commands *pulseCommands }

func (p subscriptionProvider) Open(ctx context.Context, environment, site string) (subscription.Session, error) {
	connection, err := p.commands.connect(ctx, environment, false)
	if err != nil {
		return subscription.Session{}, remoteSetupError("pulse.subscription.list", environment, site, connection.environment, err)
	}
	return subscription.Session{
		Environment: connection.environment.Alias,
		Site:        connection.environment.SiteContentURL,
		UserLUID:    connection.userLUID,
		Reader:      resourcepulse.NewSubscriptionReader(connection.client),
	}, nil
}
