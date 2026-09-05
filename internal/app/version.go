package app

import (
	"context"

	versionget "github.com/ahillspace/tadx/actions/version/get"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	versioncore "github.com/ahillspace/tadx/internal/version"
)

type currentVersion struct{}

func (currentVersion) Current() string { return versioncore.Current() }

type releaseChecker struct{ checker versioncore.Checker }

func (c releaseChecker) Latest(ctx context.Context) (versionget.Release, error) {
	item, err := c.checker.Latest(ctx)
	return versionget.Release{Version: item.Version, URL: item.URL, PublishedAt: item.PublishedAt}, err
}
func newVersionCommand(runtime *runtimeDependencies) *versioncli.Dependencies {
	return &versioncli.Dependencies{Getter: versionget.New(currentVersion{}, releaseChecker{checker: versioncore.Checker{Client: runtime.httpClient}}), Use: registryLeafUse("version.get"), Short: registryShort("version.get")}
}
