"""Offline identity checks for the fixed hosted Windows candidate build."""

from __future__ import annotations

import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


sys.path.insert(0, str(Path(__file__).resolve().parent))
from windows_build import BuildRefused, build_candidate
from windows_gate import expected_version
from windows_gate import setup_request
import windows_worker


def git(root: Path, *args: str) -> str:
    result = subprocess.run(["git", *args], cwd=root, check=True, capture_output=True,
                            text=True, encoding="utf-8")
    return result.stdout.strip()


class WindowsCandidateBuildTest(unittest.TestCase):
    def source(self, root: Path) -> tuple[Path, str]:
        source = root / "source"
        source.mkdir()
        git(source, "init", "-b", "main")
        (source / "go.mod").write_text("module github.com/ahillspace/tadx\n\ngo 1.26\n",
                                        encoding="utf-8")
        version = source / "internal" / "version"
        version.mkdir(parents=True)
        (version / "version.go").write_text(
            'package version\nvar BuildVersion = ""\n', encoding="utf-8")
        command = source / "cmd" / "tadx"
        command.mkdir(parents=True)
        (command / "main.go").write_text(
            'package main\nimport ("fmt"; "strings"; '
            '"github.com/ahillspace/tadx/internal/version")\n'
            'func main() { fmt.Print("{\\"version\\":\\""+'
            'strings.TrimPrefix(version.BuildVersion,"v")+"\\"}") }\n', encoding="utf-8")
        scripts = source / "scripts"
        scripts.mkdir()
        (scripts / "install.ps1").write_text("# synthetic installer\n", encoding="utf-8")
        (scripts / "install.sh").write_text("# synthetic installer\n", encoding="utf-8")
        (source / "LICENSE").write_text("synthetic license\n", encoding="utf-8")
        git(source, "add", ".")
        git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
            "commit", "-m", "fixture")
        return source, git(source, "rev-parse", "HEAD")

    def test_two_clean_builds_have_same_bytes_and_vcs_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, commit = self.source(root)
            version = expected_version(commit)
            outputs = [root / "first.exe", root / "second.exe"]
            hashes = [build_candidate(source, output, commit, version,
                                      environment={**os.environ, "GOPROXY": "off"})
                      for output in outputs]
            self.assertEqual(hashes[0], hashes[1])
            self.assertEqual(hashes[0], hashlib.sha256(outputs[0].read_bytes()).hexdigest())
            metadata = subprocess.run(["go", "version", "-m", str(outputs[0])], check=True,
                                      capture_output=True, text=True, encoding="utf-8").stdout
            self.assertIn("vcs.revision=" + commit, metadata)
            self.assertIn("vcs.modified=false", metadata)
            self.assertIn("GOOS=windows", metadata)
            self.assertIn("CGO_ENABLED=0", metadata)
            self.assertIn("vcs.modified=false", metadata)

    def test_wrong_revision_dirty_checkout_or_toolchain_refuses_before_build(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, commit = self.source(root)
            version = expected_version(commit)
            output = root / "candidate.exe"
            with self.assertRaises(BuildRefused):
                build_candidate(source, output, "f" * 40, version)
            with self.assertRaises(BuildRefused):
                build_candidate(source, output, commit, version,
                                expected_toolchain="go0.0.0")
            (source / "untracked.go").write_text("package main\n", encoding="utf-8")
            with self.assertRaises(BuildRefused):
                build_candidate(source, output, commit, version)
            self.assertFalse(output.exists())

    def test_invalid_version_and_output_inside_source_refuse(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, commit = self.source(root)
            for version in ("v0.1.3", "0.1.3;echo", "0.1.3\nother"):
                with self.subTest(version=version), self.assertRaises(BuildRefused):
                    build_candidate(source, root / "candidate.exe", commit, version)
            with self.assertRaises(BuildRefused):
                build_candidate(source, source / "candidate.exe", commit,
                                expected_version(commit))

    def test_worker_builds_from_separate_exact_commit_clone(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, commit = self.source(root)
            version = expected_version(commit)
            pinned = build_candidate(source, root / "pinned.exe", commit, version,
                                     environment={**os.environ, "GOPROXY": "off"})
            (source / ".g9-windows-installer-setup.json").write_text("{}\n", encoding="utf-8")
            git(source, "add", ".g9-windows-installer-setup.json")
            git(source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
                "commit", "-m", "one-file setup")
            self.assertNotEqual(git(source, "rev-parse", "HEAD"), commit)
            request = setup_request("fresh", commit, "nonce_123456789",
                                    version=version, binary_sha256=pinned,
                                    installer_sha256="c" * 64)

            def source_git(*args, **kwargs):
                return subprocess.run(args, cwd=source, capture_output=True,
                                      text=True, encoding="utf-8", check=False)

            try:
                with patch.object(windows_worker, "command", side_effect=source_git), \
                     patch.object(windows_worker, "git_value", return_value=commit):
                    binary, assets = windows_worker.build_assets(request, root)
                self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(), pinned)
                self.assertTrue((assets / f"tadx_{version}_windows_amd64.zip").is_file())
                self.assertEqual(git(root / "candidate-source", "rev-parse", "HEAD"), commit)
                self.assertTrue((root / "candidate-source" / ".git").is_dir())
            finally:
                if (root / "candidate-source").exists():
                    self.assertTrue(windows_worker.remove_candidate_checkout(root))


if __name__ == "__main__":
    unittest.main()
