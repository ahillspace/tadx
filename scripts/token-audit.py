# /// script
# requires-python = ">=3.11"
# dependencies = ["tiktoken==0.12.0"]
# ///
"""Measure local CLI help journeys with a pinned development-only tokenizer.

Run: uv run scripts/token-audit.py --before bin/tadx.exe \
    --after bin/tadx-readiness.exe --output bin/token-audit.json
No Tableau requests or remote mutations occur. Existing binaries remain unchanged.
The report measures o200k_base tokens, not a claim about every model's tokenizer.
"""

import argparse
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
from datetime import datetime, timezone

import tiktoken


SCENARIOS = {
    "workbook_inspection": [["content", "workbook", "--help"], ["content", "workbook", "inspect", "--help"]],
    "datasource_publication": [["content", "datasource", "--help"], ["content", "datasource", "publish", "--help"]],
    "group_membership": [["admin", "group-member", "--help"], ["admin", "group-member", "add", "--help"]],
    "pulse_definition": [["pulse", "definition", "--help"], ["pulse", "definition", "create", "--help"]],
    "workspace_inventory": [["workspace", "--help"], ["workspace", "list", "--help"]],
    "capability_to_publication": [["capability", "get", "workbook.publish"], ["content", "workbook", "publish", "--help"]],
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def invoke(binary, args, cwd, env):
    result = subprocess.run([str(binary), *args], cwd=cwd, env=env, capture_output=True, timeout=30, check=False)
    if result.returncode:
        diagnostic = (result.stdout + result.stderr).decode("utf-8", errors="replace")
        raise RuntimeError(f"{binary.name} {' '.join(args)} exited {result.returncode}: {diagnostic}")
    return result


def binary_provenance(binary, cwd, env):
    version = invoke(binary, ["--version"], cwd, env).stdout.decode("utf-8").strip()
    build = subprocess.run(["go", "version", "-m", str(binary)], capture_output=True, text=True, timeout=30, check=True).stdout
    settings = {}
    for line in build.splitlines():
        fields = line.strip().split("\t")
        if len(fields) == 2 and fields[0] == "build" and "=" in fields[1]:
            name, value = fields[1].split("=", 1)
            settings[name] = value
    return {"file": binary.name, "sha256": digest(binary.read_bytes()), "version": version,
            "vcs_revision": settings.get("vcs.revision"), "vcs_modified": settings.get("vcs.modified"),
            "source_note": "A revision with vcs_modified=true identifies the base commit, not the complete dirty source. The binary hash identifies the measured artifact.",
            "go_build_settings": settings}


def measure(binary, fixture, env, encoding):
    scenarios = []
    for name, commands in SCENARIOS.items():
        rows = []
        for args in commands:
            result = invoke(binary, ["--config", str(fixture / "config.yaml"), *args], fixture, env)
            stdout, stderr = result.stdout.decode("utf-8"), result.stderr.decode("utf-8")
            # Both binaries share a fixture. Normalize its random path for rerun stability.
            rendered = stdout + stderr
            for prefix in (str(fixture).replace("\\", "\\\\"), str(fixture), fixture.as_posix()):
                rendered = rendered.replace(prefix, "<fixture>")
            rows.append({"command": "tadx " + " ".join(args), "tokens": len(encoding.encode(rendered, disallowed_special=())),
                         "bytes": len(rendered.encode("utf-8")), "stdout_sha256": digest(result.stdout),
                         "stderr_sha256": digest(result.stderr), "normalized_output": rendered})
        scenarios.append({"name": name, "commands": rows, "tokens": sum(row["tokens"] for row in rows)})
    return {"provenance": binary_provenance(binary, fixture, env), "scenarios": scenarios,
            "total_tokens": sum(row["tokens"] for row in scenarios)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--before", required=True, type=Path)
    parser.add_argument("--after", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    before, after = args.before.resolve(strict=True), args.after.resolve(strict=True)
    output = args.output.resolve()
    if output in (before, after):
        parser.error("Report path must not overwrite either binary")
    package_version = importlib.metadata.version("tiktoken")
    if package_version != "0.12.0":
        parser.error("Run through uv: tiktoken==0.12.0 is required")
    encoding = tiktoken.get_encoding("o200k_base")
    env = os.environ.copy()
    # Only isolated child processes change; no user or parent setting changes.
    env.pop("TADX_ENABLE_MUTATIONS", None)
    with tempfile.TemporaryDirectory(prefix="tadx-token-audit-") as directory:
        fixture = Path(directory)
        for key, name in {"HOME": "home", "USERPROFILE": "home", "APPDATA": "config", "LOCALAPPDATA": "cache",
                          "XDG_CONFIG_HOME": "config", "XDG_CACHE_HOME": "cache", "XDG_DATA_HOME": "data"}.items():
            isolated = fixture / name
            isolated.mkdir(exist_ok=True)
            env[key] = str(isolated)
        (fixture / "config.yaml").write_text("version: 1\n", encoding="utf-8")
        results = {"before": measure(before, fixture, env, encoding), "after": measure(after, fixture, env, encoding)}
    comparisons = []
    for old, new in zip(results["before"]["scenarios"], results["after"]["scenarios"], strict=True):
        comparisons.append({"scenario": old["name"], "before_tokens": old["tokens"], "after_tokens": new["tokens"],
                            "delta_tokens": new["tokens"] - old["tokens"],
                            "change_percent": round(100 * (new["tokens"] - old["tokens"]) / old["tokens"], 2)})
    report = {"created_at": datetime.now(timezone.utc).isoformat(), "tokenizer": {"package": "tiktoken", "version": package_version, "encoding": encoding.name},
              "platform": platform.platform(), "normalization": "Shared temporary fixture path replaced with <fixture>; stdout followed by stderr; no chat framing overhead.",
              "scope": "Six fixed local help and follow-up journeys. No remote data or model billing claim. Binary source revision is unknown when absent from Go build metadata.",
              "comparisons": comparisons, **results}
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"tokenizer": report["tokenizer"], "comparisons": comparisons,
                      "before_total": results["before"]["total_tokens"], "after_total": results["after"]["total_tokens"]}, indent=2))


if __name__ == "__main__":
    main()
