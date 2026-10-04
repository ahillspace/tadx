"""Capture only fixed protected-policy paths from a stopped disposable broker."""

import hashlib
import json
from pathlib import Path
import re

LOCATOR = "/etc/tadx-policy-location.json"
POLICY_DIR = "/etc/tadx"
POLICY = POLICY_DIR + "/managed-policy.json"
ALLOWED_DIFF = {"/etc", "/etc/.tadx-policy-install.lock", LOCATOR, POLICY_DIR, POLICY}
MAX_BYTES = 1 << 20


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def capture(req, state, container, docker):
    """Copy after freeze; no host path is mounted at either protected target."""
    require(state.get("broker_guard", {}).get("family") == "g9-policy-install", "Wrong policy-install case")
    require(container == state.get("broker_container_id") and
            isinstance(container, str) and re.fullmatch(r"[0-9a-f]{64}", container),
            "Stopped broker identity differs")
    root = Path(req["private_case_dir"])
    target = root / "policy-install-native"
    require(not target.exists() and not target.is_symlink(), "Policy evidence path already exists")
    diff = docker("diff", container).stdout.splitlines()
    require(diff and all(len(line) > 2 and line[:2] in {"A ", "C "} and
                         line[2:] in ALLOWED_DIFF for line in diff),
            "Protected installer changed paths outside the exact disposable policy scope")
    target.mkdir()
    docker("cp", container + ":" + LOCATOR, str(target / "locator.json"))
    docker("cp", container + ":" + POLICY_DIR, str(target / "policy"))
    locator = target / "locator.json"
    policy_dir = target / "policy"
    require(locator.is_file() and not locator.is_symlink() and policy_dir.is_dir() and
            not policy_dir.is_symlink() and {item.name for item in policy_dir.iterdir()} == {"managed-policy.json"},
            "Protected policy evidence is incomplete or contains extra entries")
    policy = policy_dir / "managed-policy.json"
    require(policy.is_file() and not policy.is_symlink() and
            0 < locator.stat().st_size <= MAX_BYTES and 0 < policy.stat().st_size <= MAX_BYTES,
            "Protected policy evidence is unsafe or oversized")
    selected = json.loads(locator.read_bytes())
    require(selected == POLICY_DIR, "Protected policy locator selected another directory")
    return {"status": "copied", "container_id": container,
            "diff": diff,
            "locator_sha256": hashlib.sha256(locator.read_bytes()).hexdigest(),
            "policy_sha256": hashlib.sha256(policy.read_bytes()).hexdigest()}
