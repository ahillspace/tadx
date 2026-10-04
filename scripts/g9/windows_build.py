"""Build the one source-locked Windows binary used by the hosted installer fixture."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

from windows_gate import GIT_SHA, Refused, expected_version


TOOLCHAIN = "go1.26.5"
VERSION_SYMBOL = "github.com/ahillspace/tadx/internal/version.BuildVersion"


class BuildRefused(Refused):
    """The candidate source or build evidence differs from the fixed recipe."""


def _run(argv, *, source, environment, timeout):
    try:
        return subprocess.run(argv, cwd=source, env=environment, capture_output=True,
                              text=True, encoding="utf-8", errors="replace",
                              timeout=timeout, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise BuildRefused("Candidate build command did not complete: " +
                           type(exc).__name__) from None


def _output(argv, *, source, environment, timeout=30):
    result = _run(argv, source=source, environment=environment, timeout=timeout)
    if result.returncode:
        raise BuildRefused("Candidate build prerequisite failed")
    return result.stdout.strip()


def build_candidate(source_dir, output_path, source_sha, fixture_version,
                    expected_toolchain=TOOLCHAIN, *, environment=None):
    """Build from a clean exact commit and return the resulting binary SHA-256.

    The caller owns the fresh checkout and output directory. The same recipe
    runs during accepted candidate capture and the hosted Windows setup.
    """
    if (not isinstance(source_sha, str) or not GIT_SHA.fullmatch(source_sha)
            or fixture_version != expected_version(source_sha)
            or not isinstance(expected_toolchain, str)
            or not re.fullmatch(r"go[0-9]+\.[0-9]+\.[0-9]+", expected_toolchain)):
        raise BuildRefused("Candidate revision, fixture version, or toolchain is invalid")
    requested_source = Path(source_dir)
    requested_output = Path(output_path)
    if requested_source.is_symlink() or requested_output.is_symlink():
        raise BuildRefused("Candidate build path is a link")
    source = requested_source.resolve(strict=True)
    output = requested_output.resolve(strict=False)
    if (not source.is_dir() or output == source
            or source in output.parents or output.exists() or not output.parent.is_dir()):
        raise BuildRefused("Candidate build paths are not isolated")
    env = {key: value for key, value in
           (os.environ if environment is None else environment).items()
           if not key.upper().startswith("GIT_")
           and key.upper() not in {"GH_TOKEN", "GITHUB_TOKEN", "G9_GITHUB_TOKEN"}}
    env.update(CGO_ENABLED="0", GOOS="windows", GOARCH="amd64", GOAMD64="v1",
               GOFLAGS="", GOEXPERIMENT="", GOWORK="off", GOTOOLCHAIN="local")
    top = _output(["git", "rev-parse", "--show-toplevel"], source=source,
                  environment=env)
    if not Path(top).samefile(source):
        raise BuildRefused("Candidate source is not the Git root")
    if _output(["git", "rev-parse", "HEAD"], source=source,
               environment=env) != source_sha:
        raise BuildRefused("Candidate source is not the accepted commit")
    if _output(["git", "status", "--porcelain=v1", "--untracked-files=all", "--ignored"],
               source=source, environment=env):
        raise BuildRefused("Candidate source checkout is not clean")
    version = _output(["go", "version"], source=source, environment=env)
    if not re.fullmatch(r"go version " + re.escape(expected_toolchain) + r" [^\s]+", version):
        raise BuildRefused("Candidate Go toolchain differs from the accepted version")
    flags = f"-s -w -buildid= -X {VERSION_SYMBOL}=v{fixture_version}"
    build = _run(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=true",
                  "-ldflags=" + flags, "-o", str(output), "./cmd/tadx"],
                 source=source, environment=env, timeout=600)
    if build.returncode or not output.is_file() or output.is_symlink():
        raise BuildRefused("Exact candidate Windows build failed")
    metadata = _output(["go", "version", "-m", str(output)], source=source,
                       environment=env)
    required = ("vcs.revision=" + source_sha, "vcs.modified=false", "GOOS=windows",
                "GOARCH=amd64", "GOAMD64=v1", "CGO_ENABLED=0")
    missing = [item for item in required if item not in metadata]
    if missing:
        raise BuildRefused("Candidate binary lacks exact build provenance: " +
                           ", ".join(missing))
    if os.name == "nt":
        with tempfile.TemporaryDirectory(prefix="g9-version-", dir=output.parent) as home:
            probe_env = {key: value for key, value in env.items() if key.upper() in {
                "PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC"}}
            for key in ("HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP",
                        "CODEX_HOME"):
                probe_env[key] = home
            probe_env.update(TADX_FEEDBACK_MODE="off", TADX_GUIDANCE_NOTICE="0")
            observed = _run([str(output), "--config", str(Path(home) / "config.yaml"),
                             "--json", "--version"], source=source,
                            environment=probe_env, timeout=30)
            try:
                facts = json.loads(observed.stdout)
            except (TypeError, ValueError):
                facts = None
            if (observed.returncode or not isinstance(facts, dict)
                    or facts.get("version") != fixture_version):
                raise BuildRefused("Candidate binary does not report its declared build version")
    return hashlib.sha256(output.read_bytes()).hexdigest()
