"""Exact release-asset transport inside a disposable hosted Windows job only."""

import os
import json
from pathlib import Path
import re
import shutil
import sys
from urllib.parse import urlsplit


VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.]+)?\Z")


def request(args):
    if len(args) == 3 and args[0] == "https":
        uri = urlsplit(args[1])
        if (uri.scheme != "https" or uri.netloc != "github.com" or uri.query
                or uri.fragment or "%" in uri.path or "\\" in uri.path):
            return None
        parts = uri.path.split("/")
        if len(parts) != 7 or parts[:5] != ["", "ahillspace", "tadx", "releases", "download"]:
            return None
        return parts[5], parts[6], Path(args[2]), "https"
    if (len(args) == 9 and args[:2] == ["release", "download"]
            and args[3:5] == ["--repo", "ahillspace/tadx"]
            and args[5] == "--pattern" and args[7] == "--output"):
        return args[2], args[6], Path(args[8]), "gh"
    return None


def main(args):
    if args == ["auth", "status", "--hostname", "github.com"]:
        return 0
    parsed = request(args)
    if parsed is None:
        return 2
    tag, name, output, route = parsed
    version = os.environ.get("G9_FIXTURE_VERSION", "")
    if not VERSION.fullmatch(version):
        return 2
    asset = f"tadx_{version}_windows_amd64.zip"
    missing = version + "-missing"
    missing_asset = f"tadx_{missing}_windows_amd64.zip"
    if (tag not in (version, "v" + version, missing, "v" + missing)
            or name not in ("checksums.txt", asset, missing_asset)
            or name == asset and tag in (missing, "v" + missing)
            or name == missing_asset and tag in (version, "v" + version)):
        return 2
    fixture_raw = os.environ.get("G9_ASSET_DIR")
    temporary_raw = os.environ.get("G9_NATIVE_TEMP")
    log_raw = os.environ.get("G9_TRANSPORT_LOG")
    if not fixture_raw or not temporary_raw or not log_raw:
        return 2
    fixture, temporary, log = Path(fixture_raw), Path(temporary_raw), Path(log_raw)
    if (not fixture.is_absolute() or not temporary.is_absolute() or not log.is_absolute()
            or not fixture.is_dir() or not temporary.is_dir()
            or fixture.resolve().parent != temporary.resolve().parent
            or not output.resolve().is_relative_to(temporary.resolve())
            or output.exists() or output.is_symlink()):
        return 2
    if (not log.is_file() or log.is_symlink()
            or log.resolve() != temporary.resolve().parent / "transport.jsonl"):
        return 2
    failed = name == missing_asset and os.environ.get("G9_FAIL_ARCHIVE") == "1"
    source = fixture / name
    if not failed and (not source.is_file() or source.is_symlink()):
        return 2
    record = {"asset": name, "failed": failed}
    if route == "https":
        record["transport"] = "https"
    with log.open("a", encoding="ascii") as stream:
        stream.write(json.dumps(record, sort_keys=True) + "\n")
    if failed:
        return 1
    shutil.copyfile(source, output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
