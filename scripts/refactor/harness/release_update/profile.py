"""One candidate-built offline update exercise and its independent transition oracle."""
from pathlib import Path
import hashlib
import json
from bench.core import Blocked,load,save
from integration import scenario_common as common
from integration import release_update_assets

BASELINE='release-update-baseline.json'
PROTECTED_GUIDANCE_PATH='unrelated-fixture/SKILL.md'
PROTECTED_GUIDANCE_BYTES=b'protected unrelated Codex skill\n'
PROTECTED_GUIDANCE_SHA256=hashlib.sha256(PROTECTED_GUIDANCE_BYTES).hexdigest()
CONFIG_SEED={'version':1,'environments':{}}


def supports(exercise):return exercise.get('id')=='P-current-update'


def candidate_release():
    try:
        manifest = release_update_assets.load()
    except release_update_assets.AssetError as error:
        raise Blocked('Candidate-built update asset closure is not verified') from error
    return {'version': manifest['target_version'], 'starting_version': manifest['starting_version'],
            'archive': manifest['archive'], 'archive_sha256': manifest['files'][manifest['archive']],
            'checksum_sha256':manifest['files']['checksums.txt'],
            'baseline_binary_sha256': manifest['baseline_binary_sha256'],
            'binary_sha256': manifest['target_binary_sha256'],
            'guidance': manifest['guidance'], 'source_commit': manifest['source_commit'],
            'source_fingerprint': manifest['source_fingerprint'],
            'manifest_sha256': hashlib.sha256((release_update_assets.DEFAULT_ROOT/'manifest.json').read_bytes()).hexdigest(),
            'published': False}


def preflight(req,frozen):
    prompt = req['exercise']['agent']['prompt']
    if 'pinned candidate-built offline release' not in prompt or 'latest published' in prompt:
        raise Blocked('Update task must describe a pinned candidate fixture, not a published release')
    return {'qualified_requirements':['current_cli_exact_contract'], 'release':candidate_release()}


def prepare(req,frozen,work):
    target=candidate_release();work=Path(work);root=Path(req['private_case_dir'])
    (work/'sentinel.txt').write_bytes(b'protected update fixture\n')
    protected=hashlib.sha256((work/'sentinel.txt').read_bytes()).hexdigest()
    condition={'path':['update'],'capability':'update','flags':{'target':'codex'}}
    public={'task_inputs':{'release':target['version'],'starting_version':target['starting_version'],
        'target':'codex','scope':'Disposable CLI and Guidance copy',
        'provenance':'Pinned candidate-built offline fixture, not a published GitHub release'}}
    before={'protected':{'sentinel':protected,'unrelated_guidance':PROTECTED_GUIDANCE_SHA256}}
    expected={'confirmed_fields':{'version':target['version'],'binary_sha256':target['binary_sha256'],
        'guidance':target['guidance']}, 'target_identity':{'path':'/cli-state/update/tadx',
        'source_commit':target['source_commit']}}
    fixture=common.bind_fixture(req,public,{'exact_target':{'path':'/cli-state/update/tadx'},'sentinel':{'path':'sentinel.txt'}},
        expected,before,source=BASELINE,conditions=[condition])
    save(root/BASELINE,{'release':target,'fixture':fixture,'before':before})
    return {'fixture':fixture,'before':before,'config_seed':CONFIG_SEED,
            'broker_guard':{'mode':'shared-release','operations':['update'],
                'mutable_binary':'/cli-state/update/tadx','manifest_sha256':target['manifest_sha256'],
                'source_commit':target['source_commit'],'target_version':target['version'],
                'baseline_binary_sha256':target['baseline_binary_sha256'],
                'target_binary_sha256':target['binary_sha256']},
            'runtime_network':'none','mutable_binary':True,'baseline_evidence':[BASELINE],
            'baseline_assertions':[{'id':'pinned-candidate-source-and-asset-closure','status':'pass'}]}


def qualification_commands(req,fixture):return [['update','--target','codex','--json','--full']]


def grade_transition(target, before, after, transport, commands, sentinel_sha256):
    """Confirm native change and fixed transport, not just a successful command."""
    issues=[]
    if before.get('binary_sha256') != target['baseline_binary_sha256'] or before.get('version') != target['starting_version'] or before.get('exit_status') != 0:
        issues.append('Starting native executable identity was not verified')
    protected_guidance={PROTECTED_GUIDANCE_PATH:PROTECTED_GUIDANCE_SHA256}
    if before.get('guidance') != protected_guidance:
        issues.append('Starting unrelated Guidance was absent or changed')
    if after.get('binary_sha256') != target['binary_sha256'] or after.get('version') != target['version'] or after.get('exit_status') != 0:
        issues.append('Target native executable identity was not verified')
    if after.get('guidance') != target['guidance'] | protected_guidance:
        issues.append('Updated Guidance or unrelated protected skill differs')
    if before.get('binary_sha256') == after.get('binary_sha256'):
        issues.append('Update did not replace the starting executable')
    if sentinel_sha256 is None:
        issues.append('Protected sentinel is absent')
    wanted=[('release-view','served'),('auth-refused','refused'),
            ('asset-served','served'),('asset-served','served')]
    release_urls=['https://github.com/ahillspace/tadx/releases/download/'+tag+'/'+asset
                  for tag in (target['version'],'v'+target['version']) for asset in ('checksums.txt',target['archive'])]
    if [(row.get('operation'),row.get('status')) for row in transport] != wanted or \
            [row.get('asset') for row in transport[2:]] != ['checksums.txt',target['archive']] or \
            [row.get('sha256') for row in transport[2:]] != [target['checksum_sha256'],target['archive_sha256']] or \
            any(row.get('url') not in release_urls or not row.get('url','').endswith('/'+row['asset']) for row in transport[2:]) or \
            any(row.get('tag') != 'v'+target['version'] for row in transport[:1]):
        issues.append('Fixed offline release transport evidence is incomplete or differs')
    valid=[]
    for command in commands:
        try:
            words,flags=common.arguments(command['argv'])
            result=common.native(command)
            while isinstance(result,dict) and isinstance(result.get('result',result.get('output')),dict):
                result=result.get('result',result.get('output'))
            if words==['update'] and flags.get('target')=='codex' and flags.get('json') is True and flags.get('full') is True and \
                    command.get('phase')=='task' and command.get('executed') is True and \
                    command.get('binary_sha256')==target['baseline_binary_sha256'] and command.get('exit_status')==0 and \
                    result.get('status')=='updated' and result.get('version')==target['version'] and \
                    result.get('guidance')=='refreshed' and result.get('installation_path')=='/cli-state/update/tadx':
                valid.append(command)
        except (ValueError,TypeError,KeyError,AttributeError):
            continue
    if len(valid)!=1:
        issues.append('One native candidate update command with the starting binary was not proven')
    return valid,issues


def observe(req,state,config,work,commands):
    root=Path(req['private_case_dir']);baseline=load(root/BASELINE);target=baseline['release']
    if target!=candidate_release():
        raise Blocked('Candidate update source closure or prepared baseline changed')
    if state.get('release_observation_error'):
        raise Blocked('Native candidate update observation is unavailable; see freeze evidence')
    if not (root/'release-before-native.json').is_file() or not (root/'release-after-native.json').is_file():
        raise Blocked('Native starting or final update observation is absent')
    try:
        before=load(root/'release-before-native.json')
        after=load(root/'release-after-native.json')
        transcript=root/'release-transport.jsonl'
        transport=[json.loads(row) for row in transcript.read_text(encoding='utf-8').splitlines() if row] if transcript.is_file() else []
    except (OSError,ValueError) as error:
        raise Blocked('Native candidate update evidence could not be decoded') from error
    sentinel=Path(work)/'sentinel.txt'
    sentinel_hash=hashlib.sha256(sentinel.read_bytes()).hexdigest() if sentinel.is_file() else None
    rows,issues=grade_transition(target,before,after,transport,commands,sentinel_hash)
    if config!=CONFIG_SEED:
        issues.append('Unrelated TADX configuration changed')
    if sentinel_hash!=baseline['before']['protected']['sentinel']:
        issues.append('Unrelated sentinel changed')
    proof=common.proof(rows,issues)
    return {'native_command_verified':proof['status']=='verified','native_command_evidence':proof,
            'unowned_changes':issues,'result':{'confirmed_fields':{'version':after.get('version'),
                'binary_sha256':after.get('binary_sha256'),
                'guidance':{key:after.get('guidance',{}).get(key) for key in target['guidance']}},
                'target_identity':baseline['fixture']['expect']['target_identity']},
            'protected':baseline['before']['protected']}


def cleanup(req,state,work=None):return {'status':'pass','scope':'Disposable executable and native Guidance are removed with the container'}


def task_instructions(exercise,public):return common.instructions(exercise,public)
