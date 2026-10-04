"""Verify the candidate-built, offline-only update asset closure."""

from __future__ import annotations

import json
import hashlib
from pathlib import Path
import re
import tarfile


SOURCE_LOCK_PATH = Path(__file__).with_name("release_update_source.json")
DEFAULT_ROOT = Path(__file__).resolve().parents[1] / "fixtures" / "release-update"


class AssetError(ValueError):
    """The local candidate asset closure is absent or does not match its pin."""


def source_lock(path: Path = SOURCE_LOCK_PATH) -> dict:
    """Read the preparer-bound accepted source, separate from the asset claim."""
    path = Path(path)
    try:
        if path.is_symlink():
            raise AssetError("candidate source lock is a symlink")
        lock = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        if isinstance(error, AssetError):
            raise
        raise AssetError("candidate source lock is absent or malformed") from error
    if not isinstance(lock, dict) or set(lock) != {"schema_version", "source_commit",
            "source_fingerprint", "source_archive_sha256", "go_toolchain"} or \
            type(lock["schema_version"]) is not int or lock["schema_version"] != 1 or \
            not isinstance(lock["source_commit"], str) or not re.fullmatch(r"[0-9a-f]{40}", lock["source_commit"]) or \
            any(not isinstance(lock[key], str) or not re.fullmatch(r"[0-9a-f]{64}", lock[key]) for key in
                    ("source_fingerprint", "source_archive_sha256")) or \
            not isinstance(lock["go_toolchain"], str) or \
            not re.fullmatch(r"go[0-9]+\.[0-9]+\.[0-9]+", lock["go_toolchain"]):
        raise AssetError("candidate source lock facts are invalid")
    return lock


def load(root: Path = DEFAULT_ROOT, *, expected_source: dict | None = None) -> dict:
    """Return a verified asset manifest or fail closed."""
    root = Path(root)
    if root.is_symlink():
        raise AssetError("candidate asset root is not a plain directory")
    try:
        root = root.resolve(strict=True)
    except OSError as error:
        raise AssetError("candidate asset root is absent") from error
    if not root.is_dir():
        raise AssetError("candidate asset root is not a directory")
    try:
        manifest_path = root / "manifest.json"
        if manifest_path.is_symlink():
            raise AssetError("candidate manifest is a symlink")
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        lock = source_lock()
        if expected_source is not None and lock != expected_source:
            raise AssetError("candidate source lock differs from independently pinned source")
        start_version = "0.1.3-fixture." + lock["source_commit"][:7]
        target_version = "0.1.4-fixture." + lock["source_commit"][:7]
        expected = {
            "schema_version": 1,
            "kind": "candidate-built-offline-fixture",
            "published": False,
            "source_commit": lock["source_commit"],
            "source_fingerprint": lock["source_fingerprint"],
            "source_archive_sha256": lock["source_archive_sha256"],
            "starting_version": start_version,
            "target_version": target_version,
            "platform": "linux_amd64",
        }
        if any(type(manifest.get(key)) is not type(value) or manifest[key] != value
               for key, value in expected.items()):
            raise AssetError("candidate source, objective, or platform provenance differs")
        source_lock_hash = hashlib.sha256(SOURCE_LOCK_PATH.read_bytes()).hexdigest()
        if manifest.get("source_lock_sha256") != source_lock_hash:
            raise AssetError("candidate asset manifest is not bound to the accepted source lock")
        build = manifest.get("build")
        if not isinstance(build, dict) or build.get("go_toolchain") != lock["go_toolchain"] or \
                build.get("goos") != "linux" or build.get("goarch") != "amd64" or \
                build.get("cgo_enabled") is not False or \
                build.get("starting_ldflags_version") != start_version or \
                build.get("target_ldflags_version") != target_version:
            raise AssetError("candidate build provenance differs")
        archive = f"tadx_{target_version}_linux_amd64.tar.gz"
        if manifest.get("archive") != archive:
            raise AssetError("candidate archive name differs")
        files = manifest.get("files")
        guidance = manifest.get("guidance")
        if not isinstance(files, dict) or not isinstance(guidance, dict) or not guidance:
            raise AssetError("candidate file or Guidance closure is absent")
        if not {"tadx/SKILL.md", "tadx-pulse/SKILL.md"} <= set(guidance):
            raise AssetError("both bundled Guidance roots are required")
        if any(not isinstance(path, str) or path.startswith("/") or "\\" in path or
               any(part in ("", ".", "..") for part in path.split("/")) for path in guidance):
            raise AssetError("Guidance manifest contains unsafe paths")
        expected_files = {"baseline-tadx", "target-tadx", archive, "checksums.txt",
                          "transport/gh", "transport/curl", "transport/transport.cjs"} | {
            "guidance/" + path for path in guidance
        }
        if set(files) != expected_files:
            raise AssetError("candidate asset closure has missing or unexpected paths")
        actual_files = set()
        for path in root.rglob("*"):
            if path.is_symlink():
                raise AssetError("candidate closure contains a symlink")
            if path.is_file():
                actual_files.add(path.relative_to(root).as_posix())
        if actual_files != expected_files | {"manifest.json"}:
            raise AssetError("candidate closure contains missing or unlisted files")
        for relative, expected_hash in files.items():
            if not re.fullmatch(r"[0-9a-f]{64}", expected_hash):
                raise AssetError("candidate file digest is malformed")
            actual_hash = hashlib.sha256((root / relative).read_bytes()).hexdigest()
            if actual_hash != expected_hash:
                raise AssetError("candidate asset digest differs: " + relative)
        for relative, expected_hash in guidance.items():
            if files["guidance/" + relative] != expected_hash:
                raise AssetError("Guidance manifest and file closure disagree")
        baseline_hash = files["baseline-tadx"]
        if baseline_hash != manifest.get("baseline_binary_sha256"):
            raise AssetError("starting executable digest differs")
        target_hash = manifest.get("target_binary_sha256")
        if not isinstance(target_hash, str) or not re.fullmatch(r"[0-9a-f]{64}", target_hash) or target_hash == baseline_hash:
            raise AssetError("target executable is absent or identical to starting executable")
        if files["target-tadx"] != target_hash:
            raise AssetError("standalone target executable digest differs")
        checksum = (root / "checksums.txt").read_text(encoding="utf-8")
        if checksum != f"{files[archive]}  {archive}\n":
            raise AssetError("release checksum manifest differs")
        if (root / archive).stat().st_size > 268435456:
            raise AssetError("candidate release archive exceeds installer limit")
        with tarfile.open(root / archive, "r:gz") as bundle:
            members = bundle.getmembers()
            if len(members) != 1 or members[0].name != "tadx" or not members[0].isfile() or \
                    members[0].size > 100000000:
                raise AssetError("candidate archive must contain one bounded executable")
            binary = bundle.extractfile(members[0])
            if binary is None or hashlib.sha256(binary.read()).hexdigest() != target_hash:
                raise AssetError("candidate archive executable differs")
        return manifest
    except (OSError, KeyError, TypeError, ValueError, tarfile.TarError) as error:
        if isinstance(error, AssetError):
            raise
        raise AssetError("candidate asset closure could not be verified") from error
