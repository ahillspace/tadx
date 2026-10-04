"""Credential-free local authentication readiness and disposable policy fixtures."""
from __future__ import annotations
import copy
import hashlib
import json
import os
from pathlib import Path

from bench.core import Blocked, digest, load, save
from integration.audit_report import _native, decode_native_toon
from integration.local_profiles import decode_error, profile
from integration.validation_profiles import COMMON_REQUIREMENTS, _audit, _same, _snapshot, parse_arguments

ENVIRONMENT='session-local'
OTHER_ENVIRONMENT='session-unrelated'
PREFIX='BENCH_SESSION_MISSING'
OTHER_PREFIX='BENCH_SESSION_UNRELATED_MISSING'
PROJECT='session-policy-probe'
SERVER='https://example.test'
SITE='session-fixture'
OTHER_SITE='session-unrelated'
BASELINE='session-baseline.json'
OBSERVATION='session-observation.json'
PATHS={'auth.status':['auth','status'], 'mutation.status':['mutation','status'],
       'mutation.set':['mutation','set'], 'project.create':['content','project','create']}
VARIANTS={'auth-status-offline':['auth.status'], 'mutation-env-precedence':['mutation.status','mutation.set'],
          'mutation-no-setting-authorization':['mutation.status','project.create']}
PROMPT_SHA256={'auth.status':'a4ed5284345b9a8f3430ba0bd891cda7a4da1157d288a33751e28c0d6cbbb08d',
    'auth-status-offline':'0e6bf1436d354a63f4e3ac167672a8ddbdeced615ecaa93f0635136277f35e94',
    'mutation.status':'1a667fe41f70ee24342fae671856c07db0b2bbcb362606afc9594777da360496',
    'mutation-env-precedence':'a0342ae7fc0f4655fb20dbfc7010252cbf8284fb3ef9c3753cdcb85782bc6371',
    'mutation-no-setting-authorization':'2ef7365793a115c073311b7eccf71a5034cdccaaef4b5e15231e0ffcc7d5d466',
    'mutation.set':'d1e56dfe4d6e4d193f9afcfd015e23b27a20bb260ba0467fa3b5c90437b9ff4a'}


def contract(exercise):
    e=exercise.get('evaluator',{});intent=e.get('intent',{});coverage=e.get('coverage_evidence',{});name=e.get('fixture_profile')
    required=set(COMMON_REQUIREMENTS);public={};base=True
    if name=='auth.status' and intent=={'operation':'auth.status'}:
        fields=['ready','source','network_requests'];roles={'isolated_config','credentials','sentinel'}
        actions=['auth.status'];public={'site':'fixture.public.site'}
    elif name in ('mutation.status','mutation.set') and intent=={'saved_enabled':True,'server_url':SERVER,'site_content_url':SITE}:
        fields=(['saved_enabled'] if name=='mutation.set' else [])+['effective','source','server_url','site_content_url']
        roles={'isolated_config','mutation_overlay','sentinel'};actions=[name]
    elif name in VARIANTS and intent==({} if name=='auth-status-offline' else {'server_url':SERVER,'site_content_url':SITE}):
        base=False;fields=['result'];roles={'target','sentinel'};actions=VARIANTS[name]
        if name=='auth-status-offline':required.add('network_audit')
        if name=='mutation-no-setting-authorization':public={'site':'fixture.public.site'};fields+=['canonical']
    else:raise Blocked('No qualified local authentication or mutation-setting contract')
    expected={'/after/unowned_changes':[], '/after/protected':{'ref':'/before/protected'}}
    expected.update({('/after/result/'+key if base else '/after/'+key):{'ref':'/fixture/expect/'+key} for key in fields})
    checks=e.get('expected_outcomes',[])
    if (set(e.get('requirements',[]))!=required or set(e.get('required_roles',[]))!=roles or e.get('dependencies')!=[]
        or e.get('allowed_effects',{}).get('remote') is not (name=='mutation-no-setting-authorization')
        or exercise.get('agent',{}).get('public_bindings')!=public or coverage.get('required_actions')!=actions
        or coverage.get('required_variant')!=('base' if base else name) or coverage.get('command_predicates')!=[]
        or coverage.get('argv_predicate') is not None or set(e.get('oracle_requirements',[]))!={'/fixture/expect/'+key for key in fields}
        or len(checks)!=len(expected) or len({c.get('actual') for c in checks})!=len(checks)
        or any(c.get('op')!='eq' or c.get('actual') not in expected or not _same(c.get('expected'),expected[c['actual']]) for c in checks)):
        raise Blocked('Additional session assertions, scope or prerequisites are not implemented')
    return {'profile':name,'actions':actions,'base':base,'fields':fields,'auth':actions==['auth.status'],
            'writes_setting':'mutation.set' in actions,'refusal':name=='mutation-no-setting-authorization'}


def supports(exercise):
    try:contract(exercise);return True
    except (Blocked,ValueError,KeyError,TypeError):return False


def authorized(req):
    constraints=req.get('run_constraints',{})
    if constraints.get('mutation_setting_changes_authorized') is not True or constraints.get('mutation_setting_change_scope')!='disposable_test_containers':
        raise Blocked('Mutation setting fixtures require explicit permission confined to disposable test containers')


def policy_facts(saved):
    if type(saved) is not bool:raise Blocked('Independent site consent is invalid or unavailable')
    return {'saved_enabled':saved,'effective':saved,'source':'saved_site_setting',
            'server_url':SERVER,'site_content_url':SITE}


def site_setting(enabled):
    return {'server_url':SERVER,'site_content_url':SITE,'enabled':enabled}


def unrelated_setting():
    return {'server_url':SERVER,'site_content_url':OTHER_SITE,'enabled':True}


def selected_consent(config):
    settings=config.get('site_mutations',[])
    if(not isinstance(settings,list) or len(settings)!=2 or not _same(settings[1],unrelated_setting())
       or not isinstance(settings[0],dict) or settings[0].get('server_url')!=SERVER
       or settings[0].get('site_content_url')!=SITE or type(settings[0].get('enabled')) is not bool):
        raise ValueError('Disposable configuration lacks exact selected and unrelated site consent')
    return settings[0]['enabled']


def preflight(req,frozen):
    spec=contract(req['exercise'])
    if not spec['auth']:authorized(req)
    override=os.environ.get('TADX_ENABLE_MUTATIONS')
    if override not in (None,'0','1'):raise Blocked('Inherited mutation override is invalid')
    rows=load(Path(frozen['root'])/'registry.json')
    if not isinstance(rows,list):rows=rows.get('capabilities',rows.get('items',[]))
    for action in spec['actions']:
        match=[row for row in rows if row['id']==action]
        if len(match)!=1 or match[0].get('command_path')!=PATHS[action] or match[0].get('remote_mutation') is not (action=='project.create'):
            raise Blocked('Frozen registry does not expose the exact session capability')
    return {'qualified_requirements':['network_audit'] if spec['profile']=='auth-status-offline' else [],'runtime_network_required':'none'}


def _rules(spec):
    rules=[]
    for action in spec['actions']:
        flags={};allowed=['json','full']
        if action=='auth.status':flags['environment']=ENVIRONMENT
        if action in ('mutation.status','mutation.set'):flags['environment']=ENVIRONMENT
        if action=='mutation.set':flags['enabled']=spec['profile']=='mutation.set'
        if action=='project.create':flags.update(environment=ENVIRONMENT,name=PROJECT)
        rules.append({'capability':action,'words':PATHS[action],'flags':flags,'allowed_flags':sorted(set(allowed)|set(flags))})
    return rules


def prepare(req,frozen,work):
    preflight(req,frozen);spec=contract(req['exercise']);work=Path(work);work.mkdir(parents=True,exist_ok=True)
    for entry in work.iterdir():
        if entry.is_symlink() or not ((entry.name=='deliverables' and entry.is_dir() and not any(entry.iterdir()))
            or(entry.name=='sentinel.txt' and entry.is_file() and entry.read_bytes()==b'protected fixture sentinel\n')):
            raise Blocked('Session fixtures require a new isolated work directory')
    (work/'deliverables').mkdir(exist_ok=True)
    if not(work/'sentinel.txt').exists():(work/'sentinel.txt').write_bytes(b'protected fixture sentinel\n')
    target=profile(SITE,PREFIX);target.pop('default_workspace')
    unrelated=profile(OTHER_SITE,OTHER_PREFIX);unrelated.pop('default_workspace')
    seed={'version':1,'default_environment':ENVIRONMENT,
          'environments':{ENVIRONMENT:target,OTHER_ENVIRONMENT:unrelated}}
    override=os.environ.get('TADX_ENABLE_MUTATIONS')
    if spec['profile'] in ('mutation-env-precedence','mutation-no-setting-authorization'):override='1'
    if not spec['auth']:
        seed['site_mutations']=[site_setting(spec['profile'] in ('mutation.status','mutation-env-precedence')),
                                unrelated_setting()]
        # Legacy values deliberately conflict with the selected site's consent.
        seed['mutations_enabled']=spec['profile'] in ('mutation.set','mutation-no-setting-authorization')
    desired=copy.deepcopy(seed)
    if spec['writes_setting']:desired['site_mutations'][0]=site_setting(spec['profile']=='mutation.set')
    if spec['auth']:facts={'ready':False,'source':'none','network_requests':0}
    else:facts=policy_facts(selected_consent(desired))
    if spec['refusal']:facts={**facts,'refused':True,'policy_unchanged':True}
    expected={key:facts[key] for key in spec['fields']} if spec['base'] else {'result':facts}
    before={'protected':_snapshot(work)}
    if spec['refusal']:expected['canonical']={'config':seed,'files':before['protected']}
    public={'site':ENVIRONMENT,'task_inputs':{'environment':ENVIRONMENT,'operations':spec['actions']}}
    if not spec['auth']:public['task_inputs'].update(server_url=SERVER,site_content_url=SITE)
    if spec['writes_setting']:public['task_inputs']['saved_enabled']=selected_consent(desired)
    if spec['refusal']:public['task_inputs']['project_name']=PROJECT
    guard={'mode':'session','profile':spec['profile'],'environment':ENVIRONMENT,'process_policy':override,
           'server_url':SERVER,'site_content_url':SITE,'unrelated_environment':OTHER_ENVIRONMENT,
           'unrelated_site_content_url':OTHER_SITE,
           'initial_site_consent':None if spec['auth'] else selected_consent(seed),'commands':_rules(spec)}
    fixture={'handle':req['case_id'],'public':public,'roles':{'target':{'environment':ENVIRONMENT},'sentinel':{'path':'sentinel.txt'},
             'isolated_config':{'scope':'disposable_cli_container'},'credentials':{'source':'none','material_present':False},
             'mutation_overlay':{'process':override}},'expect':expected,
             'expected_bindings_provenance':{'/fixture/expect/'+key:{'phase':'setup','source_kind':'independent_baseline','evidence':[BASELINE]} for key in expected},
             'coverage_assertions':[{'id':'native-session-task-and-policy','actual':'/after/native_command_verified','op':'eq','expected':True}]}
    fixture['semantic_digest']=digest({'public':public,'expected':expected,'before':before})
    baseline={'spec':spec,'guard':guard,'config':seed,'desired_config':desired,'expected_facts':facts,'before':before,'fixture':fixture}
    save(Path(req['private_case_dir'])/BASELINE,baseline)
    result={'fixture':fixture,'before':before,'config_seed':seed,'broker_guard':guard,'runtime_network':'none','harness_report_required':False,
            'baseline_evidence':[BASELINE],'baseline_assertions':[{'id':'empty-credential-source-and-independent-policy','status':'pass'}]}
    return result


def qualification_commands(req,fixture):
    base=load(Path(req['private_case_dir'])/BASELINE)
    if not _same(fixture,base['fixture']) or not _same(contract(req['exercise']),base['spec']):raise Blocked('Session qualification fixture changed')
    rules=sorted(base['guard']['commands'],key=lambda rule:0 if rule['capability']=='mutation.set' else 1)
    commands=[]
    for rule in rules:
        argv=list(rule['words'])
        for key,value in rule['flags'].items():argv+=['--'+key+'='+str(value).lower()] if type(value) is bool else ['--'+key,value]
        commands.append(argv+['--json','--full'])
    return commands


def task_instructions(exercise,public):
    spec=contract(exercise);adapted=copy.deepcopy(exercise)
    prompt=adapted['agent']['prompt']
    if hashlib.sha256(prompt.encode('utf-8')).hexdigest()!=PROMPT_SHA256[spec['profile']]:
        raise Blocked('Changed session prompt requires review of task and report ownership')
    sentence=' Save the credential source and readiness, without secret values, to deliverables/result.json.'
    if sentence in adapted['agent']['prompt']:
        adapted['agent']['prompt']=adapted['agent']['prompt'].replace(sentence,' Include credential source and readiness without secret values.',1)
    return {'status':'separated','exercise':adapted,'public':copy.deepcopy(public),'harness_report':None,'issues':[]}


def _parsed(command):
    argv=['--enabled=true' if value=='--enabled' else value for value in command['argv']]
    words,flags=parse_arguments(argv)
    if flags.get('enabled') in ('true','false'):flags['enabled']=flags['enabled']=='true'
    if not _same(flags,command['flags']):raise ValueError('Native session argv and flags disagree')
    return words,flags


def command_matches(command,rule):
    try:
        words,flags=_parsed(command)
        return(command['capability']==rule['capability'] and words==rule['words'] and not(set(flags)-set(rule['allowed_flags']))
               and (not rule['capability'].startswith('mutation.') or flags.get('json') is True and flags.get('full') is True)
               and all(type(flags[key]) is bool for key in ('json','full','enabled') if key in flags)
               and all(_same(flags.get(key),value) or key=='environment' and key not in flags
                       and rule['capability']=='auth.status' for key,value in rule['flags'].items()))
    except (KeyError,ValueError,TypeError):return False


def auth_facts(data):
    expected={'status':'incomplete','environment':ENVIRONMENT,'server_url':'https://example.test','site_content_url':'session-fixture',
              'credential_source':'none','pat_name_present':False,'pat_secret_present':False,'stored_credential_reference_present':False}
    if not isinstance(data,dict) or any(not _same(data.get(key),value) for key,value in expected.items()):
        raise ValueError('Native credential readiness does not match the independently empty source')
    return {'ready':data['status']=='ready','source':data['credential_source'],'network_requests':0}


def _auth_document(text):
    if not isinstance(text,str) or len(text.encode('utf-8'))>2*1024*1024:raise ValueError('Native authentication status is missing or oversized')
    def unique(pairs):
        result={}
        for key,value in pairs:
            if key in result:raise ValueError('Duplicate native authentication field')
            result[key]=value
        return result
    def nonfinite(_):raise ValueError('Nonfinite native authentication field')
    data=json.loads(text,object_pairs_hook=unique,parse_constant=nonfinite) if text.lstrip().startswith(('{','[')) else decode_native_toon(text)
    if not isinstance(data,dict) or any(data.get(key) for key in ('error','errors','failures')):
        raise ValueError('Native authentication status contains an operation error')
    return data


def mutation_facts(data,action):
    if action=='mutation.status':
        sites=data.get('sites') if isinstance(data,dict) else None
        if not isinstance(sites,list) or len(sites)!=1:raise ValueError('Native status lacks one selected site')
        setting=sites[0]
    else:setting=data
    if(not isinstance(setting,dict) or setting.get('environment')!=ENVIRONMENT
       or setting.get('server_url')!=SERVER or setting.get('site_content_url')!=SITE
       or type(setting.get('enabled')) is not bool or setting.get('source')!='saved_site_setting'):
        raise ValueError('Native consent does not identify the exact canonical site')
    if action=='mutation.set' and (setting.get('saved_enabled') is not setting['enabled']
       or setting.get('persisted') is not True or setting.get('scope')!='site'
       or setting.get('source_setting')!='site_mutations'):
        raise ValueError('Native site consent was not confirmed persisted')
    return policy_facts(setting['enabled'])


def observe(req,state,config,work,commands):
    _audit(req,state,commands);private=Path(req['private_case_dir']);base=load(private/BASELINE);spec=contract(req['exercise'])
    if not _same(spec,base['spec']):raise Blocked('Session contract changed after preparation')
    delivery=load(private/'delivery-evidence.json');runtime=delivery.get('cli_session_runtime',{})
    if(runtime.get('status')!='verified' or runtime.get('credential_material_present') is not False
       or runtime.get('network_none') is not True or runtime.get('process_policy')!=base['guard']['process_policy']):
        raise Blocked('Actual credential-free CLI runtime and process policy evidence are required')
    if not spec['auth'] and (runtime.get('server_url')!=SERVER or runtime.get('site_content_url')!=SITE
        or runtime.get('initial_site_consent') is not base['guard']['initial_site_consent']):
        raise Blocked('Native runtime lacks the prepared exact-site consent')
    verified={};facts=None;issues=[]
    for command in commands:
        rule=next((rule for rule in base['guard']['commands'] if command_matches(command,rule)),None)
        if not rule or not command.get('executed'):continue
        action=rule['capability']
        if action=='project.create':
            errors=[decode_error(command.get(stream,'')) for stream in ('stdout','stderr')]
            if command['exit_status'] not in (0,124,125) and any(isinstance(e,dict) and e.get('id')=='mutation.disabled'
                and e.get('kind')=='operation' and e.get('operation')=='project.create'
                and e.get('summary')=='Remote mutation execution is disabled.' and e.get('retryable') is False for e in errors):verified[action]=command['evidence']
            continue
        if command['exit_status']!=0:continue
        try:
            data=_auth_document(command['stdout']) if spec['auth'] else _native(command['stdout'])[0]
            actual=auth_facts(data) if spec['auth'] else mutation_facts(data,action)
            expected={key:base['expected_facts'][key] for key in actual}
            if _same(actual,expected):verified[action]=command['evidence'];facts=actual
            else:issues.append({'command_id':command['id'],'reason':'Native setting facts differ from the frozen independent target'})
        except (ValueError,KeyError,TypeError) as error:issues.append({'command_id':command['id'],'reason':str(error)})
    snapshot=_snapshot(work);changes=sorted(key for key in set(snapshot)|set(base['before']['protected']) if snapshot.get(key)!=base['before']['protected'].get(key))
    if not _same(config,base['desired_config']):changes.append('config:unrequested-or-missing-change')
    complete=set(verified)==set(spec['actions'])
    if spec['auth']:result=facts
    else:
        try:result=policy_facts(selected_consent(config))
        except ValueError:result={'saved_enabled':None,'effective':None,'source':'unavailable'}
        if spec['refusal']:result.update(refused='project.create' in verified,policy_unchanged=config==base['config'])
    native={'status':'verified' if complete else 'not_verified','evidence':list(verified.values()),
            'reason':'Exact native session command and independently qualified settings'}
    after={'result':result,'protected':snapshot,'unowned_changes':changes,'canonical':{'config':config,'files':snapshot},
           'native_command_verified':complete}
    # Local readiness can truthfully be "incomplete" after a successful read.
    # Its original facts and exact native coverage assertions own that outcome.
    if not spec['refusal'] and not spec['auth']:after.update(command_evidence=copy.deepcopy(native),native_command_evidence=native)
    save(private/OBSERVATION,{'after':after,'diagnostics':issues,'observed_snapshot':snapshot,'release_safe':not changes})
    return after


def cleanup(req,state,work=None):
    from integration.local_recovery import cleanup as release_local
    return release_local(req,state,work)
