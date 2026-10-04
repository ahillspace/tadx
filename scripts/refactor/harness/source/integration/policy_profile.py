"""Disposable, network-none profiles for three non-privileged policy commands.

No policy is installed here. The protected-install case needs a separate root
worker and is deliberately absent from this profile.
"""

import hashlib
import json
import os
from pathlib import Path
import subprocess


CASES = {
    "P-policy-samples": "policy.samples",
    "P-policy-validate": "policy.validate",
    "P-policy-status": "policy.status",
}
BASELINE = "g9-policy-baseline.json"
OBSERVATION = "g9-policy-observation.json"
SENTINEL = "policy-sentinel.txt"
CANDIDATE = "policy-candidate.json"
SAMPLES = "policy-candidates"
BROKER_SENTINEL = "/cli-state/g9-policy-private/sentinel.txt"
BROKER_CANDIDATE = "/cli-state/g9-policy-private/candidate.json"
BROKER_SAMPLES = "/cli-state/g9-policy-private/candidates"
CAPTURED_SAMPLES = "policy-native-samples"
SAMPLE_NAMES = ("read-only.json", "read-write-no-admin.json", "superuser.json")
MAX_BYTES = 1 << 20


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def sha(blob):
    return hashlib.sha256(blob).hexdigest()


def digest(value):
    return sha(json.dumps(value, sort_keys=True, separators=(",", ":")).encode())


def _case(req):
    require(isinstance(req, dict) and req.get("case_id") in CASES and
            isinstance(req.get("exercise"), dict) and
            req["exercise"].get("id") == req["case_id"], "Undeclared local policy case")
    return CASES[req["case_id"]]


def _snapshot(work):
    """Bounded no-follow local state, excluding only model deliverables."""
    work = Path(work)
    require(work.is_dir() and not work.is_symlink(), "Unsafe local policy work root")
    rows = {}
    total = 0
    for directory, dirs, files in os.walk(work, followlinks=False):
        for name in list(dirs) + files:
            path = Path(directory) / name
            relative = path.relative_to(work).as_posix()
            if relative in {"deliverables", "inputs.json", "opencode.json", ".opencode"}:
                if name in dirs:
                    dirs.remove(name)
                continue
            require(not path.is_symlink(), "Local policy fixture contains a symlink")
            if path.is_dir():
                rows[relative] = "directory"
            else:
                require(path.is_file(), "Local policy fixture contains a special file")
                size = path.stat().st_size
                total += size
                require(size <= MAX_BYTES and total <= 4 * MAX_BYTES,
                        "Local policy observation exceeds byte bound")
                rows[relative] = sha(path.read_bytes())
            require(len(rows) <= 32, "Local policy observation exceeds file bound")
    return dict(sorted(rows.items()))


def _machine_policy_absent(image):
    require(isinstance(image, str) and image.startswith("sha256:") and len(image) == 71,
            "Worker image is not pinned by immutable ID")
    # No host mounts, credentials, or network. Check both the fixed locator
    # and default policy path; -L also rejects dangling symlinks.
    script = "for p in /etc/tadx-policy-location.json /etc/tadx/managed-policy.json; do [ ! -e \"$p\" ] && [ ! -L \"$p\" ] || exit 1; done"
    try:
        result = subprocess.run(
            ["docker", "run", "--rm", "--read-only", "--cap-drop=ALL",
             "--security-opt=no-new-privileges", "--user", "65532:65532",
             "--network", "none", "--entrypoint", "sh", image, "-c", script],
            stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL, timeout=30, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError("Independent machine-policy inspection is unavailable") from exc
    require(result.returncode == 0, "Disposable worker has a machine policy or cannot prove absence")


def preflight(req, frozen):
    from bench.core import Blocked, load

    try:
        action = _case(req)
        image = frozen.get("runtime_image") if isinstance(frozen, dict) else None
        root = Path(frozen["root"])
        rows = load(root / "registry.json")
        require(isinstance(rows, list), "Frozen policy registry is not a list")
        matches = [row for row in rows if isinstance(row, dict) and row.get("id") == action]
        require(len(matches) == 1 and matches[0].get("owner") == "cli" and
                matches[0].get("implementation") == "implemented" and
                matches[0].get("command_path") == action.split("."),
                "Frozen registry lacks the exact local policy command")
        _machine_policy_absent(image)
    except (ValueError, KeyError, TypeError, OSError) as exc:
        raise Blocked(str(exc)) from exc
    return {"qualified_requirements": [], "runtime_network_required": "none",
            "policy_case_id": req["case_id"], "machine_policy_absent": True,
            "inspected_image_id": image}


def prepare(req, frozen, work=None):
    from bench.core import Blocked, load, save

    try:
        action = _case(req)
        root = Path(req["private_case_dir"])
        qualified = load(root / "qualification.json")
        proof = qualified.get("profile", {})
        require(proof.get("policy_case_id") == req["case_id"] and
                proof.get("machine_policy_absent") is True and
                proof.get("inspected_image_id") == qualified.get("image_id"),
                "Independent disposable-policy preflight is missing")
        work = Path(work)
        require(work.is_dir() and not work.is_symlink(), "Policy work directory is unavailable")
        (work / SENTINEL).write_bytes(b"Protected local policy fixture sentinel\n")
        if action == "policy.validate":
            (work / CANDIDATE).write_bytes(
                b'{"version":1,"allowed_capabilities":[],"remote_mutations":false}\n')
        before = _snapshot(work)
        seed = {"version": 1, "environments": {}}
        guard = {"family": "g9-local-policy", "action": action,
                 "run_id": req["run_id"], "case_id": req["case_id"],
                 "network_none": True, "preflight_verified": True,
                 "machine_policy_absent": True,
                 "sentinel_sha256": before[SENTINEL],
                 "output_path": BROKER_SAMPLES,
                 "candidate_path": BROKER_CANDIDATE}
        if action == "policy.validate":
            guard["candidate_sha256"] = before[CANDIDATE]
        public = {"candidate_path": guard["candidate_path"]} if action == "policy.validate" else (
            {"output_path": guard["output_path"]} if action == "policy.samples" else {})
        fixture = {"handle": req["case_id"], "public": public, "expect": {},
                   "roles": {"sentinel": {"path": SENTINEL}},
                   "semantic_digest": digest({"action": action, "before": before,
                                              "image": proof["inspected_image_id"]}),
                   "coverage_assertions": []}
        baseline = {"action": action, "run_id": req["run_id"], "case_id": req["case_id"],
                    "image_id": proof["inspected_image_id"], "before": before,
                    "config_seed": seed, "guard": guard}
        save(root / BASELINE, baseline)
    except (ValueError, KeyError, TypeError, OSError) as exc:
        raise Blocked(str(exc)) from exc
    return {"fixture": fixture, "before": {"protected_sha256": before[SENTINEL]},
            "config_seed": seed, "broker_guard": guard, "runtime_network": "none",
            "harness_report_required": False, "baseline_evidence": [BASELINE, "qualification.json"],
            "baseline_assertions": [{"id": "local-policy-absent", "status": "pass",
                                     "evidence": ["qualification.json"]}]}


def _native_result(action, commands):
    from integration.validation_profiles import parse_arguments

    require(isinstance(commands, list) and len(commands) == 1,
            "One exact native policy command is required")
    command = commands[0]
    require(command.get("executed") is True and command.get("capability") == action and
            command.get("exit_status") == 0 and not command.get("completion_missing"),
            "Native policy command was not confirmed successful")
    words, flags = parse_arguments(command.get("argv"))
    expected = {"json": True, "full": True}
    if action == "policy.samples":
        expected["output"] = BROKER_SAMPLES
    positionals = action.split(".")
    if action == "policy.validate":
        positionals.append(BROKER_CANDIDATE)
    require(words == positionals and flags == expected and command.get("flags") == expected,
            "Native policy argv or decoded flags differ from exact scope")
    raw = command.get("stdout")
    require(isinstance(raw, str) and 0 < len(raw) <= 65536,
            "Native policy output is absent or unbounded")
    try:
        result = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ValueError("Native policy JSON output is invalid") from exc
    require(isinstance(result, dict), "Native policy output is not an object")
    return result


def _samples_valid(root, result, registry):
    directory = Path(root) / CAPTURED_SAMPLES
    require(directory.is_dir() and not directory.is_symlink(), "Policy samples directory is absent")
    require({p.name for p in directory.iterdir()} == set(SAMPLE_NAMES),
            "Policy samples are incomplete or have extra files")
    all_ids = {row["id"] for row in registry}
    no_admin = {row["id"] for row in registry
                if not (row.get("administrative") is True and row.get("remote_mutation") is True)}
    expected = {"read-only.json": (all_ids, False),
                "read-write-no-admin.json": (no_admin, True),
                "superuser.json": (all_ids, True)}
    for name, (ids, remote) in expected.items():
        path = directory / name
        require(path.is_file() and not path.is_symlink() and path.stat().st_size <= MAX_BYTES,
                "Policy sample is unsafe or oversized")
        document = json.loads(path.read_bytes())
        require(set(document) == {"version", "allowed_capabilities", "remote_mutations"} and
                document["version"] == 1 and document["remote_mutations"] is remote and
                isinstance(document["allowed_capabilities"], list) and
                set(document["allowed_capabilities"]) == ids and
                len(document["allowed_capabilities"]) == len(ids),
                "Policy sample content differs from frozen capabilities")
    require(result.get("status") == "created" and
            result.get("files") == [BROKER_SAMPLES + "/" + name for name in SAMPLE_NAMES],
            "Native policy sample receipt differs from observed files")


def observe(req, state, config, work, commands):
    from bench.core import Blocked, load, save

    try:
        root = Path(req["private_case_dir"])
        baseline = load(root / BASELINE)
        action = _case(req)
        require(state.get("frozen") is True and state.get("runtime_network") == "none" and
                baseline.get("action") == action and baseline.get("run_id") == req["run_id"] and
                baseline.get("case_id") == req["case_id"],
                "Local policy observation is not bound to a frozen exact case")
        delivery = load(root / "delivery-evidence.json")
        require(delivery.get("cli_host_config", {}).get("NetworkMode") == "none",
                "CLI network-none boundary is unproved")
        mounts = delivery.get("cli_mounts", [])
        require(any(isinstance(mount, dict) and mount.get("Destination") == "/work" and
                    mount.get("RW") is False for mount in mounts),
                "CLI broker still has a writable model-work mount")
        require(delivery.get("policy_private_setup", {}).get("status") == "verified",
                "Broker-private policy fixture was not verified before the task")
        require(config == baseline["config_seed"], "Disposable CLI config changed")
        result = _native_result(action, commands)
        current = _snapshot(work)
        before = baseline["before"]
        require(current.get(SENTINEL) == before.get(SENTINEL),
                "Policy sentinel changed")
        registry = load(Path(state["manifest"]["root"]) / "registry.json")
        require(isinstance(registry, list) and registry and
                all(isinstance(row, dict) and isinstance(row.get("id"), str)
                    for row in registry), "Frozen policy registry is unavailable")
        if action == "policy.samples":
            require(state.get("policy_native_output_capture", {}).get("status") == "copied",
                    "Stopped broker policy output was not captured")
            _samples_valid(root, result, registry)
            require(current == before, "Policy samples changed model-visible fixture state")
            status = "created"
        else:
            require(current == before, "Local policy case changed fixture files")
            if action == "policy.validate":
                require(result.get("status") == "valid" and
                        result.get("candidate") == BROKER_CANDIDATE and
                        result.get("allowed_capabilities") == 0 and
                        result.get("remote_mutations") is False,
                        "Policy candidate validation result differs from fixture")
                status = "valid"
            else:
                require(result.get("state") == "unmanaged" and
                        result.get("candidate_valid") is False,
                        "Policy status differs from independently absent machine policy")
                status = "unmanaged"
        after = {"policy_contract_passed": True, "native_result_status": status,
                 "protected_sha256": current[SENTINEL], "unowned_changes": []}
        save(root / OBSERVATION, {"after": after, "action": action,
                                  "command_evidence": [commands[0].get("evidence")],
                                  "network_evidence": ["delivery-evidence.json"]})
        return after
    except (ValueError, KeyError, TypeError, OSError, json.JSONDecodeError) as exc:
        raise Blocked(str(exc)) from exc


def cleanup(req, state, work=None):
    from bench.core import load, save

    if state.get("frozen") is not True or work is None:
        return {"cleanup_status": "blocked", "reason": "Frozen local boundary is unproved"}
    root = Path(req["private_case_dir"])
    try:
        baseline = load(root / BASELINE)
        delivery = load(root / "delivery-evidence.json")
        require(baseline.get("case_id") == req["case_id"] and
                delivery.get("cli_host_config", {}).get("NetworkMode") == "none",
                "Local policy cleanup is not bound to an isolated exact case")
        current = _snapshot(work)
        before = baseline["before"]
        differences = sorted(key for key in current.keys() | before.keys()
                             if current.get(key) != before.get(key))
    except (ValueError, KeyError, TypeError, OSError) as exc:
        return {"cleanup_status": "blocked", "reason": str(exc)}
    result = {"cleanup_status": "pass", "remote_allocations": [],
              "local_state_differences": differences, "case_work_reusable": False,
              "parent_must_dispose_owned_containers": True,
              "evidence": [BASELINE, "delivery-evidence.json"],
              "retained": ["agent-public", "audit"]}
    save(root / "g9-policy-cleanup.json", result)
    return result
