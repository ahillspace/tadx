"""Validate offline final or checkpoint G9 accounting; never execute live tasks."""

import argparse
from collections import Counter
import hashlib
import json
import math
from pathlib import Path, PurePosixPath
import re
import sys


MODEL = "gpt-6-luna"
CLASSIFICATIONS = {"completed", "completed_with_workaround", "failed", "excluded"}
TELEMETRY = {"wall_seconds", "model_tokens", "model_turns", "tool_calls"}


class InvalidEvidence(ValueError):
    """A required identity, coverage entry, or evidence reference is invalid."""


def require(condition, message):
    if not condition:
        raise InvalidEvidence(message)


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "Duplicate JSON field")
        result[key] = value
    return result


def parse(text):
    return json.loads(text, object_pairs_hook=unique_object)


def load(path):
    return parse(Path(path).read_text(encoding="utf-8"))


def text(value):
    return isinstance(value, str) and bool(value.strip())


def identity(value):
    require(isinstance(value, dict), "Missing candidate identity")
    require(set(value) == {"source_revision", "source_tree_sha256", "catalog_sha256", "builds"},
            "Candidate identity fields differ from schema")
    require(isinstance(value["source_revision"], str)
            and re.fullmatch(r"[0-9a-f]{40}", value["source_revision"]), "Invalid source revision")
    for name in ("source_tree_sha256", "catalog_sha256"):
        require(isinstance(value[name], str) and re.fullmatch(r"[0-9a-f]{64}", value[name]),
                "Invalid candidate hash")
    require(isinstance(value["builds"], dict) and value["builds"], "Missing candidate builds")
    for platform, sha in value["builds"].items():
        require(isinstance(platform, str) and re.fullmatch(r"[a-z0-9]+/[a-z0-9]+", platform),
                "Invalid build platform")
        require(isinstance(sha, str) and re.fullmatch(r"[0-9a-f]{64}", sha), "Invalid build hash")


def reference(ref, root):
    require(isinstance(ref, dict) and set(ref) == {"path", "sha256"}, "Invalid evidence reference")
    name = ref["path"]
    require(text(name) and "\\" not in name and ":" not in name, "Evidence path must be relative")
    path = PurePosixPath(name)
    require(not path.is_absolute() and all(part not in {".", ".."} for part in name.split("/")),
            "Evidence path escapes its root")
    target = root.joinpath(*path.parts).resolve()
    require(target.is_relative_to(root) and target.is_file(), "Evidence file missing or outside root")
    require(target.stat().st_size > 0 and digest(target) == ref["sha256"], "Evidence hash mismatch or empty file")


def validate(candidate_path, catalog_path, run_path, results_path, evidence_root, build_paths,
             checkpoint_path=None):
    candidate = load(candidate_path)
    identity(candidate)
    require(digest(catalog_path) == candidate["catalog_sha256"], "Catalog differs from candidate")
    require(set(build_paths) == set(candidate["builds"]), "Candidate build platforms differ")
    for platform, path in build_paths.items():
        require(digest(path) == candidate["builds"][platform], "Binary differs from candidate")

    catalog = load(catalog_path)
    require(isinstance(catalog, list) and catalog, "Invalid executable catalog")
    ids = [row["id"] for row in catalog]
    require(len(ids) == len(set(ids)), "Duplicate catalog identifier")
    required = {row["id"] for row in catalog if row.get("owner") == "cli"
                and row.get("implementation") == "implemented" and row.get("command_path")}
    remote_mutations = {row["id"] for row in catalog if row.get("remote_mutation")}
    require(required, "No executable actions in catalog")
    catalog_count = len(required)
    root = Path(evidence_root).resolve(strict=True)
    run = load(run_path)
    require(run.get("schema_version") == 1, "Require evidence schema 1")
    if checkpoint_path is None:
        require(run.get("scope") == "final", "Require final-sweep schema 1")
    else:
        checkpoint = load(checkpoint_path)
        require(isinstance(checkpoint, dict) and set(checkpoint) == {
            "schema_version", "checkpoint_id", "candidate", "required_actions", "scope_review"
        } and checkpoint["schema_version"] == 1 and text(checkpoint["checkpoint_id"]),
                "Invalid checkpoint schema")
        require(checkpoint["candidate"] == candidate, "Checkpoint candidate mismatch")
        selection = checkpoint["required_actions"]
        require(isinstance(selection, list) and selection and all(text(item) for item in selection)
                and len(set(selection)) == len(selection) and set(selection) <= required,
                "Invalid checkpoint action scope")
        reference(checkpoint["scope_review"], root)
        require(run.get("scope") == "checkpoint" and run.get("checkpoint") == checkpoint,
                "Checkpoint scope mismatch")
        required = set(selection)
    require(run.get("candidate") == candidate, "Run candidate identity mismatch")
    require(text(run.get("run_id")), "Missing run ID")
    reference(run.get("prior_gates_evidence"), root)
    gates = run.get("prior_gates")
    require(isinstance(gates, dict) and gates == {f"G{i}": "passed" for i in range(9)},
            "G0-G8 must pass before G9")
    expected = run.get("attempt_ids")
    require(isinstance(expected, list) and expected and all(text(item) for item in expected)
            and len(expected) == len(set(expected)), "Invalid attempt manifest")

    records = [parse(line) for line in Path(results_path).read_text(encoding="utf-8").splitlines()
               if line.strip()]
    require([row.get("attempt_id") for row in records] == expected, "Missing, reordered, or duplicate attempts")
    seen = {}
    last = {}
    covered = set()
    counts = Counter()
    missing_telemetry = 0
    failed = []
    unresolved = []
    workaround_pending = []
    for row in records:
        attempt = row["attempt_id"]
        action = row.get("action_id")
        require(action in required, "Attempt references a non-executable or unknown action")
        require(row.get("run_id") == run["run_id"] and text(row.get("case_id")), "Attempt run/case identity mismatch")
        require(row.get("candidate") == candidate, "Attempt candidate identity mismatch")
        platform = row.get("platform")
        require(platform in candidate["builds"] and row.get("binary_sha256") == candidate["builds"][platform],
                "Attempt binary/platform mismatch")
        previous = last.get(row["case_id"])
        require(row.get("previous_attempt_id") == previous, "Retry chain loses an earlier attempt")
        if previous:
            require(seen[previous]["action_id"] == action, "Retry changes action identity")
        require(type(row.get("attempt_number")) is int
                and row["attempt_number"] == (seen[previous]["attempt_number"] + 1 if previous else 1),
                "Retry attempt number is discontinuous")
        seen[attempt] = row
        last[row["case_id"]] = attempt
        classification = row.get("classification")
        require(classification in CLASSIFICATIONS, "Invalid classification")
        for key, wanted in (("requested_model", MODEL), ("requested_reasoning_effort", "medium"),
                            ("actual_model", MODEL), ("actual_provider", "openai"),
                            ("actual_reasoning_effort", "medium")):
            if classification == "excluded":
                require(key in row and (row[key] is None or text(row[key])),
                        "Excluded model metadata requires an explicit string or null")
            else:
                require(row.get(key) == wanted, "Model/provider/reasoning mismatch")
        require(text(row.get("fixture_id")) and text(row.get("reason")), "Missing fixture provenance or reason")
        require(row.get("confidence") in {"high", "medium", "low"}, "Missing grading confidence")
        require(row.get("harness_agreement") in {"agreed", "disagreed", "unavailable"}, "Missing harness comparison")
        telemetry = row.get("telemetry")
        require(isinstance(telemetry, dict) and set(telemetry) == TELEMETRY, "Telemetry requires explicit fields or null")
        for key, value in telemetry.items():
            require(value is None or (type(value) in (int, float) and math.isfinite(value) and value >= 0
                    and (key == "wall_seconds" or type(value) is int)), "Invalid telemetry value")
        missing_telemetry += any(value is None for value in telemetry.values())
        evidence = row.get("evidence")
        require(isinstance(evidence, dict), "Missing task evidence")
        for key in ("task", "transcript", "commands", "model_metadata", "fixture_authority", "assessment"):
            if classification == "excluded" and key in {"transcript", "commands", "model_metadata"}:
                require(key in evidence, "Unavailable excluded evidence requires explicit null")
                if evidence[key] is None:
                    continue
            reference(evidence.get(key), root)
        cleanup = row.get("cleanup")
        require(isinstance(cleanup, dict) and cleanup.get("status") in
                {"completed", "not_needed", "unresolved", "failed"}, "Invalid cleanup status")
        reference(cleanup.get("evidence"), root)
        if cleanup["status"] in {"unresolved", "failed"}:
            unresolved.append(attempt)
        if classification == "failed":
            failed.append(attempt)
        if classification == "completed_with_workaround":
            if row.get("workaround_review") != "accepted":
                workaround_pending.append(attempt)
            else:
                reference(evidence.get("workaround_review"), root)
        if classification in {"completed", "completed_with_workaround"}:
            proof = row.get("proof")
            require(isinstance(proof, dict) and proof.get("kind") in
                    {"native_effect", "local_state", "native_refusal"}, "Missing requested-outcome proof")
            reference(proof.get("evidence"), root)
            require(type(row.get("intentional_refusal")) is bool, "Missing intentional refusal flag")
            require((proof["kind"] == "native_refusal") == row["intentional_refusal"],
                    "Refusal does not match requested task outcome")
            require(action not in remote_mutations or proof["kind"] != "local_state",
                    "Remote mutation requires native effect or requested refusal evidence")
            if proof["kind"] == "native_effect":
                reference(evidence.get("native_acknowledgement"), root)
                reference(evidence.get("request_response"), root)
            covered.add(action)
        counts[classification] += 1
    missing = sorted(required - covered)
    require(not failed, "Correctness failures remain in run")
    require(not unresolved, "Unresolved cleanup remains in run")
    require(not workaround_pending, "Workaround friction requires explicit acceptance")
    require(not missing, "Required actions lack completed evidence: " + ", ".join(missing))
    return {"status": "evidence_accounting_passed", "scope": run["scope"],
            "catalog_executable_actions": catalog_count, "required_actions": len(required),
            "covered_actions": len(covered), "attempts": len(records),
            "classifications": {name: counts[name] for name in sorted(CLASSIFICATIONS)},
            "attempts_missing_telemetry": missing_telemetry,
            "limits": "Offline accounting only; semantic outcome, fixture authority, and G0-G8 attestations require independent validation."}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("candidate", "catalog", "run", "results", "evidence-root"):
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--build", action="append", required=True, metavar="PLATFORM=PATH")
    parser.add_argument("--checkpoint", type=Path,
                        help="Independent reviewed checkpoint scope; omit for full final catalog")
    args = parser.parse_args(argv)
    try:
        builds = {}
        for item in args.build:
            platform, separator, path = item.partition("=")
            require(separator and path and platform not in builds, "Invalid or duplicate build argument")
            builds[platform] = Path(path)
        result = validate(args.candidate, args.catalog, args.run, args.results, args.evidence_root,
                          builds, checkpoint_path=args.checkpoint)
    except InvalidEvidence as exc:
        print(json.dumps({"status": "blocked", "reason": str(exc)}))
        return 1
    except (OSError, ValueError, TypeError, KeyError, AttributeError):
        print(json.dumps({"status": "blocked", "reason": "Malformed or unreadable input; inspect sanitized inputs locally."}))
        return 1
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
