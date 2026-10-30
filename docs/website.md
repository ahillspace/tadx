# Website and installer publishing

The website is plain static HTML served by GitHub Pages at `tadx.net`.
Binaries and checksums remain GitHub Release assets; the website serves the bootstrap scripts, not a second binary distribution.
The checked-in website and workflow prepare deployment, but do not enable Pages, change DNS, publish a release, or make the repository public.

## Publish the website

The [Pages workflow](../.github/workflows/pages.yml) runs manually.
An ordinary push to `main` does not deploy the website.
Run its build-only mode to inspect the allowlisted artifact without publishing.
For an authorized deployment, run it from `main` with publication explicitly enabled, then verify the homepage, command browser, security page, and installer URLs.

For an initial deployment, configure GitHub Pages to use GitHub Actions and select `tadx.net` as the custom domain.
Verify domain ownership, configure DNS, and enable HTTPS after certificate provisioning.
Public installation also requires a public repository and an existing release containing the binary archives, checksums, and installer assets.
Test a clean installation on Windows, macOS, and Linux before treating those installation paths as validated.

See GitHub's [custom workflow](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages) and [custom domain](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site) instructions.
A `CNAME` file alone does not configure a custom domain for an Actions deployment.

## Refresh an existing release

The release workflow normally refuses to overwrite an existing tag.
To rebuild an existing release from reviewed `main`, run the manual Release workflow from `main` with its release tag and `replace_existing` enabled.
The workflow moves the tag to the selected `main` commit, replaces the release assets, and updates the release notes with the previous and new commit IDs.
It preserves the prior assets as a 30-day Actions artifact and uploads the new checksum manifest last.
Tell users to rerun the platform installer because version checks cannot distinguish an earlier build with the same version.

## Website files

Edit `site/index.html` for the homepage.
Run `node --test scripts/tests/site_test.mjs` to validate its packaging and installer commands.
Run `node scripts/build-site.mjs` to create `_site` from `index.html`, `security.html`, `capabilities.html`, `capabilities.json`, `install.sh`, and `install.ps1`.
The static packaging test is the required automated check and does not need a browser or npm dependencies.
For an optional real-browser check of the capability map, run `node scripts/check-capability-map.mjs <chromium-browser-path> [page-path]`.
For an optional real-browser check of the homepage, run `node scripts/tests/site_browser.mjs <chromium-browser-path>`.
For an optional real-browser check of the security page, run `node scripts/tests/security_browser.mjs <chromium-browser-path> [screenshot-directory]`.
All three browser checks use a supplied headless Chromium executable; the site and security checks create temporary profiles, and none contacts Tableau or modifies the authored pages.
The output directory must be new or empty; move an earlier local build aside before rebuilding.
The homepage's Docs links open `capabilities.html` on the same site.
The homepage links to `security.html`, the packaged concise security overview.
Its expandable details use native HTML and remain usable without JavaScript.
Its authorization checks use a compact HTML table with no diagram assets or rendering dependencies.
The security page links to the repository's detailed security and managed policy guides for more information.
The build copies the v2 command browser from the canonical `docs/reference/capability-map.html` to `capabilities.html`, alongside its generated `capabilities.json` inventory.
The original map is preserved under `archived/docs/2026-10-30/reference/capability-map.html` and is excluded from the website build.
Maintain only the canonical map; its published URL stays `capabilities.html`.
The command tree, filters, and totals include only registry entries owned by the CLI.
External entries remain in the complete JSON inventory; the page explains Tableau MCP querying and Desktop MCP authoring as companion tools.
Keep the map's authored layout and styling separate from its generated `capability-data` script block and static command totals.
Use `go generate ./internal/capability` when registry changes require a new snapshot.
Repository-only references in the map must use published GitHub URLs; the public build does not copy Markdown guides.
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
