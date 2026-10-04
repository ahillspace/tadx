# Capture accepted CLI inputs

This offline tool copies accepted Windows and Linux binaries, the candidate catalog, complete Guidance trees, per-command help, and native Go build metadata.
It does not build a binary, run TADX, build a worker image, contact Tableau, launch a model, or modify the external benchmark harness.
It writes only to a new output directory.

The source must be a clean Git repository root at the exact commit recorded in the independently accepted `candidate.json`.
Every file outside `.git` must be tracked, including normally ignored files that could affect a build.
The tool compares each working file with the committed Git object after applying Git's checkout filters.
The capture records hashes for every tracked source file, including embedded installer scripts.
It also computes the existing external harness's narrower Go and Guidance fingerprint for candidate and preparer compatibility.
That narrower digest is not complete source provenance.
The tool verifies each binary hash and reads its native metadata with `go version -m`.
Both records must have the exact revision, platform, architecture, and `vcs.modified=false`.
Matching metadata and hashes support input identity; independent G0 review still needs to establish source-to-build provenance and build flags.

Supply the catalog, help, and installed Guidance from an independently reviewed capture of the accepted build.
The help directory must contain one `<action-id>.txt` file for every implemented CLI catalog entry.
The Guidance root must contain exactly `tadx` and `tadx-pulse`, including each package's `SKILL.md` and references.
Do not supply credential files or private run evidence.

```text
python -B scripts/refactor/capture/capture.py --source accepted/source-repo --output ignored/new-capture --candidate accepted/candidate.json --catalog accepted/capabilities.json --build windows/amd64=accepted/windows/tadx.exe --build linux/amd64=accepted/linux/tadx --guidance accepted/installed-skills --help-dir accepted/help
```

The output `manifest.json` has the shape required by `scripts/refactor/harness/prepare.py`.
The `complete-source-fingerprint.json` file contains relative tracked paths and hashes without copying repository contents.
Keep this file and the original capture manifest as G0 review evidence because the preparer rebases only its established manifest fields.
Preparation still requires separate reviewed authority and consent evidence and returns a blocked integration snapshot.
After preparation, check the copied source and worker inputs against the checked-in source lock and the independent candidate:

```text
python -B scripts/refactor/capture/qualify.py --prepared ignored/new-preparation --candidate accepted/candidate.json
```

The qualifier returns exit code 2 with `copied_inputs_qualified_offline` when its checks pass.
It leaves live execution blocked.
The worker image ID, installed Codex version, runtime composition, actual provider/model/reasoning metadata from the same Luna task session, and G0-G8 evidence still require separate qualification.
Neither copied file hashes nor requested model flags prove those facts.

Run synthetic checks with Python 3.11 or later:

```text
python -B -m unittest discover -s scripts/refactor/capture -p "test_*.py" -v
```
