"""Validate assessment references and optionally emit a compact coverage ledger."""

import argparse
from collections import defaultdict
import json
from pathlib import Path

from collect_inventory import inventory


ROOT = Path(__file__).resolve().parent
PILOTS = ("project-pilot.json", "workbook-pilot.json", "auth-pilot.json")


def build_ledger():
    baseline = json.loads((ROOT / "inventory-dd22c33.json").read_text(encoding="utf-8"))
    if inventory(baseline["source_revision"]) != baseline:
        raise ValueError("Committed-tree inventory does not reproduce")
    files = {row["path"]: row for row in baseline["files"]}
    declarations = {
        (row["path"], item["name"])
        for row in files.values() for item in row.get("declarations", [])
    }
    samples = defaultdict(set)
    test_groups = defaultdict(set)
    identifiers = set()
    group_count = 0
    for filename in PILOTS:
        pilot = json.loads((ROOT / filename).read_text(encoding="utf-8"))
        if pilot["source_revision"] != baseline["source_revision"]:
            raise ValueError("Mixed source revisions: " + filename)
        if pilot["execution"] != "not-run":
            raise ValueError("Unexpected execution claim: " + filename)
        for row in pilot["reviewed_paths"]:
            if row["path"] not in files:
                raise ValueError("Unknown reviewed path: " + row["path"])
            samples[row["path"]].add(filename)
        references = list(pilot["workflow_trace"])
        for finding in pilot["findings"]:
            references.extend(finding["evidence"])
        for row in references:
            source = files.get(row["path"])
            if source is None or not 1 <= row["line"] <= (source["physical_lines"] or 0):
                raise ValueError("Invalid source reference: " + str(row))
        for row in pilot["findings"] + pilot["test_groups"]:
            if row["id"] in identifiers:
                raise ValueError("Duplicate assessment ID: " + row["id"])
            identifiers.add(row["id"])
        for group in pilot["test_groups"]:
            group_count += 1
            for field in ("contract", "failure_mechanism", "assertions", "limitations", "disposition"):
                if not group[field]:
                    raise ValueError("Missing test assessment: " + group["id"])
            for test in group["tests"]:
                key = (test["path"], test["symbol"])
                if key not in declarations:
                    raise ValueError("Unknown test declaration: " + str(key))
                test_groups[key].add(group["id"])
    areas = []
    for area, counts in baseline["areas"].items():
        paths = sorted(path for path in samples if files[path]["area"] == area)
        reviewed_tests = sum(files[path]["area"] == area for path, _ in test_groups)
        areas.append({
            "area": area,
            "status": "pilot-sampled-not-complete" if paths or reviewed_tests else "inventory-only",
            "inventory": counts,
            "sampled_paths": paths,
            "test_declarations_with_pilot_references": reviewed_tests,
        })
    return {
        "schema_version": 1,
        "generated_by": "validate_assessment.py; do not edit manually",
        "source_revision": baseline["source_revision"],
        "inventory": "inventory-dd22c33.json",
        "pilots": list(PILOTS),
        "default_file_status": "inventory-only; no semantic judgment",
        "default_test_status": "unassessed",
        "limits": [
            "Pilot test references can cover selected subcases, not the entire declaration.",
            "Sampled paths can be partially read; exact scope is in each pilot ledger.",
            "Neither source review nor lexical counts establish execution or behavioral coverage.",
            "No area is approved as fully assessed or ready for migration.",
        ],
        "summary": {
            "inventory_areas": len(areas),
            "fully_assessed_areas": 0,
            "sampled_paths": len(samples),
            "test_groups": group_count,
            "test_declarations_with_pilot_references": len(test_groups),
            "test_declarations_without_pilot_references": len(declarations - test_groups.keys()),
        },
        "areas": areas,
        "test_references": [
            {"path": path, "symbol": symbol, "groups": sorted(groups), "status": "pilot-source-sampled"}
            for (path, symbol), groups in sorted(test_groups.items())
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, help="Create a new ledger; never overwrite an existing file")
    args = parser.parse_args()
    report = build_ledger()
    if args.output:
        with args.output.open("x", encoding="utf-8", newline="\n") as handle:
            json.dump(report, handle, indent=2)
            handle.write("\n")
    else:
        saved = ROOT / "coverage-ledger.json"
        if saved.exists() and json.loads(saved.read_text(encoding="utf-8")) != report:
            raise ValueError("Saved coverage ledger does not reproduce")
    print(json.dumps(report["summary"], indent=2))


if __name__ == "__main__":
    main()
