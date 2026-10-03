"""Copy independently verified saved consent without granting new authority."""

import copy
import re
from urllib.parse import urlsplit


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate_authority(value):
    require(isinstance(value, dict) and set(value) == {
        "schema_version", "environment", "server_url", "site_content_url",
        "fixture_scope", "consent_changes_authorized", "credential_persistence_authorized",
        "saved_consent", "consent_evidence_sha256",
    }, "Authority fields differ from schema")
    require(value["schema_version"] == 1, "Unsupported authority schema")
    for key in ("environment", "site_content_url"):
        require(isinstance(value[key], str) and bool(value[key])
                and not any(ord(c) < 32 for c in value[key]), "Invalid exact target")
    server = value["server_url"]
    require(isinstance(server, str), "Invalid canonical server")
    url = urlsplit(server)
    require(url.scheme == "https" and bool(url.hostname) and not url.username
            and not url.password and not url.query and not url.fragment
            and url.path == "" and url.netloc == url.netloc.lower(), "Invalid canonical server")
    require(value["fixture_scope"] == "unique-run-owned-projects"
            and value["consent_changes_authorized"] is False
            and value["credential_persistence_authorized"] is False,
            "Only run-owned projects without consent changes or credential persistence are supported")
    expected = {"server_url": server, "site_content_url": value["site_content_url"],
                "enabled": True, "source": "saved_site_setting"}
    require(value["saved_consent"] == expected
            and value["saved_consent"]["enabled"] is True,
            "Exact enabled saved-site consent is required")
    require(isinstance(value["consent_evidence_sha256"], str)
            and re.fullmatch(r"[0-9a-f]{64}", value["consent_evidence_sha256"]),
            "Missing verified consent evidence hash")


def preserve_consent(config, enabled, authority):
    validate_authority(authority)
    require(enabled is True, "Changing saved consent is not authorized")
    require(isinstance(config, dict), "Missing disposable configuration")
    environments = config.get("environments")
    require(isinstance(environments, dict) and set(environments) == {authority["environment"]},
            "Disposable configuration must select only the authorized environment")
    selected = environments[authority["environment"]]
    require(isinstance(selected, dict) and selected.get("url") == authority["server_url"]
            and selected.get("site_content_url") == authority["site_content_url"],
            "Disposable target differs from verified consent")
    entry = {key: authority["saved_consent"][key]
             for key in ("server_url", "site_content_url", "enabled")}
    existing = config.get("site_mutations", [])
    require(existing == [] or existing == [entry], "Existing consent differs from verified consent")
    result = copy.deepcopy(config)
    result["site_mutations"] = [entry]
    return result


def project_scope(req, binding):
    """Validate existing consent and bind fixture ownership to this exact run."""
    constraints = req.get("run_constraints", {})
    authority = constraints.get("g9_saved_consent_authority")
    validate_authority(authority)
    require(not constraints.get("mutation_setting_changes_authorized")
            and not constraints.get("mutation_setting_change_scope"),
            "Consent-change flags are not supported")
    require(req.get("exercise", {}).get("id") in {
        "P-project-" + action for action in ("create", "update", "move", "delete", "list", "inspect")},
        "Only the six qualified project cases are supported")
    for key in ("environment", "server_url", "site_content_url"):
        require(binding.get(key) == authority[key], "Project binding differs from exact saved consent")
    for value in (binding.get("site_luid"), req.get("run_id"), req.get("case_id")):
        require(isinstance(value, str) and bool(value) and not any(ord(c) < 32 for c in value),
                "Missing exact run, case, or site identity")
    return {key: binding[key] for key in ("environment", "server_url", "site_content_url", "site_luid")} | {
        "run_id": req["run_id"], "case_id": req["case_id"]}


def owned_projects(checkpoint):
    """Require independently confirmed setup IDs, never name-only ownership."""
    original = {row["id"] for row in checkpoint["original"]["inventory"]["project"]}
    owned = checkpoint["owned"]
    require(isinstance(owned, list) and all(isinstance(item, str) and item for item in owned)
            and len(owned) == len(set(owned)) and not original.intersection(owned),
            "Fixture ownership includes invalid or preexisting project IDs")
    confirmed = {step["id"] for step in checkpoint["steps"].values()
                 if step.get("action") == "create" and step.get("phase") == "confirmed"
                 and step.get("result", {}).get("id") == step.get("id")}
    require(set(owned) <= confirmed, "Project ID lacks confirmed creation evidence")
    return set(owned)


def verify_cleanup_scope(req, binding, checkpoint):
    require(checkpoint.get("g9_scope") == project_scope(req, binding),
            "Cleanup belongs to another run, case, or exact site")
    return owned_projects(checkpoint)


def admit_project_write(checkpoint, action, key, identity, fields):
    owned = owned_projects(checkpoint)
    fields = fields or {}
    if action == "create":
        parent = fields.get("parent_id")
        require(parent in owned or (key == "setup-parent" and parent == "" and not owned
                                   and set(checkpoint["steps"]) <= {"setup-parent"}),
                "Fixture creation escapes its owned hierarchy")
    else:
        require(action in {"update", "delete"} and identity in owned,
                "Fixture write targets an unowned project")
        require("parent_id" not in fields or fields["parent_id"] in owned,
                "Fixture restoration escapes its owned hierarchy")


def record_task_creation(checkpoint, evidence, matches):
    owned = owned_projects(checkpoint)
    creation = checkpoint["task_creation"]
    require(evidence.get("status") == "verified" and len(matches) == 1
            and evidence.get("created_ids") == [matches[0]["id"]],
            "Task creation lacks one exact audited and independently observed identity")
    row = matches[0]
    require(row["id"] not in creation["before_ids"] and creation["fields"]["parent_id"] in owned
            and all(row.get(key) == value for key, value in creation["fields"].items()),
            "Task creation differs from its owned creation intent")
    checkpoint["steps"]["task-create"] = {
        "action": "create", "phase": "confirmed", "id": row["id"], "result": row,
        "fields": creation["fields"], "evidence": evidence["evidence"],
    }
    if row["id"] not in checkpoint["owned"]:
        checkpoint["owned"].append(row["id"])


def require_creation_acknowledgment(value):
    require(isinstance(value, dict) and isinstance(value.get("id"), str) and bool(value["id"]),
            "Creation has no exact acknowledged ID; quarantine instead of adopting a name match")
    return value["id"]
