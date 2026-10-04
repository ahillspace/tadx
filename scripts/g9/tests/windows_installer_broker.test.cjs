const assert=require('node:assert/strict');
const test=require('node:test');
const broker=require('../../refactor/harness/source/integration/g9_windows_installer_broker.cjs');

const guard={mode:'shared-windows-installer',case:'fresh',exercise_id:'V-installer-windows-fresh',
  source_sha:'a'.repeat(40),binary_sha256:'b'.repeat(64),installer_sha256:'c'.repeat(64),
  setup_commit:'d'.repeat(40),baseline_sha256:'e'.repeat(64),hosted_run_id:17,
  fixture_version:'0.1.3-g9',credential_input:false,runtime_network:'none'};

test('all seven exact helper commands are bound to one declared case',()=>{
  for(const caseName of ['fresh','idempotent','no-completion','completion-opt-in',
    'failed-download-preserves-binary','uninstall','no-modify-path']){
    const selected={...guard,case:caseName,exercise_id:'V-installer-windows-'+caseName};
    const args=broker.expected(caseName,guard.fixture_version);
    assert.equal(broker.allowed(args,selected),true);
    assert.equal(broker.allowed([...args,';whoami'],selected),false);
    assert.equal(broker.allowed(args,{...selected,credential_input:true}),false);
    assert.equal(broker.allowed(args,{...selected,source_sha:'a'.repeat(64)}),false);
    assert.equal(broker.allowed(args,{...selected,setup_commit:'d'.repeat(64)}),false);
  }
});

test('request and response require matching command digest and hosted outcome',()=>{
  const body=broker.request(['install','--version','0.1.3-g9'],guard,'cmd-00001');
  const response={protocol:'tadx-windows-installer-response/1',id:body.id,
    command_sha256:body.command_sha256,status:'verified',run_id:17,
    setup_commit:body.setup_commit,baseline_sha256:body.baseline_sha256,
    exit_status:0,stdout:'installed',stderr:''};
  assert.equal(broker.validResponse(body,response),true);
  assert.equal(broker.validResponse(body,{...response,command_sha256:'d'.repeat(64)}),false);
  assert.equal(broker.validResponse(body,{...response,status:'not_verified'}),false);
  assert.equal(broker.validResponse(body,{...response,run_id:null}),false);
  assert.equal(broker.validResponse(body,{...response,baseline_sha256:'f'.repeat(64)}),false);
  assert.equal(broker.validResponse(body,{...response,stdout:'x'.repeat(8193)}),false);
  assert.equal(broker.validResponse(body,{...response,status:'not_verified',exit_status:125}),true);
});

test('verified failed-download evidence preserves the native refusal',()=>{
  const failed={...guard,case:'failed-download-preserves-binary',
    exercise_id:'V-installer-windows-failed-download-preserves-binary'};
  const body=broker.request(broker.expected(failed.case,failed.fixture_version),failed,'cmd-00002');
  const response={protocol:'tadx-windows-installer-response/1',id:body.id,
    command_sha256:body.command_sha256,status:'verified',run_id:body.run_id,
    setup_commit:body.setup_commit,baseline_sha256:body.baseline_sha256,
    exit_status:1,stdout:'',stderr:'download failed'};
  assert.equal(broker.validResponse(body,response),true);
  assert.equal(broker.validResponse(body,{...response,exit_status:0}),false);
  assert.equal(broker.validResponse(body,{...response,exit_status:124}),false);
});
