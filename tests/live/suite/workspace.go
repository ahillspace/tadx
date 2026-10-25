package suite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func contentWorkspaceMove(ctx context.Context, c *harness.Case) error {
	source, pull, err := contentPull(ctx, c, "workbook", "pull")
	if err != nil {
		return err
	}
	artifact := contentString(contentMap(pull["artifact"]), "path")
	sourcePath, err := contentSafePath(c.WorkspacePath, artifact)
	if err != nil {
		return err
	}
	before, err := contentSnapshot(sourcePath)
	if err != nil {
		return err
	}
	destinationName := c.NamePrefix + "-destination"
	destinationRoot := filepath.Join(filepath.Dir(c.WorkspacePath), "destination-workspace")
	if _, err = c.Run(ctx, "workspace", "create", destinationName, "--path", destinationRoot); err != nil {
		return err
	}
	if _, err = c.Run(ctx, "workspace", "status", "--workspace", c.Workspace, "--all", "--full"); err != nil {
		return err
	}
	if _, err = c.Run(ctx, "workspace", "artifact", "move", "--source", c.Workspace, "--destination", destinationName, "--artifact", artifact); err != nil {
		return err
	}
	if _, err = os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		return errors.New("artifact move leaves source payload or sidecars")
	}
	destinationPath, err := contentSafePath(destinationRoot, artifact)
	if err != nil {
		return err
	}
	after, err := contentSnapshot(destinationPath)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("artifact move changes payload, identity, or sidecars")
	}
	status, err := c.Run(ctx, "workspace", "status", "--workspace", destinationName, "--all", "--full")
	if err != nil {
		return err
	}
	if !contentHasIdentity(status, source.ID) {
		return errors.New("destination status omits moved artifact")
	}
	p, err := c.Project(ctx, "publish-destination")
	if err != nil {
		return err
	}
	name := c.NamePrefix + "-moved-published"
	if err = c.Intent("workbook", name, p.ID); err != nil {
		return err
	}
	out, err := contentRun(ctx, c, "workbook", "publish", "--workspace", destinationName, "--artifact", artifact, "--project-id", p.ID, "--name", name)
	if err != nil {
		return err
	}
	published, err := contentFindPublished(ctx, c, "workbook", p.ID, name, out)
	if err != nil {
		return err
	}
	a, err := c.API.Download(ctx, "workbook", source.ID)
	if err != nil {
		return err
	}
	b, err := c.API.Download(ctx, "workbook", published.ID)
	if err != nil {
		return err
	}
	binding, err := contentBinding(c, "workbook", "")
	if err != nil {
		return err
	}
	if err = harness.CheckNative(ctx, "workbook", a.Bytes, binding); err != nil {
		return err
	}
	if err = harness.CheckNative(ctx, "workbook", b.Bytes, binding); err != nil {
		return err
	}
	return contentUnchanged(ctx, c, source)
}

func contentWorkspaceDelete(ctx context.Context, c *harness.Case) error {
	source, pull, err := contentPull(ctx, c, "workbook", "pull")
	if err != nil {
		return err
	}
	artifact := contentString(contentMap(pull["artifact"]), "path")
	path, err := contentSafePath(c.WorkspacePath, artifact)
	if err != nil {
		return err
	}
	project, err := c.Project(ctx, "sentinel")
	if err != nil {
		return err
	}
	sentinel, err := contentPublishSeed(ctx, c, "workbook", project, c.NamePrefix+"-sentinel", "")
	if err != nil {
		return err
	}
	if sentinel.ID == source.ID {
		return errors.New("workspace deletion sentinel does not have distinct source identity")
	}
	sentinelPull, err := contentRun(ctx, c, "workbook", "pull", "--workspace", c.Workspace, "--id", sentinel.ID)
	if err != nil {
		return err
	}
	sentinelPath, err := contentSafePath(c.WorkspacePath, contentString(contentMap(sentinelPull["artifact"]), "path"))
	if err != nil {
		return err
	}
	sentinelBefore, err := contentSnapshot(sentinelPath)
	if err != nil {
		return err
	}
	before, err := contentSnapshot(path)
	if err != nil {
		return err
	}
	if _, err = c.Run(ctx, "workspace", "artifact", "delete", "--workspace", c.Workspace, "--artifact", artifact, "--preview"); err != nil {
		return err
	}
	preview, err := contentSnapshot(path)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, preview) {
		return errors.New("local deletion preview changes artifact files")
	}
	if _, err = c.Run(ctx, "workspace", "artifact", "delete", "--workspace", c.Workspace, "--artifact", artifact); err != nil {
		return err
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("local deletion leaves artifact payload or sidecars")
	}
	sentinelAfter, err := contentSnapshot(sentinelPath)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(sentinelBefore, sentinelAfter) {
		return errors.New("local deletion changes unrelated artifact")
	}
	if err = contentUnchanged(ctx, c, source); err != nil {
		return err
	}
	return contentUnchanged(ctx, c, sentinel)
}
