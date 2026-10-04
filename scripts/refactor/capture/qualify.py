"""Check copied project runtime inputs without building images or dispatching tasks."""

import argparse
import ast
import json
from pathlib import Path
import re

from capture import build_fields, encode, parse, plain, read, relative, require, sha


BLOCKERS = [
    "Independent G0 source-to-build provenance review is required.",
    "The immutable base and final worker image IDs and installed runtime require inspection.",
    "The copied broker, consent, fixture, and cleanup composition requires runtime qualification.",
    "Actual provider, model, and reasoning effort require evidence from the same Luna task session.",
    "G0-G8 and reviewed checkpoint scope must pass before live dispatch.",
]
HERE = Path(__file__).resolve().parent


def broker_inputs(bridge):
    """Extract the reviewed image_for broker-file expression from Python syntax."""
    module = ast.parse(bridge)
    functions = [node for node in module.body if isinstance(node, ast.FunctionDef)
                 and node.name == "image_for"]
    require(len(functions) == 1, "Expected one image_for function")
    assignments = [node for node in functions[0].body if isinstance(node, ast.Assign)
                   and any(isinstance(target, ast.Name) and target.id == "broker_files"
                           for target in node.targets)]
    require(len(assignments) == 2, "Worker image broker inputs changed")
    first = ast.literal_eval(assignments[0].value)
    second = assignments[1].value
    require(isinstance(second, ast.Tuple) and len(second.elts) >= 2
            and isinstance(second.elts[0], ast.Starred)
            and isinstance(second.elts[0].value, ast.Name)
            and second.elts[0].value.id == "broker_files",
            "Worker image broker extension changed")
    extension = [ast.literal_eval(item) for item in second.elts[1:]]
    names = [*first, *extension]
    require(all(isinstance(name, str) and "/" not in name and "\\" not in name
                for name in names) and len(names) == len(set(names))
            and "Dockerfile.local" in names and "g9_project_read_broker.cjs" in names
            and "credential_broker.cjs" in names,
            "Worker image input list is incomplete")
    return names


def qualify(root, candidate_path, *, lock=None):
    root = plain(root)
    require(root.is_dir(), "Prepared snapshot is missing")
    record = parse(read(root / "preparation.json"))
    require(record.get("schema_version") == 1 and record.get("status") == "prepared_blocked"
            and record.get("live_execution_enabled") is False,
            "Prepared snapshot is not blocked")
    expected = record.get("snapshot_files")
    require(isinstance(expected, dict) and expected, "Prepared file manifest is missing")
    actual = {}
    for path in root.rglob("*"):
        plain(path)
        require(path.is_file() or path.is_dir(), "Snapshot contains a special file")
        if path.is_file() and path != root / "preparation.json":
            name = relative(path.relative_to(root).as_posix())
            actual[name] = sha(read(path))
    require(actual == expected, "Prepared file set or bytes differ from manifest")
    prepared = record.get("prepared_files")
    source = record.get("source_files")
    patches = record.get("patches")
    require(isinstance(prepared, dict) and isinstance(source, dict)
            and isinstance(patches, list) and set(prepared) == {
                name.removeprefix("source/") for name in expected
                if name.startswith("source/") and name != "source/current-cli.json"},
            "Prepared source accounting differs")
    lock = parse(read(HERE.parent / "harness/source-lock.json")) if lock is None else lock
    require(source == lock["files"] and record.get("source_lock_sha256") == sha(encode(lock)),
            "Prepared source differs from reviewed lock")
    patched = {row["path"]: row for row in patches}
    require(len(patched) == len(patches) and set(patched) <= set(prepared),
            "Patch accounting differs")
    for name, digest in prepared.items():
        require(actual["source/" + name] == digest, "Prepared source hash differs")
        if name in patched:
            require(patched[name]["before_sha256"] == source.get(name)
                    and patched[name]["after_sha256"] == digest,
                    "Patch provenance differs")
        elif name in source:
            require(source[name] == digest, "Unpatched source changed")
    require(set(source) <= set(prepared), "Locked source is missing")
    runner = parse(read(root / "source/runner.local.json"))
    require(isinstance(runner, dict) and isinstance(runner.get("runtime"), dict)
            and runner["runtime"].get("kind") == "bridge"
            and runner["runtime"].get("provider") == "openai"
            and runner["runtime"].get("model") == "gpt-6-luna"
            and runner["runtime"].get("reasoning_effort") == "medium"
            and record.get("runner_config_sha256") == prepared.get("runner.local.json")
            and isinstance(record.get("runner_config_input_sha256"), str)
            and re.fullmatch(r"[0-9a-f]{64}", record["runner_config_input_sha256"])
            and patched.get("runner.local.json", {}).get("before_sha256") is None,
            "Prepared runner configuration differs from Luna medium request")
    for name, original in (("g9_consent.py", "consent.py"),
                           ("g9_runtime_effective.py", "runtime_effective.py"),
                           ("g9_project_read.py", "project_read.py"),
                           ("g9_project_read_broker.cjs", "project_read_broker.cjs")):
        require(read(root / "source/integration" / name)
                == read(HERE.parent / "harness" / original),
                "Generated project guard differs from reviewed source")
    launcher = read(root / "source/tools/run_spark_suite.py").decode("utf-8")
    bridge_path = root / "source/integration/docker_local_bridge.py"
    bridge = read(bridge_path).decode("utf-8")
    require("raise RuntimeError('G9 preparation is blocked; see preparation.json. No live dispatch is qualified.')"
            in launcher and "raise SystemExit('G9 bridge dispatch is blocked; see preparation.json')"
            in bridge and "native_cli_execution']=False" in launcher
            and "requested_model != 'gpt-6-luna' or requested_effort != 'medium'" in launcher,
            "Live dispatch or native CLI block is missing")
    candidate = parse(read(root / "candidate/candidate.json"))
    require(candidate == record.get("candidate")
            and candidate == parse(read(candidate_path)), "Candidate identity differs")
    catalog = read(root / "candidate/catalog.json")
    require(sha(catalog) == candidate["catalog_sha256"], "Candidate catalog changed")
    capture = parse(read(root / "candidate/capture/manifest.json"))
    pointer = parse(read(root / "source/current-cli.json"))
    require(pointer == {"manifest": str(root / "candidate/capture/manifest.json"),
                        "source_tree_digest": candidate["source_tree_sha256"]},
            "Copied CLI pointer differs")
    require(capture.get("source_commit") == candidate["source_revision"]
            and capture.get("source_tree_digest") == candidate["source_tree_sha256"],
            "Captured source identity differs")
    full_blob = read(root / "candidate/capture/complete-source-fingerprint.json")
    full = parse(full_blob)
    require(sha(full_blob) == capture.get("complete_source_fingerprint_sha256")
            and isinstance(full, dict) and full.get("commit") == candidate["source_revision"]
            and full.get("digest") == candidate["source_tree_sha256"]
            and full.get("full_digest") == capture.get("complete_source_tree_digest")
            and full.get("full_ordinal_sha256") == capture.get("complete_source_ordinal_sha256"),
            "Copied complete source fingerprint differs")
    require(capture.get("registry_sha256") == sha(catalog)
            and read(root / "candidate/capture/registry.json") == catalog,
            "Captured registry differs")
    require(capture.get("binary") == capture["binaries"]["windows"]["path"]
            and capture.get("binary_sha256") == candidate["builds"]["windows/amd64"],
            "Captured host binary differs")
    for system, filename in (("windows", "tadx.exe"), ("linux", "tadx")):
        path = root / "candidate/builds" / system / "amd64" / filename
        entry = capture["binaries"][system]
        require(plain(entry["path"]) == path and sha(read(path)) == entry["sha256"]
                == candidate["builds"][system + "/amd64"],
                "Copied binary differs from candidate")
        metadata = capture["build_metadata"][system]
        info_path = root / "candidate/capture/build-metadata" / (system + ".txt")
        require(plain(metadata["path"]) == info_path and sha(read(info_path)) == metadata["sha256"],
                "Copied build metadata differs")
        fields = build_fields(read(info_path))
        require(fields.get("vcs.revision") == candidate["source_revision"]
                and fields.get("vcs.modified") == "false"
                and fields.get("GOOS") == system and fields.get("GOARCH") == "amd64",
                "Copied build metadata identity differs")
    rows = parse(catalog)
    expected_help = {row["id"] + ".txt" for row in rows if row.get("owner") == "cli"
                     and row.get("implementation") == "implemented"}
    actual_help = {name.removeprefix("candidate/capture/help/") for name in actual
                   if name.startswith("candidate/capture/help/")}
    require(set(capture["help_sha256"]) == expected_help == actual_help,
            "Copied help set differs")
    for name, digest in capture["help_sha256"].items():
        relative(name)
        require(sha(read(root / "candidate/capture/help" / name)) == digest,
                "Copied help changed")
    require(set(capture["skills"]) == {"tadx", "tadx-pulse"}, "Guidance packages differ")
    for package, hashes in capture["skills"].items():
        require("SKILL.md" in hashes and any(name.startswith("references/") for name in hashes),
                "Copied Guidance is incomplete")
        prefix = "candidate/capture/internal/agent/skills/" + package + "/"
        require({name.removeprefix(prefix) for name in actual if name.startswith(prefix)} == set(hashes),
                "Copied Guidance file set differs")
        for name, digest in hashes.items():
            relative(name)
            require(sha(read(root / "candidate/capture/internal/agent/skills" / package / name)) == digest,
                    "Copied Guidance changed")
    names = broker_inputs(bridge)
    image_hashes = {name: sha(read(root / "source/integration" / name)) for name in names}
    dockerfile = read(root / "source/integration/Dockerfile.local").decode("utf-8")
    require("COPY tadx /opt/tadx" in dockerfile and "COPY registry.json /opt/registry.json" in dockerfile
            and "COPY *_broker.cjs /opt/" in dockerfile
            and "COPY credential_pty.py /opt/" in dockerfile,
            "Worker Dockerfile input copies differ")
    return {"status": "copied_inputs_qualified_offline", "live_execution_enabled": False,
            "candidate": candidate, "worker_input_sha256": sha(encode({
                "binary": candidate["builds"]["linux/amd64"],
                "registry": capture["registry_sha256"], "integration": image_hashes})),
            "worker_input_files": image_hashes, "blockers": BLOCKERS}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepared", type=Path, required=True)
    parser.add_argument("--candidate", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        result = qualify(args.prepared, args.candidate)
    except (ValueError, OSError, KeyError, TypeError, UnicodeError, SyntaxError):
        print(json.dumps({"status": "blocked", "reason": "Copied runtime inputs failed validation."}))
        return 1
    print(json.dumps(result))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
