"""Offline source and asset contracts for the maintained release builder."""

from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import types
import unittest
import zipfile


HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("release_update_build", HERE / "release_update_build.py")
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)


class ReleaseBuildTest(unittest.TestCase):
    def test_transport_templates_match_prepared_owner_map(self):
        self.assertEqual(builder.TRANSPORT, (
            "scripts/refactor/harness/release_update/gh",
            "scripts/refactor/harness/release_update/curl",
            "scripts/refactor/harness/release_update/transport.cjs",
        ))

    def setUp(self):
        version = subprocess.run(["go", "version"], capture_output=True, text=True,
                                 check=True).stdout
        match = re.match(r"^go version (go[0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)", version)
        self.assertIsNotNone(match, "Installed Go toolchain version is unavailable")
        self.toolchain = match.group(1)
        self.temporary = tempfile.TemporaryDirectory(prefix="release-builder-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = {
            builder.BUILDER: (HERE / "release_update_build.py").read_bytes(),
            builder.ENVIRONMENT: (HERE.parents[0] / "run.py").read_bytes(),
            "go.mod": ("module github.com/ahillspace/tadx\n\ngo " +
                       ".".join(self.toolchain[2:].split(".")[:2]) + "\n").encode(),
            "cmd/tadx/main.go": b'package main\nimport "github.com/ahillspace/tadx/internal/version"\nfunc main() { println(version.BuildVersion) }\n',
            "internal/version/version.go": b'package version\nvar BuildVersion = "dev"\n',
            "internal/agent/skills/tadx/SKILL.md": b"# TADX\n",
            "internal/agent/skills/tadx-pulse/SKILL.md": b"# TADX Pulse\n",
            builder.TRANSPORT[0]: b"#!/bin/sh\nexit 1\n",
            builder.TRANSPORT[1]: b"#!/bin/sh\nexit 1\n",
            builder.TRANSPORT[2]: b"module.exports={}\n",
        }
        for name in builder.RUNTIME:
            self.source[name] = (HERE.parents[2] / name).read_bytes()
        self.archive = self.root / "source.zip"
        self.lock_path = self.root / "source-lock.json"
        self.output = self.root / "assets"
        self.write_source()

    def write_source(self):
        with zipfile.ZipFile(self.archive, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
            for name, data in sorted(self.source.items()):
                entry = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
                entry.compress_type = zipfile.ZIP_DEFLATED
                bundle.writestr(entry, data)
        records = "".join(f"{name} {hashlib.sha256(self.source[name]).hexdigest()}\n"
                          for name in sorted(self.source, key=lambda value: value.encode("utf-8")))
        self.lock = {
            "schema_version": 1, "source_commit": "a" * 40,
            "source_fingerprint": hashlib.sha256(records.encode()).hexdigest(),
            "source_archive_sha256": builder.digest(self.archive), "go_toolchain": self.toolchain,
        }
        self.lock_path.write_text(json.dumps(self.lock, sort_keys=True) + "\n", encoding="utf-8")

    def test_builds_bounded_offline_candidate_assets(self):
        manifest = builder.build(self.archive, self.lock_path, self.output)
        self.assertEqual(manifest["source_commit"], "a" * 40)
        self.assertEqual(manifest["starting_version"], "0.1.3-fixture.aaaaaaa")
        self.assertEqual(manifest["target_version"], "0.1.4-fixture.aaaaaaa")
        self.assertEqual(manifest["source_lock_sha256"], builder.digest(self.lock_path))
        self.assertRegex(manifest["build"]["go_binary_sha256"], r"^[0-9a-f]{64}$")
        self.assertEqual(manifest["build"]["recipe"][builder.BUILDER], builder.digest(HERE / "release_update_build.py"))
        self.assertEqual(set(manifest["build"]["recipe"]), set(builder.RECIPE))
        self.assertNotEqual(manifest["baseline_binary_sha256"], manifest["target_binary_sha256"])
        self.assertEqual(manifest["files"]["guidance/tadx/SKILL.md"],
                         hashlib.sha256(b"# TADX\n").hexdigest())
        with tarfile.open(self.output / manifest["archive"], "r:gz") as bundle:
            self.assertEqual([entry.name for entry in bundle.getmembers()], ["tadx"])
            self.assertEqual(hashlib.sha256(bundle.extractfile("tadx").read()).hexdigest(),
                             manifest["target_binary_sha256"])
        self.assertFalse((self.root / "source").exists())

    def test_uses_verified_helper_even_if_module_name_is_preloaded(self):
        foreign = types.ModuleType("scripts.refactor.run")
        foreign.child_environment = lambda _: self.fail("unverified helper ran")
        original = sys.modules.get("scripts.refactor.run")
        sys.modules["scripts.refactor.run"] = foreign
        try:
            manifest = builder.build(self.archive, self.lock_path, self.output)
        finally:
            if original is None:
                sys.modules.pop("scripts.refactor.run", None)
            else:
                sys.modules["scripts.refactor.run"] = original
        self.assertEqual(manifest["source_commit"], "a" * 40)

    def test_rejects_wrong_archive_without_output(self):
        self.source[builder.TRANSPORT[2]] = b"module.exports={changed:true}\n"
        self.write_source()
        self.lock["source_archive_sha256"] = "0" * 64
        self.lock_path.write_text(json.dumps(self.lock), encoding="utf-8")
        with self.assertRaisesRegex(builder.BuildError, "source archive differs"):
            builder.build(self.archive, self.lock_path, self.output)
        self.assertFalse(self.output.exists())

    def test_rejects_substituted_running_recipe_even_with_new_lock(self):
        self.source[builder.BUILDER] += b"\n# substituted\n"
        self.write_source()
        with self.assertRaisesRegex(builder.BuildError, "running builder differs"):
            builder.build(self.archive, self.lock_path, self.output)
        self.assertFalse(self.output.exists())

    def test_rejects_substituted_environment_helper_even_with_new_lock(self):
        self.source[builder.ENVIRONMENT] += b"\n# substituted\n"
        self.write_source()
        with self.assertRaisesRegex(builder.BuildError, "environment helper differs"):
            builder.build(self.archive, self.lock_path, self.output)
        self.assertFalse(self.output.exists())

    def test_rejects_source_fingerprint_mismatch_after_archive_repin(self):
        self.source["cmd/tadx/main.go"] += b"// changed\n"
        self.write_source()
        self.lock["source_fingerprint"] = "0" * 64
        self.lock_path.write_text(json.dumps(self.lock), encoding="utf-8")
        with self.assertRaisesRegex(builder.BuildError, "fingerprint differs"):
            builder.build(self.archive, self.lock_path, self.output)
        self.assertFalse(self.output.exists())

    def test_rejects_wrong_toolchain_without_output(self):
        self.lock["go_toolchain"] = "go9.9.9"
        self.lock_path.write_text(json.dumps(self.lock), encoding="utf-8")
        with self.assertRaisesRegex(builder.BuildError, "toolchain differs"):
            builder.build(self.archive, self.lock_path, self.output)
        self.assertFalse(self.output.exists())

    def test_rejects_substituted_transport_under_original_lock(self):
        original = self.lock_path.read_bytes()
        self.source[builder.TRANSPORT[0]] += b"# changed\n"
        self.write_source()
        self.lock_path.write_bytes(original)
        with self.assertRaisesRegex(builder.BuildError, "source archive differs"):
            builder.build(self.archive, self.lock_path, self.output)

    def test_rejects_duplicate_lock_field(self):
        self.lock_path.write_text('{"schema_version":1,"schema_version":1}', encoding="utf-8")
        with self.assertRaisesRegex(builder.BuildError, "duplicate source-lock field"):
            builder.source_lock(self.lock_path)

    def test_rejects_archive_path_escape(self):
        with zipfile.ZipFile(self.archive, "a") as bundle:
            bundle.writestr("../outside", b"bad")
        self.lock["source_archive_sha256"] = builder.digest(self.archive)
        self.lock_path.write_text(json.dumps(self.lock), encoding="utf-8")
        destination = self.root / "extracted"
        destination.mkdir()
        with self.assertRaisesRegex(builder.BuildError, "unsafe or duplicate path"):
            builder.extract_source(self.archive, self.lock, destination)
        self.assertFalse((self.root / "outside").exists())

    def test_rejects_windows_drive_archive_path(self):
        with zipfile.ZipFile(self.archive, "a") as bundle:
            bundle.writestr("C:/outside", b"bad")
        self.lock["source_archive_sha256"] = builder.digest(self.archive)
        self.lock_path.write_text(json.dumps(self.lock), encoding="utf-8")
        destination = self.root / "extracted"
        destination.mkdir()
        with self.assertRaisesRegex(builder.BuildError, "unsafe or duplicate path"):
            builder.extract_source(self.archive, self.lock, destination)

    def test_rejects_existing_output_without_overwrite(self):
        self.output.mkdir()
        sentinel = self.output / "sentinel"
        sentinel.write_text("keep", encoding="utf-8")
        with self.assertRaisesRegex(builder.BuildError, "new directory"):
            builder.build(self.archive, self.lock_path, self.output)
        self.assertEqual(sentinel.read_text(encoding="utf-8"), "keep")


if __name__ == "__main__":
    unittest.main()
