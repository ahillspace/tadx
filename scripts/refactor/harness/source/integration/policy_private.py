"""Seed and retain policy fixtures inside the CLI broker's private volume.

The model worker never mounts this volume. The broker's /work mount is read-only.
"""

import hashlib
import json
from pathlib import Path


PRIVATE = "/cli-state/g9-policy-private"
SENTINEL = PRIVATE + "/sentinel.txt"
CANDIDATE = PRIVATE + "/candidate.json"
SAMPLES = PRIVATE + "/candidates"
CAPTURED = "policy-native-samples"


def _sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def setup(req, state, work, docker):
    from bench.core import Blocked

    guard = state.get("broker_guard", {})
    if guard.get("family") != "g9-local-policy" or guard.get("case_id") != req.get("case_id"):
        raise Blocked("Broker-private policy setup lacks an exact case guard")
    broker = state["broker_container"]
    sources = [(Path(work) / "policy-sentinel.txt", SENTINEL, guard.get("sentinel_sha256"))]
    if guard.get("action") == "policy.validate":
        sources.append((Path(work) / "policy-candidate.json", CANDIDATE,
                        guard.get("candidate_sha256")))
    for path, destination, expected in sources:
        if not path.is_file() or path.is_symlink() or _sha(path) != expected:
            raise Blocked("Policy seed changed before private broker setup")
    if docker("exec", broker, "sh", "-c",
              "test ! -e '" + PRIVATE + "' && test ! -L '" + PRIVATE + "'",
              check=False).returncode:
        raise Blocked("Broker-private policy directory already exists")
    docker("exec", broker, "mkdir", "-m", "0700", PRIVATE)
    for _, destination, _ in sources:
        absent = docker("exec", broker, "sh", "-c",
                        "test ! -e '" + destination + "' && test ! -L '" + destination + "'",
                        check=False)
        if absent.returncode:
            raise Blocked("Broker-private policy path is already occupied")
    if guard.get("action") == "policy.samples" and docker(
            "exec", broker, "sh", "-c",
            "test ! -e '" + SAMPLES + "' && test ! -L '" + SAMPLES + "'",
            check=False).returncode:
        raise Blocked("Broker-private policy output already exists")
    for path, destination, _ in sources:
        docker("cp", str(path), broker + ":" + destination)
    script = ("const f=require('fs'),c=require('crypto');"
              "const paths=JSON.parse(process.argv[1]);"
              "process.stdout.write(JSON.stringify(paths.map(p=>"
              "c.createHash('sha256').update(f.readFileSync(p)).digest('hex'))));")
    observed = json.loads(docker("exec", broker, "node", "-e", script,
                                 json.dumps([destination for _, destination, _ in sources])).stdout)
    if observed != [expected for _, _, expected in sources]:
        raise Blocked("Broker-private policy seed readback differs from exact fixture")
    return {"status": "verified", "broker_container_id": state["broker_container_id"],
            "paths": [destination for _, destination, _ in sources],
            "sha256": observed}


def capture(req, state, broker_identity, docker):
    from bench.core import Blocked

    guard = state.get("broker_guard", {})
    if guard.get("family") != "g9-local-policy":
        raise Blocked("Policy output capture lacks its exact guard")
    if guard.get("action") != "policy.samples":
        return {"status": "not_applicable"}
    target = Path(req["private_case_dir"]) / CAPTURED
    if target.exists() or target.is_symlink():
        raise Blocked("Policy output capture target already exists")
    # The caller has stopped and re-identified this exact broker container.
    copied = docker("cp", broker_identity + ":" + SAMPLES, str(target), check=False)
    if copied.returncode:
        return {"status": "not_copied", "container_id": state["broker_container_id"]}
    return {"status": "copied", "container_id": state["broker_container_id"],
            "destination": CAPTURED}
