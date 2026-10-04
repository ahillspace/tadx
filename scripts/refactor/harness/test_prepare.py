"""Hermetic preparer tests. No models, subprocesses, credentials, or network."""

import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import consent
import prepare as prep
from project_patches import PROJECT_REPLACEMENTS


def authority():
    return {
        "schema_version": 1, "environment": "fixture", "server_url": "https://tableau.example.test",
        "site_content_url": "fixture-site", "fixture_scope": "unique-run-owned-projects",
        "consent_changes_authorized": False, "credential_persistence_authorized": False,
        "saved_consent": {"server_url": "https://tableau.example.test", "site_content_url": "fixture-site",
                          "enabled": True, "source": "saved_site_setting"},
        "consent_evidence_sha256": prep.sha(b"synthetic reviewed consent evidence"),
    }


class ConsentTests(unittest.TestCase):
    def setUp(self):
        self.authority = authority()
        self.config = {"environments": {"fixture": {
            "url": self.authority["server_url"], "site_content_url": self.authority["site_content_url"],
        }}}

    def test_copy_exact_saved_consent_without_changing_input(self):
        original = copy.deepcopy(self.config)
        result = consent.preserve_consent(self.config, True, self.authority)
        self.assertEqual(self.config, original)
        self.assertEqual(result["site_mutations"], [{k: v for k, v in self.authority["saved_consent"].items()
                                                    if k != "source"}])
        self.assertEqual(consent.preserve_consent(result, True, self.authority), result)

    def test_reject_changed_target_existing_consent_and_disabled_request(self):
        variants = [copy.deepcopy(self.config) for _ in range(4)]
        variants[0]["environments"]["fixture"]["site_content_url"] = "another-site"
        variants[1]["environments"]["fixture"]["url"] = "https://another.example.test"
        variants[2]["environments"]["other"] = dict(self.config["environments"]["fixture"])
        variants[3]["site_mutations"] = [{"server_url": self.authority["server_url"],
                                         "site_content_url": "fixture-site", "enabled": False}]
        for config in variants:
            with self.subTest(config=config), self.assertRaises(ValueError):
                consent.preserve_consent(config, True, self.authority)
        with self.assertRaises(ValueError):
            consent.preserve_consent(self.config, False, self.authority)

    def test_reject_granted_flags_credentials_extra_fields_and_noncanonical_origin(self):
        edits = [("consent_changes_authorized", True), ("credential_persistence_authorized", True),
                 ("pat_secret", "not-a-real-secret"), ("server_url", "https://user:password@example.test"),
                 ("server_url", "https://tableau.example.test/"), ("saved_consent", {}),
                 ("fixture_scope", "arbitrary-existing-projects")]
        for key, value in edits:
            item = authority()
            item[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                consent.validate_authority(item)


class PrepareTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.harness = self.root / "external"
        self.harness.mkdir()
        self.output = self.root / "new-preparation"
        self.files = {
            "integration/local_broker.cjs": (
                "const projectBroker = require('./project_broker.cjs');\n"
                "const sharedBroker = require('./shared_broker.cjs');\n"
                "function startBroker() {\n"
                "function allowed(state) {\n"
                "  if(state.guard?.execution_mode==='disposable_native'&&state.baselinePresent)return true;\n"
                "  if (Object.hasOwn(flags,'config')) return false;\n"
                "return projectBroker.allowed(state);\n}\n"
                "  if(state.guardPresent)try{state.guard=JSON.parse(fs.readFileSync('/cli-state/task-policy.json','utf8'));}catch{}\n  return state;\n"
                "    let result, executed=allowed(args,policyState);\n"
                "    const installer=body.helper==='installer'&&policyState.guard?.mode==='shared-installer';\n"
                "    if(body.helper&&!installer){res.writeHead(400);res.end();return;}\n"
                "    if(installer)executed=crypto.createHash('sha256').update(fs.readFileSync('/work/installer-assets/install.sh')).digest('hex')===policyState.guard.installer_sha256;\n"
                "      argv:[installer?'tadx-bench-installer':'tadx',...args],capability:installer?null:classify(args),flags:flagsOf(args),\n"
                "      cwd,binary_sha256:executedBinarySHA,started_at,executed,batch_input:batchInput,helper:installer?'installer':undefined})+'\\n');\n"
                "    if(executed&&installer){\n"
                "    if(installer){row.helper='installer';row.argv=['tadx-bench-installer',...args];row.capability=null;row.installer_sha256=policyState.guard.installer_sha256;}\n"),
            "tools/run_spark_suite.py": (
                "def local_config(config):\n"
                "    config.setdefault('run_constraints',{})['native_cli_execution']=True\n"
                "    source=fingerprint(manifest['repository_path'])\n"
                "    if source['digest']!=manifest.get('source_tree_digest'):\n"
                "        raise Blocked('TADX working tree changed since capture; rerun -CaptureCli before qualification')\n"
                "def main(argv=None):\n    return 0\n"),
            "integration/docker_local_bridge.py": (
                "def profile(req):\n    from integration import release_profiles,installer_profiles\n    return None\n"
                "def image_for(m):\n    broker_files=('Dockerfile.local','local_broker.cjs','project_broker.cjs')\n"
                "    broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py')\n"
                "def runtime_check(image):\n    version='codex-cli 0.154.0'\n    return version\n"
                "def _seed_site_mutation_consent(config, enabled):\n    return config\n\n\n"
                "def prepare(req):\n"
                "    return _seed_site_mutation_consent(\n"
                "                prepared.get('config_seed'), prepared['mutation_policy'] == 'enabled')\n"
                "def _deliver_split(req, s, work):\n"
                "    if not isinstance(s.get('broker_guard'), dict) or (s['broker_guard'].get('mode') not in () and not s['broker_guard'].get('mode','').startswith('shared-')):\n"
                "        raise Blocked('synthetic')\n"
                "    broker = docker('create', '--name', s['broker_container'], *_label_args(_split_labels(req, 'cli')), *common,\n"
                "        '--network', 'none')\n"
                "    if ready:\n        pass\n"
                "    else:\n        raise Blocked('Offline CLI broker did not become ready')\n"
                "    if s.get('config_seed') is not None:\n        pass\n"
                "    save(path, {'cli_mutation_policy': _policy_evidence(req, s, cli_info), 'cli_guidance_home': guidance_home,})\n"
                "def _freeze_split(req, s):\n"
                "    if broker is not None:\n        copies={}\n"
                "def _local_disposable_recovery(req, s):\n    return None\n"
                "def _local_before_worker_recovery(req, s):\n    return None\n"
                "def synthetic_model_dispatch(req,s,capture,Redactor,ROOT):\n"
                "        result=model_run(req,s,capture,Redactor,ROOT)\n"
                "if __name__=='__main__':\n    pass\n"),
        }
        self.files["integration/Dockerfile.local"] = (
            "FROM synthetic-worker:locked\nRUN npm install --global @openai/codex@0.154.0\n")
        self.files["integration/tadx_client.cjs"] = (
            "    request.on('error',reject);request.setTimeout(310000,()=>request.destroy(Error('CLI broker response timed out')));\n")
        self.files["integration/project_broker.cjs"] = "module.exports={allowed:()=>false};\n"
        self.files["integration/pulse_broker.cjs"] = (
            "const reads = new Set(['pulse.metric.list', 'pulse.metric.followers']);\n"
            "function allowed(cap) {\n  if (reads.has(cap)) {\n    return true;\n  }\n}\n"
            "module.exports={allowed};\n")
        self.files["integration/pulse_profiles.py"] = (
            "READS = {'pulse.metric.list',\n"
            "         'pulse.metric.inspect', 'pulse.metric.followers'}\n")
        self.files["integration/fault_broker.cjs"] = "module.exports={};\n"
        self.files["integration/credential_pty.py"] = "\n"
        self.files["integration/project_profiles.py"] = "\n".join(before for before, _ in PROJECT_REPLACEMENTS)
        for case in prep.PROJECT_CASES:
            for folder in ("suite/exercises", "fixtures/profiles"):
                self.files[f"{folder}/{case}.json"] = json.dumps({
                    "id": case, "agent": {"prompt": "Synthetic read task.", "public_bindings": {}},
                    "evaluator": {"requirements": []},
                })
        self.files["suite/index.json"] = json.dumps({
            "version": "synthetic", "commit": "a" * 40,
            "counts": {"exercises": 7},
            "exercises": [{"id": case, "path": "suite/exercises/" + case + ".json"}
                          for case in prep.PROJECT_CASES] +
                         [{"id": "P-other", "path": "suite/exercises/P-other.json"}],
        })
        self.lock = {"schema_version": 1, "files": {}}
        for name, content in self.files.items():
            blob = content.encode()
            self.put(self.harness / name, blob)
            self.lock["files"][name] = prep.sha(blob)
        self.catalog = self.root / "catalog.json"
        self.put(self.catalog, prep.encode([{
            "id": case.removeprefix("P-").replace("-", "."), "owner": "cli",
            "implementation": "implemented", "command_path": ["content", "project", case.split("-")[-1]],
        } for case in prep.CASES]))
        self.binary = self.root / "accepted-binary"
        self.put(self.binary, b"synthetic binary bytes; never execute")
        self.windows_binary = self.root / "accepted-windows-binary"
        self.put(self.windows_binary, b"synthetic Windows binary bytes; never execute")
        self.candidate = self.root / "candidate.json"
        self.put(self.candidate, prep.encode({
            "source_revision": "a" * 40, "source_tree_sha256": "b" * 64,
            "catalog_sha256": prep.sha(self.catalog.read_bytes()),
            "builds": {"linux/amd64": prep.sha(self.binary.read_bytes()),
                       "windows/amd64": prep.sha(self.windows_binary.read_bytes())},
        }))
        self.capture_root = self.root / "accepted-capture"
        self.capture = self.capture_root / "manifest.json"
        self.put(self.capture_root / "registry.json", self.catalog.read_bytes())
        skills = {}
        for package in ("tadx", "tadx-pulse"):
            skills[package] = {}
            for name in ("SKILL.md", "references/guide.md"):
                blob = (package + "/" + name).encode()
                self.put(self.capture_root / "internal/agent/skills" / package / name, blob)
                skills[package][name] = prep.sha(blob)
        self.help_hashes = {}
        for case in prep.CASES:
            name = case.removeprefix("P-").replace("-", ".") + ".txt"
            blob = ("synthetic help for " + case).encode()
            self.put(self.capture_root / "help" / name, blob)
            self.help_hashes[name] = prep.sha(blob)
        metadata = {}
        for system in ("linux", "windows"):
            path = self.capture_root / system / "build-metadata.txt"
            self.put(path, ("build\tvcs.revision=" + "a" * 40 + "\nbuild\tGOOS=" + system
                            + "\nbuild\tGOARCH=amd64\n").encode())
            metadata[system] = {"path": str(path), "sha256": prep.sha(path.read_bytes())}
        self.put(self.capture, prep.encode({
            "source_commit": "a" * 40, "source_tree_digest": "b" * 64,
            "capture_kind": "current-worktree-including-uncommitted-changes",
            "binary": str(self.windows_binary), "binary_sha256": prep.sha(self.windows_binary.read_bytes()),
            "binaries": {"windows": {"path": str(self.windows_binary),
                                     "sha256": prep.sha(self.windows_binary.read_bytes())},
                         "linux": {"path": str(self.binary), "sha256": prep.sha(self.binary.read_bytes())}},
            "registry_sha256": prep.sha(self.catalog.read_bytes()), "skills": skills,
            "help_sha256": self.help_hashes,
            "build_metadata": metadata,
        }))
        self.authority = self.root / "authority.json"
        self.put(self.authority, prep.encode(authority()))
        self.evidence = self.root / "consent-evidence.json"
        self.put(self.evidence, b"synthetic reviewed consent evidence")

    def put(self, path, blob):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(blob)

    def test_full_locked_inventory_requires_all_124_executable_actions(self):
        files = dict(self.files)
        for case in prep.PROJECT_CASES:
            name = "fixtures/profiles/" + case + ".json"
            files[name] = prep.encode({"id": case, "base_profile":
                                       case.removeprefix("P-").replace("-", ".")})
        for number in range(110):
            case = f"P-fixture-{number:03d}"
            files["suite/exercises/" + case + ".json"] = prep.encode({"id": case})
            files["fixtures/profiles/" + case + ".json"] = prep.encode(
                {"id": case, "base_profile": f"fixture.action{number:03d}"})
        actions = prep.required_actions(files)
        self.assertEqual(len(actions), 124)
        self.assertIn("policy.install", actions)
        self.assertIn("pulse.subscription.list", actions)
        rows = prep.parse(self.catalog.read_bytes())
        rows.extend({"id": action, "owner": "cli", "implementation": "implemented",
                     "command_path": ["synthetic", action]} for action in actions
                    if action not in {row["id"] for row in rows})
        catalog = prep.encode(rows)
        candidate = prep.parse(self.candidate.read_bytes())
        candidate["catalog_sha256"] = prep.sha(catalog)
        builds = {"linux/amd64": self.binary.read_bytes(),
                  "windows/amd64": self.windows_binary.read_bytes()}
        prep.validate_candidate(candidate, catalog, builds, actions)
        omitted = prep.encode([row for row in rows if row["id"] != "pulse.subscription.list"])
        candidate["catalog_sha256"] = prep.sha(omitted)
        with self.assertRaisesRegex(ValueError, "required executable action"):
            prep.validate_candidate(candidate, omitted, builds, actions)

    def run_prepare(self):
        # The anchor fixture joins fragments, so its project module is not valid Python.
        with patch.object(prep, "validate_source_closure"):
            return prep.prepare(self.harness, self.output, self.candidate, self.catalog,
                                {"linux/amd64": self.binary, "windows/amd64": self.windows_binary},
                                self.capture, self.authority, self.evidence, lock=self.lock)

    def test_snapshot_is_blocked_exact_and_excludes_unlisted_secrets_and_history(self):
        self.put(self.harness / "config/secret-bindings.json", b"MUST NOT COPY")
        self.put(self.harness / "old-run/task.json", b"MUST NOT COPY")
        manifest = self.run_prepare()
        self.assertEqual(manifest["status"], "prepared_blocked")
        self.assertFalse(manifest["live_execution_enabled"])
        self.assertEqual(manifest["cases"], sorted(prep.CASES))
        index = prep.parse((self.output / "source/suite/index.json").read_bytes())
        self.assertEqual(index["counts"], {"exercises": 14})
        self.assertEqual({row["id"] for row in index["exercises"]}, set(prep.CASES))
        for case in prep.JOB_CASES:
            exercise = prep.parse((self.output / "source/suite/exercises" / (case + ".json")).read_bytes())
            profile = prep.parse((self.output / "source/fixtures/profiles" / (case + ".json")).read_bytes())
            self.assertEqual(exercise["id"], case)
            self.assertEqual(profile["id"], case)
            self.assertEqual(exercise["evaluator"]["coverage_evidence"]["required_actions"],
                             [case.removeprefix("P-").replace("-", ".")])
            if case == "P-job-wait":
                self.assertNotIn("authoritative-job-status",
                                 [row["id"] for row in exercise["evaluator"]["expected_outcomes"]])
        for case in prep.POLICY_CASES:
            exercise = prep.parse((self.output / "source/suite/exercises" / (case + ".json")).read_bytes())
            profile = prep.parse((self.output / "source/fixtures/profiles" / (case + ".json")).read_bytes())
            self.assertEqual(exercise["evaluator"]["coverage_evidence"]["required_actions"],
                             [case.removeprefix("P-").replace("-", ".")])
            self.assertEqual(profile["independent_observation_contract"], "local_policy")
            self.assertFalse(exercise["evaluator"]["allowed_effects"]["remote"])
        subscription = prep.parse((self.output / "source/suite/exercises/P-pulse-subscription-list.json").read_bytes())
        self.assertEqual(subscription["evaluator"]["coverage_evidence"]["required_actions"],
                         ["pulse.subscription.list"])
        self.assertTrue((self.output / "source/suite/exercises/P-policy-install.json").exists())
        self.assertIsNone(manifest["actual_model_metadata"])
        self.assertEqual((self.output / "candidate/builds/linux/amd64/tadx").read_bytes(), self.binary.read_bytes())
        self.assertEqual((self.output / "candidate/builds/windows/amd64/tadx.exe").read_bytes(),
                         self.windows_binary.read_bytes())
        self.assertEqual((self.output / "candidate/capture/internal/agent/skills/tadx/SKILL.md").read_bytes(),
                         b"tadx/SKILL.md")
        self.assertEqual(prep.parse((self.output / "candidate/capture/manifest.json").read_bytes())
                         ["binaries"]["linux"]["sha256"], prep.sha(self.binary.read_bytes()))
        for name, digest in manifest["snapshot_files"].items():
            self.assertEqual(prep.sha((self.output / name).read_bytes()), digest)
        self.assertFalse((self.output / "source/config").exists())
        self.assertFalse((self.output / "source/old-run").exists())
        self.assertNotIn("fixture-site", (self.output / "preparation.json").read_text())
        for name, digest in manifest["prepared_files"].items():
            self.assertEqual(prep.sha((self.output / "source" / name).read_bytes()), digest)
        for name, digest in self.lock["files"].items():
            self.assertEqual(prep.sha((self.harness / name).read_bytes()), digest)
        source = (self.output / "source/tools/run_spark_suite.py").read_text()
        namespace = {"Blocked": RuntimeError}
        exec(compile(source, "synthetic-launcher", "exec"), namespace)
        with self.assertRaisesRegex(RuntimeError, "blocked"):
            namespace["main"]()
        with self.assertRaisesRegex(RuntimeError, "G0 qualification"):
            namespace["local_config"]({})
        broker = (self.output / "source/integration/local_broker.cjs").read_text()
        self.assertIn("execution_mode==='disposable_native')return false", broker)
        self.assertIn("return projectBroker.allowed(state)", broker)
        self.assertLess(broker.index("g9_policy_install_broker.cjs').allowed("),
                        broker.index("execution_mode==='disposable_native')return false"))
        self.assertLess(broker.index("g9_policy_broker.cjs').allowed("),
                        broker.index("execution_mode==='disposable_native')return false"))
        self.assertEqual((self.output / "source/integration/g9_policy_broker.cjs").read_bytes(),
                         (Path(__file__).resolve().parent / "source/integration/policy_broker.cjs").read_bytes())
        self.assertEqual((self.output / "source/integration/g9_policy_private.py").read_bytes(),
                         (Path(__file__).resolve().parent / "source/integration/policy_private.py").read_bytes())
        image = (self.output / "source/integration/Dockerfile.local").read_text()
        bridge = (self.output / "source/integration/docker_local_bridge.py").read_text()
        self.assertIn("'g9_policy_broker.cjs'", bridge)
        self.assertIn("'g9_policy_install_broker.cjs'", bridge)
        self.assertIn("broker_common.remove('--read-only')", bridge)
        self.assertIn("g9_policy_install_private.capture(req, s, broker['Id'], docker)", bridge)
        self.assertIn("broker_common[broker_common.index(work_mount)] = work_mount + ',readonly'", bridge)
        self.assertIn("g9_policy_private.setup(req, s, work, docker)", bridge)
        self.assertIn("g9_policy_private.capture(req, s, identity, docker)", bridge)
        self.assertIn("'policy_private_setup': s.get('policy_private_setup')", bridge)
        self.assertLess(bridge.index("if req.get('exercise', {}).get('id') in ('P-policy-install'"),
                        bridge.index("def _local_disposable_recovery(req, s):") + 300)
        self.assertIn("@openai/codex@0.160.0", image)
        self.assertIn("codex-cli 0.160.0", bridge)
        self.assertNotIn("0.154.0", image + bridge)
        bridge = (self.output / "source/integration/docker_local_bridge.py").read_text()
        compile(bridge, "synthetic-bridge", "exec")
        self.assertIn("g9_saved_consent_authority", bridge)
        with self.assertRaisesRegex(SystemExit, "blocked"):
            exec(compile(bridge, "synthetic-bridge", "exec"), {"__name__": "__main__"})

    def test_wrong_source_hash_fails_before_output(self):
        self.put(self.harness / "integration/local_broker.cjs", b"different source")
        with self.assertRaisesRegex(ValueError, "source differs"):
            self.run_prepare()
        self.assertFalse(self.output.exists())

    def test_matching_hash_with_wrong_or_duplicate_patch_anchor_still_rejected(self):
        path = "tools/run_spark_suite.py"
        for content in ("def other(): pass", self.files[path] + "def main(argv=None):\n    pass\n"):
            self.put(self.harness / path, content.encode())
            self.lock["files"][path] = prep.sha(content.encode())
            with self.assertRaisesRegex(ValueError, "anchor"):
                self.run_prepare()
            self.assertFalse(self.output.exists())

    def test_changed_binary_catalog_or_consent_evidence_fails(self):
        for path in (self.binary, self.windows_binary, self.catalog, self.evidence):
            original = path.read_bytes()
            path.write_bytes(original + b"changed")
            with self.subTest(path=path.name), self.assertRaises(ValueError):
                self.run_prepare()
            self.assertFalse(self.output.exists())
            path.write_bytes(original)

    def test_changed_capture_guidance_help_or_build_metadata_fails(self):
        paths = (self.capture_root / "internal/agent/skills/tadx/SKILL.md",
                 self.capture_root / "help" / next(iter(self.help_hashes)),
                 self.capture_root / "linux/build-metadata.txt")
        for path in paths:
            original = path.read_bytes()
            path.write_bytes(original + b"changed")
            with self.subTest(path=path.name), self.assertRaises(ValueError):
                self.run_prepare()
            self.assertFalse(self.output.exists())
            path.write_bytes(original)

    def test_unlisted_guidance_or_help_file_fails(self):
        for path in (self.capture_root / "internal/agent/skills/tadx/extra.md",
                     self.capture_root / "help/extra.txt"):
            self.put(path, b"unlisted")
            with self.subTest(path=path.name), self.assertRaisesRegex(ValueError, "file set"):
                self.run_prepare()
            self.assertFalse(self.output.exists())
            path.unlink()

    def test_internally_consistent_capture_with_missing_help_is_rejected(self):
        capture = prep.parse(self.capture.read_bytes())
        name = next(iter(capture["help_sha256"]))
        del capture["help_sha256"][name]
        (self.capture_root / "help" / name).unlink()
        self.put(self.capture, prep.encode(capture))
        with self.assertRaisesRegex(ValueError, "accepted executable commands"):
            self.run_prepare()
        self.assertFalse(self.output.exists())

    def test_metadata_platform_substring_is_not_accepted(self):
        path = self.capture_root / "linux/build-metadata.txt"
        self.put(path, path.read_bytes().replace(b"GOOS=linux", b"GOOS=linuxx"))
        capture = prep.parse(self.capture.read_bytes())
        capture["build_metadata"]["linux"]["sha256"] = prep.sha(path.read_bytes())
        self.put(self.capture, prep.encode(capture))
        with self.assertRaisesRegex(ValueError, "differs from candidate"):
            self.run_prepare()
        self.assertFalse(self.output.exists())

    def test_capture_from_other_source_or_binary_is_rejected(self):
        original = self.capture.read_bytes()
        for edit in ({"source_commit": "c" * 40},
                     {"source_tree_digest": "c" * 64},
                     {"binaries": {"windows": {"path": str(self.windows_binary), "sha256": "c" * 64},
                                    "linux": {"path": str(self.binary), "sha256": prep.sha(self.binary.read_bytes())}}}):
            capture = prep.parse(original)
            capture.update(edit)
            self.put(self.capture, prep.encode(capture))
            with self.subTest(edit=edit), self.assertRaises(ValueError):
                self.run_prepare()
            self.assertFalse(self.output.exists())
        self.put(self.capture, original)

    def test_missing_or_changed_runtime_source_import_rejected(self):
        bridge = (b"def image_for(m):\n    broker_files=('Dockerfile.local','missing_broker.cjs')\n"
                  b"def runtime_check(image):\n    pass\n")
        files = {"integration/docker_local_bridge.py": bridge,
                 "integration/Dockerfile.local": b"FROM fixed\n",
                 "integration/local_broker.cjs": b"require('./missing_broker.cjs');\n"}
        with self.assertRaisesRegex(ValueError, "broker import"):
            prep.validate_source_closure(files, self.harness)
        files["integration/local_broker.cjs"] = b"module.exports = {};\n"
        with self.assertRaisesRegex(ValueError, "image input"):
            prep.validate_source_closure(files, self.harness)
        self.put(self.harness / "bench/missing.py", b"pass\n")
        files["integration/docker_local_bridge.py"] = (b"from bench.missing import thing\n"
                                                      b"def image_for(m):\n    broker_files=('Dockerfile.local',)\n"
                                                      b"def runtime_check(image):\n    pass\n")
        with self.assertRaisesRegex(ValueError, "Python import"):
            prep.validate_source_closure(files, self.harness)

    def test_missing_project_action_fails_even_when_catalog_hash_matches(self):
        rows = prep.parse(self.catalog.read_bytes())[:-1]
        self.put(self.catalog, prep.encode(rows))
        candidate = prep.parse(self.candidate.read_bytes())
        candidate["catalog_sha256"] = prep.sha(self.catalog.read_bytes())
        self.put(self.candidate, prep.encode(candidate))
        with self.assertRaisesRegex(ValueError, "required executable action"):
            self.run_prepare()

    def test_existing_output_and_source_overlap_rejected(self):
        self.output.mkdir()
        with self.assertRaisesRegex(ValueError, "new directory"):
            self.run_prepare()
        self.output = self.harness / "new"
        with self.assertRaisesRegex(ValueError, "overlaps"):
            self.run_prepare()

    def test_unsafe_lock_paths_and_duplicate_json_keys_rejected(self):
        for name in ("../secret.json", "C:/secret.json", "nested\\secret.json", "/absolute.json"):
            with self.subTest(name=name), self.assertRaises(ValueError):
                prep.relative(name)
        with self.assertRaises(ValueError):
            prep.parse('{"a":1,"a":2}')

    def test_reparse_points_rejected_before_read(self):
        fake = type("Stat", (), {"st_mode": 0, "st_file_attributes": 0x400})()
        with patch.object(Path, "lstat", return_value=fake), self.assertRaisesRegex(ValueError, "reparse"):
            prep.plain_path(self.binary)


if __name__ == "__main__":
    unittest.main()
