# Website and installer publishing

The website is plain static HTML served by GitHub Pages at `tadx.net`.
Binaries and checksums remain GitHub Release assets; the website serves the bootstrap scripts, not a second binary distribution.
The checked-in website and workflow prepare deployment, but do not enable Pages, change DNS, publish a release, or make the repository public.

## Launch checklist

1. Review and merge the combined installer, updater, overview, and Guidance changes.
2. Publish a release containing those changes through the existing release workflow.
3. When ready, make the repository public so unauthenticated installation can download release assets.
4. Configure GitHub Pages to use GitHub Actions and set its custom domain to `tadx.net`.
5. Verify domain ownership and configure DNS using GitHub's current instructions, then enable HTTPS after certificate provisioning.
6. Run the manual Pages workflow from `main` with publication explicitly enabled, then verify the homepage and both installer URLs.
7. Test a clean installation on Windows, macOS, and Linux before describing the public URLs as available.

See GitHub's [custom workflow](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages) and [custom domain](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site) instructions.
A `CNAME` file alone does not configure a custom domain for an Actions deployment.

## Website files

Edit `site/index.html` for the homepage.
Run `node --test scripts/tests/site_test.mjs` to validate its packaging and installer commands.
Run `node scripts/build-site.mjs` to create `_site` from the homepage and the two checked-in installer scripts.
The static packaging test is the required automated check and does not need a browser or npm dependencies.
For an optional real-browser check of the capability map, run `node scripts/check-capability-map.mjs <chromium-browser-path> [page-path]`.
For an optional real-browser check of the homepage, run `node scripts/tests/site_browser.mjs <chromium-browser-path>`.
Both browser checks use a supplied headless Chromium executable, create temporary profiles, and do not contact Tableau or modify the authored pages.
The output directory must be new or empty; move an earlier local build aside before rebuilding.
The homepage's Docs links open `capabilities.html` on the same site.
The homepage also links to the managed policy guide in the repository for administrator deployment details.
The build copies the authored `docs/reference/capability-map.html` and its generated `capabilities.json` inventory; no second command-browser source is maintained.
Keep the map's authored layout and styling separate from its generated `capability-data` script block.
Use `go generate ./internal/capability` when registry changes require a new snapshot.
The allowlist excludes configuration, workspace files, repository source, and other local material from the website artifact.
The Pages workflow builds without publishing by default and permits publication only from `main` in a public repository.

## Installation contract

```powershell
irm https://tadx.net/install.ps1 | iex
```

```sh
curl -fsSL https://tadx.net/install.sh | sh
```

Users may download and inspect either script before executing it.
The installer selects a platform release, verifies its checksum, installs the binary, and installs bundled Guidance for detected agent directories.
Without a detected agent directory, it uses `~/.agents/skills`.
Explicit targets remain available through the downloaded installer's `-Target` or `--target` options.
Each update replaces TADX-owned skill packages, including local edits; users should keep extensions in separate skills.
The installer does not configure Tableau authentication, grant mutation permission, or modify MCP configuration.

`tadx update --check` checks the latest published release without installation changes.
`tadx update` uses its embedded installer to install that release and refresh Guidance, with checksum verification and binary rollback if Guidance installation fails.
Already completed Guidance targets may contain the newer bundle after a failure; the error reports this possibility.
Retained binary backups support recovery and are not the active executable.
Private-repository users still need authenticated GitHub release access until the repository is public.

Run `tadx` after installation to see local setup without contacting Tableau.
Its credential indicators describe configuration, not a verified Tableau session.
