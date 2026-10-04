// Credential-free session diagnostics and explicitly scoped disposable settings.
const fs=require('node:fs');
const ENVIRONMENT='session-local',OTHER_ENVIRONMENT='session-unrelated';
const PREFIX='BENCH_SESSION_MISSING',OTHER_PREFIX='BENCH_SESSION_UNRELATED_MISSING';
const PROJECT='session-policy-probe';
const SERVER='https://example.test',SITE='session-fixture',OTHER_SITE='session-unrelated';
const PATHS={'auth.status':['auth','status'],'mutation.status':['mutation','status'],
  'mutation.set':['mutation','set'],'project.create':['content','project','create']};
const VARIANTS={'auth-status-offline':['auth.status'],'mutation-env-precedence':['mutation.status','mutation.set'],
  'mutation-no-setting-authorization':['mutation.status','project.create']};
const own=(o,k)=>Object.hasOwn(o,k),same=(a,b)=>JSON.stringify(a)===JSON.stringify(b);

function extendParser(parser){parser.booleans.add('enabled');parser.values.add('environment');parser.values.add('name');}

function validGuard(guard){
  if(guard?.mode!=='session'||guard.environment!==ENVIRONMENT||![null,'0','1'].includes(guard.process_policy)
     ||guard.server_url!==SERVER||guard.site_content_url!==SITE
     ||guard.unrelated_environment!==OTHER_ENVIRONMENT||guard.unrelated_site_content_url!==OTHER_SITE
     ||(guard.profile!=='auth.status'&&guard.profile!=='auth-status-offline'&&typeof guard.initial_site_consent!=='boolean')
     ||!Array.isArray(guard.commands))return false;
  const actions=VARIANTS[guard.profile]||(['auth.status','mutation.status','mutation.set'].includes(guard.profile)?[guard.profile]:null);
  if(!actions||actions.length!==guard.commands.length)return false;
  const initial={'mutation.status':true,'mutation.set':false,
    'mutation-env-precedence':true,'mutation-no-setting-authorization':false};
  if(own(initial,guard.profile)&&guard.initial_site_consent!==initial[guard.profile])return false;
  if(['auth.status','auth-status-offline'].includes(guard.profile)&&guard.initial_site_consent!==null)return false;
  if(guard.profile==='mutation-env-precedence'&&guard.process_policy!=='1'
     ||guard.profile==='mutation-no-setting-authorization'&&guard.process_policy!=='1')return false;
  return actions.every((action,index)=>{
    const command=guard.commands[index],flags={};
    if(action==='auth.status')flags.environment=ENVIRONMENT;
    if(action==='mutation.status'||action==='mutation.set')flags.environment=ENVIRONMENT;
    if(action==='mutation.set')flags.enabled=guard.profile==='mutation.set';
    if(action==='project.create')Object.assign(flags,{environment:ENVIRONMENT,name:PROJECT});
    return command?.capability===action&&same(command.words,PATHS[action])&&command.flags&&typeof command.flags==='object'
      &&Object.keys(command.flags).length===Object.keys(flags).length&&Object.entries(flags).every(([key,value])=>same(command.flags[key],value))
      &&Array.isArray(command.allowed_flags)&&same([...command.allowed_flags].sort(),['json','full',...Object.keys(flags)].sort());
  });
}

function allowed(parsed,cap,state){
  const guard=state?.guard;
  if(!validGuard(guard)||state.networkNone!==true||state.baselinePresent!==true||state.guardPresent!==true
    ||!Array.isArray(state.registry)||!parsed||!Array.isArray(parsed.errors)||parsed.errors.length
    ||!Array.isArray(parsed.words)||!parsed.flags||Array.isArray(parsed.flags)||fs.existsSync('/cli-state/credentials.json'))return false;
  let words=parsed.words,flags={...parsed.flags};
  if(words[0]==='help'){words=words.slice(1);flags.help=true;}
  if(flags.help===true&&Object.keys(flags).length===1)
    return guard.commands.some(c=>words.length<=c.words.length&&words.every((word,index)=>word===c.words[index]));
  const rule=guard.commands.find(rule=>rule.capability===cap);
  if(!rule||!same(words,rule.words))return false;
  const rows=state.registry.filter(row=>row.id===cap);
  if(rows.length!==1||rows[0].remote_mutation!==(cap==='project.create')||!same(rows[0].command_path,PATHS[cap]))return false;
  if(cap==='project.create'&&(guard.profile!=='mutation-no-setting-authorization'
     ||guard.initial_site_consent!==false))return false;
  if(cap.startsWith('mutation.')&&(flags.json!==true||flags.full!==true))return false;
  if(Object.entries(flags).some(([key,value])=>!rule.allowed_flags.includes(key)
    ||(['json','full','enabled'].includes(key)?typeof value!=='boolean':typeof value!=='string'||!value||value.length>256)))return false;
  return Object.entries(rule.flags).every(([key,value])=>
    key==='environment'&&cap==='auth.status'&&!own(flags,key)||same(flags[key],value));
}

function runtimeEvidence(state){
  const guard=state?.guard;
  if(!validGuard(guard)||state.networkNone!==true||state.baselinePresent!==true||state.baselineUnchanged!==true||state.guardPresent!==true)
    throw Error('Session runtime evidence requires the unchanged private seed');
  const config=JSON.parse(fs.readFileSync('/cli-state/tadx/config.yaml','utf8'));
  const target=config.environments?.[ENVIRONMENT],auth=target?.auth;
  const unrelated=config.environments?.[OTHER_ENVIRONMENT],unrelatedAuth=unrelated?.auth;
  const expected={type:'pat',pat_name_env:PREFIX+'_NAME',pat_secret_env:PREFIX+'_SECRET'};
  const expectedUnrelated={type:'pat',pat_name_env:OTHER_PREFIX+'_NAME',pat_secret_env:OTHER_PREFIX+'_SECRET'};
  if(config.default_environment!==ENVIRONMENT||Object.keys(config.environments||{}).length!==2
    ||target?.url!==SERVER||target?.site_content_url!==SITE
    ||!auth||Object.keys(auth).length!==3||Object.entries(expected).some(([key,value])=>auth[key]!==value)
    ||unrelated?.url!==SERVER||unrelated?.site_content_url!==OTHER_SITE
    ||!unrelatedAuth||Object.keys(unrelatedAuth).length!==3
    ||Object.entries(expectedUnrelated).some(([key,value])=>unrelatedAuth[key]!==value)
    ||fs.existsSync('/cli-state/credentials.json')||process.env[PREFIX+'_NAME']!==undefined
    ||process.env[PREFIX+'_SECRET']!==undefined||process.env[OTHER_PREFIX+'_NAME']!==undefined
    ||process.env[OTHER_PREFIX+'_SECRET']!==undefined)
    throw Error('Session fixture must have only its exact absent credential references');
  const policy=process.env.TADX_ENABLE_MUTATIONS??null;
  if(policy!==guard.process_policy)throw Error('Actual CLI process override differs from the session fixture');
  const settings=config.site_mutations;
  if(guard.profile==='auth.status'||guard.profile==='auth-status-offline'){
    if(settings!==undefined)throw Error('Authentication fixture unexpectedly has site consent');
  }else if(!Array.isArray(settings)||settings.length!==2||Object.keys(settings[0]||{}).length!==3
    ||settings[0]?.server_url!==SERVER||settings[0]?.site_content_url!==SITE
    ||settings[0]?.enabled!==guard.initial_site_consent||Object.keys(settings[1]||{}).length!==3
    ||settings[1]?.server_url!==SERVER||settings[1]?.site_content_url!==OTHER_SITE
    ||settings[1]?.enabled!==true)
    throw Error('Session fixture lacks exact canonical site consent');
  return {status:'verified',network_none:true,credential_material_present:false,process_policy:policy,
          configured_environment:ENVIRONMENT,server_url:SERVER,site_content_url:SITE,
          initial_site_consent:guard.initial_site_consent,
          credential_references:{name:PREFIX+'_NAME',secret:PREFIX+'_SECRET'}};
}

module.exports={allowed,validGuard,extendParser,runtimeEvidence};
