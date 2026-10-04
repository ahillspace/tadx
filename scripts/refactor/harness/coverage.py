"""Build an exact, non-acceptance inventory of the 124 required base actions."""

import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path


NEW_CASES = {
    "P-job-cancel": ("jobs", "job.cancel"),
    "P-job-inspect": ("jobs", "job.inspect"),
    "P-job-wait": ("jobs", "job.wait"),
    "P-policy-install": ("policy", "policy.install"),
    "P-policy-samples": ("policy", "policy.samples"),
    "P-policy-status": ("policy", "policy.status"),
    "P-policy-validate": ("policy", "policy.validate"),
    "P-pulse-subscription-list": ("pulse", "pulse.subscription.list"),
}
OFFLINE_AUTHORED = {
    *("P-project-" + verb for verb in ("create", "delete", "inspect", "list", "move", "update")),
    "P-job-cancel", "P-job-inspect", "P-job-wait",
    "P-policy-install", "P-policy-samples", "P-policy-status", "P-policy-validate",
    "P-pulse-subscription-list",
}


def _hash(data):
    return hashlib.sha256(data).hexdigest()


def _read_json(path):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError("Duplicate JSON key in selected source")
            result[key] = value
        return result

    data = path.read_bytes()
    return json.loads(data, object_pairs_hook=unique), _hash(data)


def build(suite, source_lock, expected_existing=116):
    suite = Path(suite)
    index, index_hash = _read_json(suite / "suite/index.json")
    lock, lock_hash = _read_json(Path(source_lock))
    locked = lock["files"]
    selected = [row for row in index["exercises"] if row["id"].startswith("P-")]
    if len(selected) != expected_existing or len({row["id"] for row in selected}) != expected_existing:
        raise ValueError("Current suite base-action set changed")
    if set(NEW_CASES) & {row["id"] for row in selected}:
        raise ValueError("New required action already exists in current suite")
    rows = []
    for entry in selected:
        case = entry["id"]
        exercise_path = "suite/exercises/" + case + ".json"
        profile_path = "fixtures/profiles/" + case + ".json"
        if entry.get("path") != exercise_path:
            raise ValueError("Selected exercise path is not exact")
        exercise, exercise_hash = _read_json(suite / exercise_path)
        profile, profile_hash = _read_json(suite / profile_path)
        if exercise.get("id") != case or profile.get("id") != case:
            raise ValueError("Selected exercise/profile identity differs")
        if profile.get("base_profile") != case.removeprefix("P-").replace("-", "."):
            # Some action IDs contain dashed compound nouns; the profile is
            # authoritative, but its exact form must still be nonempty.
            if not isinstance(profile.get("base_profile"), str) or not profile["base_profile"]:
                raise ValueError("Selected profile lacks an action binding")
        source_locked = (locked.get(exercise_path) == exercise_hash and
                         locked.get(profile_path) == profile_hash)
        rows.append({"id": case, "category": entry["category"],
                     "action": profile["base_profile"],
                     "observation_contract": profile["independent_observation_contract"],
                     "exercise_sha256": exercise_hash, "profile_sha256": profile_hash,
                     "source_locked": source_locked,
                     "fixture_state": ("offline_authored_blocked" if case in OFFLINE_AUTHORED
                                       else "not_qualified")})
    for case, (category, action) in NEW_CASES.items():
        rows.append({"id": case, "category": category, "action": action,
                     "observation_contract": None, "exercise_sha256": None,
                     "profile_sha256": None, "source_locked": False,
                     "fixture_state": ("offline_authored_blocked" if case in OFFLINE_AUTHORED
                                       else "not_authored")})
    rows.sort(key=lambda row: row["id"])
    if len(rows) != expected_existing + len(NEW_CASES) or len({row["action"] for row in rows}) != len(rows):
        raise ValueError("Required action coverage is not one-to-one")
    return {"schema_version": 1, "status": "inventory_only_no_live_acceptance",
            "current_suite_index_sha256": index_hash, "source_lock_sha256": lock_hash,
            "existing_action_count": expected_existing, "required_new_action_count": len(NEW_CASES),
            "action_count": len(rows), "category_counts": dict(sorted(Counter(row["category"] for row in rows).items())),
            "actions": rows}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", required=True)
    parser.add_argument("--source-lock", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    output = Path(args.output)
    if output.exists():
        raise ValueError("Coverage inventory output already exists")
    result = build(args.suite, args.source_lock)
    output.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": result["status"], "actions": result["action_count"],
                      "unqualified": sum(row["fixture_state"] == "not_qualified" for row in result["actions"]),
                      "not_authored": sum(row["fixture_state"] == "not_authored" for row in result["actions"])}))


if __name__ == "__main__":
    main()
