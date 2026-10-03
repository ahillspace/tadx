"""Check proposed ownership accounting and pinned evidence without running TADX."""

from collections import Counter
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent
ASSESSMENTS = (
    "content-domains-assessment.json", "remote-domains-assessment.json",
    "local-services-assessment.json", "shared-architecture-assessment.json",
)


def read(root, name):
    return json.loads((root / name).read_text(encoding="utf-8"))


def check(condition, message):
    if not condition:
        raise ValueError(message)


def objects(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from objects(child)


def validate(root=ROOT):
    baseline = read(root, "inventory-dd22c33.json")
    files = {row["path"]: row for row in baseline["files"]}
    packages = {
        path.rsplit("/", 1)[0] for path in files
        if path.endswith(".go") and not path.endswith("_test.go")
    }
    symbols = {
        path: {item["name"] for item in row.get("declarations", [])}
        for path, row in files.items()
    }
    plan = read(root, "architecture-decision-map.json")
    check(plan["source_revision"] == baseline["source_revision"], "Mixed plan revision")
    sources = Counter(source for rule in plan["rules"] for source in rule["sources"])
    check(set(sources) == packages, "Package map has missing or unknown sources")
    check(all(count == 1 for count in sources.values()), "Duplicate package assignment")
    mapped = {
        source: source if rule["target"] == "same" else rule["target"]
        for rule in plan["rules"] for source in rule["sources"]
    }
    new = [item["target"] for item in plan["new_packages"]]
    check(len(new) == len(set(new)), "Duplicate new package")
    check(not set(new) & set(mapped.values()), "New package already mapped from a source")
    targets = set(mapped.values()) | set(new)
    app_files = {
        path for path in files if path.startswith("internal/app/")
        and path.endswith(".go") and not path.endswith("_test.go")
    }
    moves = Counter(row["source"] for row in plan["app_responsibility_moves"])
    check(set(moves) == app_files, "App map has missing or unknown source files")
    check(all(count == 1 for count in moves.values()), "Duplicate app file assignment")
    for row in plan["app_responsibility_moves"]:
        check(bool(row["targets"]) and set(row["targets"]) <= targets,
              "Unknown app destination: " + row["source"])

    reviewed = set()
    evidence_count = 0
    named_tests = set()
    for name in ASSESSMENTS:
        report = read(root, name)
        check(report["source_revision"] == baseline["source_revision"], "Mixed revision: " + name)
        for row in report["reviewed_paths"]:
            check(row["path"] in files, "Unknown reviewed path: " + row["path"])
            reviewed.add(row["path"])
        for row in objects(report):
            if "path" in row and "line" in row:
                source = files.get(row["path"])
                check(source is not None and isinstance(row["line"], int)
                      and 1 <= row["line"] <= (source["physical_lines"] or 0),
                      "Invalid evidence reference: " + str(row))
                evidence_count += 1
            if "path" in row and "symbol" in row:
                check(row["symbol"] in symbols.get(row["path"], set()),
                      "Unknown test symbol: " + str(row))
                named_tests.add((row["path"], row["symbol"]))
            if "paths" in row and "tests" in row and all(isinstance(t, str) for t in row["tests"]):
                for path in row["paths"]:
                    check(path in files, "Unknown test path: " + path)
                for symbol in row["tests"]:
                    matches = [path for path in row["paths"] if symbol in symbols.get(path, set())]
                    check(bool(matches), "Unknown grouped test: " + symbol)
                    named_tests.update((path, symbol) for path in matches)
    return {
        "source_packages_accounted_for": len(packages),
        "app_files_accounted_for": len(app_files),
        "current_action_packages": sum(p.startswith("actions/") for p in packages),
        "consolidated_existing_action_owners": len({target for source, target in mapped.items() if source.startswith("actions/")}),
        "proposed_action_packages_including_new": sum(p.startswith("actions/") for p in targets),
        "proposed_package_directories": len(targets),
        "source_paths_with_scoped_review_entries": len(reviewed),
        "evidence_references_checked": evidence_count,
        "named_test_references_checked": len(named_tests),
        "runtime_verification": "not performed",
    }


if __name__ == "__main__":
    print(json.dumps(validate(), indent=2))
