"""Prepare a blocked, offline project-harness integration snapshot."""

import argparse
import ast
import hashlib
import importlib.util
import json
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import tempfile

from consent import validate_authority
from coverage import NEW_CASES
from credential_patch import patch_broker as patch_credential_broker
from credential_patch import patch_bridge as patch_credential_bridge
from job_patches import JOB_CASES, job_definitions, patch_job_broker, patch_job_image
from mutation_patches import definitions as mutation_definitions
from policy_patches import POLICY_CASES, policy_definitions
from policy_install_patches import CASE as POLICY_INSTALL_CASE, definitions as policy_install_definitions
from project_patches import patch_project
from release_update_patches import definitions as release_update_definitions
from release_update_patches import patch_broker as patch_release_broker
from release_update_patches import go_toolchain as release_go_toolchain
from reset_absence_patches import patch_content_profiles, patch_operator_reset, patch_reset_site
from release_update_build import build as build_release_assets
from release_update_build import digest as file_digest
from release_update_build import RECIPE as RELEASE_RECIPE
from subscription_patches import CASE as SUBSCRIPTION_CASE, definitions as subscription_definitions
from subscription_patches import patch_pulse_broker, patch_pulse_profile
from windows_broker_patches import broker as patch_windows_broker, client as patch_windows_client
from windows_preparer import IDS as WINDOWS_IDS, extend_budget


HERE = Path(__file__).resolve().parent
WORKER = HERE.parents[1] / "g9"
SOURCE_RUNTIME_VERSION = "0.154.0"
TARGET_RUNTIME_VERSION = "0.160.0"
PROJECT_CASES = ["P-project-" + action for action in ("create", "delete", "inspect", "list", "move", "update")]
CASES = PROJECT_CASES + JOB_CASES + POLICY_CASES + [POLICY_INSTALL_CASE, SUBSCRIPTION_CASE]
AUXILIARY_SOURCE = {
    "bench/task-scope-policy.json": "0ade697155cad5338cf0cbd641c22928974223ecb62fb2d55c738eaa0c76e9e2",
    "bench/operator-task-scopes.json": "5a0b90672086a55a21c52e930bdec1608963eeb672112809a354b5e127613f28",
}
BLOCKERS = [
    "Copied launcher source provenance must be independently qualified against the exact accepted candidate before its local configuration path can run.",
    "Base worker image ID, installed runtime version, and copied image inputs need independent qualification; no rebuild or binary substitution is permitted.",
    "All 124 base action definitions are copied or authored, but source presence is not fixture qualification or native coverage.",
    "The 110 unchanged existing cases still need exact fixture admission, independent observers, and scoped cleanup qualification.",
    "Fresh run-owned project, job, and Pulse fixtures need bounded live qualification without credential persistence or older-run state.",
    "Four local policy cases need independent worker-image, native audit, and filesystem observer qualification; protected install requires a root-only disposable Linux overlay.",
    "Seven Windows installer variants have a blocked two-phase hosted fixture, but no exact candidate build recipe or live native evidence has qualified it yet.",
    "Actual provider/model/reasoning metadata is unproven; requested gpt-6-luna/medium arguments are not proof.",
    "Exact candidate G0-G8 and independently reviewed checkpoint scope remain required before live execution.",
]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def encode(value):
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode()


def parse(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "Duplicate JSON key")
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=unique)


def plain_path(path, *, missing=False):
    path = Path(path).absolute()
    for part in [*reversed(path.parents), path]:
        try:
            info = part.lstat()
        except FileNotFoundError:
            require(missing, "Missing required input")
            continue
        require(not stat.S_ISLNK(info.st_mode)
                and not getattr(info, "st_file_attributes", 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT,
                "Symlinks and reparse points are not accepted")
    return path.resolve()


def read(path):
    path = plain_path(path)
    require(path.is_file(), "Input must be a regular file")
    return path.read_bytes()


def relative(name):
    require(isinstance(name, str) and "\\" not in name and ":" not in name
            and not PurePosixPath(name).is_absolute()
            and all(part not in {"", ".", ".."} for part in name.split("/")),
            "Invalid allowlisted relative path")
    return name


def replace_once(source, before, after):
    require(source.count(before) == 1, "Patch anchor differs from qualified source")
    return source.replace(before, after, 1)


def apply_mutation_runtime(files):
    """Replace the exact copied session owner before shared bridge patches run."""
    names = ("integration/docker_local_bridge.py", "integration/session_broker.cjs",
             "integration/session_profiles.py")
    selected = sum(name.startswith("suite/exercises/P-") and name.endswith(".json")
                   for name in files)
    require(selected in (len(PROJECT_CASES), 116), "Mutation source selection differs")
    if selected == len(PROJECT_CASES):
        return []
    present = [name for name in names if name in files]
    require(len(present) == len(names), "Selected mutation runtime source is incomplete")
    patches = []
    for name in present:
        original = files[name]
        adapted = read(HERE / "source" / name)
        require(original != adapted, "Mutation owner did not change copied source")
        files[name] = adapted
        patches.append({"path": name, "before_sha256": sha(original),
                        "after_sha256": sha(adapted)})
    return patches


def append_patch(patches, row):
    """Record one original-to-final hash across reviewed sequential transforms."""
    previous = next((item for item in patches if item["path"] == row["path"]), None)
    if previous is None:
        patches.append(row)
        return
    require(previous["after_sha256"] == row["before_sha256"],
            "Patch chain differs")
    previous["after_sha256"] = row["after_sha256"]


def apply_mutation_definitions(files):
    selected_mutation = ("P-mutation-set", "P-mutation-status")
    require(not any("suite/exercises/" + case + ".json" in files for case in
                    ("V-mutation-env-precedence", "V-mutation-no-setting-authorization")),
            "Unselected mutation variants are outside the 131-case source lock")
    present = [case for case in selected_mutation
               if "suite/exercises/" + case + ".json" in files]
    require(not present or len(present) == len(selected_mutation),
            "Selected mutation exercise pair is incomplete")
    if not present:
        return []
    exercises = {case: parse(files["suite/exercises/" + case + ".json"])
                 for case in selected_mutation}
    profiles = {case: parse(files["fixtures/profiles/" + case + ".json"])
                for case in selected_mutation}
    patches = []
    for case, (exercise, profile) in mutation_definitions(exercises, profiles).items():
        for path, value in (("suite/exercises/" + case + ".json", exercise),
                            ("fixtures/profiles/" + case + ".json", profile)):
            original = files[path]
            files[path] = encode(value)
            patches.append({"path": path, "before_sha256": sha(original),
                            "after_sha256": sha(files[path])})
    return patches


def windows_runtime_sources():
    """Derive prepared Windows modules from their one tracked worker owner."""
    gate = read(WORKER / "windows_gate.py")
    relay = read(WORKER / "host_relay.py").decode("utf-8")
    prepared_relay = replace_once(
        relay,
        "from windows_gate import (Refused, baseline_issues, command_request, result_issues,\n"
        "                          setup_request, stop_request)",
        "from integration.g9_windows_gate import (Refused, baseline_issues, command_request,\n"
        "                                         result_issues, setup_request, stop_request)",
    ).encode("utf-8")
    result = {"integration/g9_windows_gate.py": gate,
              "integration/g9_windows_host_relay.py": prepared_relay}
    for name in ("g9_windows_installer_profile.py", "g9_windows_installer_watch.py",
                 "g9_windows_installer_broker.cjs"):
        result["integration/" + name] = read(HERE / "source/integration" / name)
    return result


def patch_sources(files):
    patches = []
    release_enabled = "suite/exercises/P-current-update.json" in files

    def patch(path, apply):
        original = files[path]
        # Anchor matching normalizes newlines only after the raw source hash check.
        before = original.decode("utf-8").replace("\r\n", "\n")
        after = apply(before).encode()
        files[path] = after
        patches.append({"path": path, "before_sha256": sha(original), "after_sha256": sha(after)})

    patch("integration/Dockerfile.local", lambda s: replace_once(
          s, "@openai/codex@" + SOURCE_RUNTIME_VERSION,
          "@openai/codex@" + TARGET_RUNTIME_VERSION))
    def runtime(source):
        source = replace_once(source, "VERSION = '" + SOURCE_RUNTIME_VERSION + "'",
                              "VERSION = '" + TARGET_RUNTIME_VERSION + "'")
        source = replace_once(source, "from bench.telemetry import summarize_events\n",
                              "from bench.telemetry import summarize_events\n"
                              "from integration import g9_runtime_effective as effective\n")
        source = replace_once(source,
            "*([] if req.get('exercise',{}).get('agent',{}).get('followups') else ['--ephemeral']), '--skip-git-repo-check'",
            "'--skip-git-repo-check'")
        source = replace_once(source,
            "    return {'task': task}\n\n\ndef model_followup",
            "    try:\n"
            "        thread = effective.thread_from_events(task['events'])\n"
            "        observed = _docker(_worker_command(container, ['node', '-e', effective.ROLLOUT, thread])[1:])\n"
            "        task['effective_runtime'] = effective.qualify(task, observed.stdout,\n"
            "            model=model, effort=selected_reasoning_effort(req))\n"
            "    except effective.EffectiveRuntimeError:\n"
            "        raise CodexRuntimeError('Effective same-thread Codex runtime could not be qualified') from None\n"
            "    return {'task': task}\n\n\ndef model_followup")
        source = replace_once(source,
            "    model = selected_model(req)\n    argv=_worker_command(state['container'],",
            "    model = selected_model(req)\n"
            "    _require_tmpfs_home(state['container'])\n"
            "    try:\n"
            "        before = _docker(_worker_command(state['container'],\n"
            "            ['node', '-e', effective.ROLLOUT, sessions[0]])[1:])\n"
            "        previous = effective.parse_observation(before.stdout)['context_count']\n"
            "    except effective.EffectiveRuntimeError:\n"
            "        raise CodexRuntimeError('Continuation runtime context is unavailable') from None\n"
            "    argv=_worker_command(state['container'],")
        source = replace_once(source,
            "    return {**task,'metrics':task['telemetry']}\n",
            "    try:\n"
            "        thread = effective.thread_from_events(task['events'])\n"
            "        observed = _docker(_worker_command(state['container'],\n"
            "            ['node', '-e', effective.ROLLOUT, sessions[0]])[1:])\n"
            "        task['effective_runtime'] = effective.qualify(task, observed.stdout,\n"
            "            model=model, effort=selected_reasoning_effort(req),\n"
            "            previous_contexts=previous, expected_thread=sessions[0])\n"
            "    except effective.EffectiveRuntimeError:\n"
            "        raise CodexRuntimeError('Effective continuation runtime could not be qualified') from None\n"
            "    return {**task,'metrics':task['telemetry']}\n")
        return source

    patch("integration/codex_runtime.py", runtime)
    def broker(source):
        source = patch_job_broker(replace_once(replace_once(source,
            "if(state.guard?.execution_mode==='disposable_native'&&state.baselinePresent)return true;",
            "if(state.guard?.family==='g9-policy-install')return require('./g9_policy_install_broker.cjs').allowed(parsed,classify(args,state.registry||registry),state);\n"
            "  if(state.guard?.execution_mode==='disposable_native')return false;"),
            "  if (Object.hasOwn(flags,'config')) return false;",
            "  if(state.guard?.family==='g9-project-read')return require('./g9_project_read_broker.cjs').allowed(parsed,classify(args,state.registry||registry),state);\n"
            "  if (Object.hasOwn(flags,'config')) return false;"))
        source = replace_once(source,
            "  if(state.guard?.family==='job')return g9JobBroker.allowed(parsed,classify(args,state.registry||registry),state);",
            "  if(state.guard?.family==='g9-local-policy')return require('./g9_policy_broker.cjs').allowed(parsed,classify(args,state.registry||registry),state);\n"
            "  if(state.guard?.family==='job')return g9JobBroker.allowed(parsed,classify(args,state.registry||registry),state);")
        patched = patch_windows_broker(source)
        patched = patch_release_broker(patched) if release_enabled else patched
        return patch_credential_broker(patched, replace_once)

    patch("integration/local_broker.cjs", broker)
    patch("integration/tadx_client.cjs", patch_windows_client)
    patch("integration/pulse_broker.cjs", lambda s: patch_pulse_broker(s, replace_once))
    patch("integration/pulse_profiles.py", lambda s: patch_pulse_profile(s, replace_once))
    patch("integration/project_profiles.py", lambda s: patch_project(s, replace_once))
    # The existing native command invocation and strict project guard stay intact.
    def launcher(source):
        source = replace_once(source,
            "config.setdefault('run_constraints',{})['native_cli_execution']=True",
            "config.setdefault('run_constraints',{})['native_cli_execution']=bool("
            "config.get('run_constraints',{}).get('g9_authorization'))")
        source = replace_once(source,
            "source=fingerprint(manifest['repository_path'])\n"
            "    if source['digest']!=manifest.get('source_tree_digest'):\n"
            "        raise Blocked('TADX working tree changed since capture; rerun -CaptureCli before qualification')",
            "prepared=load(root.parent/'preparation.json')\n"
            "    expected_manifest=(root.parent/'candidate/capture/manifest.json').resolve()\n"
            "    if (manifest_path.resolve()!=expected_manifest\n"
            "            or file_sha(manifest_path)!=prepared.get('snapshot_files',{}).get('candidate/capture/manifest.json')\n"
            "            or not prepared.get('capture_manifest_sha256')):\n"
            "        raise Blocked('Active captured CLI manifest differs from qualified capture')\n"
            "    if (manifest.get('source_commit') != config.get('run_constraints',{}).get('g9_accepted_gate_sha')\n"
            "            or manifest.get('source_tree_digest') != config.get('run_constraints',{}).get('g9_source_tree_sha')):\n"
            "        raise Blocked('Copied CLI source differs from accepted G0-G8 candidate')")
        source = replace_once(source,
            "    args=parser.parse_args(argv)\n",
            "    parser.add_argument('--g9-authorization',type=Path)\n"
            "    parser.add_argument('--g9-authorization-sha256')\n"
            "    args=parser.parse_args(argv)\n"
            "    g9_record=None\n"
            "    if not args.plan:\n"
            "        if (args.check or args.resume or args.category or args.tag\n"
            "                or args.skip_windows_installer or args.skip_installers or args.settings\n"
            "                or args.model or args.reasoning_effort or args.concurrency is not None\n"
            "                or args.fail_after_command is not None or args.qualification_run or not args.local):\n"
            "            raise Blocked('G9 authorization permits only the selected local model plan')\n"
            "        from integration.g9_activation import verify\n"
            "        try:\n"
            "            g9_record=verify(args.g9_authorization,args.g9_authorization_sha256,ROOT.parent,check_snapshot=True,check_launch_inputs=True)\n"
            "        except (ValueError,OSError,TypeError,KeyError) as error:\n"
            "            raise Blocked(str(error)) from None\n"
            "        if (g9_record['purpose']=='full_catalog' and (not args.all or args.case)\n"
            "                or g9_record['purpose']=='windows_qualification'\n"
            "                   and (args.all or args.case!=g9_record['selected_ids'])):\n"
            "            raise Blocked('Selected cases differ from operator-authorized G9 purpose')\n")
        source = replace_once(source,
            "    readiness=build_report(run_dirs=args.qualification_run)\n",
            "    if not args.plan and (len(entries)!=len(g9_record['selected_ids'])\n"
            "            or set(entry['id'] for entry in entries)!=set(g9_record['selected_ids'])):\n"
            "        raise Blocked('Selected cases differ from operator-authorized G9 plan')\n"
            "    readiness=build_report(run_dirs=args.qualification_run)\n")
        source = replace_once(source,
            "    external_ids.update(unavailable_platform_ids(entries))\n"
            "    external_ids.update(KNOWN_WINDOWS_EXTERNAL_IDS-set(supported))\n",
            "    hosted=set(g9_record['selected_ids']) & KNOWN_WINDOWS_EXTERNAL_IDS\n"
            "    if not hosted <= supported:\n"
            "        raise Blocked('Hosted Windows profile binding is missing')\n"
            "    external_ids.difference_update(hosted)\n"
            "    external_ids.update(unavailable_platform_ids([entry for entry in entries\n"
            "        if entry['id'] not in hosted]))\n"
            "    external_ids.update(KNOWN_WINDOWS_EXTERNAL_IDS-set(supported))\n")
        source = replace_once(source,
            "    base_config=load(ROOT/'runner.local.json')\n",
            "    base_config=load(ROOT/'runner.local.json')\n"
            "    prepared=load(ROOT.parent/'preparation.json')\n"
            "    if file_sha(ROOT/'runner.local.json')!=prepared.get('runner_config_sha256'):\n"
            "        raise Blocked('Prepared runner configuration changed after qualification')\n"
            "    base_config.setdefault('run_constraints',{}).update({\n"
            "        'g9_authorization':{'path':str(args.g9_authorization.resolve()),\n"
            "                            'sha256':args.g9_authorization_sha256},\n"
            "        'g9_accepted_gate_sha':g9_record['source_revision'],\n"
            "        'g9_source_tree_sha':g9_record['source_tree_sha256'],\n"
            "        'g9_windows_installer_version':g9_record['windows_fixture_version']})\n")
        source = replace_once(source,
            "    base_config['model_override']=requested_model\n",
            "    if requested_model != 'gpt-6-luna' or requested_effort != 'medium':\n"
            "        raise Blocked('G9 requires the reviewed Luna medium runtime configuration')\n"
            "    base_config['model_override']=requested_model\n")
        source = replace_once(source,
            "        'frozen_cli_manifest_sha256':file_sha(config['frozen_cli_manifest']),",
            "        'frozen_cli_manifest_sha256':file_sha(config['frozen_cli_manifest']),\n"
            "        'runner_config_sha256':file_sha(ROOT/'runner.local.json'),\n"
            "        'g9_authorization_sha256':args.g9_authorization_sha256,")
        return source
    patch("tools/run_spark_suite.py", launcher)
    path = "integration/docker_local_bridge.py"

    def bridge(source):
        source = replace_once(source, "def image_for(m):\n",
            "def image_for(m, accepted=None):\n")
        source = replace_once(source,
            "    # A strict production manifest always supplies an existing binary path.\n",
            "    if accepted is not None:\n"
            "        if not re.fullmatch(r'sha256:[0-9a-f]{64}',accepted):\n"
            "            raise Blocked('Accepted worker image ID is invalid')\n"
            "        existing=docker('image','inspect',tag,check=False)\n"
            "        if existing.returncode:\n"
            "            raise Blocked('Accepted worker image is absent; dispatch cannot build it')\n"
            "        details=json.loads(existing.stdout)[0]\n"
            "        if details.get('Id')!=accepted:\n"
            "            raise Blocked('Worker image tag differs from accepted immutable ID')\n"
            "        return accepted\n"
            "    # A strict production manifest always supplies an existing binary path.\n")
        source = replace_once(source, "m=manifest(req); image=image_for(m)",
            "m=manifest(req); image=image_for(m,req.get('_g9_accepted_image'))")
        source = replace_once(source,
            "    from integration import release_profiles,installer_profiles\n",
            "    if req['exercise']['id'] in ('V-installer-windows-fresh', 'V-installer-windows-idempotent',\n"
            "          'V-installer-windows-no-completion', 'V-installer-windows-completion-opt-in',\n"
            "          'V-installer-windows-failed-download-preserves-binary',\n"
            "          'V-installer-windows-uninstall', 'V-installer-windows-no-modify-path'):\n"
            "        from integration import g9_windows_installer_profile\n"
            "        return g9_windows_installer_profile\n"
            "    from integration import release_profiles,installer_profiles\n")
        source = replace_once(source, "def profile(req):\n",
                              "def profile(req):\n"
                              "    if req['exercise']['id'] == 'P-policy-install':\n"
                              "        from integration import g9_policy_install_profile\n"
                              "        return g9_policy_install_profile\n"
                              "    if req['exercise']['id'] == 'P-pulse-subscription-list':\n"
                              "        from integration import g9_pulse_subscription_profile\n"
                              "        return g9_pulse_subscription_profile\n"
                              "    if req['exercise']['id'] in ('P-policy-samples', 'P-policy-validate', 'P-policy-status'):\n"
                              "        from integration import g9_policy_profile\n"
                              "        return g9_policy_profile\n"
                              "    if req['exercise']['id'] in ('P-job-inspect', 'P-job-wait', 'P-job-cancel'):\n"
                              "        from integration import g9_job_profile\n"
                              "        return g9_job_profile\n"
                              "    if req['exercise']['id'] in ('P-project-list', 'P-project-inspect'):\n"
                              "        from integration import g9_project_read\n"
                              "        return g9_project_read\n")
        source = replace_once(source, "    method,request,response=sys.argv[1:]\n"
            "    try: result=dispatch(method,load(request))\n",
            "    method,request,response=sys.argv[1:]\n"
            "    try:\n"
            "        req=load(request)\n"
            "        binding=req.get('run_constraints',{}).get('g9_authorization')\n"
            "        if not isinstance(binding,dict) or set(binding)!={'path','sha256'}:\n"
            "            raise Blocked('G9 operator authorization is absent')\n"
            "        from integration.g9_activation import verify\n"
            "        accepted=verify(binding['path'],binding['sha256'],ROOT.parent)\n"
            "        if req.get('exercise',{}).get('id') not in accepted['selected_ids']:\n"
            "            raise Blocked('G9 case differs from authorized plan')\n"
            "        if (req.get('frozen_cli',{}).get('source_commit')!=accepted['source_revision']\n"
            "                or req.get('frozen_cli',{}).get('binaries',{}).get('linux',{}).get('sha256')\n"
            "                   !=accepted['builds']['linux/amd64']\n"
            "                or req.get('runtime_model')!='gpt-6-luna'\n"
            "                or req.get('runtime_reasoning_effort')!='medium'\n"
            "                or req.get('run_constraints',{}).get('g9_accepted_gate_sha')\n"
            "                   !=accepted['source_revision']):\n"
            "            raise Blocked('G9 request differs from accepted candidate or runtime')\n"
            "        req['_g9_accepted_image']=accepted['image']['id']\n"
            "        result=dispatch(method,req)\n")
        source = replace_once(source,
            "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py')",
            "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py','g9_project_read_broker.cjs')")
        source = replace_once(source,
            "s['broker_guard'].get('mode','').startswith('shared-')):",
            "s['broker_guard'].get('mode','').startswith('shared-') and "
            "s['broker_guard'].get('family') != 'g9-policy-install'):")
        start = "def _seed_site_mutation_consent(config, enabled):\n"
        end = "\n\ndef prepare(req):\n"
        require(source.count(start) == 1 and source.count(end) == 1, "Consent patch anchors differ")
        left, rest = source.split(start)
        _, right = rest.split(end)
        source = left + (
            "def _seed_site_mutation_consent(config, enabled, authority):\n"
            "    from integration.g9_consent import preserve_consent\n"
            "    return preserve_consent(config, enabled, authority)\n"
        ) + end + right
        source = patch_job_image(replace_once(source,
            "prepared.get('config_seed'), prepared['mutation_policy'] == 'enabled')",
            "prepared.get('config_seed'), prepared['mutation_policy'] == 'enabled',\n"
            "                req.get('run_constraints', {}).get('g9_saved_consent_authority'))"))
        source = replace_once(source,
            "'g9_job_broker.cjs')",
            "'g9_job_broker.cjs','g9_policy_broker.cjs','g9_policy_install_broker.cjs')")
        source = replace_once(source,
            "'g9_job_broker.cjs','g9_policy_broker.cjs','g9_policy_install_broker.cjs')",
            "'g9_job_broker.cjs','g9_policy_broker.cjs','g9_policy_install_broker.cjs','g9_windows_installer_broker.cjs')")
        source = replace_once(source,
            "    broker = docker('create', '--name', s['broker_container'], *_label_args(_split_labels(req, 'cli')), *common,\n",
            "    broker_common = list(common)\n"
            "    if s.get('broker_guard', {}).get('family') == 'g9-local-policy':\n"
            "        work_mount = 'type=bind,source=' + str(work) + ',target=/work'\n"
            "        if broker_common.count(work_mount) != 1: raise Blocked('Policy broker work mount differs')\n"
            "        broker_common[broker_common.index(work_mount)] = work_mount + ',readonly'\n"
            "    if s.get('broker_guard', {}).get('family') == 'g9-policy-install':\n"
            "        work_mount = 'type=bind,source=' + str(work) + ',target=/work'\n"
            "        if broker_common.count(work_mount) != 1 or broker_common.count('--read-only') != 1:\n"
            "            raise Blocked('Protected install broker boundary differs')\n"
            "        broker_common[broker_common.index(work_mount)] = work_mount + ',readonly'\n"
            "        broker_common.remove('--read-only')  # The broker alone gets a disposable root overlay.\n"
            "    broker = docker('create', '--name', s['broker_container'], *_label_args(_split_labels(req, 'cli')), *broker_common,\n")
        source = replace_once(source,
            "    else:\n        raise Blocked('Offline CLI broker did not become ready')\n    if s.get('config_seed') is not None:\n",
            "    else:\n        raise Blocked('Offline CLI broker did not become ready')\n"
            "    if s.get('broker_guard', {}).get('family') == 'g9-local-policy':\n"
            "        from integration import g9_policy_private\n"
            "        s['policy_private_setup'] = g9_policy_private.setup(req, s, work, docker)\n"
            "        write_state(req, s)\n"
            "    if s.get('config_seed') is not None:\n")
        source = replace_once(source,
            "'cli_mutation_policy': _policy_evidence(req, s, cli_info), 'cli_guidance_home': guidance_home,",
            "'cli_mutation_policy': _policy_evidence(req, s, cli_info), 'cli_guidance_home': guidance_home,\n"
            "        'cli_user': cli_info['Config'].get('User'),\n"
            "        'policy_private_setup': s.get('policy_private_setup'),")
        source = replace_once(source,
            "    if broker is not None:\n        copies={}\n",
            "    if broker is not None:\n"
            "        if s.get('broker_guard', {}).get('family') == 'g9-policy-install':\n"
            "            from integration import g9_policy_install_private\n"
            "            try:\n"
            "                s['policy_install_capture'] = g9_policy_install_private.capture(req, s, broker['Id'], docker)\n"
            "            except (Blocked, ValueError, OSError):\n"
            "                s['policy_install_capture'] = {'status': 'not_copied', 'reason': 'protected-path observation failed'}\n"
            "            write_state(req, s)\n"
            "        if s.get('broker_guard', {}).get('family') == 'g9-local-policy':\n"
            "            from integration import g9_policy_private\n"
            "            identity = s['broker_container'] if broker.get('Name') is None else broker['Id']\n"
            "            s['policy_native_output_capture'] = g9_policy_private.capture(req, s, identity, docker)\n"
            "            write_state(req, s)\n"
            "        copies={}\n")
        source = replace_once(source,
            "def _local_disposable_recovery(req, s):\n",
            "def _local_disposable_recovery(req, s):\n"
            "    if req.get('exercise', {}).get('id', '').startswith('V-installer-windows-'):\n"
            "        return None  # The hosted profile owns exact branch and job cleanup.\n"
            "    if req.get('exercise', {}).get('id') in ('P-policy-install', 'P-policy-samples', 'P-policy-validate', 'P-policy-status'):\n"
            "        return None  # The exact policy profile must verify cleanup itself.\n")
        source = replace_once(source,
            "def _local_before_worker_recovery(req, s):\n",
            "def _local_before_worker_recovery(req, s):\n"
            "    if req.get('exercise', {}).get('id', '').startswith('V-installer-windows-'):\n"
            "        return None  # The hosted profile must release its prewarmed job.\n")
        source = replace_once(source,
            "        result=model_run(req,s,capture,Redactor,ROOT)",
            "        if req['exercise']['id'].startswith('V-installer-windows-'):\n"
            "            from integration.g9_windows_installer_watch import run_model\n"
            "            result=run_model(req,s,lambda: model_run(req,s,capture,Redactor,ROOT),docker)\n"
            "        else:\n"
            "            result=model_run(req,s,capture,Redactor,ROOT)")
        source = replace_once(source,
            "    s['task_sessions']=result.get('task',{}).get('telemetry',{}).get('session_ids',[]);write_state(req,s)",
            "    if not req.get('deterministic'):\n"
            "        s['effective_runtime'] = result['task']['effective_runtime']\n"
            "    s['task_sessions']=result.get('task',{}).get('telemetry',{}).get('session_ids',[]);write_state(req,s)")
        source = replace_once(source,
            "    return model_followup(req,s,capture,Redactor,ROOT)\n",
            "    result = model_followup(req,s,capture,Redactor,ROOT)\n"
            "    s.setdefault('effective_runtime_followups', []).append(result['effective_runtime'])\n"
            "    write_state(req,s)\n"
            "    return result\n")
        source = replace_once(source,
            "    evidence['audit_capture']=s.get('audit_capture', {'status':'not_recorded'})\n",
            "    evidence['audit_capture']=s.get('audit_capture', {'status':'not_recorded'})\n"
            "    evidence['effective_runtime']=s.get('effective_runtime')\n"
            "    evidence['effective_runtime_followups']=s.get('effective_runtime_followups', [])\n")
        source = replace_once(source,
            "    evidence['audit_capture']=s['audit_capture']\n",
            "    evidence['audit_capture']=s['audit_capture']\n"
            "    evidence['effective_runtime']=s.get('effective_runtime')\n"
            "    evidence['effective_runtime_followups']=s.get('effective_runtime_followups', [])\n")
        return patch_credential_bridge(replace_once(
            source, "codex-cli " + SOURCE_RUNTIME_VERSION,
            "codex-cli " + TARGET_RUNTIME_VERSION), replace_once)

    patch(path, bridge)
    patch("tools/reset_site.py", lambda s: patch_reset_site(s, replace_once))
    patch("integration/operator_reset.py", lambda s: patch_operator_reset(s, replace_once))
    patch("integration/content_profiles.py", lambda s: patch_content_profiles(s, replace_once))
    name = "tests/test_reset_absence.py"
    require(name not in files, "Independent reset regression already exists in the source lock")
    files[name] = read(HERE / "source/tests/test_reset_absence.py")
    patches.append({"path": name, "before_sha256": None, "after_sha256": sha(files[name])})
    for action in ("list", "inspect"):
        name = f"suite/exercises/P-project-{action}.json"
        original = files[name]
        exercise = parse(original)
        if action == "list":
            instruction = "List only the direct child projects of ${public.source_project} (parent ID ${public.parent_id}). "
        else:
            instruction = "Inspect only ${public.target_name} (project ID ${public.target_id}) inside ${public.source_project}. "
        exercise["agent"]["prompt"] = instruction + exercise["agent"]["prompt"]
        exercise["agent"]["public_bindings"].update({key: "fixture.public." + key for key in
                                                     ("source_project", "parent_id", "target_id", "target_name")})
        files[name] = encode(exercise)
        patches.append({"path": name, "before_sha256": sha(original), "after_sha256": sha(files[name])})
    patches.extend(apply_mutation_definitions(files))
    present_windows = WINDOWS_IDS & {
        name.removeprefix("suite/exercises/").removesuffix(".json")
        for name in files if name.startswith("suite/exercises/") and name.endswith(".json")}
    require(not present_windows or present_windows == WINDOWS_IDS,
            "Locked Windows installer exercises are incomplete")
    for case_id in sorted(present_windows):
        name = "suite/exercises/" + case_id + ".json"
        original = files[name]
        files[name] = encode(extend_budget(parse(original)))
        patches.append({"path": name, "before_sha256": sha(original),
                        "after_sha256": sha(files[name])})
    index_name = "suite/index.json"
    original = files[index_name]
    index = parse(original)
    locked_cases = {name.removeprefix("suite/exercises/").removesuffix(".json")
                    for name in files if name.startswith("suite/exercises/P-") and name.endswith(".json")}
    selected = [row for row in index["exercises"] if row.get("id") in locked_cases | present_windows]
    require(len(locked_cases) in {len(PROJECT_CASES), 116} and
            set(PROJECT_CASES) <= locked_cases and
            (not present_windows or len(locked_cases) == 116) and
            len(selected) == len(locked_cases | present_windows) and
            {row["id"] for row in selected} == locked_cases | present_windows and
            all(row.get("path") == "suite/exercises/" + row["id"] + ".json" and
                "fixtures/profiles/" + row["id"] + ".json" in files for row in selected),
            "Selected base-action index differs from exact locked case pairs")
    template = parse(files["suite/exercises/P-project-list.json"])
    profile_template = parse(files["fixtures/profiles/P-project-list.json"])
    require(template.get("id") == "P-project-list" and profile_template.get("id") == "P-project-list",
            "Job generation template differs from locked source")
    pulse_template = parse(files["suite/exercises/P-pulse-metric-followers.json"]) if len(locked_cases) == 116 else template
    pulse_profile_template = parse(files["fixtures/profiles/P-pulse-metric-followers.json"]) if len(locked_cases) == 116 else profile_template
    generated = {**job_definitions(template, profile_template),
                 **policy_definitions(template, profile_template),
                 **policy_install_definitions(template, profile_template),
                 **subscription_definitions(pulse_template, pulse_profile_template)}
    for case_id, (exercise, profile) in generated.items():
        for generated_name, value in (("suite/exercises/" + case_id + ".json", exercise),
                                      ("fixtures/profiles/" + case_id + ".json", profile)):
            require(generated_name not in files, "Job definition already exists in locked source")
            files[generated_name] = encode(value)
            patches.append({"path": generated_name, "before_sha256": None,
                            "after_sha256": sha(files[generated_name])})
        category = "jobs" if case_id in JOB_CASES else "policy" if case_id in POLICY_CASES + [POLICY_INSTALL_CASE] else "pulse"
        selected.append({"id": case_id, "category": category,
                         "tags": ["base", category, "standard"],
                         "path": "suite/exercises/" + case_id + ".json"})
    selected.sort(key=lambda row: row["id"])
    index["exercises"] = selected
    index["counts"] = {"exercises": len(selected)}
    files[index_name] = encode(index)
    patches.append({"path": index_name, "before_sha256": sha(original),
                    "after_sha256": sha(files[index_name])})
    return patches


def validate_source_closure(files, root):
    """Reject missing static local imports and image inputs in the copied source."""
    for name, blob in files.items():
        if name.endswith(".py"):
            tree = ast.parse(blob, filename=name)
            for node in ast.walk(tree):
                modules = []
                if isinstance(node, ast.Import):
                    modules = [alias.name for alias in node.names]
                elif isinstance(node, ast.ImportFrom):
                    base = node.module or ""
                    if node.level:
                        base = name.rsplit("/", 1)[0].replace("/", ".") + ("." + base if base else "")
                    modules = [base] + [base + "." + alias.name for alias in node.names]
                for module in modules:
                    if module.startswith(("bench.", "integration.", "tools.")):
                        target = module.replace(".", "/") + ".py"
                        if target in files:
                            continue
                        if module in {"integration.g9_consent", "integration.g9_project_read",
                                      "integration.g9_job_profile", "integration.g9_job_contract",
                                      "integration.g9_job_fixture", "integration.g9_policy_profile",
                                      "integration.g9_policy_private",
                                      "integration.g9_policy_install_profile",
                                      "integration.g9_policy_install_private",
                                      "integration.g9_pulse_subscription_profile"}:
                            require(False, "Copied runtime lacks a generated project module")
                        # A missing module is never inferred from a sibling symbol.
                        require(not (root / target).is_file(), "Copied runtime has an unlocked Python import")
        elif name.endswith(".cjs"):
            for target in re.findall(r"require\(['\"]\./([^'\"]+\.cjs)['\"]\)", blob.decode("utf-8")):
                require("integration/" + target in files,
                        "Copied runtime has an unlocked broker import: " + name + " -> " + target)
    bridge = files["integration/docker_local_bridge.py"].decode("utf-8")
    image_section = bridge.split("def image_for(m, accepted=None):", 1)[1].split("def runtime_check(", 1)[0]
    for target in re.findall(r"['\"]([^'\"]+\.(?:cjs|py)|Dockerfile\.local)['\"]", image_section):
        require("integration/" + target in files, "Copied worker image input is unlocked")


def required_actions(files):
    """Bind every locked base case to its exact declared action."""
    cases = sorted(name.removeprefix("suite/exercises/").removesuffix(".json")
                   for name in files if name.startswith("suite/exercises/P-") and name.endswith(".json"))
    require(len(cases) in {len(PROJECT_CASES), 116} and len(set(cases)) == len(cases),
            "Locked base-action set differs")
    actions = set()
    for case in cases:
        exercise = parse(files["suite/exercises/" + case + ".json"])
        profile = parse(files["fixtures/profiles/" + case + ".json"])
        require(exercise.get("id") == case and profile.get("id") == case,
                "Locked base-action pair differs")
        action = profile.get("base_profile")
        if len(cases) == len(PROJECT_CASES) and action is None:
            action = case.removeprefix("P-").replace("-", ".")
        require(isinstance(action, str) and action and action not in actions,
                "Locked base case lacks a unique action")
        actions.add(action)
    for case, (_, action) in NEW_CASES.items():
        require(case not in cases and action not in actions,
                "Generated action overlaps locked suite")
        actions.add(action)
    require(len(actions) == (124 if len(cases) == 116 else len(CASES)),
            "Required action inventory is incomplete")
    return actions


def validate_candidate(candidate, catalog, builds, required=None):
    require(isinstance(candidate, dict) and set(candidate) == {
        "source_revision", "source_tree_sha256", "catalog_sha256", "builds"}, "Invalid candidate identity")
    require(isinstance(candidate["source_revision"], str)
            and re.fullmatch(r"[0-9a-f]{40}", candidate["source_revision"]), "Invalid source revision")
    for key in ("source_tree_sha256", "catalog_sha256"):
        require(isinstance(candidate[key], str) and re.fullmatch(r"[0-9a-f]{64}", candidate[key]),
                "Invalid candidate digest")
    require(sha(catalog) == candidate["catalog_sha256"], "Catalog does not match accepted candidate")
    require(isinstance(candidate["builds"], dict) and set(candidate["builds"]) == set(builds)
            and set(builds) == {"windows/amd64", "linux/amd64"},
            "Exact accepted Windows and Linux builds are required")
    for platform, blob in builds.items():
        require(re.fullmatch(r"[a-z0-9]+/[a-z0-9]+", platform) and blob
                and sha(blob) == candidate["builds"][platform], "Build does not match accepted candidate")
    rows = parse(catalog)
    require(isinstance(rows, list) and all(isinstance(row, dict) for row in rows), "Invalid catalog")
    ids = [row.get("id") for row in rows]
    require(all(isinstance(item, str) for item in ids) and len(set(ids)) == len(ids), "Duplicate catalog IDs")
    executable = {row["id"] for row in rows if row.get("owner") == "cli"
                  and row.get("implementation") == "implemented" and row.get("command_path")}
    require((required if required is not None else
             {case.removeprefix("P-").replace("-", ".") for case in CASES}) <= executable,
            "Candidate lacks a required executable action")


def isolated_runner_config(blob, authority):
    """Bind reviewed site authority and one model request before hashing."""
    validate_authority(authority)
    config = parse(blob)
    allowed_root = {"bridge_argv", "deployment", "deployment_lock", "fixture_asset_digest",
                    "fixture_version", "frozen_cli_manifest", "frozen_suite_manifest",
                    "objective_policy", "require_frozen_cli", "run_constraints", "runtime",
                    "runtime_env_allowlist", "secret_env_names", "task_reporting"}
    allowed_deployment = {"catalog_fixture_source_dir", "metadata_restore_evidence_file",
                          "metadata_restore_evidence_files", "operator_settings_file",
                          "pulse_dependency_discovery_file", "remote_binding_file",
                          "codex_auth_source_file"}
    require(isinstance(config, dict) and isinstance(config.get("runtime"), dict)
            and set(config) <= allowed_root
            and isinstance(config.get("deployment", {}), dict)
            and set(config.get("deployment", {})) <= allowed_deployment
            and config["runtime"].get("kind") == "bridge"
            and config["runtime"].get("provider") == "openai"
            and config["runtime"].get("reasoning_effort") == "medium",
            "Private runner configuration has an unsupported runtime")
    require(isinstance(config["runtime"].get("model"), str)
            and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:-]{0,127}", config["runtime"]["model"]),
            "Private runner model is invalid")
    for key in ("runtime_env_allowlist", "secret_env_names"):
        require(isinstance(config.get(key, []), list) and all(
            isinstance(name, str) and re.fullmatch(r"[A-Za-z][A-Za-z0-9_]{0,127}", name)
            for name in config.get(key, [])), "Runner environment allowlist is invalid")
    def no_secret_values(value):
        if isinstance(value, dict):
            for key, child in value.items():
                require(isinstance(key, str) and not re.search(
                    r"(?:^|_)(?:password|token|secret|api_key|pat_name|session_key)$", key, re.I),
                    "Runner configuration contains credential material")
                if key in ("runtime_env_allowlist", "secret_env_names"):
                    continue  # The validated entries are environment variable names, not values.
                no_secret_values(child)
        elif isinstance(value, list):
            for child in value:
                no_secret_values(child)
        elif isinstance(value, str):
            require(not re.search(r"(?i)(?:Bearer\s+\S+|-----BEGIN [A-Z ]*PRIVATE KEY-----|"
                                  r"\b(?:sk|pat)-[A-Za-z0-9_-]{16,}\b|"
                                  r"\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.)", value),
                    "Runner configuration contains credential material")
            require(not ("/" not in value and "\\" not in value
                         and re.fullmatch(r"[A-Za-z0-9_-]{32,}", value)),
                    "Runner configuration contains an opaque value")
    constraints = config.setdefault("run_constraints", {})
    require(isinstance(constraints, dict)
            and "g9_saved_consent_authority" not in constraints,
            "Private runner authority must be bound by the preparer")
    no_secret_values(config)
    constraints["g9_saved_consent_authority"] = authority
    config["runtime"]["model"] = "gpt-6-luna"
    return encode(config)


def capture_inputs(path, candidate, catalog, builds, output):
    """Verify and rebase an existing capture without running or rebuilding TADX."""
    capture_path = plain_path(path)
    raw = read(capture_path)
    source = parse(raw)
    require(isinstance(source, dict) and source.get("source_commit") == candidate["source_revision"]
            and source.get("source_tree_digest") == candidate["source_tree_sha256"],
            "Prebuilt capture belongs to another accepted source")
    require(source.get("capture_kind") == "current-worktree-including-uncommitted-changes",
            "Unsupported prebuilt capture kind")
    capture_root = capture_path.parent
    full_blob = read(capture_root / "complete-source-fingerprint.json")
    full = parse(full_blob)
    require(sha(full_blob) == source.get("complete_source_fingerprint_sha256")
            and isinstance(full, dict) and full.get("commit") == candidate["source_revision"]
            and full.get("digest") == candidate["source_tree_sha256"]
            and full.get("full_digest") == source.get("complete_source_tree_digest")
            and isinstance(full.get("full_files"), dict) and full["full_files"],
            "Complete accepted source capture differs")
    require(all(isinstance(name, str) and isinstance(digest, str)
                and re.fullmatch(r"[0-9a-f]{64}", digest) and relative(name)
                for name, digest in full["full_files"].items()),
            "Complete accepted source path or digest differs")
    ordinal = "".join(f"{name} {digest}\n" for name, digest in sorted(
        full["full_files"].items(), key=lambda row: row[0].encode("utf-8")))
    require(sha(ordinal.encode("utf-8")) == full.get("full_ordinal_sha256")
            == source.get("complete_source_ordinal_sha256"),
            "Complete accepted source ordinal fingerprint differs")
    result = {"candidate/capture/registry.json": read(capture_root / "registry.json")}
    result["candidate/capture/complete-source-fingerprint.json"] = full_blob
    installer = read(capture_root / "scripts/install.ps1")
    require(sha(installer) == source.get("installer_sha256")
            == full["full_files"].get("scripts/install.ps1"),
            "Captured installer source differs from complete accepted source")
    result["candidate/capture/scripts/install.ps1"] = installer
    require(sha(result["candidate/capture/registry.json"]) == source.get("registry_sha256")
            and parse(result["candidate/capture/registry.json"]) == parse(catalog),
            "Captured registry differs from accepted catalog")
    entries = source.get("binaries")
    require(isinstance(entries, dict) and set(entries) == {"windows", "linux"},
            "Capture must bind Windows and Linux binaries")
    for system in ("windows", "linux"):
        entry = entries[system]
        require(isinstance(entry, dict) and entry.get("sha256") == candidate["builds"][system + "/amd64"]
                and sha(read(entry["path"])) == entry["sha256"]
                and builds[system + "/amd64"] == read(entry["path"]),
                "Captured binary differs from accepted candidate bytes")
    require(source.get("binary") == entries["windows"]["path"]
            and source.get("binary_sha256") == entries["windows"]["sha256"],
            "Host binary identity differs from captured Windows build")
    skills = source.get("skills")
    require(isinstance(skills, dict) and set(skills) == {"tadx", "tadx-pulse"},
            "Both captured Guidance packages are required")
    for package, hashes in skills.items():
        require(isinstance(hashes, dict) and "SKILL.md" in hashes
                and any(name.startswith("references/") for name in hashes),
                "Captured Guidance package is incomplete")
        root = capture_root / "internal/agent/skills" / package
        actual = {p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file()}
        require(actual == set(hashes), "Captured Guidance file set differs")
        for name, expected in hashes.items():
            relative(name)
            blob = read(root / name)
            require(sha(blob) == expected, "Captured Guidance changed")
            result[f"candidate/capture/internal/agent/skills/{package}/{name}"] = blob
    help_hashes = source.get("help_sha256")
    require(isinstance(help_hashes, dict) and help_hashes, "Captured help is missing")
    expected_help = {row["id"] + ".txt" for row in parse(catalog)
                     if row.get("owner") == "cli" and row.get("implementation") == "implemented"}
    require(set(help_hashes) == expected_help, "Captured help differs from accepted executable commands")
    help_root = capture_root / "help"
    require({p.relative_to(help_root).as_posix() for p in help_root.rglob("*") if p.is_file()}
            == set(help_hashes), "Captured help file set differs")
    for name, expected in help_hashes.items():
        relative(name)
        blob = read(capture_root / "help" / name)
        require(sha(blob) == expected, "Captured help changed")
        result["candidate/capture/help/" + name] = blob
    metadata = source.get("build_metadata")
    require(isinstance(metadata, dict) and set(metadata) == {"windows", "linux"},
            "Both native build metadata records are required")
    adapted_metadata = {}
    for system, entry in metadata.items():
        require(isinstance(entry, dict), "Invalid native build metadata")
        blob = read(entry["path"])
        require(sha(blob) == entry.get("sha256"), "Native build metadata changed")
        info = blob.decode("utf-8")
        fields = {}
        for line in info.splitlines():
            parts = line.strip().split("\t")
            if len(parts) == 2 and parts[0] == "build" and "=" in parts[1]:
                key, value = parts[1].split("=", 1)
                require(key not in fields, "Duplicate native build metadata field")
                fields[key] = value
        require(fields.get("vcs.revision") == candidate["source_revision"]
                and fields.get("GOOS") == system and fields.get("GOARCH") == "amd64",
                "Native build metadata differs from candidate")
        name = f"candidate/capture/build-metadata/{system}.txt"
        result[name] = blob
        adapted_metadata[system] = {"path": str(output / name), "sha256": sha(blob)}
    copied = {system: {"path": str(output / "candidate/builds" / system / "amd64" /
                      ("tadx.exe" if system == "windows" else "tadx")),
                       "sha256": entries[system]["sha256"]} for system in ("windows", "linux")}
    adapted = {"source_commit": candidate["source_revision"],
               "source_tree_digest": candidate["source_tree_sha256"],
               "complete_source_tree_digest": source["complete_source_tree_digest"],
               "complete_source_ordinal_sha256": source["complete_source_ordinal_sha256"],
               "complete_source_fingerprint_sha256": source["complete_source_fingerprint_sha256"],
               "installer_sha256": source["installer_sha256"],
               "capture_kind": source["capture_kind"], "binary": copied["windows"]["path"],
               "binary_sha256": copied["windows"]["sha256"], "binaries": copied,
               "registry_sha256": source["registry_sha256"], "skills": skills,
               "help_sha256": help_hashes, "build_metadata": adapted_metadata}
    result["candidate/capture/manifest.json"] = encode(adapted)
    result["source/current-cli.json"] = encode({"manifest": str(output / "candidate/capture/manifest.json"),
                                                "source_tree_digest": candidate["source_tree_sha256"]})
    return result, sha(raw)


def prepare_release(files, candidate, captured, source_archive, output):
    """Derive one accepted source lock and rebuild private release assets offline."""
    archive = plain_path(source_archive)
    toolchain = release_go_toolchain({system: captured[f"candidate/capture/build-metadata/{system}.txt"]
                                      for system in ("windows", "linux")})
    source_lock = {
        "schema_version": 1,
        "source_commit": candidate["source_revision"],
        "source_fingerprint": parse(captured["candidate/capture/manifest.json"])[
            "complete_source_ordinal_sha256"],
        "source_archive_sha256": file_digest(archive),
        "go_toolchain": toolchain,
    }
    lock_bytes = encode(source_lock)
    patches = []
    with tempfile.TemporaryDirectory(prefix="release-prepare-", dir=output.parent) as temporary:
        private = Path(temporary)
        lock_path = private / "release_update_source.json"
        lock_path.write_bytes(lock_bytes)
        assets = private / "assets"
        asset_manifest = build_release_assets(archive, lock_path, assets)
        owner_root = HERE.parents[2]
        expected_recipe = {name: file_digest(owner_root / name) for name in RELEASE_RECIPE}
        require(asset_manifest.get("build", {}).get("recipe") == expected_recipe,
                "Release build recipe differs from tracked owners")
        verifier = private / "release_update_assets.py"
        shutil.copyfile(HERE / "release_update" / "assets.py", verifier)
        spec = importlib.util.spec_from_file_location("_prepared_release_assets", verifier)
        require(spec is not None and spec.loader is not None, "Release verifier cannot load")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        require(module.load(assets, expected_source=source_lock) == asset_manifest,
                "Release closure verifier differs from builder")
        templates = {
            "assets.py": "integration/release_update_assets.py",
            "profile.py": "integration/release_update_profile.py",
            "observer.cjs": "integration/release_update_observer.cjs",
        }
        for owner, destination in templates.items():
            require(destination not in files, "Release runtime template already exists")
            blob = read(HERE / "release_update" / owner)
            files[destination] = blob
            patches.append({"path": destination, "before_sha256": None, "after_sha256": sha(blob)})
        files["integration/release_update_source.json"] = lock_bytes
        patches.append({"path": "integration/release_update_source.json", "before_sha256": None,
                        "after_sha256": sha(lock_bytes)})
        for path in assets.rglob("*"):
            if not path.is_file():
                continue
            destination = "fixtures/release-update/" + path.relative_to(assets).as_posix()
            require(destination not in files, "Release asset already exists in original lock")
            blob = read(path)
            files[destination] = blob
            patches.append({"path": destination, "before_sha256": None, "after_sha256": sha(blob)})
    for folder in ("suite/exercises", "fixtures/profiles"):
        require(f"{folder}/P-current-update.json" in files, "Release definition pair is missing")
    exercise_path = "suite/exercises/P-current-update.json"
    profile_path = "fixtures/profiles/P-current-update.json"
    exercise, profile = release_update_definitions(parse(files[exercise_path]), parse(files[profile_path]))
    for path, value in ((exercise_path, exercise), (profile_path, profile)):
        original = files[path]
        files[path] = encode(value)
        patches.append({"path": path, "before_sha256": sha(original),
                        "after_sha256": sha(files[path])})
    return patches, sha(lock_bytes), sha(encode(asset_manifest))


def prepare(harness, output, candidate_path, catalog_path, build_paths, capture_path,
            authority_path, consent_evidence_path, *, release_source_archive=None, lock=None):
    root = plain_path(harness)
    require(root.is_dir(), "Harness must be a source directory")
    output = plain_path(output, missing=True)
    require(not output.exists() and output.parent.is_dir(), "Output must be a new directory with an existing parent")
    require(not output.is_relative_to(root) and not root.is_relative_to(output), "Output overlaps harness source")
    lock = parse(read(HERE / "source-lock-124.json")) if lock is None else lock
    require(isinstance(lock, dict) and lock.get("schema_version") == 1
            and isinstance(lock.get("files"), dict) and lock["files"], "Invalid source lock")
    files = {}
    for name, expected in lock["files"].items():
        relative(name)
        blob = read(root / name)
        require(sha(blob) == expected, "Harness source differs from reviewed lock")
        files[name] = blob
    auxiliary = {}
    for name, expected in AUXILIARY_SOURCE.items():
        require(name not in files, "Auxiliary input unexpectedly entered the 373-file lock")
        blob = read(root / name)
        require(sha(blob) == expected, "Required launcher policy input differs")
        auxiliary[name] = blob
    for case in PROJECT_CASES:
        for folder in ("suite/exercises", "fixtures/profiles"):
            require(parse(files[f"{folder}/{case}.json"]).get("id") == case, "Project definition ID differs")
    candidate = parse(read(candidate_path))
    catalog = read(catalog_path)
    builds = {platform: read(path) for platform, path in build_paths.items()}
    validate_candidate(candidate, catalog, builds, required_actions(files))
    captured, capture_sha = capture_inputs(capture_path, candidate, catalog, builds, output)
    authority_blob = read(authority_path)
    authority = parse(authority_blob)
    validate_authority(authority)
    require(sha(read(consent_evidence_path)) == authority["consent_evidence_sha256"],
            "Saved-consent evidence differs from authority input")
    patches = apply_mutation_runtime(files)
    for row in patch_sources(files):
        append_patch(patches, row)
    for name, blob in auxiliary.items():
        files[name] = blob
        patches.append({"path": name, "before_sha256": None,
                        "after_sha256": sha(blob)})
    runner_input = read(root / "runner.local.json")
    runner_output = isolated_runner_config(runner_input, authority)
    require("runner.local.json" not in files, "Runner config must remain outside the 373-file source lock")
    files["runner.local.json"] = runner_output
    patches.append({"path": "runner.local.json", "before_sha256": None,
                    "after_sha256": sha(runner_output)})
    release_lock_sha = None
    release_asset_sha = None
    if "suite/exercises/P-current-update.json" in files:
        require(release_source_archive is not None,
                "Final candidate source archive is required for release preparation")
        release_patches, release_lock_sha, release_asset_sha = prepare_release(
            files, candidate, captured, release_source_archive, output)
        patches.extend(release_patches)
    files["integration/g9_consent.py"] = read(HERE / "consent.py")
    files["integration/g9_runtime_effective.py"] = read(HERE / "runtime_effective.py")
    files["integration/g9_activation.py"] = read(HERE / "activation.py")
    files["integration/credential_broker.cjs"] = read(
        HERE / "source/integration/credential_broker.cjs")
    files["integration/g9_project_read.py"] = read(HERE / "project_read.py")
    files["integration/g9_project_read_broker.cjs"] = read(HERE / "project_read_broker.cjs")
    files["integration/g9_job_broker.cjs"] = read(HERE / "source/integration/job_broker.cjs")
    files["integration/g9_job_contract.py"] = read(HERE / "source/integration/job_contract.py")
    files["integration/g9_job_fixture.py"] = read(HERE / "source/integration/job_fixture.py")
    files["integration/g9_job_profile.py"] = read(HERE / "source/integration/job_profile.py")
    files["integration/g9_policy_broker.cjs"] = read(HERE / "source/integration/policy_broker.cjs")
    files["integration/g9_policy_profile.py"] = read(HERE / "source/integration/policy_profile.py")
    files["integration/g9_policy_private.py"] = read(HERE / "source/integration/policy_private.py")
    files["integration/g9_policy_install_profile.py"] = read(
        HERE / "source/integration/policy_install_profile.py")
    files["integration/g9_policy_install_private.py"] = read(
        HERE / "source/integration/policy_install_private.py")
    files["integration/g9_policy_install_broker.cjs"] = read(
        HERE / "source/integration/policy_install_broker.cjs")
    files["integration/g9_pulse_subscription_profile.py"] = read(
        HERE / "source/integration/pulse_subscription_profile.py")
    files.update(windows_runtime_sources())
    validate_source_closure(files, root)
    output_files = {"source/" + name: blob for name, blob in files.items()}
    output_files.update({"candidate/candidate.json": encode(candidate), "candidate/catalog.json": catalog})
    output_files.update(captured)
    for platform, blob in builds.items():
        output_files[f"candidate/builds/{platform}/tadx" +
                     (".exe" if platform.startswith("windows/") else "")] = blob
    manifest = {
        "schema_version": 1, "status": "prepared_blocked", "live_execution_enabled": False,
        "candidate": candidate,
        "cases": sorted(row["id"] for row in parse(files["suite/index.json"])["exercises"]),
        "requested_model": "gpt-6-luna",
        "requested_reasoning_effort": "medium", "actual_model_metadata": None,
        "runner_config_input_sha256": sha(runner_input),
        "runner_config_sha256": sha(runner_output),
        "runner_authority_sha256": sha(encode(authority)),
        "source_lock_sha256": sha(encode(lock)), "source_files": lock["files"],
        "patches": patches, "prepared_files": {name: sha(blob) for name, blob in files.items()},
        "snapshot_files": {name: sha(blob) for name, blob in output_files.items()},
        "authority_sha256": sha(authority_blob),
        "capture_manifest_sha256": capture_sha,
        "consent_evidence_sha256": authority["consent_evidence_sha256"],
        "release_source_lock_sha256": release_lock_sha,
        "release_asset_manifest_sha256": release_asset_sha,
        "blockers": BLOCKERS,
    }
    # Everything is validated before creating output. A partial filesystem write
    # cannot gain readiness: the blocked manifest is written first, exclusively.
    output.mkdir()
    with (output / "preparation.json").open("xb") as stream:
        stream.write(encode(manifest))
    for name, blob in output_files.items():
        destination = output / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("xb") as stream:
            stream.write(blob)
    return manifest


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("harness", "output", "candidate", "catalog", "capture", "authority", "consent-evidence", "source-archive"):
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--build", action="append", required=True, metavar="PLATFORM=PATH")
    args = parser.parse_args(argv)
    try:
        builds = {}
        for item in args.build:
            platform, path = item.split("=", 1)
            require(platform not in builds and path, "Duplicate or missing build")
            builds[platform] = Path(path)
        prepare(args.harness, args.output, args.candidate, args.catalog, builds, args.capture,
                args.authority, args.consent_evidence, release_source_archive=args.source_archive)
    except (ValueError, OSError, KeyError, TypeError, UnicodeError):
        print(json.dumps({"status": "blocked", "reason": "Input or preparation validation failed; no live dispatch occurred."}))
        return 1
    print(json.dumps({"status": "prepared_blocked", "live_execution_enabled": False, "blockers": BLOCKERS}))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
