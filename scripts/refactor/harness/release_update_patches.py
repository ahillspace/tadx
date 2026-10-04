"""Exact prepared-copy changes for the pinned candidate update exercise."""

from __future__ import annotations

import json
import re


CASE = "P-current-update"


def replace_once(source: str, before: str, after: str) -> str:
    if source.count(before) != 1:
        raise ValueError("release update patch anchor differs")
    return source.replace(before, after, 1)


def definitions(exercise: dict, profile: dict) -> tuple[dict, dict]:
    """Keep existing assertions while correcting the source and native setup."""
    if exercise.get("id") != CASE or profile.get("id") != CASE:
        raise ValueError("release update definition identity differs")
    exercise = json.loads(json.dumps(exercise))
    profile = json.loads(json.dumps(profile))
    exercise["agent"]["prompt"] = replace_once(
        exercise["agent"]["prompt"],
        "Update the installed CLI and bundled Guidance to the latest published release.",
        "Update the disposable installed CLI and bundled Guidance from the declared starting version to the pinned candidate-built offline release in inputs.json. This is not a published GitHub release.")
    exercise["agent"]["prompt"] = replace_once(
        exercise["agent"]["prompt"],
        " Use the exact asset and requested settings in inputs.json.", "")
    exercise["evaluator"]["starting_state"] = replace_once(
        exercise["evaluator"]["starting_state"],
        "Provision the declared profile, record exact IDs and hashes, and pass its independent baseline assertions before launching the agent.",
        "Verify the source-locked candidate asset closure and distinct starting executable, then record native version, binary hash, and empty Guidance before launching the agent.")
    profile["binding_provenance"] = replace_once(
        profile["binding_provenance"],
        "Each expected binding must name a literal business setting, a role-resolved authoritative ID, a pinned asset digest/rowset, or an independent pre-task API/filesystem observation.",
        "Starting and target binary and Guidance digests derive from the exact pinned candidate source archive and build manifest; the starting native version and hash are observed before the task. No public release identity or prior benchmark run is borrowed.")
    changes = {
        "publish qualified native assets via independent provisioning route":
            "build and verify both disposable Linux executables and offline release assets from the exact pinned candidate source archive",
        "independently re-read baseline and verify preconditions":
            "independently run the starting executable's native version command and verify its hash and empty Guidance",
    }
    order = profile["provisioning_order"]
    if not isinstance(order, list):
        raise ValueError("release update provisioning order differs")
    for before, after in changes.items():
        if order.count(before) != 1:
            raise ValueError("release update provisioning step differs")
        order[order.index(before)] = after
    return exercise, profile


def patch_broker(source: str) -> str:
    """Add the Codex-only release guard before disposable-native admission."""
    source = replace_once(source, "function allowed(args,state=diskState()) {\n  const parsed=parseArgs(args),flags=parsed.flags;\n",
        "function allowed(args,state=diskState()) {\n  const parsed=parseArgs(args),flags=parsed.flags;\n"
        "  // This update fixture stays narrow even when other disposable tasks enable\n"
        "  // native execution without a command allowlist.\n"
        "  if(state.guard?.mode==='shared-release'){\n"
        "    if(!state.guardPresent||!state.baselinePresent||!state.networkNone||parsed.errors.length||\n"
        "       Object.hasOwn(flags,'config')||\n"
        "       args.some(x=>x.split(/[\\\\/]/).includes('..')||(x.startsWith('/')&&!x.startsWith('/work/'))))return false;\n"
        "    const capability=classify(args,state.registry||registry);\n"
        "    if(capability==='update'){\n"
        "      if(parsed.words.join(' ')!=='update'||Object.keys(flags).some(key=>!['target','json','full','check'].includes(key))||\n"
        "         (flags.check===true?flags.target!==undefined&&flags.target!=='codex':flags.target!=='codex'))return false;\n"
        "    }\n"
        "    return sharedBroker.allowed(parsed,capability,state);\n"
        "  }\n")
    source = replace_once(source,
        "    if(executed&&policyState.guard?.mode==='shared-local'&&policyState.guard?.home){env.HOME=policyState.guard.home;env.TADX_GUIDANCE_NOTICE='0';}\n",
        "    if(executed&&policyState.guard?.mode==='shared-local'&&policyState.guard?.home){env.HOME=policyState.guard.home;env.TADX_GUIDANCE_NOTICE='0';}\n"
        "    if(executed&&policyState.guard?.mode==='shared-release'&&policyState.networkNone){\n"
        "      env.PATH='/cli-state/release-transport:'+env.PATH;\n"
        "      env.BENCH_NETWORK_MODE=process.env.BENCH_NETWORK_MODE;\n"
        "      env.BENCH_RELEASE_ASSETS_DIR='/cli-state/release-assets';\n"
        "      env.BENCH_RELEASE_TRANSPORT_LOG='/cli-state/release-transport.jsonl';\n"
        "      env.BENCH_RELEASE_MANIFEST_SHA=policyState.guard.manifest_sha256;\n"
        "      env.BENCH_RELEASE_SOURCE_COMMIT=policyState.guard.source_commit;\n"
        "      env.BENCH_RELEASE_TARGET_VERSION=policyState.guard.target_version;\n"
        "      const operation=classify(args,policyState.registry||registry);\n"
        "      const installing=operation==='update'&&flagsOf(args).check!==true;\n"
        "      if(!/^[0-9a-f]{64}$/.test(env.BENCH_RELEASE_MANIFEST_SHA)||\n"
        "         !/^[0-9a-f]{40}$/.test(env.BENCH_RELEASE_SOURCE_COMMIT)||\n"
        "         env.BENCH_RELEASE_TARGET_VERSION!==`0.1.4-fixture.${env.BENCH_RELEASE_SOURCE_COMMIT.slice(0,7)}`||\n"
        "         (installing?executedBinarySHA!==policyState.guard.baseline_binary_sha256:\n"
        "           ![policyState.guard.baseline_binary_sha256,policyState.guard.target_binary_sha256].includes(executedBinarySHA)))executed=false;\n"
        "    }\n")
    return source


def go_toolchain(metadata: dict[str, bytes]) -> str:
    """Require the same exact Go toolchain in both accepted native records."""
    versions = []
    for platform in ("windows", "linux"):
        try:
            first = metadata[platform].decode("utf-8").splitlines()[0]
        except (KeyError, UnicodeError, IndexError) as error:
            raise ValueError("native build metadata lacks Go toolchain") from error
        version = first.rsplit(":", 1)[-1].strip()
        if not re.fullmatch(r"go[0-9]+\.[0-9]+\.[0-9]+", version):
            raise ValueError("native build metadata Go toolchain differs")
        versions.append(version)
    if versions[0] != versions[1]:
        raise ValueError("native builds use different Go toolchains")
    return versions[0]
