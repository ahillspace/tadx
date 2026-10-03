"""Inventory a committed tree without checking it out or executing project code."""

import argparse
from collections import Counter, defaultdict
import io
import json
from pathlib import Path
import re
import subprocess


DECLARATION = re.compile(r"(?m)^func\s+((?:Test|Benchmark|Fuzz|Example)\w*)\s*\(")
GENERATED = re.compile(r"(?m)^// Code generated .* DO NOT EDIT\.$")


def git(*args):
    return subprocess.check_output(["git", *args])


def kind(path):
    parts = Path(path).parts
    if "testdata" in parts or "fixtures" in parts:
        return "fixture"
    if path.endswith("_test.go") or "/tests/" in path or path.endswith(".test.mjs"):
        return "test"
    if path.startswith(("scripts/", "cmd/gencapdocs/", ".github/", ".agents/")):
        return "tooling_or_automation"
    if path.endswith(".go"):
        return "go_non_test"
    if path.endswith((".md", ".html", ".svg", ".excalidraw")):
        return "documentation_or_site"
    return "other"


def area(path):
    parts = path.split("/")
    if parts[0] == "actions" and len(parts) > 2:
        return "actions." + parts[1]
    if parts[0] == "internal" and len(parts) > 2:
        return "internal." + parts[1]
    if parts[0] == "cmd" and len(parts) > 2:
        return "cmd." + parts[1]
    return parts[0] if len(parts) > 1 else "repository-root"


def inventory(revision):
    commit = git("rev-parse", "--verify", revision + "^{commit}").decode().strip()
    entries = []
    archived = 0
    for item in git("ls-tree", "-rz", "--full-tree", commit).split(b"\0"):
        if not item:
            continue
        metadata, name = item.split(b"\t", 1)
        mode, object_type, oid = metadata.decode().split()
        path = name.decode("utf-8")
        if path.startswith("archived/"):
            archived += 1
            continue
        if object_type != "blob":
            raise ValueError("Unsupported non-blob entry: " + path)
        entries.append((path, oid, mode))
    response = subprocess.run(
        ["git", "cat-file", "--batch"],
        input="".join(oid + "\n" for _, oid, _ in entries).encode(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=True,
    )
    stream = io.BytesIO(response.stdout)
    files = []
    for path, oid, mode in entries:
        header = stream.readline().decode().split()
        if len(header) != 3 or header[:2] != [oid, "blob"]:
            raise ValueError("Unexpected cat-file response for " + path)
        data = stream.read(int(header[2]))
        if stream.read(1) != b"\n":
            raise ValueError("Invalid blob terminator")
        try:
            text = None if b"\0" in data else data.decode("utf-8")
        except UnicodeDecodeError:
            text = None
        row = {
            "path": path,
            "blob": oid,
            "area": area(path),
            "kind_hint": kind(path),
            "generated_header": bool(text and GENERATED.search(text)),
            "physical_lines": None if text is None else len(text.splitlines()),
        }
        if path.endswith("_test.go") and text is not None:
            row["declarations"] = [
                {"name": match[1], "line": text.count("\n", 0, match.start()) + 1}
                for match in DECLARATION.finditer(text)
            ]
        files.append(row)
    if stream.read():
        raise ValueError("Unexpected trailing batch data")
    areas = defaultdict(lambda: {"files": 0, "go_non_test": 0, "go_test": 0,
                                 "go_lines": 0, "test_declarations": 0})
    go_packages = set()
    for row in files:
        stats = areas[row["area"]]
        stats["files"] += 1
        if row["path"].endswith(".go"):
            is_test = row["path"].endswith("_test.go")
            stats["go_test" if is_test else "go_non_test"] += 1
            stats["go_lines"] += row["physical_lines"] or 0
            if not is_test:
                go_packages.add(str(Path(row["path"]).parent).replace("\\", "/"))
        stats["test_declarations"] += len(row.get("declarations", []))
    go_files = [row for row in files if row["path"].endswith(".go")]
    return {
        "schema_version": 1,
        "source_revision": commit,
        "method": {
            "source": "Committed Git blobs only; excludes all uncommitted files.",
            "scope": "All tracked files except archived/; archived contents not read.",
            "kind_hint": "Filename-based triage only, not semantic file classification.",
            "go_non_test": "Includes tools and support code; does not mean shipped production code.",
            "generated_header": "Standard Go header detection only, not all generated artifacts.",
            "declarations": "Lexical top-level Go test/example/fuzz/benchmark declarations, not executed cases or subtests.",
            "package_count": "Directories with non-test Go files; no build-tag or dependency resolution.",
            "review_status": "Inventory alone is not a semantic review; pilot coverage is separate.",
        },
        "summary": {
            "tracked_files_in_scope": len(files),
            "archived_paths_excluded": archived,
            "kind_hints": dict(sorted(Counter(row["kind_hint"] for row in files).items())),
            "go_files": len(go_files),
            "go_non_test_files": sum(not row["path"].endswith("_test.go") for row in go_files),
            "go_test_files": sum(row["path"].endswith("_test.go") for row in go_files),
            "go_non_test_lines": sum(row["physical_lines"] or 0 for row in go_files if not row["path"].endswith("_test.go")),
            "go_test_lines": sum(row["physical_lines"] or 0 for row in go_files if row["path"].endswith("_test.go")),
            "go_non_test_directories": len(go_packages),
            "go_test_declarations": sum(len(row.get("declarations", [])) for row in go_files),
        },
        "areas": dict(sorted(areas.items())),
        "files": files,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    report = inventory(args.revision)
    # Exclusive creation keeps prior assessment evidence recoverable.
    with args.output.open("x", encoding="utf-8", newline="\n") as handle:
        json.dump(report, handle, indent=2, ensure_ascii=False)
        handle.write("\n")
    print(json.dumps(report["summary"], indent=2))


if __name__ == "__main__":
    main()
