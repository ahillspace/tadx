"""Run bounded, offline refactor contract checks against an isolated source snapshot."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import signal
import stat
import subprocess
import tempfile
import time


class GateError(Exception):
    """A required check lacks acceptable evidence."""


def digest(data):
    return hashlib.sha256(data).hexdigest()


def safe_relative(value):
    path = PurePosixPath(value)
    if not value or "\\" in value or ":" in value or path.is_absolute() or ".." in path.parts:
        raise GateError("Paths must be repository-relative and cannot traverse parents.")
    if str(path) == ".":
        raise GateError("The repository root is not an allowed change scope.")
    return str(path)


def git(repo, *args):
    result = subprocess.run(["git", "-C", str(repo), *args], capture_output=True, check=False)
    if result.returncode:
        raise GateError("Git provenance lookup failed.")
    return result.stdout


def reject_link(path):
    metadata = path.lstat()
    if stat.S_ISLNK(metadata.st_mode) or getattr(metadata, "st_file_attributes", 0) & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400):
        raise GateError("Source snapshots cannot contain symlinks, junctions, or reparse points.")


def source_manifest(source):
    reject_link(source)
    if (source / ".git").exists():
        raise GateError("Use an isolated source snapshot, not a Git working directory.")
    result = {}
    for directory, directories, files in os.walk(source, followlinks=False):
        for name in directories + files:
            reject_link(Path(directory) / name)
        for name in files:
            path = Path(directory) / name
            relative = safe_relative(path.relative_to(source).as_posix())
            result[relative] = digest(path.read_bytes())
    if "go.mod" not in result:
        raise GateError("The snapshot has no go.mod.")
    return result


def manifest_digest(manifest):
    return digest(json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode())


def capture_provenance(repo, source, revision, allowed):
    revision = git(repo, "rev-parse", "--verify", revision + "^{commit}").decode().strip()
    object_format = git(repo, "rev-parse", "--show-object-format").decode().strip()
    tree = git(repo, "ls-tree", "-rz", "--full-tree", revision)
    expected = {}
    materialized_links = []
    for entry in tree.split(b"\0"):
        if not entry:
            continue
        metadata, name = entry.split(b"\t", 1)
        mode, kind, oid = metadata.decode().split()
        if kind != "blob" or mode not in {"100644", "100755", "120000"}:
            raise GateError("Unsupported tracked source entry.")
        relative = safe_relative(name.decode())
        expected[relative] = oid
        if mode == "120000":
            materialized_links.append(relative)
    actual = source_manifest(source)
    changed = []
    for name in sorted(expected.keys() | actual.keys()):
        if name not in expected or name not in actual:
            changed.append(name)
            continue
        data = (source / name).read_bytes()
        oid = hashlib.new(object_format, b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
        if oid != expected[name]:
            changed.append(name)
    allowed = [safe_relative(path).rstrip("/") for path in allowed]
    unexpected = [name for name in changed if not any(name == p or name.startswith(p + "/") for p in allowed)]
    if unexpected:
        raise GateError("Undeclared source changes: " + ", ".join(unexpected[:10]))
    fixtures = {p: h for p, h in actual.items() if "/testdata/" in p or p.endswith("_test.go")}
    return {
        "baseline_revision": revision,
        "candidate_revision": revision if not changed else None,
        "candidate_source_sha256": manifest_digest(actual),
        "fixture_and_test_sha256": manifest_digest(fixtures),
        "changed_paths": changed,
        "allowed_change_scopes": allowed,
        "materialized_link_paths": materialized_links,
        "files": actual,
    }


def validate_manifest(manifest):
    if manifest.get("schema_version") != 1:
        raise GateError("Unsupported gate manifest version.")
    identifiers = set()
    groups = {}
    for group in manifest.get("checks", []):
        identifier = group.get("id", "")
        if not re.fullmatch(r"[a-z0-9-]+", identifier) or identifier in identifiers:
            raise GateError("Invalid or duplicate check identifier.")
        identifiers.add(identifier)
        package = group.get("package", "")
        if not package.startswith("./"):
            raise GateError("Checks must select one local package.")
        safe_relative(package[2:])
        tests = group.get("tests", [])
        if not tests or len(set(tests)) != len(tests) or any(not re.fullmatch(r"Test\w+", t) for t in tests):
            raise GateError("Checks require unique exact Go test names.")
        groups[identifier] = group
    if not groups:
        raise GateError("No contract checks configured.")
    for seed in manifest.get("seeds", []):
        if not re.fullmatch(r"[a-z0-9-]+", seed.get("id", "")) or seed["id"] in identifiers:
            raise GateError("Invalid or duplicate seed identifier.")
        identifiers.add(seed["id"])
        safe_relative(seed["path"])
        group = groups.get(seed.get("control"))
        if group is None or seed.get("test") not in group["tests"]:
            raise GateError("Every seed requires a configured control test.")
        if not seed.get("before") or seed.get("before") == seed.get("after") or not seed.get("failure_contains"):
            raise GateError("A seed needs an exact edit and expected assertion message.")
    return manifest


def evaluate_go_result(returncode, stdout, tests, expected_failure=None, failure_contains=None):
    events = []
    try:
        for line in stdout.splitlines():
            if line.strip():
                events.append(json.loads(line))
    except (ValueError, TypeError):
        raise GateError("Go test output is not a complete JSON event stream.") from None
    if any(not isinstance(event, dict) for event in events):
        raise GateError("Invalid Go test event.")
    skips = sorted({e["Test"] for e in events if e.get("Action") == "skip" and e.get("Test")})
    if skips or any(e.get("Action") == "skip" and not e.get("Test") for e in events):
        raise GateError("A required test or subtest was skipped.")
    ran = {e.get("Test") for e in events if e.get("Action") == "run"}
    terminals = {e.get("Test"): e.get("Action") for e in events if e.get("Test") and e.get("Action") in {"pass", "fail"}}
    missing = sorted(set(tests) - ran)
    if missing:
        raise GateError("Required tests did not run: " + ", ".join(missing))
    failed = {name for name, status in terminals.items() if status == "fail"}
    package_end = [e.get("Action") for e in events if not e.get("Test") and e.get("Action") in {"pass", "fail"}]
    if expected_failure:
        output = "".join(e.get("Output", "") for e in events if e.get("Test") == expected_failure or str(e.get("Test", "")).startswith(expected_failure + "/"))
        unexpected = {name for name in failed if name != expected_failure and not name.startswith(expected_failure + "/")}
        if returncode != 1 or terminals.get(expected_failure) != "fail" or package_end != ["fail"] or unexpected:
            raise GateError("Seed did not produce the designated test failure; build/setup failures are not demonstrations.")
        if failure_contains not in output or "panic:" in stdout or "[build failed]" in stdout:
            raise GateError("Seed failure did not match the expected assertion.")
        status = "detected"
    else:
        if returncode != 0 or failed or package_end != ["pass"] or any(terminals.get(t) != "pass" for t in tests):
            raise GateError("Required contract tests did not all pass.")
        status = "passed"
    return {"status": status, "required_tests": tests, "ran_count": len(ran), "failed_tests": sorted(failed), "skipped_tests": skips}


def seeded_source(source, seed):
    path = source / safe_relative(seed["path"])
    original = path.read_text(encoding="utf-8")
    if original.count(seed["before"]) != 1:
        raise GateError("Seed anchor is missing or ambiguous: " + seed["id"])
    return original.replace(seed["before"], seed["after"], 1)


def child_environment(home):
    # Copy only runtime/tool discovery variables, never inherited PATs or provider keys.
    allowed = {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "SYSTEMDRIVE", "PROGRAMFILES", "PROGRAMFILES(X86)", "COMMONPROGRAMFILES"}
    env = {k: v for k, v in os.environ.items() if k.upper() in allowed}
    offline = {"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off"}
    bootstrap = {**env, **offline}
    # These directory locations locate existing caches, not user Go configuration.
    for key in ("HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"):
        if key in os.environ:
            bootstrap[key] = os.environ[key]
    probe = subprocess.run(["go", "env", "GOCACHE", "GOMODCACHE"], env=bootstrap, cwd=tempfile.gettempdir(), capture_output=True, text=True, check=True, timeout=20)
    cache, modules = probe.stdout.strip().splitlines()
    home.mkdir()
    temporary = home / "tmp"
    temporary.mkdir()
    env.update({"HOME": str(home), "USERPROFILE": str(home), "APPDATA": str(home / "appdata"), "LOCALAPPDATA": str(home / "localappdata"), "TMP": str(temporary), "TEMP": str(temporary), "TMPDIR": str(temporary), "GOCACHE": cache, "GOMODCACHE": modules, **offline})
    return env


def stop_process_tree(process, environment):
    if os.name == "nt":
        result = subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"], env=environment, capture_output=True, timeout=15, check=False)
        if result.returncode:
            raise GateError("Timed-out process tree termination could not be confirmed.")
    else:
        os.killpg(process.pid, signal.SIGKILL)


def redact(text, roots):
    for root in sorted({str(p) for p in roots}, key=len, reverse=True):
        if root in {"/", "\\"}:
            continue
        parts = re.split(r"[\\/]+", root)
        spelling = r"[\\/]+".join(re.escape(part) for part in parts)
        text = re.sub(spelling, "<local-root>", text)
    text = re.sub(r"(?i)[a-z]:[\\/]+users[\\/]+[^\\/\r\n\"]+", "<user-root>", text)
    text = re.sub(r"(?i)[\\/]+(?:users|home)[\\/]+[^\\/\r\n\"]+", "<user-root>", text)
    return text


class Runner:
    def __init__(self, source, output, environment, timeout):
        self.source, self.output, self.environment, self.timeout = source, output, environment, timeout
        self.commands = []

    def command(self, identifier, argv):
        started = time.monotonic()
        timed_out = False
        containment_error = None
        options = {"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if os.name == "nt" else {"start_new_session": True}
        process = subprocess.Popen(argv, cwd=self.source, env=self.environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace", **options)
        try:
            stdout, stderr = process.communicate(timeout=self.timeout)
            code = process.returncode
        except subprocess.TimeoutExpired as error:
            timed_out = True
            code = None
            stdout = (error.stdout or b"").decode("utf-8", "replace") if isinstance(error.stdout, bytes) else error.stdout or ""
            stderr = (error.stderr or b"").decode("utf-8", "replace") if isinstance(error.stderr, bytes) else error.stderr or ""
            try:
                stop_process_tree(process, self.environment)
                stdout, stderr = process.communicate(timeout=15)
            except (GateError, OSError, subprocess.SubprocessError) as cleanup_error:
                containment_error = str(cleanup_error)
                process.kill()
        roots = [self.source, self.output, Path.home()]
        record = {"id": identifier, "argv": [redact(arg, roots) for arg in argv], "exit_code": code, "timed_out": timed_out, "containment_error": redact(containment_error, roots) if containment_error else None, "wall_seconds": round(time.monotonic() - started, 3), "stdout": identifier + ".stdout.txt", "stderr": identifier + ".stderr.txt"}
        (self.output / record["stdout"]).write_text(redact(stdout, roots), encoding="utf-8")
        (self.output / record["stderr"]).write_text(redact(stderr, roots), encoding="utf-8")
        self.commands.append(record)
        if containment_error:
            raise GateError("Timeout cleanup could not establish process containment: " + identifier)
        if timed_out:
            raise GateError("Command exceeded its bounded timeout: " + identifier)
        return code, stdout, stderr

    def check(self, identifier, group, overlay=None, seed=None):
        tests = [seed["test"]] if seed else group["tests"]
        argv = ["go", "test", "-json", "-count=1", "-timeout=" + str(max(1, self.timeout - 10)) + "s"]
        if overlay:
            argv.append("-overlay=" + str(overlay))
        argv += ["-run", "^(" + "|".join(re.escape(test) for test in tests) + ")$", group["package"]]
        code, stdout, _ = self.command(identifier, argv)
        result = evaluate_go_result(code, stdout, tests, seed["test"] if seed else None, seed["failure_contains"] if seed else None)
        return {"id": identifier, "gate": group["gate"], **result}


def execute(args):
    unresolved = Path(args.source).absolute()
    for path in [unresolved, *unresolved.parents]:
        reject_link(path)
    source, repo = Path(args.source).resolve(), Path(args.repo).resolve()
    output = Path(args.output).resolve() if args.output else Path(tempfile.mkdtemp(prefix="tadx-refactor-gates-"))
    if output == source or source in output.parents:
        raise GateError("Evidence output must be outside the source snapshot.")
    if args.output:
        output.mkdir(parents=True, exist_ok=False)
    manifest_path = Path(args.manifest).resolve()
    manifest = validate_manifest(json.loads(manifest_path.read_text(encoding="utf-8")))
    summary = {"schema_version": 1, "status": "failed", "scope": "initial bounded G0-G4 checks and seeded effectiveness demonstrations", "manifest_sha256": digest(manifest_path.read_bytes()), "runner_sha256": digest(Path(__file__).read_bytes()), "checks": [], "seeds": [], "not_established": ["Complete external contract coverage", "Complete persistence failure matrix", "G6 architecture conformance", "G7 full native platform/release matrix", "G8 acceptance", "G9 live verification"]}
    runner = None
    initial = None
    try:
        initial = capture_provenance(repo, source, args.revision, args.allow_change)
        (output / "source-manifest.json").write_text(json.dumps(initial, indent=2) + "\n", encoding="utf-8")
        summary["provenance"] = {k: v for k, v in initial.items() if k != "files"}
        runner = Runner(source, output, child_environment(output / "isolated-home"), args.timeout)
        code, toolchain, _ = runner.command("toolchain", ["go", "version"])
        if code:
            raise GateError("Go toolchain is unavailable.")
        summary["toolchain"] = toolchain.strip()
        binary = output / ("tadx.exe" if os.name == "nt" else "tadx")
        code, _, _ = runner.command("build", ["go", "build", "-trimpath", "-o", str(binary), "./cmd/tadx"])
        if code or not binary.is_file():
            raise GateError("Candidate build failed.")
        summary["build_sha256"] = digest(binary.read_bytes())
        groups = {group["id"]: group for group in manifest["checks"]}
        for group in groups.values():
            summary["checks"].append(runner.check(group["id"], group))
        for seed in manifest["seeds"]:
            with tempfile.TemporaryDirectory(prefix="tadx-gate-seed-") as directory:
                directory = Path(directory)
                replacement = directory / "seed.go"
                replacement.write_text(seeded_source(source, seed), encoding="utf-8", newline="\n")
                overlay = directory / "overlay.json"
                overlay.write_text(json.dumps({"Replace": {str(source / seed["path"]): str(replacement)}}), encoding="utf-8")
                record = runner.check(seed["id"], groups[seed["control"]], overlay=overlay, seed=seed)
                record.update({"source_path": seed["path"], "source_sha256": initial["files"][seed["path"]], "seed_sha256": digest(replacement.read_bytes()), "control": seed["control"]})
                summary["seeds"].append(record)
        summary["status"] = "initial_checks_passed"
    except (GateError, OSError, ValueError, subprocess.SubprocessError) as error:
        summary["error"] = redact(str(error), [source, output, repo, Path.home()])
    finally:
        if initial is not None:
            try:
                unchanged = source_manifest(source) == initial["files"]
            except (GateError, OSError):
                unchanged = False
            summary["source_unchanged"] = unchanged
            if not unchanged:
                summary["status"] = "failed"
                summary["error"] = "Source changed during verification; evidence cannot establish one candidate."
        summary["commands"] = runner.commands if runner else []
        (output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    return summary, output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, help="Isolated source snapshot without .git")
    parser.add_argument("--repo", default=".", help="Repository used for baseline object provenance")
    parser.add_argument("--revision", required=True, help="Approved baseline commit")
    parser.add_argument("--allow-change", action="append", default=[], help="Explicit permitted relative path or directory")
    parser.add_argument("--manifest", default=str(Path(__file__).with_name("gates.json")))
    parser.add_argument("--output", help="New evidence directory outside the source snapshot")
    parser.add_argument("--timeout", type=int, default=180, help="Per-command bound in seconds")
    args = parser.parse_args()
    if args.timeout < 20:
        parser.error("--timeout must be at least 20 seconds")
    try:
        summary, output = execute(args)
    except (GateError, OSError, ValueError) as error:
        print("Gate setup failed: " + redact(str(error), [Path.home()]))
        return 1
    print(json.dumps({"status": summary["status"], "checks": len(summary["checks"]), "seeds": len(summary["seeds"]), "evidence_directory_name": output.name, "error": summary.get("error")}, indent=2))
    return 0 if summary["status"] == "initial_checks_passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
