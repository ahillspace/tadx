'use strict';

const {test} = require('node:test');
const assert = require('node:assert/strict');
const broker = require('../source/integration/policy_install_broker.cjs');

function state() {
  return {baselinePresent: true, baselineUnchanged: true, guardPresent: true,
    networkNone: true, guard: {family: 'g9-policy-install', action: 'policy.install',
      network_none: true, machine_policy_absent: true, root_overlay_only: true}};
}

function parsed(flags = {template: 'read-only', output: '/etc/tadx', json: true, full: true},
                words = ['policy', 'install']) {
  return {words, flags, errors: []};
}

test('only exact protected install in the disposable root overlay is admitted', () => {
  assert.equal(broker.allowed(parsed(), 'policy.install', state()), true);
  for (const flags of [
    {template: 'superuser', output: '/etc/tadx', json: true, full: true},
    {template: 'read-only', output: '/work/policy', json: true, full: true},
    {template: 'read-only', output: '/etc/tadx', json: true, full: true, config: '/tmp/config'},
    {template: 'read-only', output: '/etc/tadx', json: true, full: true, preview: true},
    {template: 'read-only', output: '/etc/tadx', json: true, full: true, force: true},
    {template: ['read-only', 'superuser'], output: '/etc/tadx', json: true, full: true},
  ]) assert.equal(broker.allowed(parsed(flags), 'policy.install', state()), false);
  assert.equal(broker.allowed(parsed(undefined, ['policy', 'status']), 'policy.install', state()), false);
  assert.equal(broker.allowed(parsed(), 'policy.status', state()), false);
});

test('network, baseline, and root-overlay proof fail closed', () => {
  for (const key of ['baselinePresent', 'baselineUnchanged', 'guardPresent', 'networkNone']) {
    const changed = state(); changed[key] = false;
    assert.equal(broker.allowed(parsed(), 'policy.install', changed), false);
  }
  for (const key of ['network_none', 'machine_policy_absent', 'root_overlay_only']) {
    const changed = state(); changed.guard[key] = false;
    assert.equal(broker.allowed(parsed(), 'policy.install', changed), false);
  }
  const wrong = state(); wrong.guard.family = 'g9-local-policy';
  assert.equal(broker.allowed(parsed(), 'policy.install', wrong), false);
});
