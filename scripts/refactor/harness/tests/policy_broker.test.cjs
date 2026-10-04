const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const broker = require('../source/integration/policy_broker.cjs');

const sha = value => crypto.createHash('sha256').update(value).digest('hex');
const sentinel = Buffer.from('protected sentinel');
const candidate = Buffer.from('{"version":1,"allowed_capabilities":[],"remote_mutations":false}');
function files(extra = {}) {
  const entries = {'/cli-state/g9-policy-private': {type:'directory'},
    '/cli-state/g9-policy-private/sentinel.txt': {type:'file',value:sentinel},
    '/cli-state/g9-policy-private/candidate.json': {type:'file',value:candidate}, ...extra};
  return {
    lstatSync(name) {
      const item = entries[name];
      if (!item) {const error = new Error('missing');error.code='ENOENT';throw error;}
      return {isDirectory:()=>item.type==='directory',isFile:()=>item.type==='file',
        isSymbolicLink:()=>item.type==='symlink'};
    },
    readFileSync(name) {return entries[name].value;},
    realpathSync(name) {return name;},
  };
}
const make = action => ({family:'g9-local-policy',action,run_id:'run-1',
  case_id:'P-'+action.replace('.','-'),network_none:true,preflight_verified:true,
  machine_policy_absent:true,output_path:'/cli-state/g9-policy-private/candidates',
  candidate_path:'/cli-state/g9-policy-private/candidate.json',sentinel_sha256:sha(sentinel),
  candidate_sha256:sha(candidate)});
const state = guard => ({guard,guardPresent:true,baselinePresent:true,
  baselineUnchanged:true,networkNone:true});
const parsed = (words,flags) => ({words,flags,errors:[]});

test('only three exact non-privileged policy commands are admitted', () => {
  assert.equal(broker.allowed(parsed(['policy','samples'],{json:true,full:true,output:'/cli-state/g9-policy-private/candidates'}),
    'policy.samples',state(make('policy.samples')),files()),true);
  assert.equal(broker.allowed(parsed(['policy','validate','/cli-state/g9-policy-private/candidate.json'],{json:true,full:true}),
    'policy.validate',state(make('policy.validate')),files()),true);
  assert.equal(broker.allowed(parsed(['policy','status'],{json:true,full:true}),
    'policy.status',state(make('policy.status')),files()),true);
});

test('protected install, host paths, extra flags and changed baseline fail closed', () => {
  const guard=make('policy.samples');
  const request=parsed(['policy','samples'],{json:true,full:true,output:'/cli-state/g9-policy-private/candidates'});
  assert.equal(broker.allowed(parsed(['policy','install'],{json:true,full:true}),
    'policy.install',state({...guard,action:'policy.install'}),files()),false);
  assert.equal(broker.allowed(parsed(['policy','samples'],{json:true,full:true,output:'/etc/tadx'}),
    'policy.samples',state(guard),files()),false);
  assert.equal(broker.allowed(parsed(['policy','samples'],{json:true,full:true,output:'/cli-state/g9-policy-private/candidates',config:'/tmp/x'}),
    'policy.samples',state(guard),files()),false);
  assert.equal(broker.allowed(request,'policy.samples',{...state(guard),baselineUnchanged:false},files()),false);
  assert.equal(broker.allowed(request,'policy.samples',{...state(guard),networkNone:false},files()),false);
});

test('symlinks, preexisting output, changed candidate, and active locator fail before execution', () => {
  const samples = parsed(['policy','samples'],{json:true,full:true,output:'/cli-state/g9-policy-private/candidates'});
  const validate = parsed(['policy','validate','/cli-state/g9-policy-private/candidate.json'],{json:true,full:true});
  assert.equal(broker.allowed(samples,'policy.samples',state(make('policy.samples')),
    files({'/cli-state/g9-policy-private/candidates':{type:'symlink'}})),false);
  assert.equal(broker.allowed(samples,'policy.samples',state(make('policy.samples')),
    files({'/cli-state/g9-policy-private/candidates':{type:'directory'}})),false);
  assert.equal(broker.allowed(validate,'policy.validate',state(make('policy.validate')),
    files({'/cli-state/g9-policy-private/candidate.json':{type:'symlink',value:candidate}})),false);
  assert.equal(broker.allowed(validate,'policy.validate',state(make('policy.validate')),
    files({'/cli-state/g9-policy-private/candidate.json':{type:'file',value:Buffer.from('changed')}})),false);
  assert.equal(broker.allowed(samples,'policy.samples',state(make('policy.samples')),
    files({'/etc/tadx-policy-location.json':{type:'file',value:Buffer.from('active')}})),false);
});
