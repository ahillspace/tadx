const test=require('node:test');
const assert=require('node:assert/strict');
const broker=require('../source/integration/session_broker.cjs');
const fs=require('node:fs');

const environment='session-local';
const base={mode:'session',profile:'mutation-no-setting-authorization',environment,
  server_url:'https://example.test',site_content_url:'session-fixture',
  unrelated_environment:'session-unrelated',unrelated_site_content_url:'session-unrelated',
  initial_site_consent:false,process_policy:'1',commands:[
    {capability:'mutation.status',words:['mutation','status'],flags:{environment},
      allowed_flags:['environment','full','json']},
    {capability:'project.create',words:['content','project','create'],
      flags:{environment,name:'session-policy-probe'},
      allowed_flags:['environment','full','json','name']},
  ]};
const registry=[
  {id:'mutation.status',command_path:['mutation','status'],remote_mutation:false},
  {id:'project.create',command_path:['content','project','create'],remote_mutation:true},
];
const state={guard:base,networkNone:true,baselinePresent:true,guardPresent:true,registry,
  mutationDisabled:false};

test('broker admits exact-site refusal probe despite legacy enabled value',()=>{
  assert.equal(broker.validGuard(base),true);
  assert.equal(broker.allowed({words:['mutation','status'],flags:{environment,json:true,full:true},errors:[]},
    'mutation.status',state),true);
  assert.equal(broker.allowed({words:['content','project','create'],
    flags:{environment,name:'session-policy-probe',json:true},errors:[]},'project.create',state),true);
});

test('broker rejects absent full evidence and broadened target or consent',()=>{
  assert.equal(broker.allowed({words:['mutation','status'],flags:{environment,json:true},errors:[]},
    'mutation.status',state),false);
  assert.equal(broker.allowed({words:['content','project','create'],
    flags:{environment:'other',name:'session-policy-probe'},errors:[]},'project.create',state),false);
  assert.equal(broker.validGuard({...base,initial_site_consent:true}),false);
  assert.equal(broker.allowed({words:['content','project','create'],
    flags:{environment,name:'session-policy-probe'},errors:[]},'project.create',
    {...state,guard:{...base,initial_site_consent:true}}),false);
  assert.equal(broker.validGuard({...base,site_content_url:'other'}),false);
  assert.equal(broker.validGuard({...base,unrelated_site_content_url:'other'}),false);
  assert.equal(broker.validGuard({...base,process_policy:'0'}),false);
});

test('runtime proof binds selected site and ignores contradictory legacy values',()=>{
  const originalRead=fs.readFileSync,originalExists=fs.existsSync;
  const originalLegacy=process.env.TADX_ENABLE_MUTATIONS;
  const config={default_environment:environment,mutations_enabled:true,
    environments:{[environment]:{url:'https://example.test',site_content_url:'session-fixture',
      auth:{type:'pat',pat_name_env:'BENCH_SESSION_MISSING_NAME',
        pat_secret_env:'BENCH_SESSION_MISSING_SECRET'}},
      'session-unrelated':{url:'https://example.test',site_content_url:'session-unrelated',
        auth:{type:'pat',pat_name_env:'BENCH_SESSION_UNRELATED_MISSING_NAME',
          pat_secret_env:'BENCH_SESSION_UNRELATED_MISSING_SECRET'}}},
    site_mutations:[{server_url:'https://example.test',site_content_url:'session-fixture',enabled:false},
      {server_url:'https://example.test',site_content_url:'session-unrelated',enabled:true}]};
  const proof={guard:base,networkNone:true,baselinePresent:true,
    baselineUnchanged:true,guardPresent:true};
  try{
    fs.readFileSync=()=>JSON.stringify(config);fs.existsSync=()=>false;
    process.env.TADX_ENABLE_MUTATIONS='1';
    assert.equal(broker.runtimeEvidence(proof).initial_site_consent,false);
    config.site_mutations[0].site_content_url='other';
    assert.throws(()=>broker.runtimeEvidence(proof),/exact canonical site consent/);
    config.site_mutations[0].site_content_url='session-fixture';
    config.site_mutations.pop();
    assert.throws(()=>broker.runtimeEvidence(proof),/exact canonical site consent/);
    config.site_mutations.push({server_url:'https://example.test',site_content_url:'session-unrelated',enabled:false});
    assert.throws(()=>broker.runtimeEvidence(proof),/exact canonical site consent/);
    config.site_mutations[1].enabled=true;
    delete config.environments['session-unrelated'];
    assert.throws(()=>broker.runtimeEvidence(proof),/absent credential references/);
  }finally{
    fs.readFileSync=originalRead;fs.existsSync=originalExists;
    if(originalLegacy===undefined)delete process.env.TADX_ENABLE_MUTATIONS;
    else process.env.TADX_ENABLE_MUTATIONS=originalLegacy;
  }
});
