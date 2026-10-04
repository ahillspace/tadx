package app

import (
	"context"
	"time"

	metric "github.com/ahillspace/tadx/actions/pulse/metric"
	resourcepulse "github.com/ahillspace/tadx/internal/resources/pulse"
)

type pulseMetricReadProvider struct{ commands *pulseCommands }

var _ metric.ReadProvider = pulseMetricReadProvider{}

func (p pulseMetricReadProvider) CacheTarget(alias string) (metric.ReadTarget, error) {
	_, environment, err := p.commands.runtime.environment(alias, false)
	return metric.ReadTarget{Environment: environment.Alias, Site: environment.SiteContentURL}, err
}

func (p pulseMetricReadProvider) CachedList(target metric.ReadTarget) metric.CachedListPort {
	return &resourcepulse.CachedMetricListPort{Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, Support: pulseCacheSupport()}
}

func (p pulseMetricReadProvider) CachedInspect(target metric.ReadTarget) metric.CachedInspectPort {
	return &resourcepulse.CachedMetricInspectPort{Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, Support: pulseCacheSupport()}
}

func (p pulseMetricReadProvider) Open(ctx context.Context, alias, site, operation string) (metric.ReadSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return metric.ReadSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := metric.ReadTarget{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	store := p.commands.runtime.cacheStore(connection.environment)
	return metric.ReadSession{
		ReadTarget: target,
		List:       &resourcepulse.MetricListPort{Client: connection.client, Store: store, Environment: target.Environment, Site: target.Site, Now: p.commands.runtime.now},
		Inspect:    &resourcepulse.MetricInspectPort{Client: connection.client, Store: store, Environment: target.Environment, Site: target.Site, Now: p.commands.runtime.now},
	}, nil
}

func (p pulseMetricReadProvider) CacheSetupError(operation, alias string, err error) error {
	return capabilitySetupError(operation+".cache.setup", operation, alias, "", "Cache Pulse metric setup failed.", "Verify the selected environment and cache configuration.", err)
}

func (p pulseMetricReadProvider) Now() time.Time { return p.commands.runtime.now() }
