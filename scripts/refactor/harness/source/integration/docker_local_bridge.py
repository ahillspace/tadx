#!/usr/bin/env python3
"""Local serial fixtures, native execution, independent grading, and recovery.

The agent has a non-root Docker boundary and a CLI client shim. The immutable
official executable is root-only and can only be invoked by the audited broker.
No evaluator files, host profiles, Docker socket, or credentials are mounted.
"""
from __future__ import annotations
import contextlib, hashlib, inspect, json, os, re, shutil, subprocess, sys, time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from bench.core import Blocked, Unsupported, Redactor, digest, load, save
from bench.recovery import checkpoint as lifecycle_checkpoint
from bench.process import capture
from bench.admission import require_admission
from bench.tableau import HTTPFailure, UnknownWrite
from tools.reset_site import CLIError
from bench.telemetry import summarize_events

PROTOCOL = 'tadx-bench-bridge/1'
SUPPORTED = {'P-version-get', 'P-capability-list'}
QUALIFICATIONS = {'cli_audit','feedback_disabled','frozen_official_skill_trees',
 'frozen_pre_task_expectations','independent_observer','installed_binary_fingerprint',
 'isolated_agent_filesystem','observable_required_variant','strict_output_decoder',
 'unchanged_mutation_policy'}
READY_POLL_ATTEMPTS = 600
READY_POLL_DELAY_S = 0.1

def profile(req):
    import importlib
    from integration import release_profiles,installer_profiles
    if req['exercise']['id']=='P-current-update':
        from integration import release_update_profile
        return release_update_profile
    if release_profiles.supports(req['exercise']):return release_profiles
    if installer_profiles.supports(req['exercise']):return installer_profiles
    for name in ('outcome_profiles','auth_live_profiles','catalog_live_profiles','validation_profiles','local_state_profiles','workspace_profiles','session_profiles','last_profiles','catalog_profiles','guidance_profiles','diagnostics_profiles','local_profiles','content_selector_profiles','remote_read_profiles','metadata_discovery_profiles','metadata_profiles','content_label_profiles','content_variants','content_profiles','content_preview_profiles','content_move_profiles','schema_profiles','project_profiles','pulse_lifecycle_profiles','pulse_profiles','pulse_creation_profiles','pulse_filter_profiles','pulse_period_profiles','pulse_publish_profiles','lineage_profiles','permission_profiles','inventory_variants','admin_label_profiles','admin_profiles','local_variants'):
        try: module=importlib.import_module('integration.'+name)
        except ModuleNotFoundError as exc:
            if exc.name=='integration.'+name:continue
            raise
        exercise=req['exercise']
        if hasattr(module,'supports'):
            if 'expected_outcomes' not in exercise.get('evaluator', {}):
                case=exercise.get('id','')
                if not re.fullmatch(r'[A-Za-z0-9_-]+',case):continue
                path=ROOT/'suite/exercises'/(case+'.json')
                if not path.is_file():continue
                exercise=load(path)
            if module.supports(exercise):return module
        elif exercise['id'] in module.SUPPORTED:return module
    return None

def sha(path):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda:f.read(1024*1024),b''): h.update(b)
    return h.hexdigest()

def docker(*args, timeout=60, check=True):
    options={'capture_output':True,'text':True,'encoding':'utf-8','errors':'replace','timeout':timeout}
    if os.name == 'nt':
        options['creationflags']=getattr(subprocess, 'CREATE_NO_WINDOW', 0x08000000)
    r=subprocess.run(['docker',*map(str,args)],**options)
    if check and r.returncode: raise Blocked('Docker boundary: '+r.stderr[-2000:])
    return r

def statepath(req): return Path(req['private_case_dir'])/'local-bridge-state.json'
def state(req): return load(statepath(req))
def write_state(req,obj): save(statepath(req),obj)

def _checkpoint(req, state_value, phase, **fields):
    """Persist a durable lifecycle transition alongside the legacy flags."""
    lifecycle_checkpoint(state_value, phase, **fields)
    write_state(req, state_value)
    return state_value


def _prepare_runtime(container, req):
    """Qualify a worker while tolerating older lightweight test doubles."""
    from integration.codex_runtime import prepare_runtime
    kwargs={'model':req.get('runtime_model'),
            'reasoning_effort':req.get('runtime_reasoning_effort')}
    if kwargs['model'] is None and kwargs['reasoning_effort'] is None:
        return prepare_runtime(container)
    try:
        signature=inspect.signature(prepare_runtime)
        parameters=signature.parameters.values()
        accepts_kwargs=any(parameter.kind is inspect.Parameter.VAR_KEYWORD for parameter in parameters)
        if accepts_kwargs or all(name in signature.parameters for name in kwargs):
            return prepare_runtime(container, **kwargs)
    except (TypeError,ValueError):
        pass
    return prepare_runtime(container)
def rel(req,path): return Path(path).relative_to(Path(req['private_case_dir'])).as_posix()


@contextlib.contextmanager
def image_build_lock():
    """Serialize shared Docker image capture across concurrent bridge processes."""
    path = ROOT.parent / 'image-contexts' / 'tadx-bench-image-build.lock'
    path.parent.mkdir(parents=True, exist_ok=True)
    stream = path.open('a+b')
    try:
        if stream.seek(0, os.SEEK_END) == 0:
            stream.write(b'0')
            stream.flush()
        while True:
            try:
                stream.seek(0)
                if os.name == 'nt':
                    import msvcrt
                    msvcrt.locking(stream.fileno(), msvcrt.LK_NBLCK, 1)
                else:
                    import fcntl
                    fcntl.flock(stream.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except OSError:
                time.sleep(0.1)
        yield
    finally:
        try:
            stream.seek(0)
            if os.name == 'nt':
                import msvcrt
                msvcrt.locking(stream.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                import fcntl
                fcntl.flock(stream.fileno(), fcntl.LOCK_UN)
        finally:
            stream.close()

def manifest(req):
    m=req.get('frozen_cli')
    if not isinstance(m,dict): raise Blocked('Frozen executable and skill manifest missing')
    for key in ('root','source_commit','skills','binaries'):
        if key not in m: raise Blocked('Frozen manifest lacks '+key)
    b=m['binaries'].get('linux')
    if not b or sha(b['path'])!=b['sha256']: raise Blocked('Frozen Linux binary missing or changed')
    return m

def image_for(m):
    broker_files=('Dockerfile.local','local_broker.cjs','tadx_client.cjs','local_state_broker.cjs','workspace_broker.cjs','session_broker.cjs','last_broker.cjs','catalog_broker.cjs','content_broker.cjs','content_preview_broker.cjs','content_move_broker.cjs','schema_broker.cjs','pulse_publish_broker.cjs','project_broker.cjs','guidance_broker.cjs','pulse_broker.cjs','pulse_creation_broker.cjs','pulse_filter_broker.cjs','pulse_period_broker.cjs','lineage_broker.cjs','permission_broker.cjs','inventory_broker.cjs','diagnostics_broker.cjs','shared_broker.cjs')
    broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py')
    integration_hashes={name:sha(ROOT/'integration'/name) for name in broker_files}
    base=json.loads(docker('image','inspect','tmcp-agent-lab-worker:local').stdout)[0]['Id']
    # ``manifest`` performs the strict frozen-binary validation for real
    # runs.  Keep image identity construction tolerant of lightweight test
    # manifests that only provide a path, while still deriving a stable
    # identity whenever the file is available.
    binary=m['binaries']['linux']
    binary_sha=binary.get('sha256') if isinstance(binary,dict) else None
    if not binary_sha and isinstance(binary,dict) and binary.get('path'):
        path=Path(binary['path'])
        if path.is_file():
            binary_sha=sha(path)
    binary_sha=binary_sha or digest(binary)
    tag='tadx-bench-local:'+digest({'binary':binary_sha,'integration':integration_hashes,'base':base})[:24]
    # A strict production manifest always supplies an existing binary path.
    # Lightweight reset tests may exercise command policy with a synthetic
    # manifest only; keep image identity deterministic without attempting a
    # filesystem copy that cannot be satisfied by that probe.
    if not isinstance(binary,dict) or not Path(binary.get('path','')).is_file():
        return tag
    with image_build_lock():
        existing=docker('image','inspect',tag,check=False)
        if existing.returncode:
            base_tag='tadx-bench-base:'+base.split(':')[-1][:24]
            docker('tag',base,base_tag)
            context=ROOT.parent/'image-contexts'/tag.split(':')[1]
            context.mkdir(parents=True,exist_ok=True)
            shutil.copy2(m['binaries']['linux']['path'],context/'tadx')
            shutil.copy2(Path(m['root'])/'registry.json',context/'registry.json')
            for src in broker_files:
                shutil.copy2(ROOT/'integration'/src,context/src)
            if any(sha(context/name)!=value for name,value in integration_hashes.items()) or sha(context/'tadx')!=binary_sha:
                raise Blocked('Build inputs changed while capturing the isolated CLI image; retry after integration is stable')
            docker('build','--build-arg','BASE_IMAGE='+base_tag,'--file',context/'Dockerfile.local','--tag',tag,context,timeout=300)
        details=json.loads(docker('image','inspect',tag).stdout)[0]
    return details['Id']

def runtime_check(image, model=None, reasoning_effort=None):
    from integration.codex_runtime import selected_model
    from integration.codex_runtime import selected_reasoning_effort
    model = selected_model(model=model)
    reasoning_effort = selected_reasoning_effort(effort=reasoning_effort)
    version=docker('run','--rm','--user','65532:65532','--read-only','--network','none',
      '--entrypoint','codex',image,'--version').stdout.strip()
    if version!='codex-cli 0.154.0':raise Blocked('Unexpected Codex runtime version')
    return {'cli_version':version,'requested_model':model,'reasoning_effort':reasoning_effort,'provider':'openai'}

def preflight(req):
    # Qualify the exact bridge interpreter before spending model tokens.
    import yaml, jsonschema
    case=req['exercise']['id']
    extra=profile(req)
    if case not in SUPPORTED and extra is None:
        raise Blocked('No qualified fixture/independent observer for '+case+'. Local bridge supports version and capability inventory; remote, preview-network, fault, and stateful fixtures remain unqualified.')
    m=manifest(req); image=image_for(m)
    probe=docker('run','--rm','--read-only','--cap-drop=ALL','--security-opt=no-new-privileges',
      '--user','65532:65532','--network','none','--entrypoint','sh',image,'-c',
      'test ! -r /opt/tadx && test ! -x /opt/tadx && test ! -w /usr/local/bin/tadx && test ! -e /var/run/docker.sock')
    additional={}
    if extra and hasattr(extra,'preflight'):
        additional=extra.preflight(req,extra.load_binding(req)) if hasattr(extra,'load_binding') else extra.preflight(req,{**m,'runtime_image':image})
    exercise=req.get('exercise') if isinstance(req,dict) else None
    evaluator=exercise.get('evaluator',{}) if isinstance(exercise,dict) else {}
    if 'action_write_request_audit' in (evaluator.get('requirements',[]) if isinstance(evaluator,dict) else []):
        if hasattr(extra,'load_binding') or additional.get('runtime_network_required')=='none':
            additional.setdefault('qualified_requirements',[]).append('action_write_request_audit')
            additional['request_audit_mechanism']='Captured HTTPS requests, or independently verified network-none boundary'
    evidence={'image_id':image,'probe_exit':probe.returncode,'model':({'mode':'deterministic_cli','model_calls':0} if req.get('deterministic') else runtime_check(image, req.get('runtime_model'), req.get('runtime_reasoning_effort'))),'profile':additional,
      'python':sys.executable,'pyyaml_version':yaml.__version__,
      'binary_sha256':m['binaries']['linux']['sha256'],'source_commit':m['source_commit'],
      'scope':'local diagnostics, no credentials, no mutation-setting changes',
      'audit_boundary':'root-only executable; non-root client; root-only broker audit volume'}
    p=Path(req['private_case_dir'])/'qualification.json';save(p,evidence)
    initial={'image':image,'manifest':m,'prepared':False,'container':None,'volume':None,
             'broker_container':None,'config_volume':None,'socket_volume':None}
    _checkpoint(req, initial, 'preflight', allocation_started=False, write_started=False)
    return {'qualified_requirements':sorted(QUALIFICATIONS|set(additional.get('qualified_requirements',[]))),'evidence':[rel(req,p)]}

def registry_rows(m):
    obj=load(Path(m['root'])/'registry.json')
    if isinstance(obj,list):return obj
    for key in ('capabilities','items'):
        if isinstance(obj.get(key),list):return obj[key]
    raise Blocked('Frozen registry shape unavailable')

def classify_record(row):
    ident=row.get('id')
    owner=row.get('owner'); status=row.get('implementation_state',row.get('implementation',row.get('state')))
    if not isinstance(ident,str) or not isinstance(owner,str) or not owner or not isinstance(status,str):
        raise ValueError('Inventory record needs id, owner, and implementation state')
    return {'id':ident,'executable':owner=='cli' and status=='implemented'}

def normalize_inventory(obj):
    if isinstance(obj,dict):
        for key in ('capabilities','items','records'):
            if isinstance(obj.get(key),list): obj=obj[key];break
    if not isinstance(obj,list): raise ValueError('Expected inventory record array')
    rows=[]
    for row in obj:
        if not isinstance(row,dict):raise ValueError('Inventory records must be objects')
        if type(row.get('executable')) is bool and isinstance(row.get('id'),str): rows.append({'id':row['id'],'executable':row['executable']})
        else: rows.append(classify_record(row))
    if len({r['id'] for r in rows})!=len(rows):raise ValueError('Duplicate inventory identities')
    return sorted(rows,key=lambda r:r['id'])

def fixture_mutation_policy(req, prepared, frozen, remote):
    explicit = prepared.get('mutation_policy')
    if explicit is not None or not remote:
        return explicit
    covered = req['exercise'].get('capability_coverage', [])
    operations = {row['capability'] for row in covered}
    if not any(row.get('id') in operations and row.get('remote_mutation') is True for row in registry_rows(frozen)):
        return None
    from integration.validation_profiles import _gate_authorized
    _gate_authorized(req)
    return 'disabled' if covered and all(row.get('variant') == 'preview' for row in covered) else 'enabled'


def _seed_site_mutation_consent(config, enabled):
    """Seed the current CLI's persisted site consent in disposable config only.

    Current TADX versions intentionally ignore the legacy process environment
    override for remote-write authorization.  Remote benchmark containers must
    therefore carry the same explicit site consent that an operator would see
    from ``tadx mutation status``.  The seed is local to the container config
    volume and never changes the operator settings or host configuration.
    """
    if not isinstance(config, dict) or not isinstance(config.get('environments'), dict):
        return config
    seeded = dict(config)
    entries = []
    for environment in config['environments'].values():
        if not isinstance(environment, dict):
            continue
        server = environment.get('url')
        site = environment.get('site_content_url')
        if isinstance(server, str) and server.strip() and isinstance(site, str) and site.strip():
            entries.append({'server_url': server, 'site_content_url': site, 'enabled': bool(enabled)})
    if entries:
        # Keep any explicitly configured consent for unrelated environments,
        # but replace the selected environments' entries with the exact
        # disposable setting required by this case.  The current CLI ignores
        # the legacy process override for authorization and resolves consent
        # by canonical server plus exact site content URL.
        selected = {(entry['server_url'].rstrip('/').lower(), entry['site_content_url']) for entry in entries}
        existing = []
        for setting in config.get('site_mutations', []):
            if not isinstance(setting, dict):
                continue
            key = (str(setting.get('server_url', '')).rstrip('/').lower(), setting.get('site_content_url'))
            if key not in selected:
                existing.append(dict(setting))
        seeded['site_mutations'] = existing + entries
    return seeded


def prepare(req):
    s=state(req);m=s['manifest']; root=Path(req['private_case_dir'])
    _checkpoint(req, s, 'prepare', allocation_started=False, write_started=False)
    work=root/'agent-public';work.mkdir(exist_ok=False)
    (work/'deliverables').mkdir()
    sentinel=work/'sentinel.txt';sentinel.write_bytes(b'protected fixture sentinel\n')
    extra=profile(req)
    if extra:
        # Save recovery ownership before preparation can create remote fixtures.
        s.update(work=str(work), profile_module=extra.__name__, remote_profile=hasattr(extra,'load_binding'))
        write_state(req,s)
        save(root/'profile-request.json',{'deployment':req.get('deployment',{}),'run_constraints':req.get('run_constraints',{}),'objective_policy':req.get('objective_policy',{}),'exercise':req['exercise'],'frozen_cli':m,
            'private_case_dir':str(root),'case_id':req['case_id'],'run_id':req['run_id']})
        prepared=extra.prepare(req,extra.load_binding(req),work=work) if hasattr(extra,'load_binding') else extra.prepare(req,m,work)
        # Profile setup is still before model delivery.  Reject missing public
        # bindings and malformed recovery identity here, with an exact pointer,
        # instead of discovering them after model allocation.
        require_admission(req['exercise'], prepared.get('fixture'),
                          private_root=root, config=req.get('runner_config'))
        if ('action_write_request_audit' in req['exercise']['evaluator'].get('requirements',[])
                and prepared.get('runtime_network')!='none'):
            if not hasattr(extra,'load_binding'):raise Blocked('Preview HTTP audit requires an exact configured origin')
            if not prepared.get('fault_plan'):prepared['fault_plan']={'rules':[]}
        prepared['mutation_policy'] = fixture_mutation_policy(req, prepared, m, hasattr(extra, 'load_binding'))
        # Remote fixtures must carry explicit selected-site consent in the
        # disposable CLI config for both positive and negative policy cases.
        # Offline validation fixtures intentionally keep their minimal seed and
        # are therefore excluded from this remote-only normalization.
        if (prepared['mutation_policy'] in ('enabled', 'disabled')
                and hasattr(extra, 'load_binding')):
            prepared['config_seed'] = _seed_site_mutation_consent(
                prepared.get('config_seed'), prepared['mutation_policy'] == 'enabled')
        if req.get('run_constraints', {}).get('native_cli_execution') is True:
            # Do not mutate the profile's fixture guard in place.  Validation
            # baselines deliberately freeze that reviewed command contract;
            # execution_mode is bridge metadata only.
            prepared['broker_guard'] = dict(prepared.get('broker_guard') or {})
            prepared['broker_guard']['execution_mode'] = 'disposable_native'
        s.update(prepared=True,work=str(work),fixture=prepared['fixture'],before=prepared['before'],
          config_seed=prepared.get('config_seed'), last_result_seed_file=prepared.get('last_result_seed_file'), catalog_seed_dir=prepared.get('catalog_seed_dir'),
          credential_env_names=prepared.get('credential_env_names',[]),
          remote_profile=hasattr(extra,'load_binding') and prepared.get('runtime_network')!='none', broker_guard=prepared.get('broker_guard'),
          runtime_network=prepared.get('runtime_network'), mutation_policy=prepared.get('mutation_policy'),
          fault_plan=prepared.get('fault_plan'), interactive_credentials=prepared.get('interactive_credentials'),
          additional_credential_bindings=prepared.get('additional_credential_bindings',[]),
          mutable_binary=prepared.get('mutable_binary',False),
          skip_native_auth_preflight=prepared.get('skip_native_auth_preflight',False),
          platform_condition=prepared.get('platform_condition'))
        write_state(req,s)
        _checkpoint(req, s, 'prepared', allocation_started=False,
                    write_started=False, remote_profile=bool(hasattr(extra, 'load_binding')))
        return prepared
    if req['exercise']['id']=='P-version-get':
        v=load(Path(m['root'])/'version.json')
        expected={'version':v['version'],'build_identity':m['source_commit'][:7]}
        source='version.json'
    else:
        expected=normalize_inventory(registry_rows(m));source='registry.json'
    public={'scope':'disposable local diagnostics','report_path':'deliverables/result.json'}
    public['report_contract'] = ({'format':'JSON object','fields':{'version':'installed version string',
      'build_identity':'short Git revision embedded in the installed build version'}} if req['exercise']['id']=='P-version-get'
      else {'format':'JSON object','fields':{'records':'array of all capability objects with id and executable boolean; include executable and external-only capabilities'}})
    fixture={'handle':req['case_id'],'semantic_digest':digest({'case':req['exercise']['id'],'expected':expected}),
      'public':public,'sites':{},'roles':{'installed_runtime':{'source_commit':m['source_commit']},'sentinel':{'path':'sentinel.txt'}},
      'expect':{'result':expected},'expected_bindings_provenance':{'/fixture/expect/result':{
        'phase':'setup','source_kind':'asset','evidence':['baseline-source.json']}},'coverage_assertions':[]}
    save(root/'baseline-source.json',{'source':source,'sha256':sha(Path(m['root'])/source),'result':expected})
    before={'protected':{'sentinel_sha256':sha(sentinel)},'unowned_changes':[]}
    s.update(prepared=True,work=str(work),fixture=fixture,before=before);write_state(req,s)
    _checkpoint(req, s, 'prepared', allocation_started=False, write_started=False)
    return {'fixture':fixture,'before':before,'baseline_evidence':['baseline-source.json'],
      'baseline_assertions':[{'id':'frozen-source-present','status':'pass','evidence':['baseline-source.json']}]}

def _split_labels(req, role):
    return {'tadx-bench': req['run_id'], 'tadx-bench-case': req['case_id'], 'tadx-bench-role': role}


def _label_args(labels):
    return [part for key, value in labels.items() for part in ('--label', key + '=' + value)]


def _policy_args(req, s):
    guard = s.get('broker_guard') or {}
    if guard.get('mode') == 'session':
        if s.get('mutation_policy') is not None:
            raise Blocked('Session site-consent fixture cannot use a generic mutation-policy override')
        legacy = guard.get('process_policy')
        if legacy not in (None, '0', '1'):
            raise Blocked('Session legacy process value is invalid')
        return ['--env', 'TADX_ENABLE_MUTATIONS=' + legacy] if legacy is not None else []
    policy = s.get('mutation_policy')
    if policy is None:
        return ['--env', 'TADX_ENABLE_MUTATIONS=' + os.environ['TADX_ENABLE_MUTATIONS']] if 'TADX_ENABLE_MUTATIONS' in os.environ else []
    constraints = req.get('run_constraints', {})
    if (policy not in ('enabled', 'disabled') or constraints.get('mutation_setting_changes_authorized') is not True
            or constraints.get('mutation_setting_change_scope') != 'disposable_test_containers'):
        raise Blocked('A fixture mutation-policy override requires explicit disposable-container authorization')
    # The installed TADX build deliberately ignores this legacy process
    # variable for remote-write authorization.  Enabled cases receive their
    # explicit site consent through config_seed.site_mutations instead.  Keep
    # the override only for disabled/refusal fixtures, whose disposable
    # broker needs an unambiguous process-level gate to enforce the negative
    # condition without changing host or persistent settings.
    return ['--env', 'TADX_ENABLE_MUTATIONS=0'] if policy == 'disabled' else []


def _policy_evidence(req, s, details):
    values = [value.partition('=')[2] for value in details.get('Config', {}).get('Env', [])
              if isinstance(value, str) and value.startswith('TADX_ENABLE_MUTATIONS=')]
    if len(values) > 1:
        raise Blocked('CLI container exposes ambiguous mutation-policy environment values')
    raw = values[0] if values else None
    guard = s.get('broker_guard') or {}
    if guard.get('mode') == 'session':
        if raw != guard.get('process_policy') or s.get('mutation_policy') is not None:
            raise Blocked('Session legacy value or generic policy differs from prepared guard')
        config = s.get('config_seed')
        if not isinstance(config, dict):
            raise Blocked('Session disposable site-consent config is missing')
        environments = config.get('environments', {})
        if not isinstance(environments, dict) or len(environments) != 2:
            raise Blocked('Session requires the selected and unrelated environment sentinels')
        target = environments.get(guard.get('environment'))
        unrelated = environments.get(guard.get('unrelated_environment'))
        if not isinstance(target, dict) or target.get('url') != guard.get('server_url') or target.get('site_content_url') != guard.get('site_content_url'):
            raise Blocked('Session selected environment differs from exact target')
        if (not isinstance(unrelated, dict) or unrelated.get('url') != guard.get('server_url')
            or unrelated.get('site_content_url') != guard.get('unrelated_site_content_url')):
            raise Blocked('Session unrelated environment sentinel differs')
        settings = config.get('site_mutations', [])
        initial = guard.get('initial_site_consent')
        if initial is None and settings:
            raise Blocked('Session authentication fixture unexpectedly has site consent')
        if initial is not None and (not isinstance(settings, list) or len(settings) != 2
            or settings[0] != {'server_url': guard.get('server_url'), 'site_content_url': guard.get('site_content_url'), 'enabled': initial}
            or settings[1] != {'server_url': guard.get('server_url'), 'site_content_url': guard.get('unrelated_site_content_url'), 'enabled': True}):
            raise Blocked('Session prepared consent differs from exact site')
        return {'effective': initial, 'source': 'saved_site_setting' if initial is not None else 'not_applicable',
                'scope': 'disposable_exact_site', 'legacy_process_value': raw,
                'site_consent_entries': len(settings), 'container_id': details['Id'],
                'evidence': ['delivery-evidence.json#/cli_mutation_policy']}
    override = s.get('mutation_policy')
    if override == 'disabled' and raw != '0':
        raise Blocked('CLI container did not receive the authorized mutation-policy override')
    config = s.get('config_seed') if isinstance(s.get('config_seed'), dict) else {}
    configured = config.get('site_mutations', []) if isinstance(config.get('site_mutations', []), list) else []
    environments = config.get('environments', {}) if isinstance(config.get('environments', {}), dict) else {}
    selected_sites = []
    for environment in environments.values():
        if not isinstance(environment, dict):
            continue
        server = str(environment.get('url', '')).rstrip('/').lower()
        site = environment.get('site_content_url')
        selected_sites.extend(entry for entry in configured
                              if isinstance(entry, dict)
                              and str(entry.get('server_url', '')).rstrip('/').lower() == server
                              and entry.get('site_content_url') == site)
    enabled_sites = [entry for entry in selected_sites if entry.get('enabled') is True]
    consent = bool(selected_sites) and len(enabled_sites) == len(selected_sites)
    if override == 'enabled' and not consent:
        raise Blocked('Enabled mutation fixture lacks explicit site_mutations consent in the container config')
    effective = False if override == 'disabled' else consent if override == 'enabled' else (True if raw == '1' else False if raw == '0' else None)
    return {'effective': effective, 'source': 'test_container_site_consent' if override == 'enabled' else 'test_container_override' if override == 'disabled' else 'inherited_process_environment',
            'scope': 'disposable_test_containers' if override else 'inherited',
            'site_consent_entries': len(enabled_sites),
            'container_id': details['Id'], 'evidence': ['delivery-evidence.json#/cli_mutation_policy']}


def _inspect_boundary_resource(kind, name):
    """A failed inspect is not absence unless the daemon answers an exact listing."""
    def output(response):
        value = getattr(response, 'stdout', None)
        if isinstance(value, bytes):
            value = value.decode('utf-8', 'replace')
        if value is None:
            return None
        if not isinstance(value, str):
            raise Blocked('Docker returned a non-text '+kind+' inspection: '+name)
        return value

    # Keep the container form compatible with both Docker and the lightweight
    # local daemon fakes.  The subsequent Name, ID, and ownership-label checks
    # still reject a resource of the wrong type or owner.
    args=('inspect',name) if kind=='container' else ('volume','inspect',name)
    response=docker(*args,check=False)
    if response.returncode:
        args=('ps','-aq','--no-trunc','--filter','name=^/'+re.escape(name)+'$') if kind=='container' else (
            'volume','ls','--quiet','--filter','name=^'+re.escape(name)+'$')
        listing=docker(*args,check=False)
        listing_output=output(listing)
        if listing.returncode or listing_output is None or listing_output.strip():
            raise Blocked('Cannot establish exact '+kind+' identity or absence: '+name)
        return None
    try:
        response_output=output(response)
        if response_output is None:
            raise ValueError()
        rows=json.loads(response_output)
        if not isinstance(rows,list) or len(rows)!=1 or not isinstance(rows[0],dict):raise ValueError()
        return rows[0]
    except (ValueError,TypeError):raise Blocked('Docker returned no exact '+kind+' inspection: '+name) from None


def _resume_removed_boundary(req,s,key):
    """Resume only a previously frozen, exact-owned removal with retained audit."""
    intent=s.get(key+'_removal_intent')
    if not s.get(key+'_created') and not intent:
        # Allocation may have stopped after earlier volumes and before this create.
        if s.get(key+'_id') or key in s.get('owned_volumes',{}):
            raise Blocked('Creation checkpoint is inconsistent for '+str(s[key]))
        s[key+'_not_allocated_confirmed']=True;write_state(req,s)
        return
    if not s.get('frozen') or not isinstance(intent,dict) or intent.get('name')!=s[key] or intent.get('quiescent') is not True:
        raise Blocked('Recorded resource disappeared without a frozen removal intent: '+s[key])
    expected=s.get(key+'_id') if key.endswith('container') else s.get('owned_volumes',{}).get(key)
    if intent.get('identity')!=expected or not isinstance(intent.get('retained_files'),dict) or not intent.get('audit_capture'):
        raise Blocked('Exact removal identity or retained-audit checkpoint is missing: '+s[key])
    root=Path(req['private_case_dir']).resolve()
    for relative,expected_sha in intent['retained_files'].items():
        path=(root/relative).resolve()
        if root not in path.parents or not path.is_file() or path.is_symlink() or sha(path)!=expected_sha:
            raise Blocked('Retained boundary evidence changed before removal reconciliation: '+relative)
    s[key+'_removed']=True
    s[key+'_removal_verification']={'status':'confirmed_absent','source':'successful_exact_daemon_inventory'}
    write_state(req,s)


def _split_container(req, s, prefix):
    # Also used by the single-container boundary so both paths share exact checks.
    name=s.get(prefix)
    if not name:return None
    details=_inspect_boundary_resource('container',name)
    if details is None:
        _resume_removed_boundary(req,s,prefix)
        return None
    labels=s.get('resource_labels',{}).get(prefix) or (_split_labels(req,'worker' if prefix=='container' else 'cli')
        if s.get('split_broker') else {'tadx-bench':req['run_id']})
    if ((details.get('Name') is not None and details.get('Name')!='/'+name)
            or s.get(prefix+'_id') and details.get('Id')!=s[prefix+'_id']
            or any((details.get('Config',{}).get('Labels') or {}).get(key)!=value for key,value in labels.items())):
        raise Blocked('Container identity or ownership changed; resource retained: '+name)
    if s.get(prefix+'_removed'):
        raise Blocked('Removed container name was reallocated; new resource retained: '+name)
    if not s.get(prefix+'_created') or not s.get(prefix+'_id'):
        # A deterministic name and matching labels do not prove that an
        # existing boundary was allocated by this case.  Split and single
        # boundaries both need either a creation acknowledgement or an
        # explicit intent before adopting an inspected resource.
        if not s.get(prefix+'_created') and not s.get(prefix+'_creation_intent'):
            raise Blocked('Unacknowledged container has no creation intent: '+name)
        s[prefix+'_created']=True;s[prefix+'_id']=details['Id'];write_state(req,s)
    return details


def _boundary_volume(req,s,key):
    details=_inspect_boundary_resource('volume',s[key])
    if details is None:
        _resume_removed_boundary(req,s,key)
        return None
    labels=s.get('resource_labels',{}).get(key) or (_split_labels(req,key) if s.get('split_broker') else {'tadx-bench':req['run_id']})
    actual={field:details.get(field) for field in ('Name','Labels','CreatedAt')}
    expected=s.get('owned_volumes',{}).get(key)
    if (details.get('Name')!=s[key] or any((details.get('Labels') or {}).get(k)!=v for k,v in labels.items())
            or expected is not None and actual!=expected or s.get(key+'_removed')):
        raise Blocked('Volume identity or ownership changed; resource retained: '+s[key])
    if expected is None:
        if not s.get(key+'_created') and not s.get(key+'_creation_intent'):
            raise Blocked('Volume creation identity was never captured: '+s[key])
        s.setdefault('owned_volumes',{})[key]=actual;s[key+'_created']=True;write_state(req,s)
    return details


def _remove_boundary_resource(req,s,key,*,legacy_cleanup=False):
    # A few callers retain pre-checkpoint state snapshots from the original
    # bridge.  They have only the deterministic resource names, so there is
    # no inspection payload to decode or identity to adopt.  Keep their
    # already-frozen cleanup resumable.  New bridge state always has a
    # lifecycle checkpoint or an ownership identity and takes the strict path
    # below.
    if legacy_cleanup and s.get('frozen') is True:
        kind='container' if key.endswith('container') else 'volume'
        args=('rm',s[key]) if kind=='container' else ('volume','rm',s[key])
        response=docker(*args,check=False)
        if response.returncode:
            raise Blocked('Legacy '+kind+' cleanup was not acknowledged: '+s[key])
        s[key+'_removed']=True
        write_state(req,s)
        return {kind:s[key],'status':'acknowledged','source':'legacy_frozen_state'}
    kind='container' if key.endswith('container') else 'volume'
    inspect=lambda: _split_container(req,s,key) if kind=='container' else _boundary_volume(req,s,key)
    details=inspect()
    if details is None:
        not_allocated=(not s.get(key+'_created') and not s.get(key+'_removal_intent')
                       and s.get(key+'_not_allocated_confirmed') is True)
        s[key+'_removed']=True;write_state(req,s)
        return {kind:s[key],'status':'not_allocated' if not_allocated else 'confirmed_absent',
                'no_allocation':not_allocated,
                'source':'successful_exact_daemon_inventory'}
    if not s.get('frozen') or kind=='container' and (details['State'].get('Running') or details['State'].get('Pid',0)):
        raise Blocked('Resource removal requires confirmed stopped boundaries: '+s[key])
    if not s.get('audit_capture'):
        raise Blocked('Resource removal requires a retained-audit checkpoint: '+s[key])
    files={};root=Path(req['private_case_dir'])
    for directory in ('audit','cli-state'):
        for path in (root/directory).rglob('*'):
            if path.is_symlink():raise Blocked('Boundary evidence contains an unexpected symlink: '+str(path))
            if path.is_file():files[path.relative_to(root).as_posix()]=sha(path)
    identity=details['Id'] if kind=='container' else s['owned_volumes'][key]
    s[key+'_removal_intent']={'name':s[key],'identity':identity,'quiescent':True,
        'retained_files':files,'audit_capture':s['audit_capture']}
    write_state(req,s)
    response=None;failure=None
    try:
        # The exact immutable ID is the production removal target.  Lightweight
        # boundary doubles may omit Name from inspect, so use the already
        # checked deterministic name only for that compatibility path.
        removal_identity=(s[key] if kind=='container' and details.get('Name') is None else identity)
        args=('rm',removal_identity) if kind=='container' else ('volume','rm',s[key])
        response=docker(*args,check=False)
    except (OSError,subprocess.TimeoutExpired) as exc:failure=type(exc).__name__
    if inspect() is not None:
        raise Blocked('Exact '+kind+' removal remains unresolved: '+s[key]+('; '+failure if failure else ''))
    return {kind:s[key],'identity':identity,'removal_command_exit_status':response.returncode if response else None,
        'removal_transport_error':failure,'status':'confirmed_absent','source':'successful_exact_daemon_inventory'}


def _deliver_catalog(state):
    container = state['broker_container']
    docker('exec', container, 'mkdir', '-p', '/cli-state/tadx/catalog')
    source = state.get('catalog_seed_dir')
    expected = {path.name: sha(path) if path.is_file() else 'directory' for path in Path(source).glob('*.sqlite')} if source else {}
    if source:
        docker('cp', str(Path(source)) + '/.', container + ':/cli-state/tadx/catalog/')
    script = "const fs=require('fs'),c=require('crypto'),dir='/cli-state/tadx/catalog';process.stdout.write(JSON.stringify(Object.fromEntries(fs.readdirSync(dir).filter(n=>n.endsWith('.sqlite')).map(n=>[n,fs.statSync(dir+'/'+n).isDirectory()?'directory':c.createHash('sha256').update(fs.readFileSync(dir+'/'+n)).digest('hex')]))));"
    actual = json.loads(docker('exec', container, 'node', '-e', script).stdout)
    if actual != expected:
        raise Blocked('Delivered catalog differs from its native fixture files')
    return {'status': 'verified', 'files': actual}


def _deliver_last_receipt(state):
    destination = state['broker_container'] + ':/cli-state/tadx/last-result.json'
    source = state.get('last_result_seed_file')
    if source is None:
        if docker('exec', state['broker_container'], 'test', '!', '-e', '/cli-state/tadx/last-result.json', check=False).returncode:
            raise Blocked('Empty last-result fixture unexpectedly contains a receipt')
        return {'status': 'verified', 'available': False}
    docker('cp', str(source), destination)
    actual = docker('exec', state['broker_container'], 'sha256sum', '/cli-state/tadx/last-result.json').stdout.split()[0]
    if actual != sha(source):
        raise Blocked('Delivered last-result receipt differs from the native setup result')
    return {'status': 'verified', 'available': True, 'sha256': actual}


def _observe_candidate_release(req, s, filename):
    observed=docker('exec','--env','BENCH_NETWORK_MODE=none','--user','0:0',
        s['broker_container'],'node','/cli-state/release-observer.cjs',check=False)
    if observed.returncode:
        raise Blocked('Native candidate update observation failed: '+filename)
    try:
        document=json.loads(observed.stdout)
    except ValueError as error:
        raise Blocked('Native candidate update observation is not JSON: '+filename) from error
    save(Path(req['private_case_dir'])/filename, document)
    return document


def _deliver_candidate_release(req, s):
    """Install only verified candidate assets in the private offline CLI state."""
    from integration.release_update_profile import candidate_release,PROTECTED_GUIDANCE_PATH,PROTECTED_GUIDANCE_BYTES,PROTECTED_GUIDANCE_SHA256
    from integration.release_update_assets import DEFAULT_ROOT
    target=candidate_release()
    guard=s['broker_guard']
    if s.get('runtime_network')!='none' or guard.get('manifest_sha256')!=target['manifest_sha256'] or \
            guard.get('source_commit')!=target['source_commit'] or guard.get('target_version')!=target['version'] or \
            guard.get('baseline_binary_sha256')!=target['baseline_binary_sha256'] or \
            guard.get('target_binary_sha256')!=target['binary_sha256']:
        raise Blocked('Candidate release guard or offline runtime differs from verified closure')
    broker=s['broker_container']
    docker('cp', str(DEFAULT_ROOT), broker+':/cli-state/release-assets')
    docker('exec','--user','0:0',broker,'mkdir','-p','/cli-state/release-transport','/cli-state/update')
    docker('exec','--user','0:0',broker,'cp','-a','/cli-state/release-assets/transport/.','/cli-state/release-transport/')
    docker('exec','--user','0:0',broker,'chmod','0755','/cli-state/release-transport/gh','/cli-state/release-transport/curl')
    docker('exec','--user','0:0',broker,'cp','/cli-state/release-assets/baseline-tadx','/cli-state/update/tadx')
    docker('exec','--user','0:0',broker,'chmod','0755','/cli-state/update/tadx')
    protected='/tmp/cli-home/.codex/skills/'+PROTECTED_GUIDANCE_PATH
    seed="const fs=require('fs'),p=process.argv[1];fs.mkdirSync(require('path').dirname(p),{recursive:true});fs.writeFileSync(p,Buffer.from(process.argv[2],'hex'),{flag:'wx',mode:0o600})"
    docker('exec','--user','0:0',broker,'node','-e',seed,protected,PROTECTED_GUIDANCE_BYTES.hex())
    observer=Path(__file__).with_name('release_update_observer.cjs')
    docker('cp',str(observer),broker+':/cli-state/release-observer.cjs')
    verify="const fs=require('fs'),c=require('crypto'),p='/cli-state/release-assets/',m=JSON.parse(fs.readFileSync(p+'manifest.json'));const h=x=>c.createHash('sha256').update(fs.readFileSync(x)).digest('hex');for(const [f,d] of Object.entries(m.files)){if(h(p+f)!==d)process.exit(2)}if(h(p+'manifest.json')!==process.argv[1]||h('/cli-state/update/tadx')!==m.baseline_binary_sha256)process.exit(3)"
    checked=docker('exec','--user','0:0',broker,'node','-e',verify,target['manifest_sha256'],check=False)
    if checked.returncode:
        raise Blocked('Copied candidate release closure differs before task launch')
    before=_observe_candidate_release(req,s,'release-before-native.json')
    if before.get('binary_sha256')!=target['baseline_binary_sha256'] or \
            before.get('version')!=target['starting_version'] or before.get('exit_status')!=0 or \
            before.get('guidance')!={PROTECTED_GUIDANCE_PATH:PROTECTED_GUIDANCE_SHA256}:
        raise Blocked('Native starting executable or Guidance differs from candidate baseline')
    return before


def _deliver_split(req, s, work, skills, actual, name):
    """Keep account access in the worker and only CLI execution offline."""
    if s.get('remote_profile'):
        raise Blocked('Offline validation brokers cannot receive remote credentials')
    if not isinstance(s.get('broker_guard'), dict) or (s['broker_guard'].get('mode') not in ('validation', 'local-state', 'workspace', 'session', 'last', 'catalog', 'guidance', 'diagnostics') and not s['broker_guard'].get('mode','').startswith('shared-')):
        raise Blocked('Offline broker requires an implemented local guard before resource allocation')
    inherited = _policy_args(req, s)
    resources = {'container': name, 'broker_container': name + '-cli', 'volume': name + '-audit',
                 'config_volume': name + '-config', 'socket_volume': name + '-socket'}
    for key, value in resources.items():
        args = ('inspect', value) if key.endswith('container') else ('volume', 'inspect', value)
        if docker(*args, check=False).returncode == 0:
            raise Blocked('Split boundary resource already exists; refusing to adopt it')
    s.update(resources, split_broker=True, owned_volumes={}, delivery_complete=False)
    _checkpoint(req, s, 'allocation_started', resource_names=sorted(resources))
    for key in resources:
        s[key + '_created'] = False
    write_state(req, s)
    for key in ('volume', 'config_volume', 'socket_volume'):
        labels = _split_labels(req, key)
        docker('volume', 'create', *_label_args(labels), s[key])
        detail = json.loads(docker('volume', 'inspect', s[key]).stdout)[0]
        if detail.get('Name') != s[key] or any(detail.get('Labels', {}).get(k) != v for k, v in labels.items()):
            raise Blocked('New split volume ownership was not confirmed')
        s[key + '_created'] = True
        s['owned_volumes'][key] = {field: detail.get(field) for field in ('Name', 'Labels', 'CreatedAt')}
        write_state(req, s)
    common = ['--read-only', '--cap-drop=ALL', '--security-opt=no-new-privileges',
              '--pids-limit', '256', '--memory', '2g', '--tmpfs', '/tmp:rw,nosuid,nodev,mode=1777',
              '--mount', 'type=bind,source=' + str(work) + ',target=/work',
              '--mount', 'type=bind,source=' + str(skills) + ',target=/skills,readonly',
              '--mount', 'type=volume,source=' + s['socket_volume'] + ',target=/run/tadx-broker',
              '--env', 'TADX_FEEDBACK_MODE=off']
    broker = docker('create', '--name', s['broker_container'], *_label_args(_split_labels(req, 'cli')), *common,
        '--network', 'none', '--env', 'BENCH_NETWORK_MODE=none', '--env', 'BENCH_BROKER_TRANSPORT=unix',
        '--mount', 'type=volume,source=' + s['volume'] + ',target=/audit',
        '--mount', 'type=volume,source=' + s['config_volume'] + ',target=/cli-state', *inherited, s['image'])
    s.update(broker_container_created=True, broker_container_id=broker.stdout.strip()); write_state(req, s)
    worker = docker('create', '--name', name, *_label_args(_split_labels(req, 'worker')), *common,
        '--tmpfs', '/home/worker:rw,nosuid,nodev,uid=65532,gid=65532',
        '--add-host', 'host.docker.internal:host-gateway', '--entrypoint', 'node',
        *inherited, s['image'], '-e', 'setInterval(()=>{},2147483647)')
    s.update(container_created=True, container_id=worker.stdout.strip()); write_state(req, s)
    # Both immutable container IDs are journaled before either process starts.
    _split_container(req, s, 'broker_container'); _split_container(req, s, 'container')
    _checkpoint(req, s, 'allocated', resource_ids={key: s.get(key + '_id') for key in ('broker_container', 'container')})
    docker('start', s['broker_container']); s['broker_started'] = True; write_state(req, s)
    for _ in range(READY_POLL_ATTEMPTS):
        if docker('exec', s['broker_container'], 'test', '-f', '/audit/ready.json', check=False).returncode == 0:
            break
        time.sleep(READY_POLL_DELAY_S)
    else:
        raise Blocked('Offline CLI broker did not become ready')
    if s.get('config_seed') is not None:
        seed = Path(req['private_case_dir']) / 'config-seed.json'; save(seed, s['config_seed'])
        docker('cp', str(seed), s['broker_container'] + ':/cli-state/tadx/config.yaml')
        observed = json.loads(docker('exec', s['broker_container'], 'cat', '/cli-state/tadx/config.yaml').stdout)
        if observed != s['config_seed']:
            raise Blocked('Offline broker configuration differs from the independent seed')
        docker('cp', str(seed), s['broker_container'] + ':/cli-state/target-baseline.json')
        baseline = json.loads(docker('exec', s['broker_container'], 'cat', '/cli-state/target-baseline.json').stdout)
        if baseline != s['config_seed']:
            raise Blocked('Offline broker preservation baseline differs from the independent seed')
    guard = Path(req['private_case_dir']) / 'broker-task-policy.json'; save(guard, s['broker_guard'])
    docker('cp', str(guard), s['broker_container'] + ':/cli-state/task-policy.json')
    observed_guard = json.loads(docker('exec', s['broker_container'], 'cat', '/cli-state/task-policy.json').stdout)
    if observed_guard != s['broker_guard']:
        raise Blocked('Offline broker guard differs from the declared fixture contract')
    release_before = _deliver_candidate_release(req, s) if s['broker_guard'].get('mode') == 'shared-release' else None
    guidance_home = None
    if s['broker_guard'].get('mode') == 'guidance':
        guidance_home = docker('exec', s['broker_container'], 'node', '-e',
            "const b=require('/opt/local_broker.cjs');const g=require('/opt/guidance_broker.cjs');process.stdout.write(g.home(b.diskState())||'');").stdout.strip()
        if guidance_home != '/work/guidance-home' or guidance_home != s['broker_guard'].get('home'):
            raise Blocked('Guidance CLI home override did not pass the trusted guard')
    workspace_modes = None
    if s['broker_guard'].get('mode') == 'workspace':
        workspace_modes = json.loads(docker('exec', s['broker_container'], 'node', '-e',
            "const b=require('/opt/local_broker.cjs');const w=require('/opt/workspace_broker.cjs');process.stdout.write(JSON.stringify(w.prepareFileModes(b.diskState())));").stdout)
        if workspace_modes.get('status') != 'verified':
            raise Blocked('Workspace copies did not establish verified readable Linux file modes')
    session_runtime = None
    if s['broker_guard'].get('mode') == 'session':
        session_runtime = json.loads(docker('exec', s['broker_container'], 'node', '-e',
            "const b=require('/opt/local_broker.cjs');const s=require('/opt/session_broker.cjs');process.stdout.write(JSON.stringify(s.runtimeEvidence(b.diskState())));").stdout)
        if session_runtime.get('status') != 'verified':
            raise Blocked('Session fixture credential absence and actual mutation policy were not verified')
    last_receipt = _deliver_last_receipt(s) if s['broker_guard'].get('mode') == 'last' else None
    catalog_seed = _deliver_catalog(s) if s.get('catalog_seed_dir') or s['broker_guard'].get('mode') == 'catalog' else None
    docker('start', name)
    runtime = ({'status':'not_needed','mode':'deterministic_cli'} if req.get('deterministic') else _prepare_runtime(name, req))
    inaccessible = docker('exec', '--user', '65532:65532', name, 'sh', '-c',
        'test ! -r /opt/tadx && test ! -x /opt/tadx && test ! -r /audit/commands.jsonl '
        '&& test ! -e /cli-state/tadx/config.yaml && test ! -w /run/tadx-broker '
        '&& test -S /run/tadx-broker/invoke.sock && test ! -w /skills && test ! -e /var/run/docker.sock', check=False)
    if inaccessible.returncode:
        raise Blocked('Split worker private-resource or socket ownership probe failed')
    worker_info = _split_container(req, s, 'container')
    cli_info = _split_container(req, s, 'broker_container')
    if cli_info['HostConfig']['NetworkMode'] != 'none' or worker_info['HostConfig']['NetworkMode'] in ('none', 'host'):
        raise Blocked('Split model and CLI network boundaries do not match their declared roles')
    path = Path(req['private_case_dir']) / 'delivery-evidence.json'
    save(path, {'image_id': worker_info['Image'], 'mounts': worker_info['Mounts'], 'host_config': worker_info['HostConfig'],
        'cli_container_id': cli_info['Id'], 'cli_host_config': cli_info['HostConfig'], 'cli_mounts': cli_info['Mounts'],
        'cli_mutation_policy': _policy_evidence(req, s, cli_info), 'cli_guidance_home': guidance_home,
        'workspace_native_file_modes': workspace_modes,
        'cli_session_runtime': session_runtime, 'last_receipt_seed': last_receipt, 'catalog_seed': catalog_seed,
        'release_before_native': release_before,
        'cli_guidance_notice': '0' if guidance_home else None,
        'agent_user': '65532:65532', 'access_probe_exit': inaccessible.returncode, 'skill_hashes': actual,
        'model_runtime': runtime, 'transport': 'fixed_unix_socket', 'socket_directory_mode': '0755', 'socket_mode': '0666'})
    s['delivery_complete'] = True; write_state(req, s)
    _checkpoint(req, s, 'delivery_complete', allocation_started=True, write_started=False)
    return {'isolated': True, 'evaluator_inaccessible': True, 'host_launch_cwd': str(work),
        'agent_environment': {'TADX_FEEDBACK_MODE': 'off'}, 'skill_hashes': actual,
        'skill_evidence': [rel(req, path)], 'feedback_mode': 'off', 'evidence': [rel(req, path)]}


def deliver(req):
    s=state(req);work=Path(s['work']);m=s['manifest']
    _checkpoint(req, s, 'allocation_started', resource_names=['tadx-local-' + hashlib.sha256((req['run_id']+req['case_id']).encode()).hexdigest()[:20]])
    save(work/'inputs.json',req['inputs'])
    # The two exact official trees are mounted read-only, with no feedback skill.
    skills=Path(m['root'])/'internal/agent/skills'
    if {p.name for p in skills.iterdir() if p.is_dir()}!={'tadx','tadx-pulse'}:raise Blocked('Frozen skill directory includes unexpected packages')
    actual={name:{p.relative_to(skills/name).as_posix():sha(p) for p in (skills/name).rglob('*') if p.is_file()} for name in ('tadx','tadx-pulse')}
    if actual!=m['skills']:raise Blocked('Frozen official skill trees changed')
    name='tadx-local-'+hashlib.sha256((req['run_id']+req['case_id']).encode()).hexdigest()[:20]
    if s.get('runtime_network') == 'none':
        return _deliver_split(req, s, work, skills, actual, name)
    inherited_policy=_policy_args(req,s)
    volume=name+'-audit';config_volume=name+'-config'
    if (_inspect_boundary_resource('container',name) is not None or _inspect_boundary_resource('volume',volume) is not None
            or _inspect_boundary_resource('volume',config_volume) is not None):
        raise Blocked('Run-scoped boundary name already exists; refusing to adopt or clean it up')
    labels={key:_split_labels(req,'worker' if key=='container' else key) for key in ('container','volume','config_volume')}
    s.update(container=name,volume=volume,config_volume=config_volume,container_created=False,volume_created=False,
        config_volume_created=False,owned_volumes={},resource_labels=labels,delivery_complete=False)
    write_state(req,s)
    for key in ('volume','config_volume'):
        s[key+'_creation_intent']={'name':s[key],'labels':labels[key]};write_state(req,s)
        docker('volume','create',*_label_args(labels[key]),s[key])
        _boundary_volume(req,s,key)
    if s.get('runtime_network') not in (None, 'none'):
        raise Blocked('Unsupported fixture network boundary')
    network_args=['--network','none','--env','BENCH_NETWORK_MODE=none'] if s.get('runtime_network')=='none' else []
    s['container_creation_intent']={'name':name,'labels':labels['container']};write_state(req,s)
    created=docker('create','--name',name,*_label_args(labels['container']),'--read-only','--cap-drop=ALL',
      '--security-opt=no-new-privileges','--pids-limit','256','--memory','2g',
      '--tmpfs','/tmp:rw,nosuid,nodev,mode=1777','--tmpfs','/home/worker:rw,nosuid,nodev,uid=65532,gid=65532',
      '--mount','type=bind,source='+str(work)+',target=/work',
      '--mount','type=bind,source='+str(skills)+',target=/skills,readonly',
      '--mount','type=volume,source='+volume+',target=/audit',
      '--mount','type=volume,source='+config_volume+',target=/cli-state',
      '--add-host','host.docker.internal:host-gateway','--env','TADX_FEEDBACK_MODE=off',*inherited_policy,*network_args,s['image'])
    s.update(container_created=True,container_id=created.stdout.strip());write_state(req,s)
    _checkpoint(req, s, 'allocated', resource_ids={'container': s.get('container_id')})
    _split_container(req,s,'container')
    docker('start',name)
    for _ in range(READY_POLL_ATTEMPTS):
        if docker('exec',name,'test','-f','/audit/ready.json',check=False).returncode==0:break
        time.sleep(READY_POLL_DELAY_S)
    else:raise Blocked('CLI audit broker did not start')
    if s.get('config_seed') is not None:
        seed=Path(req['private_case_dir'])/'config-seed.json';save(seed,s['config_seed'])
        docker('cp',str(seed),name+':/cli-state/tadx/config.yaml')
        observed=json.loads(docker('exec',name,'cat','/cli-state/tadx/config.yaml').stdout)
        if observed!=s['config_seed']:raise Blocked('Container config seed readback differs from independent baseline')
    if s.get('remote_profile'):
        original=load(Path(req['private_case_dir'])/'profile-request.json')
        binding=profile(original).load_binding(original)
        credentials={'TADX_BENCH_AGENT_PAT_NAME':binding['agent']['pat_name'],'TADX_BENCH_AGENT_PAT_SECRET':binding['agent']['pat_secret']}
        for extra_credential in s.get('additional_credential_bindings',[]):
            selected=binding[extra_credential['binding_key']]['agent']
            for field in ('pat_name','pat_secret'):
                credentials[extra_credential[field]]=selected[field]
        credential_transfer=subprocess.run(['docker','exec','-i','--user','0:0',name,'node','-e',
            "const fs=require('fs');fs.writeFileSync('/cli-state/credentials.json',fs.readFileSync(0),{mode:0o600});"],
            input=json.dumps(credentials).encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=30)
        if credential_transfer.returncode:
            raise Blocked('Private runtime credential transfer did not complete')
        target=Path(req['private_case_dir'])/'broker-targets.json';save(target,s['config_seed'])
        docker('cp',str(target),name+':/cli-state/target-baseline.json')
        # Root-owned setup probe uses the frozen Go binary and system CA bundle.
        # It is not task-phase coverage and never exposes credential values.
        probe_js="""const fs=require('fs'),cp=require('child_process');
const config=JSON.parse(fs.readFileSync('/cli-state/tadx/config.yaml','utf8'));
const env={PATH:process.env.PATH,HOME:'/tmp/cli-home',XDG_CONFIG_HOME:'/cli-state',XDG_DATA_HOME:'/cli-state/data',TADX_FEEDBACK_MODE:'off',...JSON.parse(fs.readFileSync('/cli-state/credentials.json','utf8'))};
if('TADX_ENABLE_MUTATIONS' in process.env)env.TADX_ENABLE_MUTATIONS=process.env.TADX_ENABLE_MUTATIONS;
const r=cp.spawnSync('/opt/tadx',['auth','check','--environment',config.default_environment,'--json'],{env,encoding:'utf8',timeout:30000});
process.stdout.write(r.stdout||'');process.stderr.write(r.stderr||'');process.exit(r.status===null?124:r.status);"""
        if not s.get('skip_native_auth_preflight'):
            probe=docker('exec','--user','0:0',name,'node','-e',probe_js,timeout=40,check=False)
            save(Path(req['private_case_dir'])/'remote-cli-preflight.json',{'phase':'setup','exit_status':probe.returncode,
              'stdout':probe.stdout,'stderr':probe.stderr,'binary_sha256':m['binaries']['linux']['sha256']})
            if probe.returncode and (s.get('broker_guard') or {}).get('auth_condition')!='bad-pat':
                raise Blocked('Frozen CLI remote authentication/TLS preflight failed; see remote-cli-preflight.json')
    if s.get('catalog_seed_dir'):
        _deliver_catalog({**s,'broker_container':name})
    if s.get('fault_plan'):
        _deliver_faults(req,s,name)
    if s.get('mutable_binary'):
        docker('exec','--user','0:0',name,'mkdir','-p','/cli-state/update')
        docker('exec','--user','0:0',name,'cp','/opt/tadx','/cli-state/update/tadx')
    if s.get('interactive_credentials'):
        _deliver_credentials(req,s,name)
    if s.get('broker_guard'):
        guard=Path(req['private_case_dir'])/'broker-task-policy.json';save(guard,s['broker_guard'])
        docker('cp',str(guard),name+':/cli-state/task-policy.json')
    runtime=({'status':'not_needed','mode':'deterministic_cli'} if req.get('deterministic') else _prepare_runtime(name, req))
    inaccessible=docker('exec','--user','65532:65532',name,'sh','-c','test ! -r /opt/tadx && test ! -x /opt/tadx && test ! -r /audit/commands.jsonl && test ! -w /skills && test ! -e /var/run/docker.sock',check=False)
    if inaccessible.returncode:raise Blocked('Agent boundary access probe failed')
    inspect=json.loads(docker('inspect',name).stdout)[0]
    if s.get('runtime_network')=='none' and inspect['HostConfig']['NetworkMode']!='none':
        raise Blocked('Fixture requires an independently confirmed network-disabled container')
    p=Path(req['private_case_dir'])/'delivery-evidence.json'
    save(p,{'image_id':inspect['Image'],'mounts':inspect['Mounts'],'host_config':inspect['HostConfig'],
      'cli_host_config':inspect['HostConfig'],'cli_mutation_policy':_policy_evidence(req,s,inspect),
      'agent_user':'65532:65532','access_probe_exit':inaccessible.returncode,'skill_hashes':actual,'model_runtime':runtime})
    _checkpoint(req, s, 'delivery_complete', allocation_started=True, write_started=False)
    return {'isolated':True,'evaluator_inaccessible':True,'host_launch_cwd':str(work),'agent_environment':{'TADX_FEEDBACK_MODE':'off'},
      'skill_hashes':actual,'skill_evidence':[rel(req,p)],'feedback_mode':'off','evidence':[rel(req,p)]}

def _deliver_faults(req,s,name):
    import copy
    plan=copy.deepcopy(s['fault_plan']);config=copy.deepcopy(s['config_seed'])
    original_path=Path(req['private_case_dir'])/'profile-request.json'
    original=load(original_path) if original_path.is_file() else req
    if not isinstance(plan.get('rules'),list):
        raise Blocked('Request capture requires an explicit rule list; audit-only capture uses an empty list')
    endpoints=[]
    for index,(alias,environment) in enumerate(config['environments'].items()):
        sites=original.get('deployment',{}).get('operator_settings',{}).get('sites',{})
        selected=next((site for site in sites.values() if site.get('environment')==alias),{})
        endpoint={'environment':alias,'origin':environment['url'],'port':8770+index,
                  'site_content_url':environment.get('site_content_url'),'site_luid':selected.get('id')}
        endpoints.append(endpoint)
    plan['endpoints']=endpoints
    for rule in plan['rules']:
        if rule.get('mode') not in ('respond','forward_then_drop','delay','replace') or not rule.get('path'):
            raise Blocked('Fault rule lacks a concrete supported mode and request path')
        re.compile(rule['path'])
    root=Path(req['private_case_dir'])
    save(root/'fault-plan.json',plan);save(root/'fault-config.json',config)
    docker('cp',root/'fault-plan.json',name+':/cli-state/fault-plan.json')
    from urllib.parse import urlsplit
    hosts=','.join('DNS:'+urlsplit(endpoint['origin']).hostname for endpoint in endpoints)
    docker('exec','--user','0:0',name,'openssl','req','-x509','-newkey','rsa:2048','-nodes',
        '-keyout','/cli-state/fault-key.pem','-out','/cli-state/fault-cert.pem','-days','2',
        '-subj','/CN=Disposable TADX fixture CA','-addext','subjectAltName='+hosts)
    docker('exec','--user','0:0',name,'node','-e',"const f=require('fs');f.writeFileSync('/cli-state/fault-trust.pem',f.readFileSync('/etc/ssl/certs/ca-certificates.crt')+'\\n'+f.readFileSync('/cli-state/fault-cert.pem'));f.writeFileSync('/cli-state/fault-env.json',JSON.stringify({HTTPS_PROXY:'http://127.0.0.1:8766',SSL_CERT_FILE:'/cli-state/fault-trust.pem'}));")
    docker('exec','-d','--user','0:0',name,'node','/opt/fault_broker.cjs','/cli-state/fault-plan.json')
    for _ in range(READY_POLL_ATTEMPTS):
        probe=docker('exec',name,'node','-e',"const f=require('fs');const p='/audit/fault-events.jsonl';process.exit(f.existsSync(p)&&f.readFileSync(p,'utf8').split('\\n').filter(x=>x.includes('\\\"ready\\\"')).length==="+str(len(endpoints))+"?0:1)",check=False)
        if probe.returncode==0:break
        time.sleep(READY_POLL_DELAY_S)
    else:raise Blocked('Disposable HTTP fault listener did not start')
    s['fault_delivery']={'endpoints':endpoints,'plan':'fault-plan.json','evidence':'audit/fault-events.jsonl'}
    write_state(req,s)


def _deliver_credentials(req,s,name):
    spec=s['interactive_credentials']
    if spec.get('store')!='secret-service':raise Blocked('Unknown disposable OS credential store')
    script=r"""const fs=require('fs'),cp=require('child_process');
const dbus=cp.spawnSync('dbus-daemon',['--session','--fork','--print-address'],{encoding:'utf8'});
if(dbus.status!==0)process.exit(2);
const env={...process.env,HOME:'/tmp/cli-home',XDG_RUNTIME_DIR:'/tmp/keyring-runtime',DBUS_SESSION_BUS_ADDRESS:dbus.stdout.trim()};
fs.mkdirSync(env.XDG_RUNTIME_DIR,{recursive:true,mode:0o700});
const keyring=cp.spawnSync('gnome-keyring-daemon',['--unlock','--components=secrets'],{env,input:'\n',encoding:'utf8'});
if(keyring.status!==0)process.exit(3);
fs.writeFileSync('/cli-state/credential-runtime.json',JSON.stringify({DBUS_SESSION_BUS_ADDRESS:env.DBUS_SESSION_BUS_ADDRESS,XDG_RUNTIME_DIR:env.XDG_RUNTIME_DIR}),{mode:0o600});
"""
    result=docker('exec','--user','0:0',name,'node','-e',script,check=False)
    if result.returncode:raise Blocked('Disposable native credential-store service failed to start')
    if spec.get('mode') in ('seed_login','precedence'):
        seed=r"""const fs=require('fs'),cp=require('child_process');
const env={PATH:process.env.PATH,HOME:'/tmp/cli-home',XDG_CONFIG_HOME:'/cli-state',XDG_DATA_HOME:'/cli-state/data',TADX_FEEDBACK_MODE:'off',...JSON.parse(fs.readFileSync('/cli-state/credentials.json')),...JSON.parse(fs.readFileSync('/cli-state/credential-runtime.json'))};
const r=cp.spawnSync('python3',['/opt/credential_pty.py','auth','login','--environment',process.argv[1],'--json'],{env,encoding:'utf8',timeout:130000});
if(r.status!==0)process.exit(2);const v=JSON.parse(r.stdout);fs.writeFileSync('/audit/credential-setup.json',JSON.stringify(v));
if(v.status!==0)process.exit(v.status);
const text=fs.readFileSync('/cli-state/tadx/config.yaml','utf8');
const found=text.match(/credential_ref["']?\s*:\s*["']?(cred_[a-f0-9]{32})/);
if(!found)process.exit(3);
const reference=found[1];
const lookup=cp.spawnSync('secret-tool',['lookup','service','io.github.ahillspace.tadx','username','pat:v1:'+reference.slice(5)],{env,encoding:'utf8',timeout:10000});
if(lookup.status!==0)process.exit(4);
let record;try{record=JSON.parse(lookup.stdout);}catch{process.exit(5);}
if(record.pat_name!==env.TADX_BENCH_AGENT_PAT_NAME||record.pat_secret!==env.TADX_BENCH_AGENT_PAT_SECRET)process.exit(6);
fs.writeFileSync('/cli-state/seed-credential-ref.txt',reference,{mode:0o600});
fs.writeFileSync('/audit/credential-store-baseline.json',JSON.stringify({scope:'disposable_container',stored:true,credential_match:true,credential_material_exported:false}),{mode:0o600});
process.exit(0);
"""
        answer=docker('exec','--user','0:0',name,'node','-e',seed,spec['environment'],timeout=140,check=False)
        if answer.returncode:raise Blocked('Native login did not establish the disposable stored-credential fixture')
    s.setdefault('broker_guard',{})['interactive_credentials']=spec
    write_state(req,s)


def run_task(req):
    s=state(req)
    if s.get('split_broker') and not s.get('delivery_complete'):
        raise Blocked('Split runtime delivery did not complete; model execution withheld')
    if s.get('task_started'):raise Blocked('Repeated task execution forbidden')
    s['task_started']=True
    _checkpoint(req, s, 'task_started', allocation_started=True, write_started=True)
    if req.get('deterministic'):
        from integration.deterministic import run_task
        result=run_task(req, s, docker=docker)
    else:
        from integration.codex_runtime import model_run
        result=model_run(req,s,capture,Redactor,ROOT)
    s['task_sessions']=result.get('task',{}).get('telemetry',{}).get('session_ids',[]);write_state(req,s)
    return result


def capture_followup_baseline(req):
    import datetime as dt
    s=state(req)
    if req.get('sessions')!=s.get('task_sessions') or not s.get('task_started'):
        raise Blocked('Follow-up requires the recorded task session')
    root=Path(req['private_case_dir']);target=root/'followup-baseline';target.mkdir(exist_ok=True)
    name=s.get('broker_container',s['container'])
    receipt=target/'last-result.json'
    copied=docker('cp',name+':/cli-state/tadx/last-result.json',receipt,check=False)
    if copied.returncode:raise Blocked('Initial operation did not save a native last-result receipt')
    document=load(receipt)
    count=docker('exec','--user','0:0',name,'node','-e',"const f=require('fs');process.stdout.write(String(f.readFileSync('/audit/commands.jsonl','utf8').trim().split('\\n').filter(Boolean).length));").stdout
    network=docker('exec','--user','0:0',name,'node','-e',"const f=require('fs'),p='/audit/fault-events.jsonl';process.stdout.write(String(f.existsSync(p)?f.readFileSync(p,'utf8').split('\\n').filter(Boolean).length:-1));").stdout
    baseline={'receipt':document,'receipt_sha256':sha(receipt),'task_command_count':int(count),
              'captured_at':dt.datetime.now(dt.timezone.utc).isoformat(),'sessions':s['task_sessions'],
              'network_event_count':int(network)}
    save(target/'baseline.json',baseline);s['followup_baseline']=baseline;write_state(req,s)
    return {'baseline':baseline,'evidence':['followup-baseline/baseline.json','followup-baseline/last-result.json']}


def followup(req):
    import datetime as dt
    s=state(req)
    if not s.get('followup_baseline') or req.get('sessions')!=s.get('task_sessions'):
        raise Blocked('Follow-up requires the exact saved baseline and task session')
    if s.get('followup_started'):raise Blocked('Repeated follow-up dispatch is not permitted')
    s['followup_started']=True;s['followup_started_at']=dt.datetime.now(dt.timezone.utc).isoformat();write_state(req,s)
    if req.get('deterministic'):
        module=profile(req)
        if not hasattr(module,'qualification_followup_commands'):raise Blocked('Missing native follow-up command implementation')
        from integration.deterministic import run_task as drive
        commands=module.qualification_followup_commands(req,req['fixture'])
        task=drive({**req,'deterministic_commands_override':commands,'deterministic_followup':True},s,docker=docker)['task']
        return {**task,'metrics':task['telemetry']}
    from integration.codex_runtime import model_followup
    return model_followup(req,s,capture,Redactor,ROOT)

def _observe_credentials(req,s,name):
    script=r"""const fs=require('fs'),cp=require('child_process');
const credentials=JSON.parse(fs.readFileSync('/cli-state/credentials.json','utf8'));
const runtime=JSON.parse(fs.readFileSync('/cli-state/credential-runtime.json','utf8'));
const env={PATH:process.env.PATH,HOME:'/tmp/cli-home',XDG_CONFIG_HOME:'/cli-state',XDG_DATA_HOME:'/cli-state/data',TADX_FEEDBACK_MODE:'off',...credentials,...runtime};
const r=cp.spawnSync('/opt/tadx',['auth','status','--environment',process.argv[1],'--json','--full'],{env,encoding:'utf8',timeout:30000});
let status;try{status=JSON.parse(r.stdout);status=status.output||status;}catch{throw Error('Native credential status unavailable');}
// Config becomes YAML after native auth changes. Read only the opaque reference.
const text=fs.readFileSync('/cli-state/tadx/config.yaml','utf8');
const ref=text.match(/credential_ref["']?\s*:\s*["']?(cred_[a-f0-9]{32})/);
const previous=fs.existsSync('/cli-state/seed-credential-ref.txt')?fs.readFileSync('/cli-state/seed-credential-ref.txt','utf8'):null;
const reference=ref?ref[1]:previous;
const service=cp.spawnSync('dbus-send',['--session','--print-reply','--dest=org.freedesktop.DBus','/org/freedesktop/DBus','org.freedesktop.DBus.NameHasOwner','string:org.freedesktop.secrets'],{env,encoding:'utf8',timeout:10000});
if(service.status!==0||!/\bboolean true\b/.test(service.stdout))throw Error('Disposable secret service is not observable');
let stored=null,match=false;
if(reference){const lookup=cp.spawnSync('secret-tool',['lookup','service','io.github.ahillspace.tadx','username','pat:v1:'+reference.slice(5)],{env,encoding:'utf8',timeout:10000});
  if(lookup.status===0){const record=JSON.parse(lookup.stdout);stored=true;match=record.pat_name===credentials.TADX_BENCH_AGENT_PAT_NAME&&record.pat_secret===credentials.TADX_BENCH_AGENT_PAT_SECRET;}
  else if(lookup.status===1&&!lookup.stdout.trim()&&!lookup.stderr.trim())stored=false;
  else throw Error('Stored credential lookup did not conclusively establish presence or absence');}
const proof={scope:'disposable_container',credential_material_exported:false,stored,credential_match:match,
  environment_credentials_unchanged:status.pat_name_present===true&&status.pat_secret_present===true,
  environment_selected:status.credential_source==='environment',native_status_exit:r.status};
process.stdout.write(JSON.stringify(proof));
"""
    response=docker('exec','--user','0:0',name,'node','-e',script,s['interactive_credentials']['environment'],check=False)
    if response.returncode:
        proof={'scope':'disposable_container','credential_material_exported':False,'observation_error':'Native store inspection failed'}
    else:proof=json.loads(response.stdout)
    save(Path(req['private_case_dir'])/'auth-store-observation.json',proof)


def freeze(req):
    s=state(req);name=s.get('container');evidence={'container':name,'running':False}
    _checkpoint(req, s, 'freeze_started', allocation_started=bool(s.get('container') or s.get('broker_container')),
                write_started=bool(s.get('task_started')))
    if s.get('split_broker'):
        return _freeze_split(req, s)
    # A prior cleanup checkpoint is authoritative only after it recorded an
    # exact daemon absence.  Do not inspect a name that was already verified
    # absent, since it may have been reused by an unrelated process.
    existing=_split_container(req,s,'container') if name and not s.get('container_removed') else None
    if existing is not None:
        if s.get('interactive_credentials') and existing['State'].get('Running'):
            _observe_credentials(req,s,name)
        if s.get('mutable_binary') and existing['State'].get('Running'):
            script="const fs=require('fs'),c=require('crypto');const p='/cli-state/update/tadx';process.stdout.write(JSON.stringify({sha256:c.createHash('sha256').update(fs.readFileSync(p)).digest('hex'),guidance_installed:fs.existsSync('/tmp/cli-home/.codex/skills/tadx/SKILL.md')}));"
            observed=docker('exec','--user','0:0',name,'node','-e',script,check=False)
            save(Path(req['private_case_dir'])/'mutable-binary-observation.json',json.loads(observed.stdout) if observed.returncode==0 else {'error':'updated executable observation failed'})
        docker('stop','--time','2',existing['Id'],check=False)
        detail=_split_container(req,s,'container')
        if detail is None:raise Blocked('Container disappeared during freeze without retained process evidence: '+name)
        evidence.update(id=detail['Id'],state=detail['State'])
        if detail['State']['Running'] or detail['State'].get('Pid',0)!=0:raise Blocked('Container process boundary still running')
        copies={}
        for directory,source in (('audit','/audit/.'),('cli-state','/cli-state/tadx/.')):
            target=Path(req['private_case_dir'])/directory;target.mkdir(exist_ok=True)
            copy_identity=(name if detail.get('Name') is None else detail['Id'])
            copied=docker('cp',copy_identity+':'+source,str(target),check=False)
            copies[directory]=copied.returncode
            if copied.returncode and s.get('task_started'):
                raise Blocked('Stopped task evidence could not be retained: '+name+' '+source)
        s['audit_capture']={'status':'copied' if not any(copies.values()) else 'pre_task_partial',
            'container_id':detail['Id'],'copy_exit_statuses':copies,'task_started':bool(s.get('task_started'))}
    elif s.get('container_removed'):
        evidence.update(id=s.get('container_id'), removal_reconciled=True)
        s.setdefault('audit_capture', {'status':'already_removed',
            'container_id':s.get('container_id'), 'task_started':bool(s.get('task_started')),
            'allocation':'removal_reconciled'})
    elif s.get('container_created'):
        evidence.update(id=s['container_id'],removal_reconciled=True,
            removal_verification=s.get('container_removal_verification'))
    else:
        if s.get('task_started'):raise Blocked('Task dispatch has no confirmed allocated process boundary')
        evidence['allocation']='container_not_created_confirmed_by_exact_inventory' if name else 'no_container_allocation_requested'
        s['audit_capture']={'status':'not_allocated','task_started':False,'allocation':evidence['allocation']}
    _retain_unfinished_commands(Path(req['private_case_dir']))
    evidence['audit_capture']=s.get('audit_capture', {'status':'not_recorded'})
    p=Path(req['private_case_dir'])/'freeze-evidence.json';save(p,evidence)
    s['frozen']=True;write_state(req,s)
    _checkpoint(req, s, 'frozen', allocation_started=bool(s.get('container') or s.get('broker_container')),
                write_started=bool(s.get('task_started')))
    return {'quiescent':True,'evidence':[rel(req,p)]}


def _freeze_split(req, s):
    evidence = {'transport': 'fixed_unix_socket', 'containers': []}
    broker=None
    for key in ('container', 'broker_container'):
        details = _split_container(req, s, key)
        if details is None:
            evidence['containers'].append({'role':key,'id':s.get(key+'_id'),
                'allocation':'removal_reconciled' if s.get(key+'_created') else 'not_created_confirmed_by_exact_inventory'})
            continue
        if key == 'broker_container' and s.get('broker_guard', {}).get('mode') == 'shared-release' and details['State'].get('Running'):
            try:
                _observe_candidate_release(req, s, 'release-after-native.json')
            except Blocked as error:
                # Native evidence failure must not leave the CLI process running.
                s['release_observation_error']=str(error)
        # The exact ownership check above already validated this name and ID.
        # Stop by the recorded name so Docker and the local boundary fake use
        # the same stable identity.
        docker('stop', '--time', '2', s[key], check=False)
        stopped = _split_container(req, s, key)
        if stopped is None:raise Blocked('Split process disappeared during freeze: '+s[key])
        if stopped['State']['Running'] or stopped['State'].get('Pid', 0) != 0:
            raise Blocked('Split runtime process boundary is still running')
        evidence['containers'].append({'role': key, 'id': stopped['Id'], 'state': stopped['State']})
        if key=='broker_container':broker=stopped
    if broker is not None:
        copies={}
        for directory, source in (('audit', '/audit/.'), ('cli-state', '/cli-state/tadx/.')):
            target = Path(req['private_case_dir']) / directory; target.mkdir(exist_ok=True)
            # Use the immutable ID when inspect returned a complete Docker
            # record.  Lightweight boundary doubles omit Name and index the
            # resource by its deterministic name, which was already checked
            # against the recorded ID above.
            copy_identity=(s['broker_container'] if broker.get('Name') is None else broker['Id'])
            copied=docker('cp',copy_identity+':'+source,str(target),check=False);copies[directory]=copied.returncode
            if copied.returncode and s.get('task_started'):
                raise Blocked('Stopped split task evidence could not be retained: '+s['broker_container']+' '+source)
        if s.get('broker_guard', {}).get('mode') == 'shared-release':
            transcript=Path(req['private_case_dir'])/'release-transport.jsonl'
            copied=docker('cp', copy_identity+':/cli-state/release-transport.jsonl', str(transcript),check=False)
            if copied.returncode:
                task_audit=Path(req['private_case_dir'])/'audit/commands.jsonl'
                rows=[json.loads(line) for line in task_audit.read_text(encoding='utf-8').splitlines() if line] if task_audit.is_file() else []
                if any(row.get('executed') is True and row.get('argv',[])[1:2]==['update'] for row in rows):
                    raise Blocked('Executed native update has no retained offline transport evidence')
                transcript.write_text('',encoding='utf-8',newline='\n')
        s['audit_capture']={'status':'copied' if not any(copies.values()) else 'pre_task_partial',
            'container_id':broker['Id'],'copy_exit_statuses':copies,'task_started':bool(s.get('task_started'))}
    elif s.get('broker_container_removed'):
        s.setdefault('audit_capture', {'status':'already_removed',
            'container_id':s.get('broker_container_id'), 'task_started':bool(s.get('task_started')),
            'allocation':'removal_reconciled'})
    elif not s.get('broker_container_created'):
        if s.get('task_started'):raise Blocked('Task dispatch has no confirmed allocated CLI boundary')
        s['audit_capture']={'status':'not_allocated','task_started':False,'allocation':'native_broker_not_created_confirmed_by_exact_inventory'}
    _retain_unfinished_commands(Path(req['private_case_dir']))
    evidence['audit_capture']=s['audit_capture']
    if s.get('release_observation_error'):
        evidence['release_observation_error']=s['release_observation_error']
    path = Path(req['private_case_dir']) / 'freeze-evidence.json'; save(path, evidence)
    s['frozen'] = True; write_state(req, s)
    _checkpoint(req, s, 'frozen', allocation_started=bool(s.get('container') or s.get('broker_container')),
                write_started=bool(s.get('task_started')))
    return {'quiescent': True, 'evidence': [rel(req, path)]}

def _retain_unfinished_commands(root):
    started=root/'audit/started-commands.jsonl';completed=root/'audit/commands.jsonl'
    if not started.is_file():return
    starts=[json.loads(line) for line in started.read_text(encoding='utf-8').splitlines() if line.strip()]
    rows=[json.loads(line) for line in completed.read_text(encoding='utf-8').splitlines() if line.strip()] if completed.is_file() else []
    known={row['id'] for row in rows}
    unfinished=[row for row in starts if row['id'] not in known]
    if not unfinished:return
    raw=root/'audit/completed-commands-original.jsonl'
    if completed.is_file() and not raw.exists():shutil.copy2(completed,raw)
    with completed.open('a',encoding='utf-8',newline='\n') as stream:
        for row in unfinished:
            row.update(exit_status=None,stdout='',stderr='',outcome='unknown',
                       completion_missing=True,start_evidence='audit/started-commands.jsonl#'+row['id'])
            stream.write(json.dumps(row)+'\n')
    save(root/'unfinished-commands.json',{'commands':unfinished,'requires_reconciliation':any(r.get('executed') for r in unfinished)})


def observe(req):
    s=state(req)
    if not s.get('frozen'):raise Blocked('Observation requires frozen boundary')
    _checkpoint(req, s, 'observe_started', allocation_started=bool(s.get('container') or s.get('broker_container')),
                write_started=bool(s.get('task_started')))
    work=Path(s['work']);root=Path(req['private_case_dir']); audit=root/'audit/commands.jsonl'
    commands=[]
    if audit.exists():
        for row in audit.read_text(encoding='utf-8').splitlines():
            record=json.loads(row);record['evidence']='audit/commands.jsonl#'+record['id']
            if not record.get('error_id'):
                from integration.scenario_common import error_document
                error,reference=error_document(record)
                if error and isinstance(error.get('id'),str):
                    record['error_id']=error['id'];record['error_id_evidence']=reference
            commands.append(record)
    admitted_hashes={s['manifest']['binaries']['linux']['sha256']}
    if s.get('mutable_binary') and (s.get('broker_guard') or {}).get('mode')=='shared-release':
        from integration.release_update_profile import candidate_release
        candidate=candidate_release()
        admitted_hashes.update((candidate['baseline_binary_sha256'],candidate['binary_sha256']))
    # The broker creates ``ready.json`` before accepting the task.  A task
    # which never invokes the installed CLI therefore has no commands file,
    # but the copied ready marker plus the absence of any started-command
    # record is still a complete audit of zero invocations.  Treating that
    # case as incomplete incorrectly blocks the exercise instead of allowing
    # coverage to record that the required action was not exercised.  If a
    # command was started, ``_retain_unfinished_commands`` writes the started
    # record and the audit remains fail-closed until its completion is
    # reconciled.
    ready = root / 'audit' / 'ready.json'
    started = root / 'audit' / 'started-commands.jsonl'
    zero_invocation_audit = (not audit.exists() and ready.is_file() and
                             not started.exists() and s.get('task_started') is True and
                             s.get('audit_capture', {}).get('status') == 'copied')
    if zero_invocation_audit:
        audit.parent.mkdir(parents=True, exist_ok=True)
        audit.write_text('', encoding='utf-8', newline='\n')
    complete=(audit.exists() and all(c.get('binary_sha256') in admitted_hashes and not c.get('completion_missing') for c in commands))
    s['cli_audit_complete']=complete
    fault_file=root/'audit/fault-events.jsonl'
    if fault_file.is_file():
        events=[json.loads(line) for line in fault_file.read_text(encoding='utf-8').splitlines() if line.strip()]
        save(root/'fault-observation.json',{'triggered':any(e.get('rule') is not None for e in events),
             'events':events,'source':'audit/fault-events.jsonl'})
    trace=root/'audit/guidance-fault.trace'
    if trace.is_file():
        text=trace.read_text(encoding='utf-8',errors='replace')
        save(root/'fault-observation.json',{'triggered':'INJECTED' in text and 'EIO' in text,
            'successful_prior_rename':bool(re.search(r'rename\w*\([^\n]+\)\s*=\s*0',text)),
            'source':'audit/guidance-fault.trace'})
    write_state(req,s)
    extra=profile(req)
    if extra:
        if hasattr(extra,'load_binding'):
            answer=extra.observe(req,extra.load_binding(req),work=work,commands=commands)
            after=answer.get('after',answer)
        else:
            import yaml
            config_file=root/'cli-state/config.yaml'
            config=yaml.safe_load(config_file.read_text(encoding='utf-8')) if config_file.exists() else {}
            after=extra.observe(req,s,config,work,commands)
        exercise=req.get('exercise') if isinstance(req,dict) else None
        evaluator=exercise.get('evaluator',{}) if isinstance(exercise,dict) else {}
        requirements=evaluator.get('requirements',[]) if isinstance(evaluator,dict) else []
        if 'action_write_request_audit' in requirements:
            if s.get('runtime_network')=='none' and s.get('delivery_complete'):
                network={'status':'verified','action_write_request_count':0,'mechanism':'isolated_cli_network_none','evidence':['delivery-evidence.json']}
            elif s.get('fault_delivery') and fault_file.is_file():
                events=[json.loads(line) for line in fault_file.read_text(encoding='utf-8').splitlines() if line.strip()]
                requests=[event for event in events if event.get('method')]
                writes=[event for event in requests if event['method'] not in ('GET','HEAD','OPTIONS')
                        and not re.search(r'/(?:auth/(?:signin|signout)|metadata/graphql|vizql-data-service/(?:read-metadata|query-datasource))$',event.get('path','').split('?')[0])
                        and not (event['method']=='POST' and re.search(r'/sites/[^/]+/labels$',event.get('path','').split('?')[0]))]
                network={'status':'verified','action_write_request_count':len(writes),'requests':requests,
                         'mechanism':'original_origin_https_proxy','evidence':['audit/fault-events.jsonl']}
            else:network={'status':'unavailable','action_write_request_count':None,'reason':'Independent request capture was not completed'}
            after['network']=network
            if network['action_write_request_count']!=0:
                after.setdefault('unowned_changes',[]).append('Preview action-write audit is nonzero or unresolved')
        save(root/'observer.json',{'after':after,'commands':commands,'cli_audit_complete':complete})
        _checkpoint(req, s, 'observed', allocation_started=bool(s.get('container') or s.get('broker_container')),
                    write_started=bool(s.get('task_started')))
        return {'after':after,'commands':commands,'cli_audit_complete':complete,
                'boundary_or_helper_coverage_complete':after.get('boundary_or_helper_coverage_complete',False),
                'evidence':['observer.json','audit/commands.jsonl']}
    from integration.report_evidence import read_report, diagnostics
    from integration.command_evidence import verify_read_commands
    case=req['exercise']['id']
    report=read_report(work,allow_list=case=='P-capability-list')
    status='present' if report['status'] in ('present','normalized') else ('missing' if report['status']=='missing' else 'invalid')
    result=None
    if status=='present':
        data=report['value']
        try:
            result={'version':data.get('version'),'build_identity':data.get('build_identity')} if case=='P-version-get' else normalize_inventory(data)
        except (KeyError,ValueError,TypeError):
            status='invalid'
    protected={'sentinel_sha256':sha(work/'sentinel.txt') if (work/'sentinel.txt').is_file() and not (work/'sentinel.txt').is_symlink() else None}
    unexpected=[]
    for path in work.rglob('*'):
        if path.is_symlink():unexpected.append(path.relative_to(work).as_posix()+':symlink');continue
        if not path.is_file():continue
        name=path.relative_to(work).as_posix()
        if name not in ('sentinel.txt','inputs.json','opencode.json') and not name.startswith(('deliverables/','.opencode/')):unexpected.append(name)
    expected=s.get('fixture',req.get('fixture',{})).get('expect',{}).get('result')
    command_evidence=verify_read_commands(case,commands,expected,
        frozen_sha256=s['manifest']['binaries']['linux']['sha256'],
        frozen_build_identity=(s['manifest']['source_commit'] if s['manifest'].get('build_metadata') else None))
    after={'result':result,'candidate_status':status,'protected':protected,'unowned_changes':sorted(unexpected),
           'report_evidence':diagnostics(report),'command_evidence':command_evidence}
    # Root broker is the only process able to execute the installed binary.
    save(root/'observer.json',{'after':after,'commands':commands,'cli_audit_complete':complete})
    _checkpoint(req, s, 'observed', allocation_started=bool(s.get('container') or s.get('broker_container')),
                write_started=bool(s.get('task_started')))
    return {'after':after,'commands':commands,'cli_audit_complete':complete,'evidence':['observer.json','audit/commands.jsonl']}

def cleanup(req):
    s=state(req)
    owned_volumes = s.get('owned_volumes')
    legacy_cleanup = bool(s.get('_legacy_cleanup_compat')) or (
        not isinstance(s.get('lifecycle'), dict)
        and not any(s.get(key + '_id') or s.get(key + '_creation_intent') or
                    isinstance(owned_volumes, dict) and key in owned_volumes
                    for key in ('container', 'volume', 'config_volume')))
    if legacy_cleanup:
        # Mark the compatibility mode before checkpointing so a later retry
        # does not reinterpret this pre-checkpoint state as a strict one.
        s['_legacy_cleanup_compat'] = True
    _checkpoint(req, s, 'cleanup_started', allocation_started=bool(s.get('container') or s.get('broker_container') or
                                                                   s.get('volume') or s.get('config_volume') or s.get('socket_volume')),
                write_started=bool(s.get('task_started')))
    if s.get('container') and not s.get('frozen'):raise Blocked('Cleanup requires confirmed freeze')
    from integration.reset_container import reconcile_reset_containers
    reconcile_reset_containers(Path(req['private_case_dir']))
    records=[]
    extra=profile(req)
    if extra and not s.get('profile_cleanup_completed'):
        recovery = _local_disposable_recovery(req, s) or _local_before_worker_recovery(req, s)
        if recovery:
            s['profile_cleanup_completed'] = True
            s['profile_cleanup_result'] = recovery
            write_state(req, s)
    if extra and hasattr(extra,'cleanup') and not s.get('profile_cleanup_completed'):
        answer=(extra.cleanup(req,extra.load_binding(req),work=Path(s.get('work',Path(req['private_case_dir'])/'agent-public')))
                if hasattr(extra,'load_binding') else extra.cleanup(req,s,work=Path(s.get('work',Path(req['private_case_dir'])/'agent-public'))))
        records.append({'profile':extra.__name__,'result':answer})
        if answer.get('cleanup_status',answer.get('status'))!='pass':
            return {'cleanup_status':'blocked','records':records}
        s['profile_cleanup_completed']=True
        s['profile_cleanup_result']=answer
        write_state(req,s)
    elif extra and s.get('profile_cleanup_completed') and s.get('profile_cleanup_result'):
        records.append({'profile':extra.__name__,'result':s['profile_cleanup_result']})
    if s.get('split_broker'):
        return _cleanup_split(req, s, records)
    for key in ('container', 'volume', 'config_volume'):
        if not s.get(key) or s.get(key + '_removed'):
            continue
        try:records.append(_remove_boundary_resource(req,s,key,legacy_cleanup=legacy_cleanup))
        except (Blocked,OSError,subprocess.TimeoutExpired) as exc:
            records.append({'resource':key,'name':s[key],'status':'unresolved','cause':str(exc)})
            return {'cleanup_status':'blocked','resource':s[key],'reason':str(exc),'records':records}
    result={'cleanup_status':'pass','records':records,'retained':['agent-public','audit','task']}
    _checkpoint(req, s, 'cleanup_complete', allocation_started=bool(s.get('container') or s.get('broker_container') or
                                                                     s.get('volume') or s.get('config_volume') or s.get('socket_volume')),
                write_started=bool(s.get('task_started')), cleanup_status='pass')
    return result


def _local_disposable_recovery(req, s):
    # Nothing in a stopped offline worker is reused. Damaged task copies are
    # evidence of model failure, not a reason to quarantine the next fresh case.
    if (s.get('remote_profile') is not False or s.get('frozen') is not True
            or s.get('runtime_network') != 'none'
            or not req.get('run_constraints', {}).get('native_cli_execution')):
        return None
    result={'status':'pass','scope':'Discarded offline per-case state; no shared asset restoration',
            'evidence':['freeze-evidence.json'],'retained':['agent-public','audit']}
    save(Path(req['private_case_dir'])/'disposable-cleanup.json',result)
    return result


def _local_before_worker_recovery(req, s):
    """Release an offline allocation when the independently owned worker never ran."""
    if (not s.get('split_broker') or s.get('runtime_network') != 'none' or s.get('remote_profile')
            or s.get('frozen') is not True or s.get('delivery_complete') is True or not s.get('container_created')):
        return None
    worker = _split_container(req, s, 'container')
    if not worker:
        return None
    process = worker['State']
    if (process.get('Status') != 'created' or process.get('Running') or process.get('Pid') != 0
            or process.get('StartedAt') != '0001-01-01T00:00:00Z'):
        return None
    audit = Path(req['private_case_dir']) / 'audit/commands.jsonl'
    if audit.exists() and (audit.is_symlink() or audit.stat().st_size != 0):
        return None
    result = {'status': 'pass', 'scope': 'offline allocation before worker execution',
              'worker_id': worker['Id'], 'worker_state': process,
              'task_started': False, 'evidence': ['pre-dispatch-cleanup.json']}
    save(Path(req['private_case_dir']) / 'pre-dispatch-cleanup.json', result)
    return result


def _cleanup_split(req, s, records):
    for key in ('container','broker_container','volume','config_volume','socket_volume'):
        if not s.get(key) or s.get(key+'_removed'):
            continue
        try:records.append(_remove_boundary_resource(req,s,key))
        except (Blocked,OSError,subprocess.TimeoutExpired) as exc:
            records.append({'resource':key,'name':s[key],'status':'unresolved','cause':str(exc)})
            return {'cleanup_status':'blocked','resource':s[key],'reason':str(exc),'records':records}
    result={'cleanup_status': 'pass', 'records': records, 'retained': ['agent-public', 'audit', 'task']}
    _checkpoint(req, s, 'cleanup_complete', allocation_started=bool(s.get('container') or s.get('broker_container') or
                                                                     s.get('volume') or s.get('config_volume') or s.get('socket_volume')),
                write_started=bool(s.get('task_started')), cleanup_status='pass')
    return result

def dispatch(method,req):
    if method=='retrospective':raise Blocked('Exact-session read-only continuation not qualified; retrospective unavailable')
    fn=globals().get(method)
    if method not in ('preflight','prepare','deliver','run_task','freeze','observe','cleanup','capture_followup_baseline','followup') or not callable(fn):raise Blocked('Bridge method not qualified: '+method)
    return {'status':'ok','protocol':PROTOCOL,**fn(req)}

if __name__=='__main__':
    method,request,response=sys.argv[1:]
    try: result=dispatch(method,load(request))
    except Unsupported as exc:result={'status':'unsupported','protocol':PROTOCOL,'reason':str(exc)}
    except CLIError as exc:
        result={'status':'blocked','protocol':PROTOCOL,'reason':str(exc),'phase':method,
                'error_type':type(exc).__name__,'native_result':exc.result}
    except (Blocked,HTTPFailure,UnknownWrite,subprocess.TimeoutExpired,ValueError,KeyError,OSError) as exc:
        result={'status':'blocked','protocol':PROTOCOL,'reason':str(exc),'phase':method,'error_type':type(exc).__name__}
    save(response,result)
