"""Explicit current-suite anchors for the blocked release preparation."""

from __future__ import annotations

import ast
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest
import zipfile


HERE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(HERE))
import prepare as prep  # noqa: E402
from release_update_patches import definitions, patch_broker, go_toolchain  # noqa: E402
from release_update_build import RUNTIME  # noqa: E402


class ReleaseUpdateIntegrationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        supplied = os.environ.get("TADX_G9_RELEASE_TEST_SUITE")
        if not supplied:
            raise AssertionError("An explicit current suite is required for release anchor tests")
        cls.suite = Path(supplied).resolve(strict=True)
        cls.lock = prep.parse((HERE / "source-lock-124.json").read_bytes())
        if len(cls.lock["files"]) != 373:
            raise AssertionError("The reviewed current-suite source lock differs")
        version = subprocess.run(["go", "version"], capture_output=True, text=True,
                                 check=True).stdout
        match = re.match(r"^go version (go[0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)", version)
        if match is None:
            raise AssertionError("Installed Go toolchain version is unavailable")
        cls.toolchain = match.group(1)

    def read(self, name):
        expected = self.lock["files"].get(name)
        self.assertIsNotNone(expected)
        data = (self.suite / name).read_bytes()
        self.assertEqual(prep.sha(data), expected)
        return data

    def test_definition_changes_match_reviewed_v2_without_dropped_assertions(self):
        exercise_name = "suite/exercises/P-current-update.json"
        profile_name = "fixtures/profiles/P-current-update.json"
        original_exercise = prep.parse(self.read(exercise_name))
        original_profile = prep.parse(self.read(profile_name))
        exercise, profile = definitions(original_exercise, original_profile)
        self.assertEqual(prep.sha(prep.encode(exercise)),
                         "55dca612483fe4e85aebf62af94c483169e06ae38bfff2a9ca932e0c0f3e5f7c")
        self.assertEqual(prep.sha(prep.encode(profile)),
                         "a1b5f1016a44d569afc6460a705e75f9e32d3298695ebc445091f475367058ba")
        self.assertEqual(exercise["evaluator"]["requirements"], original_exercise["evaluator"]["requirements"])
        self.assertEqual(profile["expected_outcome_assertions"], original_profile["expected_outcome_assertions"])

    def test_broker_guard_matches_reviewed_v2_original_relative_delta(self):
        original = self.read("integration/local_broker.cjs").decode("utf-8").replace("\r\n", "\n")
        patched = patch_broker(original)
        self.assertEqual(prep.sha(patched.encode()),
                         "e61f97baa77b54a815f2eeeb7dc606860868d7ff5436a3841aa396f6eb06f47e")
        self.assertLess(patched.index("if(state.guard?.mode==='shared-release')"),
                        patched.index("if(state.guard?.execution_mode==='disposable_native'"))

    def test_full_copy_preserves_mutation_and_adds_release_without_unlocking_source(self):
        files = {name: self.read(name) for name in self.lock["files"]}
        original_lock = json.dumps(self.lock, sort_keys=True)
        patches = prep.apply_mutation_runtime(files)
        patches.extend(prep.patch_sources(files))
        self.assertEqual(json.dumps(self.lock, sort_keys=True), original_lock)
        self.assertTrue(any(row["path"] == "integration/docker_local_bridge.py" for row in patches))
        bridge = files["integration/docker_local_bridge.py"].decode("utf-8")
        broker = files["integration/local_broker.cjs"].decode("utf-8")
        ast.parse(bridge)
        self.assertIn("def _policy_args(req, s):", bridge)
        self.assertIn("def _policy_evidence(req, s, details):", bridge)
        self.assertIn("def _deliver_candidate_release(req, s):", bridge)
        self.assertIn("def _observe_candidate_release(req, s, filename):", bridge)
        self.assertIn("if(state.guard?.mode==='shared-release')", broker)
        self.assertIn("if(state.guard?.family==='g9-policy-install')", broker)
        self.assertIn("g9_windows_installer_broker.cjs", broker)
        with tempfile.TemporaryDirectory(prefix="release-broker-syntax-") as temporary:
            path = Path(temporary) / "local_broker.cjs"
            path.write_text(broker, encoding="utf-8")
            result = subprocess.run(["node", "--check", str(path)], capture_output=True, check=False)
            self.assertEqual(result.returncode, 0, "Combined broker syntax differs")

    def test_native_metadata_requires_one_toolchain(self):
        both = {system: f"candidate: {self.toolchain}\nbuild\tGOOS=linux\n".encode()
                for system in ("windows", "linux")}
        self.assertEqual(go_toolchain(both), self.toolchain)
        both["windows"] = b"candidate: go9.9.9\n"
        with self.assertRaisesRegex(ValueError, "different Go toolchains"):
            go_toolchain(both)

    def test_preparer_rebuilds_assets_and_binds_independent_source_lock(self):
        source = {
            "scripts/refactor/harness/release_update_build.py":
                (HERE / "release_update_build.py").read_bytes(),
            "scripts/refactor/run.py": (HERE.parent / "run.py").read_bytes(),
            "go.mod": ("module github.com/ahillspace/tadx\n\ngo " +
                       ".".join(self.toolchain[2:].split(".")[:2]) + "\n").encode(),
            "cmd/tadx/main.go": b'package main\nimport "github.com/ahillspace/tadx/internal/version"\nfunc main() { println(version.BuildVersion) }\n',
            "internal/version/version.go": b'package version\nvar BuildVersion = "dev"\n',
            "internal/agent/skills/tadx/SKILL.md": b"# TADX\n",
            "internal/agent/skills/tadx-pulse/SKILL.md": b"# TADX Pulse\n",
        }
        for name in ("gh", "curl", "transport.cjs"):
            source[f"scripts/refactor/harness/release_update/{name}"] = (
                HERE / "release_update" / name).read_bytes()
        for name in RUNTIME:
            source[name] = (HERE.parents[2] / name).read_bytes()
        with tempfile.TemporaryDirectory(prefix="release-preparer-test-") as temporary:
            root = Path(temporary)
            archive = root / "source.zip"
            with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
                for name, blob in sorted(source.items()):
                    bundle.writestr(name, blob)
            rows = "".join(f"{name} {prep.sha(source[name])}\n" for name in sorted(source))
            candidate = {"source_revision": "a" * 40,
                         "source_tree_sha256": hashlib.sha256(rows.encode()).hexdigest()}
            captured = {f"candidate/capture/build-metadata/{system}.txt":
                        f"candidate: {self.toolchain}\n".encode()
                        for system in ("windows", "linux")}
            names = ("suite/exercises/P-current-update.json",
                     "fixtures/profiles/P-current-update.json")
            files = {name: self.read(name) for name in names}
            patches, lock_hash, asset_hash = prep.prepare_release(
                files, candidate, captured, archive, root / "prepared")
            self.assertEqual(prep.sha(files["integration/release_update_source.json"]), lock_hash)
            self.assertEqual(prep.parse(files["integration/release_update_source.json"])["source_fingerprint"],
                             candidate["source_tree_sha256"])
            self.assertEqual(prep.sha(files["fixtures/release-update/manifest.json"]), asset_hash)
            self.assertIn("fixtures/release-update/target-tadx", files)
            self.assertIn("integration/release_update_profile.py", files)
            self.assertEqual(len({row["path"] for row in patches}), len(patches))

            rejected = {name: self.read(name) for name in names}
            candidate["source_tree_sha256"] = "0" * 64
            with self.assertRaisesRegex(ValueError, "fingerprint differs"):
                prep.prepare_release(rejected, candidate, captured, archive, root / "prepared")

            observer = "scripts/refactor/harness/release_update/observer.cjs"
            source[observer] += b"\n// substituted runtime owner\n"
            changed_archive = root / "substituted.zip"
            with zipfile.ZipFile(changed_archive, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
                for name, blob in sorted(source.items()):
                    bundle.writestr(name, blob)
            rows = "".join(f"{name} {prep.sha(source[name])}\n" for name in sorted(source))
            candidate["source_tree_sha256"] = hashlib.sha256(rows.encode()).hexdigest()
            with self.assertRaisesRegex(ValueError, "recipe differs"):
                prep.prepare_release(rejected, candidate, captured, changed_archive,
                                     root / "prepared")
            self.assertEqual(set(rejected), set(names))


if __name__ == "__main__":
    unittest.main()
