"""Credential patch must bind the exact 373-file current-suite source lock."""

import hashlib
import json
import os
from pathlib import Path
import sys
import unittest


HERE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(HERE))
import prepare  # noqa: E402
from credential_patch import patch_broker  # noqa: E402


class CredentialTransportIntegrationTests(unittest.TestCase):
    def test_actual_locked_bridge_and_broker_have_no_plaintext_volume_path(self):
        supplied = os.environ.get("TADX_G9_CURRENT_SUITE")
        if not supplied:
            raise RuntimeError("Explicit current source-locked suite is required")
        suite = Path(supplied).resolve(strict=True)
        lock = json.loads((HERE / "source-lock-124.json").read_text(encoding="utf-8"))
        self.assertEqual(len(lock["files"]), 373)
        files = {}
        for name, expected in lock["files"].items():
            content = (suite / name).read_bytes()
            self.assertEqual(hashlib.sha256(content).hexdigest(), expected, name)
            files[name] = content
        self.assertIn(b"fs.readFileSync('/cli-state/credentials.json'",
                      files["integration/local_broker.cjs"])
        prepare.apply_mutation_runtime(files)
        prepare.patch_sources(files)
        broker = files["integration/local_broker.cjs"].decode("utf-8")
        bridge = files["integration/docker_local_bridge.py"].decode("utf-8")
        self.assertIn("credentialBroker.environment()", broker)
        self.assertIn("credentialBroker.start()", broker)
        with self.assertRaises(ValueError):
            patch_broker(broker, prepare.replace_once)
        self.assertNotIn("fs.readFileSync('/cli-state/credentials.json'", broker)
        self.assertNotIn("fs.writeFileSync('/cli-state/credentials.json'", bridge)
        self.assertNotIn("fs.readFileSync('/cli-state/credentials.json'", bridge)
        image = bridge.split("def image_for(m):", 1)[1].split("def runtime_check(", 1)[0]
        self.assertIn("'credential_broker.cjs'", image)
        self.assertTrue((HERE / "source/integration/credential_broker.cjs").is_file())


if __name__ == "__main__":
    unittest.main()
