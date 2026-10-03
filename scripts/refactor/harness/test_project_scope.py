"""Offline consent, fixture ownership, and strict project-read policy checks."""

import copy
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import types
import unittest
from unittest.mock import patch

import consent
from test_prepare import authority


def request():
    return {"run_id": "fresh-run", "case_id": "fresh-case", "exercise": {"id": "P-project-update"},
            "run_constraints": {"g9_saved_consent_authority": authority()}}


def binding():
    return {"environment": "fixture", "server_url": "https://tableau.example.test",
            "site_content_url": "fixture-site", "site_luid": "site-id"}


def journal():
    rows = [{"id": identity, "name": identity, "parent_id": "" if identity == "parent" else "parent"}
            for identity in ("parent", "target", "destination")]
    return {"g9_scope": consent.project_scope(request(), binding()), "owned": [row["id"] for row in rows],
            "original": {"inventory": {"project": [{"id": "preexisting"}]}},
            "steps": {"setup-" + row["id"]: {"action": "create", "phase": "confirmed", "id": row["id"], "result": row}
                      for row in rows}}


class ScopeTests(unittest.TestCase):
    def test_setup_creation_cannot_adopt_name_match_after_unknown_write(self):
        for value in (None, {}, {"name": "Fresh"}, {"id": ""}, {"id": ["outside"]}):
            with self.subTest(value=value), self.assertRaises(ValueError):
                consent.require_creation_acknowledgment(value)
        self.assertEqual(consent.require_creation_acknowledgment({"id": "acknowledged"}), "acknowledged")

    def test_exact_saved_authority_replaces_change_permission(self):
        self.assertEqual(consent.project_scope(request(), binding())["site_luid"], "site-id")
        for key in ("environment", "server_url", "site_content_url"):
            changed = binding()
            changed[key] = "other"
            with self.subTest(key=key), self.assertRaises(ValueError):
                consent.project_scope(request(), changed)
        for key, value in (("mutation_setting_changes_authorized", True),
                           ("mutation_setting_change_scope", "disposable_test_containers")):
            req = request()
            req["run_constraints"][key] = value
            with self.assertRaises(ValueError):
                consent.project_scope(req, binding())

    def test_cleanup_rejects_other_run_case_site_and_unproven_ids(self):
        self.assertEqual(consent.verify_cleanup_scope(request(), binding(), journal()), {"parent", "target", "destination"})
        for key in ("run_id", "case_id"):
            req = request()
            req[key] = "other"
            with self.assertRaises(ValueError):
                consent.verify_cleanup_scope(req, binding(), journal())
        changed = binding()
        changed["site_luid"] = "other"
        with self.assertRaises(ValueError):
            consent.verify_cleanup_scope(request(), changed, journal())
        for identity in ("preexisting", "unknown", "target"):
            checkpoint = journal()
            checkpoint["owned"].append(identity)
            with self.assertRaises(ValueError):
                consent.owned_projects(checkpoint)

    def test_writes_and_cleanup_stay_inside_exact_owned_ids(self):
        checkpoint = journal()
        consent.admit_project_write(checkpoint, "delete", "cleanup-target", "target", {})
        consent.admit_project_write(checkpoint, "create", "child", None, {"parent_id": "parent"})
        for action, identity, fields in (("delete", "outside", {}), ("update", "preexisting", {}),
                                         ("create", None, {"parent_id": "outside"}),
                                         ("update", "target", {"parent_id": "outside"})):
            with self.subTest(action=action), self.assertRaises(ValueError):
                consent.admit_project_write(checkpoint, action, "step", identity, fields)
        fresh = journal()
        fresh.update(owned=[], steps={})
        consent.admit_project_write(fresh, "create", "setup-parent", None, {"parent_id": ""})
        with self.assertRaises(ValueError):
            consent.admit_project_write(fresh, "create", "another-root", None, {"parent_id": ""})

    def test_task_creation_requires_exact_audit_and_independent_match(self):
        row = {"id": "created", "name": "Created", "parent_id": "parent", "description": "Requested"}
        checkpoint = journal()
        checkpoint["task_creation"] = {"before_ids": ["preexisting", *checkpoint["owned"]],
                                       "fields": {key: value for key, value in row.items() if key != "id"}}
        evidence = {"status": "verified", "created_ids": ["created"], "evidence": ["audit/command.json"]}
        for change in ({"created_ids": ["outside"]}, {"created_ids": []}, {"status": "not_verified"}):
            with self.assertRaises(ValueError):
                consent.record_task_creation(copy.deepcopy(checkpoint), evidence | change, [row])
        consent.record_task_creation(checkpoint, evidence, [row])
        self.assertIn("created", consent.owned_projects(checkpoint))
        consent.admit_project_write(checkpoint, "delete", "cleanup-created", "created", {})


def read_guard():
    return {"family": "g9-project-read", "environment": "fixture", "capability": "project.list",
            "owned_ids": ["parent", "target", "destination"], "parent_id": "parent", "target_id": "target",
            "paths": {"parent": "Fresh", "target": "Fresh/Target", "destination": "Fresh/Destination"}}


def broker_request(action="list", **flags):
    return {"parsed": {"words": ["content", "project", action], "flags": flags, "errors": []},
            "cap": "project." + action, "state": {"guard": read_guard(), "guardPresent": True,
                "baselinePresent": True, "baselineUnchanged": True, "defaultEnvironment": "fixture"}}


class ReadPolicyTests(unittest.TestCase):
    def evaluate(self, rows):
        node = shutil.which("node")
        self.assertIsNotNone(node, "Node is required to qualify the actual project read guard")
        script = "const m=require(process.argv[1]),fs=require('fs');console.log(JSON.stringify(JSON.parse(fs.readFileSync(0,'utf8')).map(r=>m.allowed(r.parsed,r.cap,r.state))))"
        result = subprocess.run([node, "-e", script, str(Path(__file__).with_name("project_read_broker.cjs"))],
                                input=json.dumps(rows), capture_output=True, text=True, check=True, timeout=10)
        return json.loads(result.stdout)

    def test_owned_reads_allowed_and_outside_scope_mutations_rejected(self):
        yes = [broker_request(**{"parent-id": "parent"}), broker_request("inspect", id="target"),
               broker_request("inspect", project="Fresh/Destination"),
               broker_request(**{"parent-id": "parent", "limit": "25", "json": True})]
        self.assertEqual(self.evaluate(yes), [True] * len(yes))
        no = [broker_request(), broker_request(**{"parent-id": "outside"}),
              broker_request("inspect", id="outside"), broker_request("inspect", project="Outside"),
              broker_request("delete", id="target"), broker_request(**{"parent-id": "parent", "all": True}),
              broker_request(**{"parent-id": "parent", "config": "other.json"}),
              broker_request(**{"parent-id": "parent", "environment": "other"}),
              broker_request(**{"parent-id": ["parent", "outside"]}),
              broker_request(**{"parent-id": "parent", "limit": "26"})]
        self.assertEqual(self.evaluate(no), [False] * len(no))

    def test_read_guard_requires_unchanged_fixture_and_cannot_change_consent(self):
        rows = []
        for key in ("guardPresent", "baselinePresent", "baselineUnchanged"):
            value = broker_request(**{"parent-id": "parent"})
            value["state"][key] = False
            rows.append(value)
        value = broker_request()
        value.update(cap="mutation.set", parsed={"words": ["mutation", "set"], "flags": {"enabled": True}, "errors": []})
        rows.append(value)
        self.assertEqual(self.evaluate(rows), [False] * len(rows))


def load_read_adapter():
    """Stub imports only; the adapter's result and subtree checks stay real."""
    integration = types.ModuleType("integration")
    integration.project_profiles = types.SimpleNamespace()
    integration.scenario_common = types.SimpleNamespace()
    core = types.ModuleType("bench.core")
    core.Blocked = ValueError
    core.digest = core.load = core.save = lambda *_: None
    spec = importlib.util.spec_from_file_location("qualified_project_read", Path(__file__).with_name("project_read.py"))
    module = importlib.util.module_from_spec(spec)
    with patch.dict(sys.modules, {"integration": integration, "integration.g9_consent": consent, "bench.core": core}):
        spec.loader.exec_module(module)
    return module


class ReadResultTests(unittest.TestCase):
    def test_exact_native_result_rejects_wrong_identity_and_false_truncation(self):
        adapter = load_read_adapter()
        expected = [{"id": "target", "name": "Target"}]
        output = {"output": {"status": "listed", "projects": [{"luid": "target", "name": "Target"}],
                             "page": {"more_available": False}}}
        self.assertTrue(adapter.exact_read_result(output, read_guard(), expected))
        changed = copy.deepcopy(output)
        changed["output"]["projects"][0]["luid"] = "outside"
        self.assertFalse(adapter.exact_read_result(changed, read_guard(), expected))
        output["output"]["page"]["more_available"] = True
        self.assertFalse(adapter.exact_read_result(output, read_guard(), expected))
        inspect = {"status": "found", "project": {"luid": "target", "name": "Target"}}
        self.assertTrue(adapter.exact_read_result(inspect, read_guard() | {"capability": "project.inspect"}, expected))

    def test_unowned_descendant_blocks_read_fixture(self):
        adapter = load_read_adapter()
        rows = [{"id": "parent", "parent_id": ""}, {"id": "outside", "parent_id": "parent"}]
        with self.assertRaises(ValueError):
            adapter.scoped_rows({"inventory": {"project": rows}}, {"parent"})

    def test_fresh_read_adapter_prepares_observes_and_cleans_only_its_fixture(self):
        for action in ("list", "inspect"):
            with self.subTest(action=action):
                adapter = load_read_adapter()
                req = request() | {"private_case_dir": "synthetic-case", "exercise": {"id": "P-project-" + action}}
                checkpoint = journal()
                rows = [step["result"] for step in checkpoint["steps"].values()]
                snapshot = {"inventory": {"project": rows, "workbook": [], "datasource": [], "flow": []}, "permissions": {}}
                target = next(row for row in rows if row["id"] == "target")
                documents = {}
                calls = []
                adapter.digest = lambda value: json.dumps(value, sort_keys=True)
                adapter.save = lambda path, value: documents.__setitem__(Path(path).name, copy.deepcopy(value))
                adapter.load = lambda path: ({"id": "P-project-update"} if Path(path).name == "P-project-update.json"
                                             else copy.deepcopy(documents[Path(path).name]))

                def prepare_fixture(setup, selected, work):
                    calls.append(("prepare", setup["exercise"]["id"], setup["run_id"], setup["case_id"]))
                    self.assertEqual(selected, binding())
                    documents["project-baseline.json"] = {"target": target, "snapshot": copy.deepcopy(snapshot)}
                    documents["project-recovery.json"] = checkpoint
                    return {"fixture": {"roles": {"parent_project": {"id": "parent"}, "project": target},
                                        "public": {"source_project": "Fresh"}}, "baseline_evidence": ["project-baseline.json"]}

                class Reader:
                    def signout(self):
                        calls.append(("signout",))

                adapter.projects = types.SimpleNamespace(
                    BASELINE="project-baseline.json", JOURNAL="project-recovery.json",
                    preflight=lambda setup, selected: consent.project_scope(setup, selected),
                    prepare=prepare_fixture, _open=lambda *_: Reader(),
                    _snapshot=lambda *_: copy.deepcopy(snapshot),
                    remote=types.SimpleNamespace(_sentinel_digest=lambda _: "sentinel-digest"),
                    native=types.SimpleNamespace(_redactor=lambda _: lambda events: events),
                    cleanup=lambda setup, selected, work: {"owned": sorted(consent.verify_cleanup_scope(setup, selected, checkpoint))},
                )
                adapter.common = types.SimpleNamespace(
                    bind_fixture=lambda req, public, roles, expect, before, **_: {"public": public, "roles": roles, "expect": expect},
                    audit_commands=lambda req, commands: [item for item in commands if item.get("executed")],
                    arguments=lambda argv: (argv[:3], dict(zip(argv[3::2], argv[4::2]))),
                    succeeded=lambda command: command["exit_status"] == 0,
                    native=lambda command: command["document"],
                    proof=lambda records: {"status": "verified" if records else "not_verified"},
                )
                prepared = adapter.prepare(req, binding())
                self.assertEqual(calls[0], ("prepare", "P-project-update", "fresh-run", "fresh-case"))
                self.assertEqual(prepared["broker_guard"]["family"], "g9-project-read")
                self.assertEqual(prepared["mutation_policy"], "enabled")
                command = adapter.qualification_commands(req, prepared["fixture"])[0]
                self.assertIn("parent" if action == "list" else "target", command)
                expected = prepared["fixture"]["expect"]["records"]
                selector = "parent-id" if action == "list" else "id"
                selected_id = "parent" if action == "list" else "target"
                document = ({"status": "listed", "projects": [{"luid": row["id"], "name": row["name"]} for row in expected],
                             "page": {"more_available": False}} if action == "list" else
                            {"status": "found", "project": {"luid": target["id"], "name": target["name"]}})
                audit = {"executed": True, "exit_status": 0, "argv": ["content", "project", action, selector, selected_id],
                         "document": document}
                result = adapter.observe(req, binding(), commands=[audit])
                self.assertTrue(result["after"]["native_command_verified"])
                wrong = audit | {"argv": ["content", "project", action, selector, "outside"]}
                self.assertFalse(adapter.observe(req, binding(), commands=[wrong])["after"]["native_command_verified"])
                for kind in ("workbook", "datasource", "flow"):
                    snapshot["inventory"][kind] = [{"id": "unexpected", "project_id": "target"}]
                    self.assertFalse(adapter.observe(req, binding(), commands=[audit])["after"]["native_command_verified"])
                    snapshot["inventory"][kind] = []
                snapshot["permissions"]["target"] = [{"capability": "Write", "mode": "Allow"}]
                self.assertFalse(adapter.observe(req, binding(), commands=[audit])["after"]["native_command_verified"])
                self.assertEqual(adapter.cleanup(req, binding())["owned"], ["destination", "parent", "target"])


if __name__ == "__main__":
    unittest.main()
