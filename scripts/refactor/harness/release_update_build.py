"""Build a private, offline update fixture from an accepted source archive."""

from __future__ import annotations

import argparse
from contextlib import contextmanager
import gzip
import hashlib
import importlib.util
import json
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import tarfile
import tempfile
import time
import zipfile


SOURCE_KEYS = {"schema_version", "source_commit", "source_fingerprint",
               "source_archive_sha256", "go_toolchain"}
BUILDER = "scripts/refactor/harness/release_update_build.py"
ENVIRONMENT = "scripts/refactor/run.py"
TRANSPORT = (
    "scripts/refactor/harness/release_update/gh",
    "scripts/refactor/harness/release_update/curl",
    "scripts/refactor/harness/release_update/transport.cjs",
)
RUNTIME = (
    "scripts/refactor/harness/prepare.py",
    "scripts/refactor/harness/release_update_patches.py",
    "scripts/refactor/harness/release_update/assets.py",
    "scripts/refactor/harness/release_update/profile.py",
    "scripts/refactor/harness/release_update/observer.cjs",
)
RECIPE = (BUILDER, ENVIRONMENT, *TRANSPORT, *RUNTIME)
MAX_SOURCE_FILES = 10000
MAX_SOURCE_BYTES = 1 << 30


class BuildError(ValueError):
    """The accepted source or offline build did not meet its contract."""


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def _unique_pairs(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            raise BuildError("duplicate source-lock field")
        value[key] = item
    return value


def source_lock(path: Path) -> tuple[dict, str]:
    """Read the independently supplied final-source lock, not an asset claim."""
    path = Path(path)
    if path.is_symlink() or not path.is_file():
        raise BuildError("source lock is absent or linked")
    try:
        raw = path.read_bytes()
        lock = json.loads(raw, object_pairs_hook=_unique_pairs)
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise BuildError("source lock is not valid JSON") from error
    if not isinstance(lock, dict) or set(lock) != SOURCE_KEYS or type(lock["schema_version"]) is not int or lock["schema_version"] != 1:
        raise BuildError("source-lock schema differs")
    for key, length in (("source_commit", 40), ("source_fingerprint", 64),
                        ("source_archive_sha256", 64)):
        if not isinstance(lock[key], str) or not re.fullmatch(r"[0-9a-f]{" + str(length) + r"}", lock[key]):
            raise BuildError("source-lock digest differs")
    if not isinstance(lock["go_toolchain"], str) or not re.fullmatch(r"go[0-9]+\.[0-9]+\.[0-9]+", lock["go_toolchain"]):
        raise BuildError("source-lock toolchain differs")
    return lock, hashlib.sha256(raw).hexdigest()


def _safe_name(name: str) -> bool:
    return bool(name) and not name.startswith("/") and "\\" not in name and ":" not in name and \
        all(part not in ("", ".", "..") for part in name.split("/")) and \
        PurePosixPath(name).as_posix() == name


def extract_source(archive: Path, lock: dict, destination: Path) -> dict[str, str]:
    """Validate the ZIP and ordinal source tree before any build runs."""
    archive = Path(archive)
    if archive.is_symlink() or not archive.is_file() or digest(archive) != lock["source_archive_sha256"]:
        raise BuildError("source archive differs from accepted source lock")
    file_hashes = {}
    folded = set()
    total = 0
    try:
        with zipfile.ZipFile(archive) as bundle:
            for entry in bundle.infolist():
                name = entry.filename
                directory = name.endswith("/")
                relative = name[:-1] if directory else name
                if not _safe_name(relative) or relative.casefold() in folded:
                    raise BuildError("source archive has an unsafe or duplicate path")
                folded.add(relative.casefold())
                mode = entry.external_attr >> 16
                if mode & 0o170000 == 0o120000:
                    raise BuildError("source archive contains a link")
                if directory:
                    continue
                if entry.file_size > MAX_SOURCE_BYTES or total + entry.file_size > MAX_SOURCE_BYTES:
                    raise BuildError("source archive exceeds size limit")
                total += entry.file_size
                if len(file_hashes) >= MAX_SOURCE_FILES:
                    raise BuildError("source archive has too many files")
                target = destination / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                value = hashlib.sha256()
                with bundle.open(entry) as reader, target.open("xb") as writer:
                    for chunk in iter(lambda: reader.read(1024 * 1024), b""):
                        writer.write(chunk)
                        value.update(chunk)
                file_hashes[relative] = value.hexdigest()
    except (OSError, zipfile.BadZipFile, RuntimeError) as error:
        if isinstance(error, BuildError):
            raise
        raise BuildError("source archive could not be validated") from error
    records = "".join(f"{name} {file_hashes[name]}\n" for name in sorted(file_hashes, key=lambda value: value.encode("utf-8")))
    if hashlib.sha256(records.encode("utf-8")).hexdigest() != lock["source_fingerprint"]:
        raise BuildError("extracted source fingerprint differs")
    if not {"go.mod", *RECIPE} <= set(file_hashes):
        raise BuildError("accepted source lacks release build inputs")
    if digest(Path(__file__)) != file_hashes[BUILDER]:
        raise BuildError("running builder differs from accepted source")
    return file_hashes


def _environment(home: Path, hashes: dict[str, str]) -> dict[str, str]:
    current = Path(__file__).resolve().parents[3] / ENVIRONMENT
    if digest(current) != hashes[ENVIRONMENT]:
        raise BuildError("running environment helper differs from accepted source")
    specification = importlib.util.spec_from_file_location("_release_build_environment", current)
    if specification is None or specification.loader is None:
        raise BuildError("verified environment helper is unavailable")
    helper = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(helper)
    environment = helper.child_environment(home)
    environment.update({"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0",
                        "GOTELEMETRY": "off", "SOURCE_DATE_EPOCH": "0", "TZ": "UTC"})
    return environment


def _run(command: list[str], *, source: Path, environment: dict[str, str], timeout: int) -> str:
    try:
        result = subprocess.run(command, cwd=source, env=environment, capture_output=True,
                                text=True, timeout=timeout, check=False)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise BuildError("offline Go command did not complete") from error
    if result.returncode:
        raise BuildError("offline Go command failed")
    return result.stdout


def _copy_guidance(source: Path, assets: Path) -> dict[str, str]:
    guidance = {}
    for package in ("tadx", "tadx-pulse"):
        origin = source / "internal" / "agent" / "skills" / package
        if not origin.is_dir() or origin.is_symlink():
            raise BuildError("accepted Guidance package is absent")
        for file in origin.rglob("*"):
            if file.is_symlink():
                raise BuildError("accepted Guidance contains a link")
            if not file.is_file():
                continue
            relative = Path(package) / file.relative_to(origin)
            target = assets / "guidance" / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(file, target)
            guidance[relative.as_posix()] = digest(target)
    if not {"tadx/SKILL.md", "tadx-pulse/SKILL.md"} <= set(guidance):
        raise BuildError("accepted Guidance roots are incomplete")
    return guidance


def _archive_target(binary: Path, archive: Path) -> None:
    with archive.open("xb") as raw:
        with gzip.GzipFile(fileobj=raw, mode="wb", mtime=0, filename="") as compressed:
            with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as bundle:
                member = tarfile.TarInfo("tadx")
                member.size = binary.stat().st_size
                member.mode = 0o755
                member.mtime = 0
                with binary.open("rb") as reader:
                    bundle.addfile(member, reader)


@contextmanager
def _temporary_workspace(parent: Path):
    """Remove exact task-owned scratch, including transient Windows Go files."""
    temporary = Path(tempfile.mkdtemp(prefix="release-build-", dir=parent))
    try:
        yield temporary
    finally:
        if temporary.resolve().parent != parent.resolve():
            raise BuildError("temporary build path escaped output parent")
        for attempt in range(30):
            try:
                shutil.rmtree(temporary)
                break
            except PermissionError as error:
                if attempt == 29:
                    raise BuildError("temporary build state could not be removed") from error
                time.sleep(0.1)


def build(source_archive: Path, lock_path: Path, output: Path) -> dict:
    """Create a new private asset directory, or leave no output on failure."""
    lock, lock_hash = source_lock(lock_path)
    output = Path(output)
    if output.exists() or output.is_symlink() or not output.parent.is_dir():
        raise BuildError("asset output must be a new directory")
    with _temporary_workspace(output.parent) as scratch:
        source = scratch / "source"
        source.mkdir()
        hashes = extract_source(source_archive, lock, source)
        assets = scratch / "assets"
        assets.mkdir()
        environment = _environment(scratch / "home", hashes)
        go_binary = shutil.which("go", path=environment.get("PATH"))
        if not go_binary:
            raise BuildError("offline Go toolchain is absent")
        go_binary_hash = digest(Path(go_binary))
        toolchain = _run(["go", "version"], source=source, environment=environment, timeout=30)
        if not re.match(r"^go version " + re.escape(lock["go_toolchain"]) + r"(?:\s|$)", toolchain):
            raise BuildError("installed Go toolchain differs")
        suffix = lock["source_commit"][:7]
        start = "0.1.3-fixture." + suffix
        target = "0.1.4-fixture." + suffix
        for filename, version in (("baseline-tadx", start), ("target-tadx", target)):
            binary = assets / filename
            flags = "-s -w -X github.com/ahillspace/tadx/internal/version.BuildVersion=" + version + " -buildid="
            _run(["go", "build", "-trimpath", "-buildvcs=false", "-mod=readonly",
                  "-ldflags=" + flags, "-o", str(binary), "./cmd/tadx"],
                 source=source, environment=environment, timeout=300)
            info = _run(["go", "version", "-m", str(binary)],
                        source=source, environment=environment, timeout=30)
            if version.encode("ascii") not in binary.read_bytes() or not all(
                    fact in info for fact in ("GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")):
                raise BuildError("built binary does not match version or target")
        if digest(assets / "baseline-tadx") == digest(assets / "target-tadx"):
            raise BuildError("starting and target binaries are identical")
        guidance = _copy_guidance(source, assets)
        for tracked in TRANSPORT:
            destination = assets / "transport" / Path(tracked).name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source / tracked, destination)
        archive_name = f"tadx_{target}_linux_amd64.tar.gz"
        _archive_target(assets / "target-tadx", assets / archive_name)
        (assets / "checksums.txt").write_text(
            f"{digest(assets / archive_name)}  {archive_name}\n", encoding="utf-8", newline="\n")
        files = {path.relative_to(assets).as_posix(): digest(path)
                 for path in assets.rglob("*") if path.is_file()}
        recipe = {name: hashes[name] for name in RECIPE}
        manifest = {
            "schema_version": 1, "kind": "candidate-built-offline-fixture", "published": False,
            "source_commit": lock["source_commit"], "source_fingerprint": lock["source_fingerprint"],
            "source_archive_sha256": lock["source_archive_sha256"],
            "source_lock_sha256": lock_hash, "starting_version": start, "target_version": target,
            "platform": "linux_amd64",
            "build": {"go_toolchain": lock["go_toolchain"], "goos": "linux", "goarch": "amd64",
                      "go_binary_sha256": go_binary_hash, "cgo_enabled": False,
                      "starting_ldflags_version": start,
                      "target_ldflags_version": target, "recipe": recipe},
            "files": files, "baseline_binary_sha256": files["baseline-tadx"],
            "target_binary_sha256": files["target-tadx"], "archive": archive_name,
            "guidance": guidance,
        }
        (assets / "manifest.json").write_text(
            json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n")
        assets.rename(output)
        return manifest


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-archive", required=True, type=Path)
    parser.add_argument("--source-lock", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args(argv)
    try:
        manifest = build(args.source_archive, args.source_lock, args.output)
    except (BuildError, OSError, ValueError):
        print(json.dumps({"status": "blocked", "reason": "Offline release source or build validation failed."}))
        return 1
    print(json.dumps({"status": "built_offline", "manifest_sha256": digest(args.output / "manifest.json"),
                      "source_commit": manifest["source_commit"]}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
