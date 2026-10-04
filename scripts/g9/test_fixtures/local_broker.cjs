// Trusted container process. The non-root agent can invoke, but cannot read or
// execute, the frozen CLI. No credentials or evaluator inputs enter this image.
const http = require('node:http');
const fs = require('node:fs');
const cp = require('node:child_process');
const crypto = require('node:crypto');
const path = require('node:path');
const localStateBroker = require('./local_state_broker.cjs');
const contentBroker = require('./content_broker.cjs');
const guidanceBroker = require('./guidance_broker.cjs');
const pulseBroker = require('./pulse_broker.cjs');
const pulseCreationBroker = require('./pulse_creation_broker.cjs');
const pulseFilterBroker = require('./pulse_filter_broker.cjs');
const pulsePeriodBroker = require('./pulse_period_broker.cjs');
const lineageBroker = require('./lineage_broker.cjs');
const permissionBroker = require('./permission_broker.cjs');
const inventoryBroker = require('./inventory_broker.cjs');
const diagnosticsBroker = require('./diagnostics_broker.cjs');
const workspaceBroker = require('./workspace_broker.cjs');
const contentPreviewBroker = require('./content_preview_broker.cjs');
const sessionBroker = require('./session_broker.cjs');
const lastBroker = require('./last_broker.cjs');
const catalogBroker = require('./catalog_broker.cjs');
const projectBroker = require('./project_broker.cjs');
const sharedBroker = require('./shared_broker.cjs');
const contentMoveBroker = require('./content_move_broker.cjs');
const schemaBroker = require('./schema_broker.cjs');
const pulsePublishBroker = require('./pulse_publish_broker.cjs');
const SOCKET_DIRECTORY = '/run/tadx-broker';
const SOCKET_PATH = SOCKET_DIRECTORY + '/invoke.sock';
let registry = [];
const aliases = {a:'all',l:'limit',lim:'limit',jsn:'json',f:'full',ful:'full',pv:'preview',p:'preview',h:'help',
  e:'environment',env:'environment',cfg:'config',i:'id',mdi:'metadata-id',tbi:'table-id',dbi:'database-id',n:'name',btf:'batch-file'};
const booleans = new Set(['all','json','full','preview','help','cache','check','force','clear-cache-max-concurrency',
  'clear-api-version','clear-default-workspace','clear-pat-name-env','clear-pat-secret-env','clear-site']);
const values = new Set(['environment','config','id','metadata-id','table-id','database-id','name','limit','batch-file',
  'description','add-tag','remove-tag','contact-id','url','site','api-version','default-workspace','pat-name-env','pat-secret-env',
  'cache-max-concurrency','path','workspace','owner','project','project-name','project-id']);
contentBroker.extendParser({aliases,booleans,values});
pulseBroker.extendParser({aliases,booleans,values});
pulseCreationBroker.extendParser({aliases,booleans,values});
pulseFilterBroker.extendParser({aliases,booleans,values});
pulsePeriodBroker.extendParser({aliases,booleans,values});
lineageBroker.extendParser({aliases,booleans,values});
permissionBroker.extendParser({aliases,booleans,values});
inventoryBroker.extendParser({aliases,booleans,values});
diagnosticsBroker.extendParser({aliases,booleans,values});
workspaceBroker.extendParser({aliases,booleans,values});
sessionBroker.extendParser({aliases,booleans,values});
projectBroker.extendParser({aliases,booleans,values});
catalogBroker.extendParser({aliases,booleans,values});
sharedBroker.extendParser({aliases,booleans,values});
contentMoveBroker.extendParser({aliases,booleans,values});
schemaBroker.extendParser({aliases,booleans,values});
pulsePublishBroker.extendParser({aliases,booleans,values});
values.add('target');
function parseArgs(args) {
  const flags=Object.create(null), words=[], errors=[];
  function append(key,value) {
    if(Object.hasOwn(flags,key))flags[key]=Array.isArray(flags[key])?[...flags[key],value]:[flags[key],value];
    else flags[key]=value;
  }
  for(let i=0;i<args.length;i++) {
    if(args[i]==='--'){words.push(...args.slice(i+1));break;}
    if(!args[i].startsWith('-')){words.push(args[i]);continue;}
    const match=/^-{1,2}([a-z][a-z-]*)(?:=([\s\S]*))?$/.exec(args[i]);
    if(!match){errors.push('unsupported flag spelling');continue;}
    const key=aliases[match[1]]||match[1];let value=match[2];
    if(booleans.has(key)&&!(key==='check'&&words[0]==='catalog'&&words[1]==='audit')){
      if(value!==undefined&&!['true','false'].includes(value))errors.push('invalid boolean value');
      append(key,value===undefined||value==='true'?true:value==='false'?false:value);continue;
    }
    if(value===undefined&&i+1<args.length&&(values.has(key)||!args[i+1].startsWith('-')))value=args[++i];
    if(value===undefined){errors.push('missing flag value');value=null;}
    append(key,['limit','cache-max-concurrency','depth'].includes(key)&&typeof value==='string'&&/^\d+$/.test(value)?Number(value):value);
  }
  return {flags,words,errors};
}
function flagsOf(args) { return parseArgs(args).flags; }
function helpOnly(parsed) { return !parsed.errors.length&&(parsed.flags.help===true||parsed.words[0]==='help'); }
function classify(args, entries=registry) {
  const parsed=parseArgs(args);
  if (helpOnly(parsed)) return null;
  if (parsed.words[0] === 'workspace' && parsed.words[1] === 'move') return 'workspace.move';
  if (['version','ver'].includes(args[0]) || ['--version','-v'].includes(args[0])) return 'version.get';
  if (['capability','cap'].includes(args[0]) && ['list','ls'].includes(args[1])) return 'capability.list';
  if (['capability','cap'].includes(args[0]) && ['get','inspect'].includes(args[1])) return 'capability.get';
  for (const row of [...entries].sort((a,b)=>(b.command_path||[]).length-(a.command_path||[]).length)) { if (row.command_path && row.command_path.every((v,i)=>(sharedBroker.wordAliases[parsed.words[i]]||parsed.words[i])===v)) return row.id; }
  return null;
}
function diskState() {
  const state={registry,baselinePresent:fs.existsSync('/cli-state/target-baseline.json'),baselineUnchanged:false,
    guardPresent:fs.existsSync('/cli-state/task-policy.json'),guard:null,defaultEnvironment:null,configuredEnvironments:[],
    networkNone:process.env.BENCH_NETWORK_MODE==='none',
    mutationDisabled:process.env.TADX_ENABLE_MUTATIONS==='0'};
  try {
    const config=JSON.parse(fs.readFileSync('/cli-state/tadx/config.yaml','utf8'));
    state.defaultEnvironment=config.default_environment;
    state.configuredEnvironments=Object.keys(config.environments||{});
    if(state.baselinePresent)state.baselineUnchanged=JSON.stringify(config)===JSON.stringify(JSON.parse(fs.readFileSync('/cli-state/target-baseline.json','utf8')));
  } catch {}
  if(state.guardPresent)try{state.guard=JSON.parse(fs.readFileSync('/cli-state/task-policy.json','utf8'));}catch{}
  return state;
}
function scalar(value) { return typeof value==='string'&&value.length>0&&!value.includes('\0'); }
function validGuard(guard) {
  if(!guard||typeof guard!=='object'||!scalar(guard.environment)||!/^catalog\.(database|table|column)\.(list|inspect|update)$/.test(guard.capability||''))return false;
  const [,kind,action]=guard.capability.split('.'),target=guard.target,input=guard.task_inputs;
  if(!target||target.kind!==kind||!scalar(target.id)||!scalar(target.metadata_id)||!input||input.operation!==guard.capability)return false;
  if(!input.target||input.target.kind!==kind||input.target.id!==target.id)return false;
  for(const parent of ['table_id','database_id'])if(Object.hasOwn(input.target,parent)&&input.target[parent]!==target[parent])return false;
  if(kind==='column'&&!scalar(target.table_id))return false;
  if(action==='update'){
    if(Object.hasOwn(input,'description')&&typeof input.description!=='string')return false;
    for(const key of ['add_tags','remove_tags'])if(Object.hasOwn(input,key)&&(!Array.isArray(input[key])||input[key].some(tag=>!scalar(tag))||new Set(input[key]).size!==input[key].length))return false;
    if(!Object.hasOwn(input,'description')&&!(input.add_tags||[]).length&&!(input.remove_tags||[]).length)return false;
    if((input.add_tags||[]).some(tag=>(input.remove_tags||[]).includes(tag)))return false;
  }
  return true;
}
function environmentOnGuard(parsed,state,mutation=false) {
  const environment=parsed.flags.environment;
  if(environment!==undefined)return typeof environment==='string'&&environment===state.guard.environment;
  return mutation?state.configuredEnvironments.length===1&&state.configuredEnvironments[0]===state.guard.environment:
    state.defaultEnvironment===state.guard.environment;
}
function metadataAllowed(parsed,cap,state) {
  const guard=state.guard,target=guard.target,input=guard.task_inputs,flags=parsed.flags;
  if(parsed.errors.length||!/^catalog\.(database|table|column)\.(list|inspect|update)$/.test(cap||''))return false;
  const [,kind,action]=cap.split('.');
  if(parsed.words.join(' ')!==`catalog ${kind} ${action}`||!environmentOnGuard(parsed,state,action==='update'))return false;
  const common=['environment','json','full'];
  const permitted=new Set([...common,...(action==='update'?['id','table-id','description','add-tag','remove-tag','preview']:
    action==='inspect'?['id','metadata-id','table-id']:['name','all','limit','database-id','table-id'])]);
  if(Object.keys(flags).some(key=>!permitted.has(key)))return false;
  if(Object.entries(flags).some(([key,value])=>Array.isArray(value)&&!['add-tag','remove-tag'].includes(key)))return false;
  for(const key of ['json','full','preview','all'])if(Object.hasOwn(flags,key)&&typeof flags[key]!=='boolean')return false;
  if(action==='update'){
    if(cap!==guard.capability||kind!==target.kind||flags.id!==target.id)return false;
    if(kind==='column'?flags['table-id']!==target.table_id:Object.hasOwn(flags,'table-id'))return false;
    let effects=0;
    if(Object.hasOwn(flags,'description')){
      if(!Object.hasOwn(input,'description')||flags.description!==input.description)return false;
      effects++;
    }
    for(const [flag,key] of [['add-tag','add_tags'],['remove-tag','remove_tags']])if(Object.hasOwn(flags,flag)){
      const tags=Array.isArray(flags[flag])?flags[flag]:[flags[flag]],expected=input[key]||[];
      if(!tags.length||new Set(tags).size!==tags.length||tags.some(tag=>!scalar(tag)||!expected.includes(tag)))return false;
      effects+=tags.length;
    }
    return effects>0;
  }
  const known={database:new Set(),table:new Set(),column:new Set()};
  known[target.kind].add(target.id);
  if(target.database_id)known.database.add(target.database_id);
  if(target.table_id)known.table.add(target.table_id);
  if(action==='inspect'){
    if(Object.hasOwn(flags,'id')===Object.hasOwn(flags,'metadata-id'))return false;
    if(Object.hasOwn(flags,'metadata-id'))return kind===target.kind&&flags['metadata-id']===target.metadata_id&&!Object.hasOwn(flags,'table-id');
    if(!known[kind].has(flags.id))return false;
    return kind==='column'?flags['table-id']===target.table_id:!Object.hasOwn(flags,'table-id');
  }
  if(flags.all===true&&Object.hasOwn(flags,'limit'))return false;
  if(Object.hasOwn(flags,'limit')&&(!Number.isInteger(flags.limit)||flags.limit<1||flags.limit>10000))return false;
  if(Object.hasOwn(flags,'name')&&typeof flags.name!=='string')return false;
  if(kind==='database')return !Object.hasOwn(flags,'database-id')&&!Object.hasOwn(flags,'table-id');
  if(kind==='table')return !Object.hasOwn(flags,'table-id')&&(!Object.hasOwn(flags,'database-id')||known.database.has(flags['database-id']));
  return !Object.hasOwn(flags,'database-id')&&known.table.has(flags['table-id']);
}
function validationAllowed(parsed,cap,state) {
  const guard=state.guard, predicate=guard?.predicate;
  if(!state.networkNone||guard?.mode!=='validation'||parsed.errors.length||!predicate||cap!==guard.capability)return false;
  const entry=(state.registry||registry).find(row=>row.id===cap);
  if(!entry||JSON.stringify(entry.command_path)!==JSON.stringify(guard.command_path))return false;
  if(cap==='mutation.set')return false;
  if(entry.remote_mutation===true&&(guard.validation_family!=='mutation-disabled'||state.mutationDisabled!==true))return false;
  if(guard.validation_family==='mutation-disabled'&&(state.mutationDisabled!==true
     ||(Object.hasOwn(parsed.flags,'preview')&&parsed.flags.preview!==false)))return false;
  if(JSON.stringify(parsed.words)!==JSON.stringify([...guard.command_path,...(guard.positionals||[])]))return false;
  if(state.baselinePresent&&!state.baselineUnchanged)return false;
  if(!Array.isArray(predicate.allowed_flags)||!Array.isArray(predicate.absent_flags)||!predicate.required_flags)return false;
  if(Object.keys(parsed.flags).some(key=>!predicate.allowed_flags.includes(key)))return false;
  if(predicate.absent_flags.some(key=>Object.hasOwn(parsed.flags,key)))return false;
  for(const [key,value] of Object.entries(predicate.required_flags)){
    if(key==='preview'&&value===false&&!Object.hasOwn(parsed.flags,key))continue;
    if(JSON.stringify(parsed.flags[key])!==JSON.stringify(value))return false;
  }
  for(const key of ['json','full'])if(Object.hasOwn(parsed.flags,key)&&typeof parsed.flags[key]!=='boolean')return false;
  return true;
}
function safeWorkspacePath(value) {
  if(typeof value!=='string'||value.includes('\0')||value.split(/[\\/]/).includes('..'))return false;
  const resolved=path.posix.resolve('/work',value);
  if(resolved!=='/work'&&!resolved.startsWith('/work/'))return false;
  let existing=resolved;
  while(!fs.existsSync(existing)&&existing!=='/')existing=path.posix.dirname(existing);
  try {const real=fs.realpathSync(existing);return real==='/work'||real.startsWith('/work/');}catch{return false;}
}
function allowed(args,state=diskState()) {
  const parsed=parseArgs(args),flags=parsed.flags;
  // Private disposable runs evaluate commands; they must not replace native
  // syntax/refusal behavior with a benchmark allowlist. Existing strict mode
  // remains available, but it is not used by Run-LocalSuite.ps1.
  if(state.guard?.execution_mode==='disposable_native'&&state.baselinePresent)return true;
  if (Object.hasOwn(flags,'config')) return false;
  if (args.some(x => x.split(/[\\/]/).includes('..') || (x.startsWith('/') && !x.startsWith('/work/')))) return false;
  for(const key of ['path','file','batch-file','output','workspace-path'])if(Object.hasOwn(flags,key)){
    const values=Array.isArray(flags[key])?flags[key]:[flags[key]];
    if(values.some(value=>!safeWorkspacePath(value)))return false;
  }
  const cap=classify(args,state.registry||registry), readCaps=['version.get','capability.list','capability.get',
    'auth.status','doctor.run','mutation.status','session.overview','env.profile.list','env.profile.get','workspace.list','workspace.status'];
  if(state.guardPresent&&state.guard?.mode==='validation'){
    if(!state.networkNone)return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return validationAllowed(parsed,cap,state);
  }
  const conflictLists=['workbook.list','datasource.list','flow.list','project.list','admin.user.list','admin.group.list','pulse.definition.list','pulse.metric.list'];
  if(state.guardPresent&&String(state.guard?.mode||'').startsWith('shared-'))return sharedBroker.allowed(parsed,cap,state);
  if(state.guardPresent&&state.guard?.mode==='guidance'){
    if(!state.networkNone||!state.baselinePresent||guidanceBroker.home(state)===null)return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return guidanceBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='diagnostics'){
    if(!state.networkNone||!state.baselinePresent||!diagnosticsBroker.validGuard(state.guard))return false;
    return diagnosticsBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='workspace'){
    if(!state.networkNone||!state.baselinePresent||!workspaceBroker.validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return workspaceBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='content-preview'){
    if(!state.baselinePresent||!state.baselineUnchanged||!contentPreviewBroker.validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    return contentPreviewBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='catalog'){
    return catalogBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='last'){
    return lastBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='session'){
    if(!state.networkNone||!state.baselinePresent||!sessionBroker.validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return sessionBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.family==='project'){
    if(!state.baselinePresent||!state.baselineUnchanged||!projectBroker.validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return projectBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&['lineage','permission','inventory-variant','pulse-period','pulse-creation','pulse-filter','schema','pulse-publish'].includes(state.guard?.mode)){
    const handler={'lineage':lineageBroker,'permission':permissionBroker,'inventory-variant':inventoryBroker,'pulse-period':pulsePeriodBroker,'pulse-creation':pulseCreationBroker,'pulse-filter':pulseFilterBroker,'schema':schemaBroker,'pulse-publish':pulsePublishBroker}[state.guard.mode];
    if(!state.baselinePresent||!state.baselineUnchanged||!handler.validGuard(state.guard))return false;
    if(helpOnly(parsed)||(!parsed.words.length&&!Object.keys(parsed.flags).length))return true;
    if(cap==='version.get'&&parsed.words.join(' ')==='version'&&Object.keys(parsed.flags).every(key=>['json','full'].includes(key)))return true;
    return handler.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='pulse'){
    if(!state.baselinePresent||!state.baselineUnchanged||!pulseBroker.validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return pulseBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.mode==='local-state'){
    if(!state.networkNone||!state.baselinePresent)return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return localStateBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.family==='content-move'){
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return contentMoveBroker.allowed(parsed,cap,state);
  }
  if(state.guardPresent&&state.guard?.family==='content'){
    if(!state.baselinePresent||!state.baselineUnchanged||!contentBroker.validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(['version.get','capability.list','capability.get'].includes(cap))return true;
    return contentBroker.allowed(parsed,cap,state);
  }
  if(!state.baselinePresent&&!state.guardPresent&&conflictLists.includes(cap)&&flags.all===true&&Object.hasOwn(flags,'limit'))return true;
  if(!state.baselinePresent&&!state.guardPresent&&cap==='env.profile.update'&&flags['clear-cache-max-concurrency']===true&&Object.hasOwn(flags,'cache-max-concurrency'))return true;
  if (state.baselinePresent) {
    if(!state.baselineUnchanged)return false;
    readCaps.push('workbook.list','datasource.list','flow.list','project.list','admin.user.list','admin.group.list',
      'workbook.inspect','datasource.inspect','flow.inspect','auth.check');
  } else readCaps.push('env.profile.add','env.profile.list','env.profile.get','workspace.create','workspace.list','workspace.status');
  if(state.guardPresent){
    if(!state.baselinePresent||!validGuard(state.guard))return false;
    if(args.length===0||helpOnly(parsed))return true;
    if(parsed.errors.length||Array.isArray(flags.id)||Array.isArray(flags.environment))return false;
    if(/^catalog\./.test(cap||''))return metadataAllowed(parsed,cap,state);
    if(!environmentOnGuard(parsed,state))return false;
  }
  return args.length===0||helpOnly(parsed)||readCaps.includes(cap);
}
function startBroker() {
  fs.mkdirSync('/audit', {recursive:true, mode:0o700});
  fs.chmodSync('/audit', 0o700);
  for (const dir of ['/tmp/cli-home','/cli-state','/cli-state/tadx','/cli-state/data']) { fs.mkdirSync(dir,{recursive:true,mode:0o700}); fs.chmodSync(dir,0o700); }
  fs.mkdirSync('/tmp/cli-home/.agents',{recursive:true,mode:0o700});
  fs.symlinkSync('/skills','/tmp/cli-home/.agents/skills','dir');
  const binarySHA=crypto.createHash('sha256').update(fs.readFileSync('/opt/tadx')).digest('hex');
  registry=JSON.parse(fs.readFileSync('/opt/registry.json','utf8'));
  let sequence=0;
  const server=http.createServer((req,res) => {
  if (req.method !== 'POST' || req.url !== '/invoke') { res.writeHead(404); res.end(); return; }
  let bytes=0, chunks=[];
  req.on('data', chunk => { bytes+=chunk.length; if(bytes>1048576) req.destroy(); else chunks.push(chunk); });
  req.on('end', () => {
    let body; try { body=JSON.parse(Buffer.concat(chunks)); } catch { res.writeHead(400);res.end();return; }
    const args=body.argv;
    const cwd=body.cwd===undefined?'/work':body.cwd;
    if(typeof cwd!=='string'||!(cwd==='/work'||cwd.startsWith('/work/'))||cwd.split('/').includes('..')){res.writeHead(400);res.end();return;}
    if (!Array.isArray(args) || args.length>1000 || args.some(x=>typeof x!=='string' || x.includes('\0'))) {res.writeHead(400);res.end();return;}
    const id='cmd-'+String(++sequence).padStart(5,'0'), started_at=new Date().toISOString(), start=process.hrtime.bigint();
    const policyState=diskState();
    let result, executed=allowed(args,policyState);
    const installer=body.helper==='installer'&&policyState.guard?.mode==='shared-installer';
    if(body.helper&&!installer){res.writeHead(400);res.end();return;}
    if(installer)executed=crypto.createHash('sha256').update(fs.readFileSync('/work/installer-assets/install.sh')).digest('hex')===policyState.guard.installer_sha256;
    const taskBinary=policyState.guard?.mutable_binary==='/cli-state/update/tadx'?'/cli-state/update/tadx':'/opt/tadx';
    const executedBinarySHA=executed?crypto.createHash('sha256').update(fs.readFileSync(taskBinary)).digest('hex'):binarySHA;
    const env={PATH:'/usr/local/bin:/usr/bin:/bin',HOME:'/tmp/cli-home',XDG_CONFIG_HOME:'/cli-state',XDG_DATA_HOME:'/cli-state/data',TADX_FEEDBACK_MODE:'off'};
    if (fs.existsSync('/cli-state/credentials.json')) Object.assign(env,JSON.parse(fs.readFileSync('/cli-state/credentials.json','utf8')));
    if ('TADX_ENABLE_MUTATIONS' in process.env) env.TADX_ENABLE_MUTATIONS=process.env.TADX_ENABLE_MUTATIONS;
    if(fs.existsSync('/cli-state/credential-runtime.json'))Object.assign(env,JSON.parse(fs.readFileSync('/cli-state/credential-runtime.json','utf8')));
    if(fs.existsSync('/cli-state/fault-env.json'))Object.assign(env,JSON.parse(fs.readFileSync('/cli-state/fault-env.json','utf8')));
    if(executed&&policyState.guard?.mode==='shared-local'&&policyState.guard?.home){env.HOME=policyState.guard.home;env.TADX_GUIDANCE_NOTICE='0';}
    if(executed&&policyState.guard?.mode==='guidance'){
      const taskHome=guidanceBroker.home(policyState);
      if(taskHome===null)executed=false;
      else {env.HOME=taskHome;env.TADX_GUIDANCE_NOTICE='0';}
    }
    let batchInput;
    const batchFile=flagsOf(args)['batch-file'];
    if(executed&&typeof batchFile==='string'){
      const file=path.posix.resolve(cwd,batchFile);
      try{
        if(safeWorkspacePath(file)&&fs.statSync(file).isFile()&&fs.statSync(file).size<=1048576){
          const bytes=fs.readFileSync(file),document=JSON.parse(bytes.toString('utf8'));
          batchInput={path:file,sha256:crypto.createHash('sha256').update(bytes).digest('hex'),document,
            items:Array.isArray(document)?document:document.items};
        }
      }catch{}
    }
    fs.appendFileSync('/audit/started-commands.jsonl',JSON.stringify({id,phase:process.env.BENCH_PHASE||'task',
      argv:[installer?'tadx-bench-installer':'tadx',...args],capability:installer?null:classify(args),flags:flagsOf(args),
      cwd,binary_sha256:executedBinarySHA,started_at,executed,batch_input:batchInput,helper:installer?'installer':undefined})+'\n');
    if(executed&&installer){
      fs.chmodSync('/work/installer-transport/curl',0o755);
      Object.assign(env,{HOME:'/work/installer-home',XDG_CONFIG_HOME:'/work/installer-home/.config',SHELL:'/bin/bash',PATH:'/work/installer-transport:/usr/local/bin:/usr/bin:/bin',BENCH_INSTALL_DOWNLOAD_FAILURE:policyState.guard.download_failure?'1':'0'});
      result=cp.spawnSync('sh',['/work/installer-assets/install.sh',...args],{cwd,encoding:'utf8',timeout:300000,maxBuffer:16777216,env});
    }
    else if(executed&&policyState.guard?.interactive_credentials&&args[0]==='auth'&&args[1]==='login'){
      const p=cp.spawnSync('python3',['/opt/credential_pty.py',...args],{cwd,encoding:'utf8',timeout:135000,maxBuffer:16777216,env});
      try{result=JSON.parse(p.stdout);}catch{result={status:p.status||125,stdout:'',stderr:'Disposable native login terminal did not return a result.'};}
    }
    else if(executed&&policyState.guard?.filesystem_fault){
      const fault=policyState.guard.filesystem_fault;
      if(!['rename','renameat','renameat2'].includes(fault.syscall)||fault.errno!=='EIO'||!Number.isInteger(fault.when)||fault.when<1)throw Error('Unbound filesystem fault');
      result=cp.spawnSync('strace',['-f','-o','/audit/guidance-fault.trace','-e','trace=rename,renameat,renameat2','-e','inject='+fault.syscall+':error=EIO:when='+fault.when,taskBinary,...args],{cwd,encoding:'utf8',timeout:300000,maxBuffer:16777216,env});
    }
    else if (executed) result=cp.spawnSync(taskBinary,args,{cwd,encoding:'utf8',timeout:300000,maxBuffer:16777216,env});
    else result={status:125,stdout:'',stderr:'This command is outside the exact operation, targets, or requested changes allowed for this fixture.\n'};
    const redact=text=>Object.entries(env).filter(([k,v])=>/PAT_(NAME|SECRET)$/.test(k)&&v).reduce((s,[k,v])=>s.split(v).join('[REDACTED]'),String(text));
    const stdout=redact(result.stdout||''), stderr=redact((result.stderr||'')+(result.error?String(result.error):''));
    const row={id,phase:process.env.BENCH_PHASE||'task',argv:['tadx',...args],capability:executed?classify(args):null,flags:flagsOf(args),cwd,binary_sha256:executedBinarySHA,started_at,ended_at:new Date().toISOString(),duration_s:Number(process.hrtime.bigint()-start)/1e9,exit_status:result.status===null?124:result.status,stdout,stderr,executed};
    if(batchInput)row.batch_input=batchInput;
    if(installer){row.helper='installer';row.argv=['tadx-bench-installer',...args];row.capability=null;row.installer_sha256=policyState.guard.installer_sha256;}
    if(policyState.guard?.mode==='guidance'){row.cli_home=env.HOME;row.cli_guidance_notice=env.TADX_GUIDANCE_NOTICE;}
    for(const text of [stdout,stderr]){try { let decoded=JSON.parse(text);for(let depth=0;depth<4&&decoded;depth++){if(decoded.error?.id){row.error_id=decoded.error.id;break;}decoded=decoded.output||decoded.result;} } catch {}}
    fs.writeFileSync('/audit/'+id+'.stdout.txt',stdout);
    fs.writeFileSync('/audit/'+id+'.stderr.txt',stderr);
    row.stdout_evidence='audit/'+id+'.stdout.txt';row.stderr_evidence='audit/'+id+'.stderr.txt';
    fs.appendFileSync('/audit/commands.jsonl',JSON.stringify(row)+'\n');
    res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify({stdout,stderr,exit_status:row.exit_status}));
  });
});
  const ready=()=>fs.writeFileSync('/audit/ready.json',JSON.stringify({binary_sha256:binarySHA,pid:process.pid,
    transport:process.env.BENCH_BROKER_TRANSPORT==='unix'?'unix':'loopback'}));
  if(process.env.BENCH_BROKER_TRANSPORT==='unix'){
    fs.mkdirSync(SOCKET_DIRECTORY,{recursive:true,mode:0o755});
    const directory=fs.lstatSync(SOCKET_DIRECTORY);
    if(directory.isSymbolicLink()||!directory.isDirectory()||directory.uid!==0)throw Error('Untrusted CLI socket directory');
    fs.chmodSync(SOCKET_DIRECTORY,0o755);
    if(fs.existsSync(SOCKET_PATH))throw Error('Existing CLI socket requires ownership reconciliation');
    server.listen(SOCKET_PATH,()=>{fs.chmodSync(SOCKET_PATH,0o666);ready();});
  }else{
    if(process.env.BENCH_BROKER_TRANSPORT)throw Error('Unsupported CLI broker transport');
    server.listen(8765,'127.0.0.1',ready);
  }
  return server;
}
module.exports={parseArgs,flagsOf,classify,allowed,validGuard,metadataAllowed,validationAllowed,safeWorkspacePath,diskState};
if(require.main===module)startBroker();
