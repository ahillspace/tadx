"""The actual patched 373-file entrypoints refuse unauthenticated dispatch."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


HERE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(HERE))
import prepare  # noqa: E402


class ActivationEntrypointTests(unittest.TestCase):
    def test_launcher_and_bridge_refuse_before_any_worker_call(self):
        supplied = os.environ.get("TADX_G9_CURRENT_SUITE")
        if not supplied:
            raise RuntimeError("Exact current source-locked suite is required")
        original = Path(supplied).resolve(strict=True)
        lock = json.loads((HERE / "source-lock-124.json").read_text())
        self.assertEqual(len(lock["files"]), 373)
        files = {}
        for name, expected in lock["files"].items():
            blob = (original / name).read_bytes()
            self.assertEqual(prepare.sha(blob), expected, name)
            files[name] = blob
        prepare.apply_mutation_runtime(files)
        prepare.patch_sources(files)
        files["integration/g9_activation.py"] = (HERE / "activation.py").read_bytes()
        files["integration/g9_runtime_effective.py"] = (HERE / "runtime_effective.py").read_bytes()
        with tempfile.TemporaryDirectory(prefix="g9-entrypoint-") as temporary:
            root = Path(temporary)
            for name, blob in files.items():
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(blob)
            for name in ("task-scope-policy.json", "operator-task-scopes.json"):
                (root / "bench" / name).write_bytes((original / "bench" / name).read_bytes())
            environment = {**os.environ, "PYTHONDONTWRITEBYTECODE": "1"}
            output = root / "model-run"
            launcher = subprocess.run(
                [sys.executable, "-B", str(root / "tools/run_spark_suite.py"),
                 "--local", "--all", "--output", str(output)],
                cwd=root, env=environment, capture_output=True, text=True, timeout=20)
            self.assertEqual(launcher.returncode, 2, launcher.stderr[-500:])
            self.assertIn("operator digest missing", launcher.stderr)
            self.assertFalse(output.exists())
            request = root / "request.json"
            response = root / "response.json"
            request.write_text(json.dumps({"protocol": "tadx-bench-bridge/1",
                                           "exercise": {"id": "P-version-get"}}))
            bridge = subprocess.run(
                [sys.executable, "-B", str(root / "integration/docker_local_bridge.py"),
                 "preflight", str(request), str(response)],
                cwd=root, env=environment, capture_output=True, text=True, timeout=20)
            self.assertEqual(bridge.returncode, 0, bridge.stderr[-500:])
            self.assertEqual(json.loads(response.read_text())["status"], "blocked")
            self.assertIn("authorization is absent", response.read_text())


if __name__ == "__main__":
    unittest.main()
