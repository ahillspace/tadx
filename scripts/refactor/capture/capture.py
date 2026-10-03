"""Capture accepted CLI inputs without building binaries or changing the harness."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess


HEX40 = re.compile(r"[0-9a-f]{40}\Z")
HEX64 = re.compile(r"[0-9a-f]{64}\Z")
PLATFORMS = {"windows": "tadx.exe", "linux": "tadx"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(blob):
    return hashlib.sha256(blob).hexdigest()


def encode(value):
    return (json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n").encode()


def parse(blob):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "Duplicate JSON key")
            result[key] = value
        return result
    return json.loads(blob, object_pairs_hook=unique)


def plain(path, *, missing=False):
    path = Path(os.path.abspath(path))
    for part in [*reversed(path.parents), path]:
        try:
            info = part.lstat()
        except FileNotFoundError:
            require(missing, "Missing input path")
            continue
        require(not stat.S_ISLNK(info.st_mode)
                and not getattr(info, "st_file_attributes", 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT,
                "Symlink or reparse point is not accepted")
    return path.resolve()


def read(path):
    path = plain(path)
    require(path.is_file(), "Input must be a regular file")
    return path.read_bytes()


def relative(name):
    require(isinstance(name, str) and "\\" not in name and ":" not in name
            and not PurePosixPath(name).is_absolute()
            and all(part not in {"", ".", ".."} for part in name.split("/")),
            "Invalid relative path")
    return name


def fingerprint(source):
    """Verify the complete clean checkout and retain the harness compatibility digest."""
    source = plain(source)
    require(source.is_dir(), "Source repository is missing")

    def git(*args):
        result = subprocess.run(["git", "-C", str(source), *args], capture_output=True, check=False)
        require(result.returncode == 0, "Source Git inspection failed")
        return result.stdout

    commit = git("rev-parse", "HEAD").decode("ascii").strip()
    require(bool(HEX40.fullmatch(commit)), "Source commit is invalid")
    top = plain(git("rev-parse", "--show-toplevel").decode("utf-8").strip())
    require(top == source, "Source must be the repository root")
    require(not git("status", "--porcelain", "--untracked-files=all"),
            "Source checkout is dirty")
    names = git("ls-files", "--cached", "-z").split(b"\0")
    committed = {}
    for raw in git("ls-tree", "-r", "-z", "--full-tree", "HEAD").split(b"\0"):
        if not raw:
            continue
        header, filename = raw.split(b"\t", 1)
        mode, kind, object_id = header.split(b" ")
        name = filename.decode("utf-8")
        relative(name)
        require(kind == b"blob" and mode in {b"100644", b"100755", b"120000"}
                and name not in committed, "Source tree contains unsupported Git entry")
        committed[name] = object_id.decode("ascii")
    tracked = {raw.decode("utf-8") for raw in names if raw}
    require(tracked == set(committed), "Source index differs from committed tree")
    full_files = {}
    for raw in sorted(set(names) - {b""}):
        name = raw.decode("utf-8")
        relative(name)
        path = source / name
        full_files[name] = sha(read(path))
    actual = set()
    for directory, dirs, leaves in os.walk(source, followlinks=False):
        current = Path(directory)
        if current == source and ".git" in dirs:
            dirs.remove(".git")
        for dirname in dirs:
            plain(current / dirname)
        for leaf in leaves:
            path = current / leaf
            if path == source / ".git":
                continue
            plain(path)
            actual.add(relative(path.relative_to(source).as_posix()))
    require(actual == set(full_files), "Source contains missing or untracked files")
    require(all("\n" not in name and "\r" not in name for name in full_files),
            "Source contains a path unsupported by batch Git verification")
    ordered = sorted(full_files)
    hashes = subprocess.run(["git", "-C", str(source), "hash-object", "--stdin-paths"],
                            cwd=source, input=("\n".join(ordered) + "\n").encode(),
                            capture_output=True, check=False)
    require(hashes.returncode == 0, "Source Git object verification failed")
    actual_objects = hashes.stdout.decode("ascii").splitlines()
    require(len(actual_objects) == len(ordered)
            and all(actual_objects[index] == committed[name]
                    for index, name in enumerate(ordered)),
            "Source bytes differ from committed Git objects")
    files = {name: digest for name, digest in full_files.items()
             if name.endswith(".go") or name in {"go.mod", "go.sum"}
             or name.startswith("internal/agent/skills/")}
    require("go.mod" in files and "scripts/install.ps1" in full_files
            and "scripts/install.sh" in full_files,
            "Source lacks required build inputs")
    digest = sha(json.dumps(files, sort_keys=True, separators=(",", ":"),
                            ensure_ascii=False).encode())
    full_digest = sha(json.dumps(full_files, sort_keys=True, separators=(",", ":"),
                                 ensure_ascii=False).encode())
    return {"commit": commit, "files": files, "digest": digest,
            "full_files": full_files, "full_digest": full_digest}


def tree(root):
    root = plain(root)
    require(root.is_dir(), "Input tree is missing")
    result = {}
    for path in root.rglob("*"):
        plain(path)
        require(path.is_file() or path.is_dir(), "Input tree has a special file")
        if path.is_file():
            name = relative(path.relative_to(root).as_posix())
            result[name] = read(path)
    return result


def build_fields(blob):
    fields = {}
    for line in blob.decode("utf-8").splitlines():
        parts = line.strip().split("\t")
        if len(parts) == 2 and parts[0] == "build" and "=" in parts[1]:
            key, value = parts[1].split("=", 1)
            require(key not in fields, "Duplicate Go build metadata field")
            fields[key] = value
    return fields


def inspect_build(path):
    result = subprocess.run(["go", "version", "-m", str(path)], capture_output=True,
                            check=False, timeout=30)
    require(result.returncode == 0, "Go could not inspect accepted binary")
    return result.stdout


def capture(source, output, candidate_path, catalog_path, builds, guidance_root,
            help_root, *, inspect=inspect_build):
    source = plain(source)
    output = plain(output, missing=True)
    require(not output.exists() and output.parent.is_dir(), "Output must be a new directory")
    require(not output.is_relative_to(source) and not source.is_relative_to(output),
            "Output overlaps source")
    candidate = parse(read(candidate_path))
    require(isinstance(candidate, dict) and set(candidate) == {
        "source_revision", "source_tree_sha256", "catalog_sha256", "builds"},
        "Invalid candidate identity")
    require(bool(HEX40.fullmatch(candidate["source_revision"]))
            and bool(HEX64.fullmatch(candidate["source_tree_sha256"])),
            "Invalid candidate source identity")
    before = fingerprint(source)
    require(before["commit"] == candidate["source_revision"]
            and before["digest"] == candidate["source_tree_sha256"],
            "Source differs from accepted candidate")
    catalog = read(catalog_path)
    require(sha(catalog) == candidate["catalog_sha256"], "Catalog differs from accepted candidate")
    rows = parse(catalog)
    require(isinstance(rows, list) and all(isinstance(row, dict) for row in rows),
            "Invalid catalog")
    ids = [row.get("id") for row in rows]
    require(all(isinstance(item, str) and item for item in ids) and len(ids) == len(set(ids)),
            "Invalid catalog IDs")
    executable = {row["id"] + ".txt" for row in rows if row.get("owner") == "cli"
                  and row.get("implementation") == "implemented"}
    require(executable, "Catalog has no implemented CLI actions")
    help_files = tree(help_root)
    require(set(help_files) == executable and all(help_files.values()),
            "Help set differs from implemented CLI actions")
    skills = {}
    files = {"registry.json": catalog}
    guidance_root = plain(guidance_root)
    require({p.name for p in guidance_root.iterdir()} == {"tadx", "tadx-pulse"},
            "Guidance root must contain exactly two packages")
    for package in ("tadx", "tadx-pulse"):
        contents = tree(guidance_root / package)
        require("SKILL.md" in contents and any(name.startswith("references/") for name in contents),
                "Guidance package is incomplete")
        skills[package] = {name: sha(blob) for name, blob in contents.items()}
        for name, blob in contents.items():
            files[f"internal/agent/skills/{package}/{name}"] = blob
    for name, blob in help_files.items():
        files["help/" + name] = blob
    require(set(builds) == {"windows/amd64", "linux/amd64"}
            and set(candidate["builds"]) == set(builds),
            "Both accepted platform builds are required")
    binary_entries = {}
    metadata = {}
    for system, filename in PLATFORMS.items():
        platform = system + "/amd64"
        path = plain(builds[platform])
        binary = read(path)
        require(binary and sha(binary) == candidate["builds"][platform],
                "Binary differs from accepted candidate")
        info = inspect(path)
        fields = build_fields(info)
        require(fields.get("vcs.revision") == candidate["source_revision"]
                and fields.get("vcs.modified") == "false"
                and fields.get("GOOS") == system and fields.get("GOARCH") == "amd64",
                "Native build metadata differs from candidate")
        binary_name = f"{system}/{filename}"
        metadata_name = f"{system}/build-metadata.txt"
        files[binary_name] = binary
        files[metadata_name] = info
        binary_entries[system] = {"path": str(output / binary_name), "sha256": sha(binary)}
        metadata[system] = {"path": str(output / metadata_name), "sha256": sha(info)}
    require(fingerprint(source) == before, "Source changed during capture")
    manifest = {
        "source_commit": before["commit"], "source_tree_digest": before["digest"],
        "complete_source_tree_digest": before["full_digest"],
        "complete_source_fingerprint_sha256": sha(encode(before)),
        "capture_kind": "current-worktree-including-uncommitted-changes",
        "binary": binary_entries["windows"]["path"],
        "binary_sha256": binary_entries["windows"]["sha256"],
        "binaries": binary_entries, "registry_sha256": sha(catalog),
        "skills": skills, "help_sha256": {name: sha(blob) for name, blob in help_files.items()},
        "build_metadata": metadata,
    }
    files["complete-source-fingerprint.json"] = encode(before)
    files["manifest.json"] = encode(manifest)
    # Exclusive creation makes a partial result unusable on retry.
    output.mkdir()
    for name, blob in files.items():
        destination = output / relative(name)
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("xb") as stream:
            stream.write(blob)
    return {"status": "captured_offline", "manifest": str(output / "manifest.json"),
            "source_tree_sha256": before["digest"], "files": len(files),
            "live_execution_enabled": False}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("source", "output", "candidate", "catalog", "guidance", "help"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--build", action="append", required=True, metavar="PLATFORM=PATH")
    args = parser.parse_args(argv)
    try:
        builds = {}
        for item in args.build:
            platform, path = item.split("=", 1)
            require(platform not in builds and path, "Duplicate or missing build")
            builds[platform] = Path(path)
        result = capture(args.source, args.output, args.candidate, args.catalog,
                         builds, args.guidance, args.help)
    except (ValueError, OSError, KeyError, TypeError, UnicodeError,
            subprocess.TimeoutExpired):
        print(json.dumps({"status": "blocked", "reason": "Capture inputs failed validation."}))
        return 1
    print(json.dumps(result))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
