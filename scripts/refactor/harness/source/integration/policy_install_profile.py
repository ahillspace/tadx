"""Root-only managed policy install in one network-none disposable Linux overlay."""

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from integration import g9_policy_profile as local
from integration import g9_policy_install_private as private

CASE = "P-policy-install"
ACTION = "policy.install"
BASELINE = "g9-policy-install-baseline.json"
OBSERVATION = "g9-policy-install-observation.json"


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def _case(req):
    require(req.get("case_id") == CASE and req.get("exercise", {}).get("id") == CASE,
            "Undeclared protected policy install case")


def _machine_policy_absent_root(image):
    require(isinstance(image, str) and re.fullmatch(r"sha256:[0-9a-f]{64}", image),
            "Protected installer image is not immutable")
    script = ("for p in /etc/tadx-policy-location.json /etc/tadx/managed-policy.json "
              "/etc/.tadx-policy-install.lock; do [ ! -e \"$p\" ] && [ ! -L \"$p\" ] || exit 1; done")
    try:
        result = subprocess.run(["docker", "run", "--rm", "--read-only", "--cap-drop=ALL",
                                 "--security-opt=no-new-privileges", "--user", "0:0",
                                 "--network", "none", "--entrypoint", "sh", image, "-c", script],
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, timeout=30, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError("Protected machine-policy inspection is unavailable") from exc
    require(result.returncode == 0, "Disposable root image is not free of protected policy state")


def preflight(req, frozen):
    from bench.core import Blocked, load

    try:
        _case(req)
        image = frozen.get("runtime_image")
        rows = load(Path(frozen["root"]) / "registry.json")
        matches = [row for row in rows if isinstance(row, dict) and row.get("id") == ACTION]
        require(len(matches) == 1 and matches[0].get("owner") == "cli" and
                matches[0].get("implementation") == "implemented" and
                matches[0].get("command_path") == ["policy", "install"],
                "Frozen registry lacks exact policy install")
        _machine_policy_absent_root(image)
    except (ValueError, KeyError, TypeError, OSError) as exc:
        raise Blocked(str(exc)) from exc
    return {"qualified_requirements": [], "runtime_network_required": "none",
            "policy_case_id": CASE, "machine_policy_absent": True,
            "inspected_image_id": image, "root_overlay_required": True}


def prepare(req, frozen, work=None):
    from bench.core import Blocked, load, save

    try:
        _case(req)
        root = Path(req["private_case_dir"])
        qualification = load(root / "qualification.json")
        proof = qualification.get("profile", {})
        require(proof.get("policy_case_id") == CASE and proof.get("machine_policy_absent") is True
                and proof.get("root_overlay_required") is True
                and proof.get("inspected_image_id") == qualification.get("image_id"),
                "Protected policy preflight is missing")
        work = Path(work)
        require(work.is_dir() and not work.is_symlink(), "Policy fixture work root is unsafe")
        sentinel = work / "policy-install-sentinel.txt"
        sentinel.write_bytes(b"protected disposable policy fixture\n")
        sentinel_hash = hashlib.sha256(sentinel.read_bytes()).hexdigest()
        guard = {"family": "g9-policy-install", "action": ACTION,
                 "run_id": req["run_id"], "case_id": CASE,
                 "network_none": True, "machine_policy_absent": True,
                 "root_overlay_only": True, "sentinel_sha256": sentinel_hash}
        fixture = {"handle": req["case_id"],
                   "semantic_digest": local.digest({"guard": guard, "image": proof["inspected_image_id"]}),
                   "public": {"output_path": private.POLICY_DIR, "template": "read-only"},
                   "roles": {"sentinel": {"path": sentinel.name}}, "expect": {},
                   "coverage_assertions": [{"id": "native-protected-policy-install",
                                             "actual": "/after/native_command_verified", "op": "eq", "expected": True}]}
        before = {"protected_sha256": sentinel_hash, "machine_policy_absent": True}
        seed = {"version": 1, "environments": {}}
        save(root / BASELINE, {"action": ACTION, "case_id": CASE,
                               "run_id": req["run_id"], "image_id": proof["inspected_image_id"],
                               "guard": guard, "before": before, "config_seed": seed})
    except (ValueError, KeyError, TypeError, OSError) as exc:
        raise Blocked(str(exc)) from exc
    return {"fixture": fixture, "before": before, "config_seed": seed,
            "broker_guard": guard, "runtime_network": "none",
            "harness_report_required": False,
            "baseline_evidence": [BASELINE, "qualification.json"],
            "baseline_assertions": [{"id": "disposable-policy-absent", "status": "pass",
                                     "evidence": ["qualification.json"]}]}


def _native_result(commands):
    from integration.validation_profiles import parse_arguments

    require(isinstance(commands, list) and len(commands) == 1,
            "Exactly one native protected policy install is required")
    command = commands[0]
    require(command.get("capability") == ACTION and command.get("phase") == "task"
            and command.get("executed") is True and command.get("exit_status") == 0
            and not command.get("completion_missing"),
            "Native protected policy install did not complete")
    words, flags = parse_arguments(command.get("argv"))
    expected = {"json": True, "full": True, "template": "read-only", "output": private.POLICY_DIR}
    require(words == ["policy", "install"] and flags == expected and command.get("flags") == expected,
            "Protected policy native argv exceeds exact scope")
    raw = command.get("stdout")
    require(isinstance(raw, str) and 0 < len(raw.encode()) <= 65536,
            "Protected policy native output is absent or oversized")
    result = json.loads(raw)
    require(isinstance(result, dict) and result.get("path") == private.POLICY
            and result.get("template") == "read-only" and result.get("policy_written") is True
            and result.get("locator_published") is True and result.get("active") is True
            and result.get("phase") == "complete", "Protected policy receipt is incomplete")
    return command, result


def _expected_mount_sources(path):
    resolved = Path(path).resolve(strict=True)
    sources = {str(resolved), resolved.as_posix()}
    if os.name == "nt":
        drive = resolved.drive.rstrip(":").lower()
        require(len(drive) == 1 and drive.isalpha(), "Unsupported Docker bind drive")
        tail = "/".join(resolved.parts[1:]).replace("\\", "/")
        sources.update({f"/run/desktop/mnt/host/{drive}/{tail}",
                        f"/host_mnt/{drive}/{tail}"})
    return sources


def _mounts_proven(state, delivery, work):
    cli = delivery.get("cli_mounts")
    worker = delivery.get("mounts")
    if not isinstance(cli, list) or not isinstance(worker, list):
        return False
    expected = {"/work": ("bind", False), "/skills": ("bind", False),
                "/run/tadx-broker": ("volume", True), "/audit": ("volume", True),
                "/cli-state": ("volume", True)}
    if len(cli) != len(expected) or {row.get("Destination") for row in cli} != set(expected):
        return False
    worker_binds = {row.get("Destination"): row for row in worker
                    if isinstance(row, dict) and row.get("Destination") in ("/work", "/skills")}
    if set(worker_binds) != {"/work", "/skills"}:
        return False
    root = state.get("manifest", {}).get("root")
    if not isinstance(root, str) or not root:
        return False
    expected_sources = {"/work": _expected_mount_sources(work),
                        "/skills": _expected_mount_sources(Path(root) / "internal/agent/skills")}
    volume_names = {"/run/tadx-broker": state.get("socket_volume"),
                    "/audit": state.get("volume"), "/cli-state": state.get("config_volume")}
    for row in cli:
        if not isinstance(row, dict):
            return False
        dest = row["Destination"]
        if (row.get("Type"), row.get("RW")) != expected[dest]:
            return False
        if dest in worker_binds:
            source = row.get("Source")
            if (not isinstance(source, str) or not source
                    or worker_binds[dest].get("Type") != "bind"
                    or worker_binds[dest].get("Source") != source
                    or source not in expected_sources[dest]):
                return False
        elif not volume_names[dest] or row.get("Name") != volume_names[dest]:
            return False
    return True


def observe(req, state, config, work, commands):
    from bench.core import Blocked, load, save

    try:
        _case(req)
        root = Path(req["private_case_dir"])
        baseline = load(root / BASELINE)
        delivery = load(root / "delivery-evidence.json")
        cli = delivery.get("cli_host_config", {})
        mounts = delivery.get("cli_mounts", [])
        require(state.get("frozen") is True and state.get("runtime_network") == "none"
                and state.get("cli_audit_complete") is True
                and state.get("broker_container_id") == delivery.get("cli_container_id")
                and baseline.get("run_id") == req["run_id"] and baseline.get("image_id") == delivery.get("image_id"),
                "Protected policy observer is not bound to the frozen broker")
        require(cli.get("NetworkMode") == "none" and cli.get("ReadonlyRootfs") is False
                and cli.get("Privileged") is False and "ALL" in cli.get("CapDrop", [])
                and delivery.get("cli_user") in ("root", "0", "0:0")
                and any("no-new-privileges" in value for value in cli.get("SecurityOpt", [])),
                "Protected policy root overlay isolation is unproved")
        require(_mounts_proven(state, delivery, work),
                "Protected policy broker mount source, volume, or access differs from the isolated worker")
        require(config == baseline["config_seed"], "Disposable CLI config changed")
        capture = state.get("policy_install_capture", {})
        require(capture.get("status") == "copied" and
                capture.get("container_id") == state["broker_container_id"],
                "Stopped broker protected paths were not captured")
        command, result = _native_result(commands)
        identity = command.get("id", "")
        expected_hash = state.get("manifest", {}).get("binaries", {}).get("linux", {}).get("sha256")
        require(re.fullmatch(r"cmd-[0-9]+", identity) and isinstance(expected_hash, str)
                and re.fullmatch(r"[0-9a-f]{64}", expected_hash)
                and command.get("binary_sha256") == expected_hash
                and command.get("evidence") == "audit/commands.jsonl#" + identity
                and command.get("stdout_evidence") == "audit/" + identity + ".stdout.txt",
                "Protected policy install lacks exact native broker and binary evidence")
        target = root / "policy-install-native"
        locator = target / "locator.json"
        policy = target / "policy/managed-policy.json"
        require(locator.is_file() and policy.is_file() and not locator.is_symlink()
                and not policy.is_symlink() and
                hashlib.sha256(locator.read_bytes()).hexdigest() == capture["locator_sha256"] and
                hashlib.sha256(policy.read_bytes()).hexdigest() == capture["policy_sha256"],
                "Protected policy evidence changed after capture")
        document = json.loads(policy.read_bytes())
        registry = load(Path(state["manifest"]["root"]) / "registry.json")
        expected = {row["id"] for row in registry}
        require(set(document) == {"version", "allowed_capabilities", "remote_mutations"}
                and document["version"] == 1 and document["remote_mutations"] is False
                and isinstance(document["allowed_capabilities"], list)
                and len(document["allowed_capabilities"]) == len(expected)
                and set(document["allowed_capabilities"]) == expected,
                "Installed read-only policy differs from the frozen capability registry")
        sentinel = Path(work) / "policy-install-sentinel.txt"
        require(sentinel.is_file() and not sentinel.is_symlink() and
                hashlib.sha256(sentinel.read_bytes()).hexdigest() == baseline["before"]["protected_sha256"],
                "Protected policy fixture sentinel changed")
        after = {"native_command_verified": True, "policy_active": True,
                 "installed_path": private.POLICY, "protected_sha256": baseline["before"]["protected_sha256"],
                 "unowned_changes": []}
        save(root / OBSERVATION, {"after": after, "receipt": result,
                                  "command_evidence": [command.get("evidence")],
                                  "capture_evidence": ["policy-install-native/locator.json",
                                                       "policy-install-native/policy/managed-policy.json"]})
        return after
    except (ValueError, KeyError, TypeError, OSError, json.JSONDecodeError) as exc:
        raise Blocked(str(exc)) from exc


def cleanup(req, state, work=None):
    if state.get("frozen") is not True or state.get("runtime_network") != "none":
        return {"cleanup_status": "blocked", "reason": "Disposable policy broker was not frozen"}
    # The bridge destroys the labeled broker overlay and verifies its absence.
    return {"cleanup_status": "pass", "scope": "Exact stopped broker overlay only",
            "evidence": ["policy-install-native/locator.json", "policy-install-native/policy/managed-policy.json"]}
