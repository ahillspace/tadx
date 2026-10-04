"""Extend a verified six-case lock to the explicit current suite's 116 base cases."""

import argparse
import hashlib
import json
from pathlib import Path


def sha(data):
    return hashlib.sha256(data).hexdigest()


def extend(suite, original):
    suite = Path(suite)
    lock = json.loads(Path(original).read_text(encoding="utf-8"))
    files = dict(lock["files"])
    for name, expected in files.items():
        if sha((suite / name).read_bytes()) != expected:
            raise ValueError("Original source lock differs from current suite")
    index = json.loads((suite / "suite/index.json").read_bytes())
    cases = sorted(row["id"] for row in index["exercises"] if row["id"].startswith("P-"))
    if len(cases) != 116 or len(set(cases)) != 116:
        raise ValueError("Expected 116 unique current-suite base actions")
    additions = []
    for case in cases:
        for folder in ("suite/exercises", "fixtures/profiles"):
            name = folder + "/" + case + ".json"
            if not (suite / name).is_file():
                raise ValueError("Current-suite action pair is incomplete")
            additions.append(name)
    for source in (suite / "integration").iterdir():
        if source.is_file() and source.suffix in {".py", ".cjs"}:
            additions.append("integration/" + source.name)
    for name in additions:
        actual = sha((suite / name).read_bytes())
        if name in files and files[name] != actual:
            raise ValueError("Existing locked source changed")
        files[name] = actual
    return {"schema_version": 1,
            "description": "Exact current-suite source closure for 116 base actions; live dispatch remains blocked.",
            "files": dict(sorted(files.items()))}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", required=True)
    parser.add_argument("--original", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    output = Path(args.output)
    if output.exists():
        raise ValueError("Extended source lock already exists")
    value = extend(args.suite, args.original)
    output.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": "locked_blocked", "files": len(value["files"])}))


if __name__ == "__main__":
    main()
