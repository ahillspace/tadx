"""Read-only agent cases backed by fresh, journal-owned project fixtures."""

import copy
from pathlib import Path

from integration import project_profiles as projects, scenario_common as common
from integration.g9_consent import project_scope, verify_cleanup_scope
from bench.core import Blocked, digest, load, save

SUPPORTED = {"P-project-list", "P-project-inspect"}
BASELINE = "g9-project-read.json"


def setup_request(req):
    result = copy.deepcopy(req)
    result["exercise"] = load(Path(__file__).resolve().parents[1] / "suite/exercises/P-project-update.json")
    return result


def load_binding(req):
    return projects.load_binding(setup_request(req))


def preflight(req, binding):
    if req["exercise"]["id"] not in SUPPORTED:
        raise Blocked("Unsupported owned project read")
    project_scope(req, binding)
    return projects.preflight(setup_request(req), binding)


def scoped_rows(snapshot, owned):
    rows = snapshot["inventory"]["project"]
    if any(row["id"] not in owned and row.get("parent_id") in owned for row in rows):
        raise Blocked("An unowned descendant entered the disposable hierarchy")
    return sorted((row for row in rows if row["id"] in owned), key=lambda row: row["id"])


def scoped_state(snapshot, owned):
    return {"inventory": {"project": scoped_rows(snapshot, owned),
                          **{kind: snapshot["inventory"][kind] for kind in ("workbook", "datasource", "flow")}},
            "permissions": snapshot["permissions"]}


def prepare(req, binding, work=None):
    preflight(req, binding)
    prepared = projects.prepare(setup_request(req), binding, work)
    root = Path(req["private_case_dir"])
    base = load(root / projects.BASELINE)
    checkpoint = load(root / projects.JOURNAL)
    owned = verify_cleanup_scope(req, binding, checkpoint)
    parent_id = prepared["fixture"]["roles"]["parent_project"]["id"]
    target = base["target"]
    rows = scoped_rows(base["snapshot"], owned)
    expected = [row for row in rows if row["parent_id"] == parent_id] if req["exercise"]["id"].endswith("list") else [target]
    records = [{"id": row["id"], "name": row["name"]} for row in expected]
    public = {"site": binding["environment"], "source_project": prepared["fixture"]["public"]["source_project"],
              "target_name": target["name"], "parent_id": parent_id, "target_id": target["id"]}
    paths = {row["id"]: public["source_project"] + "/" + row["name"] for row in rows if row["parent_id"] == parent_id}
    paths[parent_id] = public["source_project"]
    guard = {"family": "g9-project-read", "environment": binding["environment"],
             "capability": "project." + req["exercise"]["id"].split("-")[-1],
             "owned_ids": sorted(owned), "parent_id": parent_id, "target_id": target["id"], "paths": paths}
    before = {"protected": {"remote": digest(scoped_state(base["snapshot"], owned)),
                            "sentinel": projects.remote._sentinel_digest(Path(work or root / "agent-public"))}}
    fixture = common.bind_fixture(req, public, prepared["fixture"]["roles"], {"records": records}, before,
                                  source=BASELINE, conditions={"exact_owned_projects": guard})
    save(root / BASELINE, {"guard": guard, "before": before, "records": records})
    return {**prepared, "fixture": fixture, "before": before, "broker_guard": guard,
            "mutation_policy": "enabled", "harness_report_required": False,
            "baseline_evidence": [*prepared["baseline_evidence"], BASELINE]}


def qualification_commands(req, fixture):
    action = req["exercise"]["id"].split("-")[-1]
    public = fixture["public"]
    selector = ["--parent-id", public["parent_id"]] if action == "list" else ["--id", public["target_id"]]
    return [["content", "project", action, *selector, "--environment", public["site"], "--full", "--json"]]


def exact_read_result(document, guard, records):
    if not isinstance(document, dict) or document.get("error"):
        return False
    output = document.get("output", document)
    if not isinstance(output, dict):
        return False
    if guard["capability"] == "project.list":
        rows = output.get("projects")
        page = output.get("page", {})
        if output.get("status") != "listed" or not isinstance(page, dict) or page.get("more_available") is not False:
            return False
    else:
        rows = [output.get("project")]
        if output.get("status") != "found":
            return False
    if not isinstance(rows, list) or not all(isinstance(row, dict) for row in rows):
        return False
    observed = [{"id": row.get("luid"), "name": row.get("name")} for row in rows]
    return len(observed) == len(records) and all(row in records for row in observed) and len({row["id"] for row in observed}) == len(records)


def observe(req, binding, work=None, commands=None):
    root = Path(req["private_case_dir"])
    baseline = load(root / BASELINE)
    owned = verify_cleanup_scope(req, binding, load(root / projects.JOURNAL))
    guard = baseline["guard"]
    events, reader = [], None
    try:
        reader = projects._open(binding, events.append)
        snapshot = scoped_state(projects._snapshot(reader, owned), owned)
        protected = {"remote": digest(snapshot), "sentinel": projects.remote._sentinel_digest(Path(work or root / "agent-public"))}
        changes = [] if protected == baseline["before"]["protected"] else ["Owned projects or sentinel changed"]
        records = []
        for command in common.audit_commands(req, commands or []):
            try:
                words, flags = common.arguments(command["argv"])
                action = guard["capability"].split(".")[-1]
                selector = "parent-id" if action == "list" else "id"
                identity = guard["parent_id"] if action == "list" else guard["target_id"]
                if (words != ["content", "project", action] or flags.get(selector) != identity
                        or flags.get("environment", binding["environment"]) != binding["environment"]
                        or set(flags) - {selector, "environment", "json", "full", "limit"}
                        or not common.succeeded(command)):
                    continue
                if exact_read_result(common.native(command), guard, baseline["records"]):
                    records.append(command)
            except (ValueError, TypeError, KeyError):
                continue
        proof = common.proof(records)
        after = {"native_command_verified": bool(records) and not changes, "native_command_evidence": proof,
                 "command_evidence": proof, "protected": protected, "unowned_changes": changes}
        save(root / "g9-project-read-after.json", after)
        return {"after": after, "commands": commands or [], "evidence": ["g9-project-read-after.json", "g9-project-read-http.json"]}
    finally:
        if reader:
            reader.signout()
        save(root / "g9-project-read-http.json", projects.native._redactor(binding)(events))


def cleanup(req, binding, work=None):
    return projects.cleanup(setup_request(req), binding, work)
