"""Exact authenticated-user Pulse subscription read with one owned follow fixture."""

import copy
import re
import urllib.parse
from pathlib import Path

from bench.core import Blocked, digest, load, save
from integration import pulse_profiles as pulse
from integration import remote_read_profiles as remote
from integration.validation_profiles import _gate_authorized

CASE = "P-pulse-subscription-list"
ACTION = "pulse.subscription.list"
MAX_PAGES = 100
MAX_RECORDS = 10000


def load_binding(req):
    return pulse.load_binding(req)


def preflight(req, binding):
    result = pulse.preflight(req, binding)
    _gate_authorized(req)  # Setup creates one owned relation, although the task is read-only.
    return result


def user_subscriptions(reader, user_id):
    """Read the same user filter independently, without borrowing CLI output."""
    rows, seen_ids, seen_tokens, token = [], set(), set(), ""
    for _ in range(MAX_PAGES):
        query = {"user_id": pulse.exact(user_id, "authenticated user LUID"), "page_size": 100}
        if token:
            query["page_token"] = token
        page = reader.pulse("subscriptions?" + urllib.parse.urlencode(query))
        batch = page.get("subscriptions")
        if not isinstance(batch, list) or len(batch) > 100:
            raise Blocked("User-filtered Pulse page is incomplete")
        for raw in batch:
            row = pulse.subscription(raw)
            identity = row["subscription_id"]
            if identity in seen_ids or row["principal_type"] == "USER" and row["principal_id"] != user_id:
                raise Blocked("User-filtered Pulse page has a duplicate or different user")
            seen_ids.add(identity)
            rows.append(row)
        if len(rows) > MAX_RECORDS:
            raise Blocked("User-filtered Pulse inventory exceeds its bound")
        token = page.get("next_page_token", "")
        if not isinstance(token, str) or token and (not token.strip() or token in seen_tokens or not batch):
            raise Blocked("User-filtered Pulse continuation is inconsistent")
        if not token:
            total = page.get("total_available")
            if total is not None and (isinstance(total, bool) or not str(total).isdigit()
                                      or int(total) != len(rows)):
                raise Blocked("User-filtered Pulse total is inconsistent")
            return sorted(rows, key=lambda row: row["subscription_id"])
        seen_tokens.add(token)
    raise Blocked("User-filtered Pulse inventory exceeds its page bound")


def _wanted(config):
    return {"metric_id": config["metric_id"], "principal_type": "USER", "principal_id": config["user_id"]}


def _protected(snapshot, users, wanted):
    return digest({"pulse": pulse._protected(snapshot, wanted),
                   "user_subscriptions": [row for row in users if pulse.relation(row) != wanted]})


def _checkpoint(req, binding, config, original, original_users, wanted, identity):
    root = Path(req["private_case_dir"])
    save(root / pulse.CHECKPOINT, {"settings": config, "operation": ACTION, "original": original,
                                  "original_user_subscriptions": original_users, "relation": wanted,
                                  "binding_digest": pulse._binding_digest(binding), "identity": identity})


def prepare(req, binding, work=None):
    preflight(req, binding)
    config = pulse.settings(req)
    root = Path(req["private_case_dir"])
    work = Path(work or root / "agent-public")
    work.mkdir(parents=True, exist_ok=True)
    (work / "deliverables").mkdir(exist_ok=True)
    if not (work / "sentinel.txt").exists():
        (work / "sentinel.txt").write_text("protected Pulse subscription fixture\n", encoding="utf-8")
    wanted = _wanted(config)
    evidence, oracle, agent = [], None, None
    try:
        agent = pulse._open(binding, evidence.append, "agent", wanted)
        oracle = pulse._open(binding, evidence.append)
        if agent.identity != oracle.identity or agent.identity["user_id"] != config["user_id"]:
            raise Blocked("Owned follow must be created by the authenticated task user")
        original = pulse._snapshot(oracle)
        pulse._validate_targets(original, config)
        original_users = user_subscriptions(oracle, config["user_id"])
        if any(pulse.relation(row) == wanted for row in original["subscriptions"] + original_users):
            raise Blocked("Select a user without this metric follow; original IDs must be preserved")
        if len(original_users) >= MAX_RECORDS:
            raise Blocked("No bounded capacity remains for the owned follow fixture")
        _checkpoint(req, binding, config, original, original_users, wanted, oracle.identity)
        agent.follow()
        agent.signout()
        agent = None
        snapshot = pulse._snapshot(oracle)
        users = user_subscriptions(oracle, config["user_id"])
        additions = [row for row in users if row not in original_users]
        if (len(additions) != 1 or pulse.relation(additions[0]) != wanted
                or len([row for row in snapshot["subscriptions"] if pulse.relation(row) == wanted]) != 1
                or _protected(snapshot, users, wanted) != _protected(original, original_users, wanted)):
            raise Blocked("Owned follow or protected Pulse state was not independently confirmed")
        owned_id = additions[0]["subscription_id"]
        before = {"protected": _protected(snapshot, users, wanted), "records": users,
                  "subscriptions": users, "owned_subscription_id": owned_id,
                  "sentinel_sha256": remote._sentinel_digest(work)}
        fixture = {"handle": req["case_id"], "semantic_digest": digest({"config": config, "before": before}),
                   "public": {"site": binding["environment"], "task_inputs": {"operation": ACTION}},
                   "roles": {**{key: {"id": config[key + "_id"]}
                               for key in ("definition", "metric", "datasource", "user")},
                             "sentinel": {"path": "sentinel.txt"}},
                   "sites": {"A": {"id": binding["site_luid"], "environment": binding["environment"]}},
                   "expect": {"owned_subscription_id": owned_id},
                   "expected_bindings_provenance": {"/fixture/expect/owned_subscription_id": {
                       "phase": "setup", "source_kind": "independent_observation",
                       "evidence": [pulse.BASELINE, "pulse-before-http.json"]}},
                   "coverage_assertions": [{"id": "native-pulse-subscription-list",
                                             "actual": "/after/native_command_verified", "op": "eq", "expected": True}]}
        save(root / pulse.BASELINE, {"settings": config, "operation": ACTION, "snapshot": snapshot,
                                     "fixture": fixture, "before": before,
                                     "binding_digest": pulse._binding_digest(binding),
                                     "identity": oracle.identity})
        seed = {"version": 1, "default_environment": binding["environment"],
                "environments": {binding["environment"]: {
                    "url": binding["server_url"], "site_content_url": binding["site_content_url"],
                    "api_version": binding["api_version"], "auth": {"type": "pat",
                    "pat_name_env": remote.ENV_NAMES["pat_name"],
                    "pat_secret_env": remote.ENV_NAMES["pat_secret"]}}}}
        return {"fixture": fixture, "before": before, "config_seed": seed,
                "credential_env_names": remote.ENV_NAMES, "mutation_policy": "enabled",
                "harness_report_required": False,
                "broker_guard": {"mode": "pulse", "environment": binding["environment"],
                                 "capability": ACTION, **{key: value for key, value in config.items() if key != "pulse"},
                                 "task_inputs": {"operation": ACTION, **{key: value for key, value in config.items() if key != "pulse"}}},
                "baseline_evidence": [pulse.BASELINE, pulse.CHECKPOINT, "pulse-before-http.json"],
                "baseline_assertions": [{"id": "authenticated-owned-follow", "status": "pass",
                                         "evidence": ["pulse-before-http.json"]}]}
    finally:
        try:
            if agent:
                agent.signout()
            if oracle:
                oracle.signout()
        finally:
            save(root / "pulse-before-http.json", pulse._redactor(binding)(evidence))


def native_evidence(req, commands, binding, baseline):
    references, issues, reports = [], [], []
    expected_hash = req.get("frozen_cli", {}).get("binaries", {}).get("linux", {}).get("sha256")
    expected = baseline["before"]["records"]
    for command in commands:
        if command.get("capability") != ACTION or command.get("phase") != "task" or command.get("executed") is not True:
            continue
        try:
            if command.get("exit_status") != 0:
                raise ValueError("Native subscription list did not succeed")
            flags = pulse._audited_flags(command, ACTION)
            if (flags.get("all") is not True or flags.get("json") is not True
                    or flags.get("full") is not True or flags.get("cursor") or flags.get("limit")
                    or flags.get("cache") or flags.get("preview") or flags.get("help")
                    or flags.get("environment", binding["environment"]) != binding["environment"]):
                raise ValueError("Native list did not use complete exact-user scope")
            identity = command.get("id", "")
            if (not re.fullmatch(r"cmd-[0-9]+", identity)
                    or not isinstance(expected_hash, str) or not re.fullmatch(r"[0-9a-f]{64}", expected_hash)
                    or command.get("binary_sha256") != expected_hash
                    or command.get("evidence") != "audit/commands.jsonl#" + identity
                    or command.get("stdout_evidence") != "audit/" + identity + ".stdout.txt"):
                raise ValueError("Native list lacks exact broker or binary evidence")
            raw = command.get("stdout")
            if (not isinstance(raw, str) or len(raw.encode()) > pulse.MAX_BODY
                    or not raw.lstrip().startswith("{")):
                raise ValueError("Native subscription list lacks JSON output")
            data = pulse._json(raw)
            coverage = data.get("coverage", {})
            records = data.get("subscriptions")
            if (data.get("status") != "listed" or data.get("user_luid") != baseline["settings"]["user_id"]
                    or data.get("environment") != binding["environment"]
                    or data.get("site") != binding["site_content_url"]
                    or not isinstance(records, list) or type(data.get("count")) is not int
                    or data.get("count") != len(records)
                    or coverage.get("scope") != "authenticated_user_filter"
                    or coverage.get("complete") is not True or coverage.get("more_available") is not False
                    or coverage.get("next_cursor")):
                raise ValueError("Native list lacks a complete authenticated-user response")
            normalized = sorted(({"subscription_id": row["subscription_luid"], "metric_id": row["metric_luid"],
                                  "principal_type": row["follower_type"], "principal_id": row["follower_luid"]}
                                 for row in records), key=lambda row: row["subscription_id"])
            if normalized != expected or baseline["before"]["owned_subscription_id"] not in {
                    row["subscription_id"] for row in normalized}:
                raise ValueError("Native list differs from the independent user-filtered fixture")
            reports.append({"records": normalized})
            references.extend([command["evidence"], command["stdout_evidence"]])
        except (ValueError, KeyError, TypeError, Blocked) as exc:
            issues.append(str(exc))
    if len(reports) != 1:
        issues.append("Exactly one complete native subscription list is required")
    return {"status": "verified" if not issues else "not_verified", "issues": issues,
            "evidence": sorted(set(references)), "candidate": reports[0] if len(reports) == 1 and not issues else None}


def observe(req, binding, work=None, commands=None):
    if commands is None:
        raise Blocked("Complete broker audit is required for subscription observation")
    root = Path(req["private_case_dir"])
    baseline = load(root / pulse.BASELINE)
    if pulse.settings(req) != baseline["settings"] or pulse._binding_digest(binding) != baseline["binding_digest"]:
        raise Blocked("Subscription binding changed after baseline freeze")
    binding["_pulse_scope"] = {key: baseline["settings"][key] for key in
                               ("definition_id", "metric_id", "datasource_id", "user_id")}
    state = load(root / pulse.BRIDGE_STATE)
    if state.get("frozen") is not True or state.get("cli_audit_complete") is not True:
        raise Blocked("Subscription task must be quiescent with complete audit")
    evidence, oracle = [], None
    try:
        oracle = pulse._open(binding, evidence.append)
        if oracle.identity != baseline["identity"]:
            raise Blocked("Subscription observer identity changed")
        snapshot = pulse._snapshot(oracle)
        users = user_subscriptions(oracle, baseline["settings"]["user_id"])
    finally:
        try:
            if oracle:
                oracle.signout()
        finally:
            save(root / "pulse-after-http.json", pulse._redactor(binding)(evidence))
    wanted = _wanted(baseline["settings"])
    protected = _protected(snapshot, users, wanted)
    sentinel = remote._sentinel_digest(Path(work or root / "agent-public"))
    native = native_evidence(req, commands, binding, baseline)
    after = {"protected": protected,
             "unowned_changes": [] if protected == baseline["before"]["protected"]
             and users == baseline["before"]["records"]
             and sentinel == baseline["before"]["sentinel_sha256"]
             else ["Pulse user inventory, protected state, or local sentinel changed"],
             "candidate": native["candidate"], "candidate_status": "present" if native["candidate"] else "missing",
             "native_command_verified": native["status"] == "verified", "native_command_evidence": native,
             "independent_observation": {"status": "verified", "evidence": ["pulse-after-http.json"]}}
    save(root / "pulse-observer.json", after)
    return {"after": after, "commands": commands, "evidence": ["pulse-observer.json", "pulse-after-http.json"]}


def cleanup(req, binding, work=None):
    root = Path(req["private_case_dir"])
    result = pulse.cleanup(req, binding, work)
    if result.get("status") != "pass" or not (root / pulse.CHECKPOINT).exists():
        return result
    checkpoint = load(root / pulse.CHECKPOINT)
    evidence, oracle = [], None
    try:
        oracle = pulse._open(binding, evidence.append)
        if oracle.identity != checkpoint["identity"] or user_subscriptions(oracle, checkpoint["settings"]["user_id"]) != checkpoint["original_user_subscriptions"]:
            raise Blocked("User-filtered Pulse inventory was not restored")
    except Exception as exc:
        result.update(status="blocked", restored=False, quarantined=True,
                      reason=pulse._redactor(binding).text(str(exc)))
    finally:
        if oracle:
            oracle.signout()
        save(root / "pulse-cleanup-user-http.json", pulse._redactor(binding)(evidence))
        save(root / "pulse-cleanup.json", result)
    result["evidence"] = sorted(set(result.get("evidence", []) + ["pulse-cleanup-user-http.json"]))
    return result
