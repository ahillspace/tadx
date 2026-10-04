"""Verify one operator-approved G9 run against immutable prepared inputs.

The expected record digest is supplied by the operator at invocation, never
read from the record itself. This module does not grant authority or dispatch.
"""

import hashlib
import json
from pathlib import Path
import re


HEX64 = re.compile(r"[0-9a-f]{64}\Z")
GATES = {f"G{number}" for number in range(9)}
MODEL = {"provider": "openai", "model": "gpt-6-luna", "effort": "medium"}


def _require(condition, reason):
    if not condition:
        raise ValueError("G9 authorization: " + reason)


def _sha(data):
    return hashlib.sha256(data).hexdigest()


def _json(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            _require(key not in result, "duplicate JSON field")
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=unique)


def _read(path):
    _require(isinstance(path, (str, Path)), "input path missing")
    path = Path(path).absolute()
    _require(path.is_file() and not path.is_symlink(), "missing or linked input")
    return path.read_bytes()


def _bound_file(binding):
    _require(isinstance(binding, dict) and set(binding) == {"path", "sha256"}
             and isinstance(binding["path"], str)
             and isinstance(binding["sha256"], str)
             and HEX64.fullmatch(binding["sha256"]), "invalid evidence binding")
    blob = _read(binding["path"])
    _require(_sha(blob) == binding["sha256"], "evidence changed")
    return _json(blob)


def _remote_target(path):
    binding = _json(_read(path))
    _require(isinstance(binding, dict), "remote binding is not an object")
    fields = ("environment", "server_url", "site_content_url", "site_luid", "api_version")
    target = {field: binding.get(field) for field in fields}
    _require(all(isinstance(value, str) and value.strip() for value in target.values()),
             "remote target identity is incomplete")
    return target


def verify(record_path, expected_sha256, prepared_root, *, check_snapshot=False,
           check_launch_inputs=False):
    """Return the accepted record only when the operator digest and inputs agree."""
    _require(isinstance(expected_sha256, str) and HEX64.fullmatch(expected_sha256),
             "operator digest missing")
    root = Path(prepared_root).absolute()
    blob = _read(record_path)
    _require(_sha(blob) == expected_sha256, "operator digest differs")
    record = _json(blob)
    _require(isinstance(record, dict) and record.get("schema_version") == 1
             and record.get("status") == "authorized_g9", "record is not authorized")
    preparation_blob = _read(root / "preparation.json")
    preparation = _json(preparation_blob)
    candidate_blob = _read(root / "candidate/candidate.json")
    candidate = _json(candidate_blob)
    runner_blob = _read(root / "source/runner.local.json")
    runner = _json(runner_blob)
    _require(preparation.get("status") == "prepared_blocked"
             and preparation.get("live_execution_enabled") is False
             and record.get("preparation_sha256") == _sha(preparation_blob)
             and record.get("candidate_sha256") == _sha(candidate_blob)
             and record.get("runner_config_sha256") == _sha(runner_blob)
             and record.get("source_revision") == candidate.get("source_revision")
             and record.get("source_tree_sha256") == candidate.get("source_tree_sha256")
             and record.get("builds") == candidate.get("builds")
             and record.get("source_revision") == preparation.get("candidate", {}).get("source_revision")
             and record.get("cases") == preparation.get("cases")
             and isinstance(record.get("cases"), list)
             and len(record["cases"]) == 131
             and len(set(record["cases"])) == 131,
             "candidate, preparation, runner, or case plan differs")
    _require(record.get("authority_sha256") == preparation.get("authority_sha256")
             and record.get("consent_evidence_sha256") == preparation.get("consent_evidence_sha256")
             and runner.get("run_constraints", {}).get("g9_saved_consent_authority", {}).get(
                 "consent_evidence_sha256") == record.get("consent_evidence_sha256")
             and _sha((json.dumps(runner["run_constraints"]["g9_saved_consent_authority"],
                                  sort_keys=True, indent=2) + "\n").encode())
             == preparation.get("runner_authority_sha256"),
             "saved consent authority differs")
    deployment = runner.get("deployment", {})
    settings_path = deployment.get("operator_settings_file")
    settings = record.get("operator_settings_input")
    _require((settings_path is None and settings is None)
             or (isinstance(settings_path, str) and isinstance(settings, dict)
                 and set(settings) == {"path", "sha256"}
                 and settings["path"] == settings_path
                 and isinstance(settings["sha256"], str)
                 and HEX64.fullmatch(settings["sha256"])),
             "operator settings binding differs")
    remote_path = deployment.get("remote_binding_file")
    remote = record.get("remote_target")
    _require((remote_path is None and remote is None)
             or (isinstance(remote_path, str) and isinstance(remote, dict)
                 and set(remote) == {"path", "identity"}
                 and remote["path"] == remote_path
                 and isinstance(remote["identity"], dict)
                 and set(remote["identity"]) == {"environment", "server_url",
                                                 "site_content_url", "site_luid", "api_version"}),
             "remote target binding differs")
    if check_launch_inputs:
        if settings is not None:
            _require(_sha(_read(settings_path)) == settings["sha256"],
                     "operator settings changed before launch")
        if remote is not None:
            _require(_remote_target(remote_path) == remote["identity"],
                     "remote target changed before launch")
    selected = record.get("selected_ids")
    _require(isinstance(selected, list) and selected
             and len(selected) == len(set(selected))
             and set(selected) <= set(record["cases"]), "selected case plan differs")
    purpose = record.get("purpose")
    if purpose == "full_catalog":
        _require(selected == record["cases"], "full catalog must select all 131 cases")
    elif purpose == "windows_qualification":
        hosted = {"V-installer-windows-fresh", "V-installer-windows-idempotent",
                  "V-installer-windows-no-completion", "V-installer-windows-completion-opt-in",
                  "V-installer-windows-failed-download-preserves-binary",
                  "V-installer-windows-uninstall", "V-installer-windows-no-modify-path"}
        _require(len(selected) == 1 and selected[0] in hosted,
                 "Windows qualification must select one hosted case")
    else:
        _require(False, "unknown run purpose")
    _require(record.get("prior_gates") == dict.fromkeys(sorted(GATES), "passed"),
             "G0-G8 acceptance missing")
    evidence = _bound_file(record.get("prior_gates_evidence"))
    _require(isinstance(evidence, dict), "G0-G8 evidence is not an object")
    qualified = _bound_file(record.get("copied_inputs_qualification"))
    _require(isinstance(qualified, dict)
             and qualified.get("status") == "copied_inputs_qualified_offline"
             and qualified.get("live_execution_enabled") is False
             and qualified.get("candidate") == candidate
             and record.get("worker_input_sha256") == qualified.get("worker_input_sha256"),
             "copied-input qualification differs")
    image = record.get("image")
    _require(isinstance(image, dict) and set(image) == {"id", "worker_input_sha256"}
             and isinstance(image["id"], str)
             and re.fullmatch(r"sha256:[0-9a-f]{64}", image["id"])
             and image["worker_input_sha256"] == record["worker_input_sha256"],
             "accepted immutable image missing")
    setup = record.get("model_setup")
    _require(isinstance(setup, dict) and set(setup) == {
                 "provider", "model", "effort", "thread_started", "thread_matched",
                 "turn_completed", "process_exit", "image_id", "runtime_version",
                 "container_removed"}
             and all(setup.get(key) == value for key, value in MODEL.items())
             and setup.get("thread_started") is True
             and setup.get("thread_matched") is True
             and setup.get("turn_completed") is True
             and setup.get("container_removed") is True
             and setup.get("process_exit") == 0
             and setup.get("image_id") == image["id"]
             and setup.get("runtime_version") == "codex-cli 0.160.0",
             "same-thread Luna medium setup proof missing")
    _require(_bound_file(record.get("model_setup_evidence")) == setup,
             "same-thread setup evidence differs")
    _require(record.get("windows_fixture_version") ==
             "1.0.0-g9.git" + record["source_revision"][:12],
             "Windows fixture version differs from accepted source")
    if check_snapshot:
        snapshots = preparation.get("snapshot_files")
        _require(isinstance(snapshots, dict) and snapshots, "prepared snapshot missing")
        for name, expected in snapshots.items():
            _require(isinstance(name, str) and name and not name.startswith("/")
                     and ".." not in Path(name).parts and isinstance(expected, str)
                     and HEX64.fullmatch(expected) and _sha(_read(root / name)) == expected,
                     "prepared snapshot changed")
    return record
