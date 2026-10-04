"""Synthetic, credential-free qualification of same-thread Codex metadata."""

import json
import shutil
import subprocess
from pathlib import Path
import tempfile
import unittest

import runtime_effective as runtime


THREAD = "123e4567-e89b-12d3-a456-426614174000"


def task(thread=THREAD):
    return {"exit_status": 0, "timed_out": False, "events": [
        {"type": "thread.started", "thread_id": thread},
        {"type": "turn.started"}, {"type": "turn.completed"},
    ]}


def observation(**changes):
    item = {"thread_matched": True, "context_count": 1, "context_consistent": True,
            "provider": "openai", "model": "gpt-6-luna", "effort": "medium"}
    item.update(changes)
    return json.dumps(item)


class RuntimeQualificationTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which("node"), "Node is unavailable")
    def test_pinned_rollout_scanner_has_valid_javascript(self):
        result = subprocess.run(["node", "--check"], input=runtime.ROLLOUT,
                                text=True, capture_output=True, timeout=10, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)

    @unittest.skipUnless(shutil.which("node"), "Node is unavailable")
    def test_rollout_scanner_matches_only_thread_and_excludes_secret(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            path = directory / "rollout.jsonl"
            rows = [
                {"type": "session_meta", "payload": {"id": THREAD,
                    "model_provider": "openai", "secret": "PRIVATE_SENTINEL"}},
                {"type": "turn_context", "payload": {"model": "gpt-6-luna",
                    "effort": "medium", "secret": "PRIVATE_SENTINEL"}},
            ]
            path.write_text("\n".join(json.dumps(row) for row in rows) + "\n", encoding="utf-8")
            script = runtime.ROLLOUT.replace("/home/worker/.codex/sessions", directory.as_posix())
            result = subprocess.run(["node", "-e", script, THREAD], text=True,
                                    capture_output=True, timeout=10, check=False)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertNotIn("PRIVATE_SENTINEL", result.stdout)
            self.assertEqual(runtime.parse_observation(result.stdout)["context_count"], 1)
            wrong = subprocess.run(["node", "-e", script, "wrong-thread"], text=True,
                                   capture_output=True, timeout=10, check=False)
            with self.assertRaises(runtime.EffectiveRuntimeError):
                runtime.parse_observation(wrong.stdout)
            rows.append({"type": "turn_context", "payload": {"model": "other-model", "effort": "medium"}})
            path.write_text("\n".join(json.dumps(row) for row in rows) + "\n", encoding="utf-8")
            mixed = subprocess.run(["node", "-e", script, THREAD], text=True,
                                   capture_output=True, timeout=10, check=False)
            self.assertEqual(mixed.returncode, 0, mixed.stderr)
            with self.assertRaises(runtime.EffectiveRuntimeError):
                runtime.parse_observation(mixed.stdout)

    def test_completed_same_thread_returns_only_allowlisted_fields(self):
        result = runtime.qualify(task(), observation(), model="gpt-6-luna", effort="medium",
                                 expected_thread=THREAD)
        self.assertEqual(result, {"thread_matched": True, "turn_completed": True,
                                  "provider": "openai", "model": "gpt-6-luna", "effort": "medium",
                                  "context_count": 1})
        self.assertNotIn(THREAD, json.dumps(result))

    def test_missing_wrong_thread_and_stale_followup_fail(self):
        for item, raw, previous, expected in (
                (task(), observation(thread_matched=False), 0, THREAD),
                (task(), observation(context_consistent=False), 0, THREAD),
                (task(), observation(context_count=0), 0, THREAD),
                (task(), observation(context_count=1), 1, THREAD),
                (task(thread="another-thread"), observation(), 0, THREAD)):
            with self.subTest(item=item, raw=raw), self.assertRaises(runtime.EffectiveRuntimeError):
                runtime.qualify(item, raw, model="gpt-6-luna", effort="medium",
                                previous_contexts=previous, expected_thread=expected)

    def test_wrong_effective_provider_model_or_effort_fails(self):
        for change in ({"provider": "unverified"}, {"model": "gpt-6-sol"},
                       {"effort": "low"}, {"model": "secret-value with spaces"}):
            with self.subTest(change=change), self.assertRaises(runtime.EffectiveRuntimeError):
                runtime.qualify(task(), observation(**change), model="gpt-6-luna", effort="medium")

    def test_failed_or_ambiguous_turn_and_extra_fields_fail(self):
        variants = [task(), task(), task(), task()]
        variants[0]["exit_status"] = 1
        variants[1]["events"][-1] = {"type": "turn.failed"}
        variants[2]["events"].append({"type": "turn.completed"})
        variants[3]["events"].append({"type": "thread.started", "thread_id": THREAD})
        for item in variants:
            with self.subTest(item=item), self.assertRaises(runtime.EffectiveRuntimeError):
                runtime.qualify(item, observation(), model="gpt-6-luna", effort="medium")
        with self.assertRaises(runtime.EffectiveRuntimeError):
            runtime.qualify(task(), observation(auth_token="secret"),
                            model="gpt-6-luna", effort="medium")


if __name__ == "__main__":
    unittest.main()
