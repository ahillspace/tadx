# VS Code extension releases and CLI upgrades

The extension and CLI share this repository and have separate packages, versions, and release schedules.
The extension identity remains `ahillspace.tadx`; changing that identity creates a different installation and update path.
The extension's `package.json` and lockfile own its version.
The CLI owns Tableau access, configuration, credentials, artifacts, and operation records.

## Update the extension

For Marketplace installations, use VS Code's normal extension update controls.
Users can enable automatic updates or select a published version manually.
Users who installed a VSIX must enable automatic updates explicitly after moving to a Marketplace installation with the same identity.
VSIX installation disables automatic updates by default.
See the [VS Code update documentation](https://code.visualstudio.com/docs/configure/extensions/extension-marketplace#extension-auto-update).

For manual or offline distribution, download the versioned VSIX and its checksum files from the extension's GitHub release.
Verify `SHA256SUMS`, install the VSIX over the existing extension, and reload the window.
The extension package does not contain a TADX executable.
Updating the extension does not automatically replace the CLI.

## Select and install the CLI

Run **TADX: CLI Status and Setup** from the command palette or the Sites view menu.
The status picker identifies the executable, version, compatibility, and workspace host.
**Choose TADX Executable** validates an existing executable and saves its path in VS Code's machine-scoped setting.
Reload the window when requested to apply that path.

**Install TADX CLI** installs the verified release into the normal shared user location.
The default location is `%LOCALAPPDATA%\Programs\tadx\bin` on Windows or `~/.local/bin` on Linux and macOS.
The same installation can serve the extension, terminals, and coding agents.
On SSH, WSL, and container sessions, installation applies to the workspace host.

Setup presents the exact destination and release before requesting confirmation.
The maintained bootstrap installers also refresh bundled agent Guidance; the confirmation includes that change.
PATH and shell-completion changes are optional and initially unselected.
Credentials and saved Tableau mutation consent are not changed by these actions.

**Update TADX CLI** updates or repairs the shared user installation to the release verified for this extension.
It does not replace a custom or package-manager executable, replace a development build, or downgrade a newer release.
Update custom installations through their installation owner, then select their executable again.
Older CLIs remain usable when they provide the required command contracts.

Finish TADX commands and detached transfers before setup, including work started outside this window.
The extension requests explicit acknowledgment because it cannot enumerate every process or transfer on the host.
Its maintenance lock prevents new extension CLI calls and rejects setup while a CLI invocation is active.
The native installer also protects its installation directory against concurrent replacement.
Cancellation or verification failure reports potentially partial installation; it does not promise rollback of binaries, Guidance, or shared state.

## Compatibility and verified release policy

Activation reads `version` and complete capability metadata through the CLI.
The compatibility check evaluates implementation, preview, and batch support for the commands the extension uses.
Saved mutation consent and administrator policy remain execution restrictions, not CLI version incompatibilities.
Unavailable commands fail before launch while supported commands remain usable.
**Reload Inventory** checks compatibility again after an external update.
JSON parsing continues to validate each actual response because capability declarations alone cannot prove output compatibility.

[`cli-release.json`](../extensions/vscode/cli-release.json) pins the guided setup version and archive SHA-256 for each supported platform.
The bootstrapper checks both the release manifest and this independent archive pin before extraction or installation.
A changed release asset therefore requires a reviewed policy update, even when the tag or advertised CLI version stays the same.
Guided setup does not resolve an arbitrary latest release at confirmation time.

The initial policy selects CLI `1.0.0`.
The Windows amd64 archive has passed the extension's actual CLI read-contract tests.
Other platform hashes come from the same release checksum manifest; those hashes do not establish native runtime coverage.
The manual workflow tests the pinned release on its Linux and Windows runners before publication.
Live Tableau transfers require separate acceptance evidence.

To change the policy, review the official release assets, update their pins, and run both source-built and pinned-release contract tests.
Advance the extension version with that policy change.
Do not silently replace a pin merely because a release asset changed.

## Prepare an extension version

Use numeric `major.minor.patch` versions without suffixes.
Stable releases use even minor versions, such as `0.2.0`; prereleases use odd minor versions, such as `0.3.0`.
Each published build needs a distinct, increasing version.
Keep prerelease versions ahead of stable versions when prerelease testers should remain on that channel.
See [Marketplace prerelease versioning](https://code.visualstudio.com/api/working-with-extensions/publishing-extension#pre-release-extensions).

Change the manifest and lockfile together with npm, without creating a Git tag automatically:

```sh
cd extensions/vscode
npm version patch --no-git-tag-version
```

Use `extension-vX.Y.Z` tags, such as `extension-v0.2.0`.
This prefix does not match the CLI release workflow's `v*.*.*` trigger.
Do not reuse a published extension version or move its tag.
Extension GitHub releases use `--latest=false` so CLI update discovery continues to select a CLI release.

## Validate and package

Run the [extension checks](../extensions/vscode/README.md#develop-and-validate) and relevant [repository checks](../CONTRIBUTING.md) before committing or pushing.
The CLI adapter checks use isolated configuration and fixtures without Tableau credentials or network calls to Tableau.
The pinned-release check requires GitHub release access and verifies the archive before executing it.

```sh
npm run check
npm run test:cli
npm run test:released-cli
npm run test:ui
npm run test:host
node scripts/test-host.mjs --version=1.95.3
npm run release:check -- --tag extension-v0.2.0 --channel stable
npm run release:package -- --tag extension-v0.2.0 --channel stable
npm run release:verify -- --tag extension-v0.2.0 --channel stable
```

Use the current manifest version in the release commands.
The release package contains a versioned VSIX, `release.json`, and `SHA256SUMS` in `extensions/vscode/dist/release`.
Packaging refuses an existing output directory to prevent accidental replacement.
Use a fresh checkout or move the previous local output aside before preparing another package.
Local development can still use `npm run package` to build `dist/tadx.vsix`.

Publication additionally requires `--require-tag`: the exact extension tag must point to HEAD, and the source checkout must be clean.
Artifact verification checks the package identity, version, channel marker, required files, allowlist, source revision, and checksums.
A package recorded as originating from dirty source cannot pass the publication check.

## Publish explicitly

The [VS Code extension workflow](../.github/workflows/vscode-extension.yml) uses `workflow_dispatch` only.
It does not run from ordinary pushes, pull requests, or extension tags.
Select the exact source revision and provide its matching tag and channel.

The workflow supports three actions:

- **validate** runs quality checks and retains a verified package as a workflow artifact.
- **github-release** publishes the verified package and checksums as a new GitHub release.
- **github-and-marketplace** publishes that same package to GitHub and the Marketplace.

The workflow validates current and minimum VS Code hosts, browser scenarios, source and released CLI contracts, and the maintained installer fixtures.
Linux and Windows checks are gates; an unavailable or failed check is not a pass.
Publishing consumes the package produced by those gates without rebuilding it.
Existing releases and assets are not overwritten.

Marketplace publishing requires ownership of the `ahillspace` publisher and a trusted publishing policy for this repository and workflow.
After configuring that policy, set the repository variable `VSCODE_MARKETPLACE_TRUSTED_PUBLISHING=true`.
The publishing job obtains its short-lived identity through `vsce --oidc`; it does not require a stored Marketplace PAT.
See the [official trusted publishing instructions](https://github.com/microsoft/vscode-vsce#trusted-publishing).
No publisher policy, repository variable, release, or Marketplace publication is created by adding this workflow.

If Marketplace publication fails after GitHub publication succeeds, rerun the failed job using the retained verified artifact.
Do not rebuild and replace the already released version.

## Current toolchain advisory

The pinned `vsce` development toolchain currently includes an MSAL Node dependency affected by [GHSA-xcx6-jx8p-x3h5](https://github.com/advisories/GHSA-xcx6-jx8p-x3h5).
The advisory concerns the authorization-code flow; the configured publishing job uses OIDC trusted publishing.
These development dependencies are excluded from the VSIX runtime package.
The current dependency ranges do not offer a patched compatible MSAL release, and `npm audit fix` does not resolve the report.
Retain this verification limit until an upstream-compatible update is available; do not force an untested major dependency override or report a clean audit.

Configuration-page features remain in the [Linux handoff backlog](handoffs/2026-10-24-linux/REQUESTED-CAPABILITIES.md).
