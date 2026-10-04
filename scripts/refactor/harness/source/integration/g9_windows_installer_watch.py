"""Bridge one audited offline helper request to its prewarmed Windows job.

The model cannot read the root-only audit volume or write this process's
private case directory. This watcher never accepts an arbitrary shell command.
"""

import hashlib
import json
from pathlib import Path
import threading
import time

from integration import g9_windows_host_relay as relay
from integration import g9_windows_installer_profile as profile


REQUEST = "g9-windows-request.json"
RESPONSE = "g9-windows-response.json"
WATCH = "g9-windows-watch.json"
REQUEST_LIMIT = 4096


def _unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise relay.Refused("Duplicate hosted broker request field")
        result[key] = value
    return result


def _save(path, value):
    with Path(path).open("x", encoding="utf-8") as stream:
        json.dump(value, stream, sort_keys=True, separators=(",", ":"))
        stream.write("\n")


def _copy_request(docker, container, destination):
    if destination.exists() or destination.is_symlink():
        raise relay.Refused("Hosted broker request path already exists")
    copied = docker("cp", container + ":/audit/windows-request.json", str(destination),
                    check=False)
    if copied.returncode:
        if destination.exists() or destination.is_symlink():
            raise relay.Refused("Partial hosted broker request copy")
        return None
    if not destination.is_file() or destination.is_symlink() or destination.stat().st_size > REQUEST_LIMIT:
        raise relay.Refused("Hosted broker request is not a bounded regular file")
    value = json.loads(destination.read_text(encoding="utf-8"), object_pairs_hook=_unique)
    if not isinstance(value, dict):
        raise relay.Refused("Hosted broker request is not an object")
    return value


def _copy_response(docker, container, local):
    remote = container + ":/audit/windows-response.json"
    exists = docker("exec", "--user", "0:0", container, "node", "-e",
                    "process.exit(require('fs').existsSync('/audit/windows-response.json')?0:1)",
                    check=False)
    if exists.returncode != 1:
        raise relay.Refused("Hosted broker response path is already present or not inspectable")
    docker("cp", str(local), remote)
    script = ("const f=require('fs'),c=require('crypto');"
              "process.stdout.write(c.createHash('sha256').update("
              "f.readFileSync('/audit/windows-response.json')).digest('hex'))")
    observed = docker("exec", "--user", "0:0", container, "node", "-e", script).stdout.strip()
    expected = hashlib.sha256(local.read_bytes()).hexdigest()
    if observed != expected:
        raise relay.Refused("Hosted broker response differs after private volume transfer")


def run_model(req, state, model_call, docker, *, request_limit_seconds=6 * 60):
    """Run one model task while a bounded watcher services its exact helper."""
    root = Path(req["private_case_dir"])
    session = json.loads((root / profile.SESSION).read_text(encoding="utf-8"),
                         object_pairs_hook=_unique)
    broker = state.get("broker_container_id")
    if not isinstance(broker, str) or not broker or not state.get("delivery_complete"):
        raise relay.Refused("Exact offline broker container was not delivered")
    stop = threading.Event()
    command_seen = threading.Event()
    finished = threading.Event()
    errors = []

    def serve():
        command_started = False
        try:
            request_path = root / REQUEST
            deadline = time.monotonic() + request_limit_seconds
            request = None
            while time.monotonic() < deadline:
                request = _copy_request(docker, broker, request_path)
                if request is not None:
                    break
                if stop.wait(2):
                    # A task with no helper call fails coverage, not fixture cleanup.
                    cleanup = relay.abort_hosted(session)
                    _save(root / WATCH, {"status": "no_request", **cleanup})
                    return
            if request is None:
                raise relay.Refused("Model did not issue its exact installer command within the bound")
            relay.verify_broker_request(session, request)
            command_started = True
            command_seen.set()
            outcome = relay.execute_hosted(session, request["argv"], threading.Event())
            _save(root / profile.OUTCOME, outcome)
            response_path = root / RESPONSE
            _save(response_path, relay.broker_response(request, outcome))
            _copy_response(docker, broker, response_path)
            _save(root / WATCH, {"status": "responded", "branch_cleanup": "verified"})
        except BaseException as exc:
            errors.append(type(exc).__name__)
            if not command_started:
                try:
                    cleanup = relay.abort_hosted(session)
                    _save(root / WATCH, {"status": "refused", **cleanup})
                except BaseException:
                    pass
            docker("stop", broker, check=False)
        finally:
            finished.set()

    watcher = threading.Thread(target=serve, name="g9-windows-one-command", daemon=True)
    watcher.start()
    result = None
    model_error = None
    try:
        result = model_call()
    except BaseException as exc:
        model_error = exc
    finally:
        stop.set()
        watcher.join(timeout=relay.EXECUTE_LIMIT + 240 if command_seen.is_set() else 210)
    if watcher.is_alive() or not finished.is_set():
        raise relay.Refused("Hosted installer watcher did not stop within its bound")
    if errors:
        raise relay.Refused("Hosted installer watcher blocked: " + errors[0])
    if model_error is not None:
        raise model_error
    return result
