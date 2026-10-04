"""Synthetic offline capture and copied-input qualification tests."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import capture as cap
import qualify

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "harness"))
import test_prepare  # noqa: E402


class CaptureTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        self.source.mkdir()
        self.output = self.root / "capture"
        self.put(self.source / "go.mod", b"module fixture.test/capture\n\ngo 1.26\n")
        self.put(self.source / "main.go", b"package main\nfunc main() {}\n")
        self.put(self.source / "scripts/embed.go", b"package scripts\n//go:embed install.ps1\n")
        self.put(self.source / "scripts/install.ps1", b"Write-Output 'accepted'\n")
        self.put(self.source / "scripts/install.sh", b"echo accepted\n")
        self.put(self.source / ".gitignore", b"ignored.bin\n")
        self.git("init", "-q")
        self.git("add", ".")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
                 "commit", "-qm", "fixture")
        self.identity = cap.fingerprint(self.source)
        self.catalog = self.root / "catalog.json"
        self.put(self.catalog, cap.encode([{
            "id": "project.list", "owner": "cli", "implementation": "implemented",
            "command_path": ["content", "project", "list"],
        }]))
        self.builds = {}
        for system in ("windows", "linux"):
            path = self.root / (system + "-accepted")
            self.put(path, (system + " accepted binary").encode())
            self.builds[system + "/amd64"] = path
        self.candidate = self.root / "candidate.json"
        self.put(self.candidate, cap.encode({
            "source_revision": self.identity["commit"],
            "source_tree_sha256": self.identity["digest"],
            "catalog_sha256": cap.sha(self.catalog.read_bytes()),
            "builds": {name: cap.sha(path.read_bytes()) for name, path in self.builds.items()},
        }))
        self.guidance = self.root / "guidance"
        for package in ("tadx", "tadx-pulse"):
            self.put(self.guidance / package / "SKILL.md", b"synthetic skill")
            self.put(self.guidance / package / "references/guide.md", b"synthetic reference")
        self.help = self.root / "help"
        self.put(self.help / "project.list.txt", b"synthetic help")

    @staticmethod
    def put(path, blob):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(blob)

    def git(self, *args):
        subprocess.run(["git", "-C", str(self.source), *args], check=True,
                       capture_output=True)

    def inspect(self, path):
        system = "windows" if "windows" in path.name else "linux"
        return ("build\tvcs.revision=" + self.identity["commit"] + "\n"
                + "build\tvcs.modified=false\n"
                + "build\tGOOS=" + system + "\nbuild\tGOARCH=amd64\n").encode()

    def run_capture(self):
        return cap.capture(self.source, self.output, self.candidate, self.catalog,
                           self.builds, self.guidance, self.help, inspect=self.inspect)

    def test_cli_help_and_required_help_directory(self):
        script = Path(cap.__file__).resolve()
        usage = subprocess.run([sys.executable, "-B", str(script), "--help"],
                               capture_output=True, text=True, encoding="utf-8", check=False)
        self.assertEqual(usage.returncode, 0, usage.stderr)
        self.assertIn("--help-dir HELP", usage.stdout)

        invocation = subprocess.run([
            sys.executable, "-B", str(script), "--source", str(self.source),
            "--output", str(self.output), "--candidate", str(self.candidate),
            "--catalog", str(self.catalog), "--guidance", str(self.guidance),
            "--help-dir", str(self.help),
            "--build", "windows/amd64=" + str(self.builds["windows/amd64"]),
            "--build", "linux/amd64=" + str(self.builds["linux/amd64"]),
        ], capture_output=True, text=True, encoding="utf-8", check=False)
        self.assertEqual(invocation.returncode, 1)
        self.assertEqual(json.loads(invocation.stdout)["status"], "blocked")
        self.assertNotIn("argparse", invocation.stderr)
        self.assertFalse(self.output.exists())

    def test_copies_only_accepted_bytes_and_preparer_manifest_shape(self):
        result = self.run_capture()
        self.assertFalse(result["live_execution_enabled"])
        manifest = cap.parse((self.output / "manifest.json").read_bytes())
        self.assertEqual(manifest["binaries"]["linux"]["sha256"],
                         cap.sha(self.builds["linux/amd64"].read_bytes()))
        self.assertEqual((self.output / "registry.json").read_bytes(), self.catalog.read_bytes())
        self.assertEqual(set(manifest["help_sha256"]), {"project.list.txt"})
        self.assertEqual(set(manifest["skills"]), {"tadx", "tadx-pulse"})
        full = cap.parse((self.output / "complete-source-fingerprint.json").read_bytes())
        self.assertEqual(full["full_digest"], manifest["complete_source_tree_digest"])
        self.assertEqual(full["full_ordinal_sha256"], manifest["complete_source_ordinal_sha256"])
        ordinal = "".join(f"{name} {full['full_files'][name]}\n"
                          for name in sorted(full["full_files"], key=lambda value: value.encode("utf-8")))
        self.assertEqual(manifest["complete_source_ordinal_sha256"], cap.sha(ordinal.encode()))
        self.assertNotEqual(manifest["complete_source_ordinal_sha256"],
                            manifest["source_tree_digest"])
        self.assertIn("scripts/install.ps1", full["full_files"])
        self.assertIn("scripts/install.sh", full["full_files"])
        self.assertEqual(manifest["installer_sha256"], full["full_files"]["scripts/install.ps1"])
        self.assertEqual((self.output / "scripts/install.ps1").read_bytes(),
                         (self.source / "scripts/install.ps1").read_bytes())
        self.assertEqual(set(p.relative_to(self.output).as_posix() for p in self.output.rglob("*")
                             if p.is_file()), {
            "registry.json", "manifest.json", "complete-source-fingerprint.json",
            "scripts/install.ps1",
            "windows/tadx.exe", "linux/tadx",
            "windows/build-metadata.txt", "linux/build-metadata.txt",
            "help/project.list.txt", "internal/agent/skills/tadx/SKILL.md",
            "internal/agent/skills/tadx/references/guide.md",
            "internal/agent/skills/tadx-pulse/SKILL.md",
            "internal/agent/skills/tadx-pulse/references/guide.md",
        })

    def test_changed_binary_help_or_metadata_blocks_before_output(self):
        self.builds["linux/amd64"].write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "Binary differs"):
            self.run_capture()
        self.assertFalse(self.output.exists())

        self.builds["linux/amd64"].write_bytes(b"linux accepted binary")
        (self.help / "project.list.txt").unlink()
        with self.assertRaisesRegex(ValueError, "Help set"):
            self.run_capture()
        self.assertFalse(self.output.exists())
        self.put(self.help / "project.list.txt", b"synthetic help")
        with self.assertRaisesRegex(ValueError, "build metadata"):
            cap.capture(self.source, self.output, self.candidate, self.catalog,
                        self.builds, self.guidance, self.help, inspect=lambda path: b"wrong")
        self.assertFalse(self.output.exists())

    def test_clean_non_go_change_changes_full_ordinal_not_narrow_digest(self):
        previous = self.identity
        self.put(self.source / "scripts/install.sh", b"echo changed accepted installer\n")
        self.git("add", "scripts/install.sh")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
                 "commit", "-qm", "non-Go change")
        current = cap.fingerprint(self.source)
        self.assertEqual(current["digest"], previous["digest"])
        self.assertNotEqual(current["full_ordinal_sha256"], previous["full_ordinal_sha256"])

    def test_modified_untracked_and_ignored_installers_block(self):
        installer = self.source / "scripts/install.ps1"
        original = installer.read_bytes()
        installer.write_bytes(b"Write-Output 'changed'\n")
        with self.assertRaisesRegex(ValueError, "dirty"):
            self.run_capture()
        installer.write_bytes(original)
        new_installer = self.source / "scripts/extra.ps1"
        new_installer.write_bytes(b"untracked")
        with self.assertRaisesRegex(ValueError, "dirty"):
            self.run_capture()
        new_installer.unlink()
        ignored = self.source / "scripts/ignored.bin"
        ignored.write_bytes(b"ignored but not accepted")
        with self.assertRaisesRegex(ValueError, "untracked files"):
            self.run_capture()
        self.assertFalse(self.output.exists())
        ignored.unlink()

    def test_dirty_build_metadata_blocks(self):
        with self.assertRaisesRegex(ValueError, "build metadata"):
            cap.capture(self.source, self.output, self.candidate, self.catalog,
                        self.builds, self.guidance, self.help,
                        inspect=lambda path: self.inspect(path).replace(
                            b"vcs.modified=false", b"vcs.modified=true"))
        self.assertFalse(self.output.exists())

    def test_existing_output_blocks(self):
        self.run_capture()
        with self.assertRaisesRegex(ValueError, "new directory"):
            self.run_capture()


class QualificationTests(unittest.TestCase):
    def setUp(self):
        self.fixture = test_prepare.PrepareTests(
            "test_snapshot_is_blocked_exact_and_excludes_unlisted_secrets_and_history")
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        auxiliary_patch = mock.patch.object(qualify, "AUXILIARY_SOURCE",
                                           test_prepare.prep.AUXILIARY_SOURCE)
        auxiliary_patch.start()
        self.addCleanup(auxiliary_patch.stop)
        self.fixture.files["integration/Dockerfile.local"] = (
            "FROM synthetic-worker:locked\n"
            "RUN npm install --global @openai/codex@0.154.0\n"
            "COPY tadx /opt/tadx\n"
            "COPY registry.json /opt/registry.json\nCOPY *_broker.cjs /opt/\n"
            "COPY credential_pty.py /opt/\n")
        blob = self.fixture.files["integration/Dockerfile.local"].encode()
        self.fixture.put(self.fixture.harness / "integration/Dockerfile.local", blob)
        self.fixture.lock["files"]["integration/Dockerfile.local"] = cap.sha(blob)
        manifest = cap.parse(self.fixture.capture.read_bytes())
        for system in ("linux", "windows"):
            path = self.fixture.capture_root / system / "build-metadata.txt"
            self.fixture.put(path, path.read_bytes() + b"build\tvcs.modified=false\n")
            manifest["build_metadata"][system]["sha256"] = cap.sha(path.read_bytes())
        self.fixture.put(self.fixture.capture, cap.encode(manifest))
        self.fixture.run_prepare()

    def test_default_qualifier_reads_current_124_source_lock(self):
        expected = qualify.HERE.parent / "harness/source-lock-124.json"
        original_read = qualify.read

        def read_with_marker(path):
            if path == expected:
                raise LookupError("current source lock reached")
            return original_read(path)

        with mock.patch.object(qualify, "read", side_effect=read_with_marker):
            with self.assertRaisesRegex(LookupError, "current source lock reached"):
                qualify.qualify(self.fixture.output, self.fixture.candidate)

    def test_qualifies_copied_inputs_but_keeps_live_blocked(self):
        result = qualify.qualify(self.fixture.output, self.fixture.candidate,
                                 lock=self.fixture.lock)
        self.assertEqual(result["status"], "copied_inputs_qualified_offline")
        self.assertFalse(result["live_execution_enabled"])
        self.assertIn("g9_project_read_broker.cjs", result["worker_input_files"])
        self.assertTrue(result["blockers"])

    def test_missing_or_changed_copied_installer_refuses_qualification(self):
        installer = self.fixture.output / "candidate/capture/scripts/install.ps1"
        original = installer.read_bytes()
        installer.unlink()
        with self.assertRaises((FileNotFoundError, ValueError)):
            qualify.qualify(self.fixture.output, self.fixture.candidate,
                            lock=self.fixture.lock)
        installer.write_bytes(b"changed installer")
        with self.assertRaisesRegex(ValueError, "file set or bytes|installer source"):
            qualify.qualify(self.fixture.output, self.fixture.candidate,
                            lock=self.fixture.lock)
        installer.write_bytes(original)

    def test_generated_windows_preflight_covers_seven_and_refuses_changed_installer(self):
        script = """import json
from pathlib import Path
import sys
import types
root = Path(sys.argv[1])
sys.path.insert(0, str(root / 'source'))
from integration import g9_windows_gate as gate
sys.modules['integration.scenario_common'] = types.ModuleType('integration.scenario_common')
sys.modules['integration.g9_windows_host_relay'] = types.ModuleType('integration.g9_windows_host_relay')
from integration import g9_windows_installer_profile as profile
frozen = json.loads((root / 'candidate/capture/manifest.json').read_text())
frozen['root'] = str(root / 'candidate/capture')
version = gate.expected_version(frozen['source_commit'])
for case in gate.CASES:
    request = {'exercise': {'id': 'V-installer-windows-' + case},
               'run_constraints': {'g9_accepted_gate_sha': frozen['source_commit'],
                                   'g9_windows_installer_version': version}}
    assert 'platform:windows' in profile.preflight(request, frozen)['qualified_requirements']
installer = root / 'candidate/capture/scripts/install.ps1'
installer.write_bytes(b'changed')
try:
    profile.preflight(request, frozen)
except gate.Refused as error:
    assert 'differs from accepted capture' in str(error)
else:
    raise AssertionError('changed installer passed preflight')
installer.unlink()
try:
    profile.preflight(request, frozen)
except gate.Refused as error:
    assert 'source is missing' in str(error)
else:
    raise AssertionError('missing installer passed preflight')
"""
        result = subprocess.run([sys.executable, "-B", "-c", script,
                                 str(self.fixture.output)], capture_output=True,
                                text=True, encoding="utf-8", check=False)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_generated_patch_cannot_claim_a_locked_original(self):
        record_path = self.fixture.output / "preparation.json"
        record = cap.parse(record_path.read_bytes())
        generated = "fixtures/profiles/P-job-cancel.json"
        self.assertNotIn(generated, self.fixture.lock["files"])
        row = next(row for row in record["patches"] if row["path"] == generated)
        self.assertIsNone(row["before_sha256"])
        row["before_sha256"] = "0" * 64
        record_path.write_bytes(cap.encode(record))
        with self.assertRaisesRegex(ValueError, "Patch provenance differs"):
            qualify.qualify(self.fixture.output, self.fixture.candidate,
                            lock=self.fixture.lock)

    def test_mutated_broker_or_missing_authorization_guard_fails(self):
        broker = self.fixture.output / "source/integration/g9_project_read_broker.cjs"
        original_broker = broker.read_bytes()
        broker.write_bytes(broker.read_bytes() + b"changed")
        with self.assertRaisesRegex(ValueError, "file set or bytes"):
            qualify.qualify(self.fixture.output, self.fixture.candidate,
                            lock=self.fixture.lock)
        broker.write_bytes(original_broker)
        launcher = self.fixture.output / "source/tools/run_spark_suite.py"
        launcher.write_bytes(launcher.read_bytes().replace(
            b"g9_record=verify(args.g9_authorization,args.g9_authorization_sha256,ROOT.parent,check_snapshot=True,check_launch_inputs=True)",
            b"g9_record={}"))
        record_path = self.fixture.output / "preparation.json"
        record = json.loads(record_path.read_text())
        name = "source/tools/run_spark_suite.py"
        record["snapshot_files"][name] = cap.sha(launcher.read_bytes())
        record["prepared_files"][name.removeprefix("source/")] = record["snapshot_files"][name]
        for row in record["patches"]:
            if row["path"] == name.removeprefix("source/"):
                row["after_sha256"] = record["snapshot_files"][name]
        record_path.write_bytes(cap.encode(record))
        # Complete byte accounting still cannot qualify a removed admission check.
        with self.assertRaisesRegex(ValueError, "Operator-bound G9 dispatch guard"):
            qualify.qualify(self.fixture.output, self.fixture.candidate,
                            lock=self.fixture.lock)


if __name__ == "__main__":
    unittest.main()
