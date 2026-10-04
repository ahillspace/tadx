const test = require('node:test');
const assert = require('node:assert/strict');
const broker = require('../source/integration/job_broker.cjs');

const job = '11111111-1111-4111-8111-111111111111';
const siteId = '22222222-2222-4222-8222-222222222222';
const proof = 'a'.repeat(64);
const guard = action => ({family:'job',action,job_id:job,environment:'fixture',
  site:'fixture-site',site_luid:siteId,run_id:'run-1',case_id:'P-'+action.replace('.','-'),
  ownership_sha256:proof,baseline_sha256:proof});
const state = action => ({guard:guard(action),guardPresent:true,baselinePresent:true,
  baselineUnchanged:true,jobOwned:true,jobFresh:true,cancelAttempted:false});
const parsed = (action, extra={}) => ({words:['job',action.split('.')[1]],errors:[],
  flags:{environment:'fixture',site:'fixture-site',id:job,json:true,full:true,...extra}});

test('all three exact owned-job commands are admitted', () => {
  for (const action of ['job.inspect','job.wait','job.cancel']) {
    assert.equal(broker.validGuard(guard(action)),true);
    assert.equal(broker.allowed(parsed(action),action,state(action)),true);
  }
});

test('wrong identity, site, receipt, and extra flags are rejected', () => {
  for (const flags of [{id:siteId},{site:'other'},{receipt:'/work/other.json'},
    {'operation-id':'other'},{config:'/work/config.json'},{force:true}]) {
    assert.equal(broker.allowed(parsed('job.wait',flags),'job.wait',state('job.wait')),false);
  }
  assert.equal(broker.allowed(parsed('job.wait'),'job.cancel',state('job.wait')),false);
});

test('cancellation needs fresh independent ownership and cannot repeat', () => {
  for (const delta of [{jobOwned:false},{jobFresh:false},{cancelAttempted:true},
    {baselineUnchanged:false}]) {
    assert.equal(broker.allowed(parsed('job.cancel'),'job.cancel',
      {...state('job.cancel'),...delta}),false);
  }
  assert.equal(broker.allowed(parsed('job.cancel',{preview:true}),'job.cancel',
    state('job.cancel')),true);
});

test('read follow-up stays on the owned exact job', () => {
  assert.equal(broker.allowed(parsed('job.inspect'),'job.inspect',state('job.cancel')),true);
  assert.equal(broker.allowed(parsed('job.wait'),'job.wait',state('job.cancel')),true);
  assert.equal(broker.allowed(parsed('job.cancel'),'job.cancel',state('job.inspect')),false);
  assert.equal(broker.allowed(parsed('job.inspect',{id:siteId}),'job.inspect',state('job.cancel')),false);
});

test('invalid or incomplete guard fails closed', () => {
  assert.equal(broker.validGuard({...guard('job.cancel'),ownership_sha256:'bad'}),false);
  assert.equal(broker.validGuard({...guard('job.cancel'),job_id:'not-a-luid'}),false);
  assert.equal(broker.allowed(parsed('job.cancel'),'job.cancel',
    {...state('job.cancel'),cancelAttempted:undefined}),false);
});
