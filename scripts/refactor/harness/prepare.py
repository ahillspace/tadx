"""Prepare a blocked, offline project-harness integration snapshot."""

import argparse
import ast
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import stat

from consent import validate_authority
from project_patches import patch_project


HERE = Path(__file__).resolve().parent
CASES = ["P-project-" + action for action in ("create", "delete", "inspect", "list", "move", "update")]
BLOCKERS = [
    "Copied launcher source provenance must be independently qualified against the exact accepted candidate before its local configuration path can run.",
    "Base worker image ID, installed runtime version, and copied image inputs need independent qualification; no rebuild or binary substitution is permitted.",
    "Copied project consent, ownership, and read adapters have offline checks; complete runtime composition remains unqualified.",
    "Fresh run-owned top-level fixture provisioning, three credential roles, and cleanup need bounded qualification without credential persistence.",
    "Actual provider/model/reasoning metadata is unproven; requested gpt-6-luna/medium arguments are not proof.",
    "Exact candidate G0-G8 and independently reviewed checkpoint scope remain required before live execution.",
]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def encode(value):
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode()


def parse(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "Duplicate JSON key")
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=unique)


def plain_path(path, *, missing=False):
    path = Path(path).absolute()
    for part in [*reversed(path.parents), path]:
        try:
            info = part.lstat()
        except FileNotFoundError:
            require(missing, "Missing required input")
            continue
        require(not stat.S_ISLNK(info.st_mode)
                and not getattr(info, "st_file_attributes", 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT,
                "Symlinks and reparse points are not accepted")
    return path.resolve()


def read(path):
    path = plain_path(path)
    require(path.is_file(), "Input must be a regular file")
    return path.read_bytes()


def relative(name):
    require(isinstance(name, str) and "\\" not in name and ":" not in name
            and not PurePosixPath(name).is_absolute()
            and all(part not in {"", ".", ".."} for part in name.split("/")),
            "Invalid allowlisted relative path")
    return name


def replace_once(source, before, after):
    require(source.count(before) == 1, "Patch anchor differs from qualified source")
    return source.replace(before, after, 1)


def patch_sources(files):
    patches = []

    def patch(path, apply):
        original = files[path]
        # Anchor matching normalizes newlines only after the raw source hash check.
        before = original.decode("utf-8").replace("\r\n", "\n")
        after = apply(before).encode()
        files[path] = after
        patches.append({"path": path, "before_sha256": sha(original), "after_sha256": sha(after)})

    patch("integration/local_broker.cjs", lambda s: replace_once(replace_once(s,
          "if(state.guard?.execution_mode==='disposable_native'&&state.baselinePresent)return true;",
          "if(state.guard?.execution_mode==='disposable_native')return false;"),
          "  if (Object.hasOwn(flags,'config')) return false;",
          "  if(state.guard?.family==='g9-project-read')return require('./g9_project_read_broker.cjs').allowed(parsed,classify(args,state.registry||registry),state);\n"
          "  if (Object.hasOwn(flags,'config')) return false;"))
    patch("integration/project_profiles.py", lambda s: patch_project(s, replace_once))
    # The existing native command invocation and strict project guard stay intact.
    patch("tools/run_spark_suite.py", lambda s: replace_once(replace_once(replace_once(s,
          "config.setdefault('run_constraints',{})['native_cli_execution']=True",
          "config.setdefault('run_constraints',{})['native_cli_execution']=False"),
          "source=fingerprint(manifest['repository_path'])\n"
          "    if source['digest']!=manifest.get('source_tree_digest'):\n"
          "        raise Blocked('TADX working tree changed since capture; rerun -CaptureCli before qualification')",
          "raise Blocked('G9 copied source provenance requires independent G0 qualification')"),
          "def main(argv=None):\n",
          "def main(argv=None):\n    raise RuntimeError('G9 preparation is blocked; see preparation.json. No live dispatch is qualified.')\n"))
    path = "integration/docker_local_bridge.py"

    def bridge(source):
        source = replace_once(source, "def profile(req):\n",
                              "def profile(req):\n"
                              "    if req['exercise']['id'] in ('P-project-list', 'P-project-inspect'):\n"
                              "        from integration import g9_project_read\n"
                              "        return g9_project_read\n")
        source = replace_once(source, "if __name__=='__main__':\n",
                              "if __name__=='__main__':\n"
                              "    raise SystemExit('G9 bridge dispatch is blocked; see preparation.json')\n")
        source = replace_once(source,
                              "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py')",
                              "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py','g9_project_read_broker.cjs')")
        start = "def _seed_site_mutation_consent(config, enabled):\n"
        end = "\n\ndef prepare(req):\n"
        require(source.count(start) == 1 and source.count(end) == 1, "Consent patch anchors differ")
        left, rest = source.split(start)
        _, right = rest.split(end)
        source = left + (
            "def _seed_site_mutation_consent(config, enabled, authority):\n"
            "    from integration.g9_consent import preserve_consent\n"
            "    return preserve_consent(config, enabled, authority)\n"
        ) + end + right
        return replace_once(source,
            "prepared.get('config_seed'), prepared['mutation_policy'] == 'enabled')",
            "prepared.get('config_seed'), prepared['mutation_policy'] == 'enabled',\n"
            "                req.get('run_constraints', {}).get('g9_saved_consent_authority'))")

    patch(path, bridge)
    for action in ("list", "inspect"):
        name = f"suite/exercises/P-project-{action}.json"
        original = files[name]
        exercise = parse(original)
        if action == "list":
            instruction = "List only the direct child projects of ${public.source_project} (parent ID ${public.parent_id}). "
        else:
            instruction = "Inspect only ${public.target_name} (project ID ${public.target_id}) inside ${public.source_project}. "
        exercise["agent"]["prompt"] = instruction + exercise["agent"]["prompt"]
        exercise["agent"]["public_bindings"].update({key: "fixture.public." + key for key in
                                                     ("source_project", "parent_id", "target_id", "target_name")})
        files[name] = encode(exercise)
        patches.append({"path": name, "before_sha256": sha(original), "after_sha256": sha(files[name])})
    name = "suite/index.json"
    original = files[name]
    index = parse(original)
    selected = [row for row in index["exercises"] if row.get("id") in CASES]
    require(len(selected) == len(CASES) and {row["id"] for row in selected} == set(CASES)
            and all(row.get("path") == "suite/exercises/" + row["id"] + ".json" for row in selected),
            "Selected project index differs from reviewed cases")
    index["exercises"] = selected
    index["counts"] = {"exercises": len(CASES)}
    files[name] = encode(index)
    patches.append({"path": name, "before_sha256": sha(original), "after_sha256": sha(files[name])})
    return patches


def validate_source_closure(files, root):
    """Reject missing static local imports and image inputs in the copied source."""
    for name, blob in files.items():
        if name.endswith(".py"):
            tree = ast.parse(blob, filename=name)
            for node in ast.walk(tree):
                modules = []
                if isinstance(node, ast.Import):
                    modules = [alias.name for alias in node.names]
                elif isinstance(node, ast.ImportFrom):
                    base = node.module or ""
                    if node.level:
                        base = name.rsplit("/", 1)[0].replace("/", ".") + ("." + base if base else "")
                    modules = [base] + [base + "." + alias.name for alias in node.names]
                for module in modules:
                    if module.startswith(("bench.", "integration.", "tools.")):
                        target = module.replace(".", "/") + ".py"
                        if target in files:
                            continue
                        if module in {"integration.g9_consent", "integration.g9_project_read"}:
                            require(False, "Copied runtime lacks a generated project module")
                        # A missing module is never inferred from a sibling symbol.
                        require(not (root / target).is_file(), "Copied runtime has an unlocked Python import")
        elif name.endswith(".cjs"):
            for target in re.findall(r"require\(['\"]\./([^'\"]+\.cjs)['\"]\)", blob.decode("utf-8")):
                require("integration/" + target in files, "Copied runtime has an unlocked broker import")
    bridge = files["integration/docker_local_bridge.py"].decode("utf-8")
    image_section = bridge.split("def image_for(m):", 1)[1].split("def runtime_check(", 1)[0]
    for target in re.findall(r"['\"]([^'\"]+\.(?:cjs|py)|Dockerfile\.local)['\"]", image_section):
        require("integration/" + target in files, "Copied worker image input is unlocked")


def validate_candidate(candidate, catalog, builds):
    require(isinstance(candidate, dict) and set(candidate) == {
        "source_revision", "source_tree_sha256", "catalog_sha256", "builds"}, "Invalid candidate identity")
    require(isinstance(candidate["source_revision"], str)
            and re.fullmatch(r"[0-9a-f]{40}", candidate["source_revision"]), "Invalid source revision")
    for key in ("source_tree_sha256", "catalog_sha256"):
        require(isinstance(candidate[key], str) and re.fullmatch(r"[0-9a-f]{64}", candidate[key]),
                "Invalid candidate digest")
    require(sha(catalog) == candidate["catalog_sha256"], "Catalog does not match accepted candidate")
    require(isinstance(candidate["builds"], dict) and set(candidate["builds"]) == set(builds)
            and set(builds) == {"windows/amd64", "linux/amd64"},
            "Exact accepted Windows and Linux builds are required")
    for platform, blob in builds.items():
        require(re.fullmatch(r"[a-z0-9]+/[a-z0-9]+", platform) and blob
                and sha(blob) == candidate["builds"][platform], "Build does not match accepted candidate")
    rows = parse(catalog)
    require(isinstance(rows, list) and all(isinstance(row, dict) for row in rows), "Invalid catalog")
    ids = [row.get("id") for row in rows]
    require(all(isinstance(item, str) for item in ids) and len(set(ids)) == len(ids), "Duplicate catalog IDs")
    executable = {row["id"] for row in rows if row.get("owner") == "cli"
                  and row.get("implementation") == "implemented" and row.get("command_path")}
    require({case.removeprefix("P-").replace("-", ".") for case in CASES} <= executable,
            "Candidate lacks one of the six required project actions")


def capture_inputs(path, candidate, catalog, builds, output):
    """Verify and rebase an existing capture without running or rebuilding TADX."""
    capture_path = plain_path(path)
    raw = read(capture_path)
    source = parse(raw)
    require(isinstance(source, dict) and source.get("source_commit") == candidate["source_revision"]
            and source.get("source_tree_digest") == candidate["source_tree_sha256"],
            "Prebuilt capture belongs to another accepted source")
    require(source.get("capture_kind") == "current-worktree-including-uncommitted-changes",
            "Unsupported prebuilt capture kind")
    capture_root = capture_path.parent
    result = {"candidate/capture/registry.json": read(capture_root / "registry.json")}
    require(sha(result["candidate/capture/registry.json"]) == source.get("registry_sha256")
            and parse(result["candidate/capture/registry.json"]) == parse(catalog),
            "Captured registry differs from accepted catalog")
    entries = source.get("binaries")
    require(isinstance(entries, dict) and set(entries) == {"windows", "linux"},
            "Capture must bind Windows and Linux binaries")
    for system in ("windows", "linux"):
        entry = entries[system]
        require(isinstance(entry, dict) and entry.get("sha256") == candidate["builds"][system + "/amd64"]
                and sha(read(entry["path"])) == entry["sha256"]
                and builds[system + "/amd64"] == read(entry["path"]),
                "Captured binary differs from accepted candidate bytes")
    require(source.get("binary") == entries["windows"]["path"]
            and source.get("binary_sha256") == entries["windows"]["sha256"],
            "Host binary identity differs from captured Windows build")
    skills = source.get("skills")
    require(isinstance(skills, dict) and set(skills) == {"tadx", "tadx-pulse"},
            "Both captured Guidance packages are required")
    for package, hashes in skills.items():
        require(isinstance(hashes, dict) and "SKILL.md" in hashes
                and any(name.startswith("references/") for name in hashes),
                "Captured Guidance package is incomplete")
        root = capture_root / "internal/agent/skills" / package
        actual = {p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file()}
        require(actual == set(hashes), "Captured Guidance file set differs")
        for name, expected in hashes.items():
            relative(name)
            blob = read(root / name)
            require(sha(blob) == expected, "Captured Guidance changed")
            result[f"candidate/capture/internal/agent/skills/{package}/{name}"] = blob
    help_hashes = source.get("help_sha256")
    require(isinstance(help_hashes, dict) and help_hashes, "Captured help is missing")
    expected_help = {row["id"] + ".txt" for row in parse(catalog)
                     if row.get("owner") == "cli" and row.get("implementation") == "implemented"}
    require(set(help_hashes) == expected_help, "Captured help differs from accepted executable commands")
    help_root = capture_root / "help"
    require({p.relative_to(help_root).as_posix() for p in help_root.rglob("*") if p.is_file()}
            == set(help_hashes), "Captured help file set differs")
    for name, expected in help_hashes.items():
        relative(name)
        blob = read(capture_root / "help" / name)
        require(sha(blob) == expected, "Captured help changed")
        result["candidate/capture/help/" + name] = blob
    metadata = source.get("build_metadata")
    require(isinstance(metadata, dict) and set(metadata) == {"windows", "linux"},
            "Both native build metadata records are required")
    adapted_metadata = {}
    for system, entry in metadata.items():
        require(isinstance(entry, dict), "Invalid native build metadata")
        blob = read(entry["path"])
        require(sha(blob) == entry.get("sha256"), "Native build metadata changed")
        info = blob.decode("utf-8")
        fields = {}
        for line in info.splitlines():
            parts = line.strip().split("\t")
            if len(parts) == 2 and parts[0] == "build" and "=" in parts[1]:
                key, value = parts[1].split("=", 1)
                require(key not in fields, "Duplicate native build metadata field")
                fields[key] = value
        require(fields.get("vcs.revision") == candidate["source_revision"]
                and fields.get("GOOS") == system and fields.get("GOARCH") == "amd64",
                "Native build metadata differs from candidate")
        name = f"candidate/capture/build-metadata/{system}.txt"
        result[name] = blob
        adapted_metadata[system] = {"path": str(output / name), "sha256": sha(blob)}
    copied = {system: {"path": str(output / "candidate/builds" / system / "amd64" /
                      ("tadx.exe" if system == "windows" else "tadx")),
                       "sha256": entries[system]["sha256"]} for system in ("windows", "linux")}
    adapted = {"source_commit": candidate["source_revision"],
               "source_tree_digest": candidate["source_tree_sha256"],
               "capture_kind": source["capture_kind"], "binary": copied["windows"]["path"],
               "binary_sha256": copied["windows"]["sha256"], "binaries": copied,
               "registry_sha256": source["registry_sha256"], "skills": skills,
               "help_sha256": help_hashes, "build_metadata": adapted_metadata}
    result["candidate/capture/manifest.json"] = encode(adapted)
    result["source/current-cli.json"] = encode({"manifest": str(output / "candidate/capture/manifest.json"),
                                                "source_tree_digest": candidate["source_tree_sha256"]})
    return result, sha(raw)


def prepare(harness, output, candidate_path, catalog_path, build_paths, capture_path,
            authority_path, consent_evidence_path, *, lock=None):
    root = plain_path(harness)
    require(root.is_dir(), "Harness must be a source directory")
    output = plain_path(output, missing=True)
    require(not output.exists() and output.parent.is_dir(), "Output must be a new directory with an existing parent")
    require(not output.is_relative_to(root) and not root.is_relative_to(output), "Output overlaps harness source")
    lock = parse(read(HERE / "source-lock.json")) if lock is None else lock
    require(isinstance(lock, dict) and lock.get("schema_version") == 1
            and isinstance(lock.get("files"), dict) and lock["files"], "Invalid source lock")
    files = {}
    for name, expected in lock["files"].items():
        relative(name)
        blob = read(root / name)
        require(sha(blob) == expected, "Harness source differs from reviewed lock")
        files[name] = blob
    for case in CASES:
        for folder in ("suite/exercises", "fixtures/profiles"):
            require(parse(files[f"{folder}/{case}.json"]).get("id") == case, "Project definition ID differs")
    candidate = parse(read(candidate_path))
    catalog = read(catalog_path)
    builds = {platform: read(path) for platform, path in build_paths.items()}
    validate_candidate(candidate, catalog, builds)
    captured, capture_sha = capture_inputs(capture_path, candidate, catalog, builds, output)
    authority_blob = read(authority_path)
    authority = parse(authority_blob)
    validate_authority(authority)
    require(sha(read(consent_evidence_path)) == authority["consent_evidence_sha256"],
            "Saved-consent evidence differs from authority input")
    patches = patch_sources(files)
    files["integration/g9_consent.py"] = read(HERE / "consent.py")
    files["integration/g9_project_read.py"] = read(HERE / "project_read.py")
    files["integration/g9_project_read_broker.cjs"] = read(HERE / "project_read_broker.cjs")
    validate_source_closure(files, root)
    output_files = {"source/" + name: blob for name, blob in files.items()}
    output_files.update({"candidate/candidate.json": encode(candidate), "candidate/catalog.json": catalog})
    output_files.update(captured)
    for platform, blob in builds.items():
        output_files[f"candidate/builds/{platform}/tadx" +
                     (".exe" if platform.startswith("windows/") else "")] = blob
    manifest = {
        "schema_version": 1, "status": "prepared_blocked", "live_execution_enabled": False,
        "candidate": candidate, "cases": CASES, "requested_model": "gpt-6-luna",
        "requested_reasoning_effort": "medium", "actual_model_metadata": None,
        "source_lock_sha256": sha(encode(lock)), "source_files": lock["files"],
        "patches": patches, "prepared_files": {name: sha(blob) for name, blob in files.items()},
        "snapshot_files": {name: sha(blob) for name, blob in output_files.items()},
        "authority_sha256": sha(authority_blob),
        "capture_manifest_sha256": capture_sha,
        "consent_evidence_sha256": authority["consent_evidence_sha256"],
        "blockers": BLOCKERS,
    }
    # Everything is validated before creating output. A partial filesystem write
    # cannot gain readiness: the blocked manifest is written first, exclusively.
    output.mkdir()
    with (output / "preparation.json").open("xb") as stream:
        stream.write(encode(manifest))
    for name, blob in output_files.items():
        destination = output / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("xb") as stream:
            stream.write(blob)
    return manifest


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("harness", "output", "candidate", "catalog", "capture", "authority", "consent-evidence"):
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--build", action="append", required=True, metavar="PLATFORM=PATH")
    args = parser.parse_args(argv)
    try:
        builds = {}
        for item in args.build:
            platform, path = item.split("=", 1)
            require(platform not in builds and path, "Duplicate or missing build")
            builds[platform] = Path(path)
        prepare(args.harness, args.output, args.candidate, args.catalog, builds, args.capture,
                args.authority, args.consent_evidence)
    except (ValueError, OSError, KeyError, TypeError, UnicodeError):
        print(json.dumps({"status": "blocked", "reason": "Input or preparation validation failed; no live dispatch occurred."}))
        return 1
    print(json.dumps({"status": "prepared_blocked", "live_execution_enabled": False, "blockers": BLOCKERS}))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
