import assert from 'node:assert/strict';
import test from 'node:test';
import { TadxClient, type CommandRunner } from '../src/cli.js';
import { assessCompatibility, cliRequirements, CompatibilityError, requireCompatibleCommand } from '../src/compatibility.js';

function discovery() {
  return { page: { more_available: false }, capabilities: cliRequirements.map(item => ({ id: item.id,
    implementation_state: 'implemented', supports_preview: !!item.preview, supports_batch: !!item.batch,
    execution_enabled: false, policy_denied: true,
  })) };
}

test('compatibility accepts older and development versions without confusing policy with implementation', () => {
  for (const version of ['0.1.0', '1.0.0', '1.0.1-0.20261020100614-deadbeef+dirty', 'dev']) {
    const report = assessCompatibility({ version }, discovery());
    assert.equal(report.version, version);
    assert.deepEqual(report.issues, []);
    assert.doesNotThrow(() => requireCompatibleCommand(report, ['content', 'workbook', 'publish', '--artifact', 'artifacts/workbook/A']));
  }
});

test('unavailable and previewless actions are blocked without disabling supported reads', () => {
  const document = discovery();
  document.capabilities = document.capabilities.filter(item => item.id !== 'flow.list');
  document.capabilities.find(item => item.id === 'workbook.publish')!.supports_preview = false;
  document.capabilities.find(item => item.id === 'datasource.pull')!.implementation_state = 'planned';
  const report = assessCompatibility({ version: '1.0.0' }, document);
  assert.equal(report.issues.length, 3);
  for (const args of [['content', 'flow', 'list'], ['content', 'workbook', 'publish'], ['content', 'datasource', 'pull']]) {
    assert.throws(() => requireCompatibleCommand(report, args), /compatible CLI/);
  }
  assert.doesNotThrow(() => requireCompatibleCommand(report, ['workspace', 'status']));
});

test('single transfers remain usable when a CLI lacks batch support', () => {
  const document = discovery();
  document.capabilities.find(item => item.id === 'workbook.pull')!.supports_batch = false;
  const report = assessCompatibility({ version: '1.0.0' }, document);
  assert.deepEqual(report.issues, ['workbook.pull does not support batches']);
  assert.doesNotThrow(() => requireCompatibleCommand(report, ['content', 'workbook', 'pull', '--id', 'first']));
  assert.throws(() => requireCompatibleCommand(report, ['content', 'workbook', 'pull', '--id', 'first', '--id', 'second']), /does not support this batch/);
});

test('incomplete, duplicate, and malformed discovery never authorizes execution', () => {
  const document = discovery();
  for (const value of [{}, { ...document, page: { more_available: true } },
    { ...document, capabilities: [...document.capabilities, document.capabilities[0]] },
    { ...document, capabilities: [{ id: 'workbook.pull' }] }]) {
    assert.throws(() => assessCompatibility({ version: '1.0.0' }, value), CompatibilityError);
  }
  assert.throws(() => assessCompatibility({}, document), CompatibilityError);
});

test('client gates commands before launch and permits explicit recovery after an upgrade', async () => {
  let supported = false;
  const calls: string[][] = [];
  const runner: CommandRunner = async (_executable, args) => {
    calls.push([...args]);
    let result: unknown;
    if (args[0] === 'version') result = { version: '1.0.0' };
    else if (args[0] === 'capability') {
      const document = discovery();
      if (!supported) document.capabilities = document.capabilities.filter(item => item.id !== 'env.profile.list');
      result = document;
    } else result = { page: { more_available: false }, environments: [] };
    return { stdout: JSON.stringify(result), stderr: '', exitCode: 0 };
  };
  const client = new TadxClient('tadx', { runner });
  await client.checkCompatibility();
  await assert.rejects(client.environments(), /env.profile.list/);
  assert.equal(calls.some(args => args[0] === 'env'), false);
  supported = true;
  await client.checkCompatibility();
  assert.deepEqual(await client.environments(), []);
  assert.equal(calls.filter(args => args[0] === 'env').length, 1);
});

test('failed compatibility discovery remains fail closed until rechecked', async () => {
  const calls: string[][] = [];
  const client = new TadxClient('tadx', { runner: async (_executable, args) => {
    calls.push([...args]);
    return { stdout: JSON.stringify(args[0] === 'version' ? { version: 'dev' } : {}), stderr: '', exitCode: 0 };
  } });
  await assert.rejects(client.checkCompatibility(), CompatibilityError);
  await assert.rejects(client.workspaces(), CompatibilityError);
  assert.equal(calls.some(args => args[0] === 'workspace'), false);
});

test('CLI maintenance excludes in-flight and new subprocesses and releases after failure', async () => {
  let finishRead!: () => void;
  const pending = new Promise<void>(resolve => { finishRead = resolve; });
  let calls = 0;
  const client = new TadxClient('tadx', { runner: async () => {
    calls++;
    await pending;
    return { stdout: '{"version":"1.0.0"}', stderr: '', exitCode: 0 };
  } });
  const read = client.version();
  let maintenanceRan = false;
  await assert.rejects(client.withMaintenance(async () => { maintenanceRan = true; }), /Wait for TADX commands/);
  assert.equal(maintenanceRan, false);
  finishRead();
  await read;
  await assert.rejects(client.withMaintenance(async () => {
    await assert.rejects(client.version(), /setup is in progress/);
    await assert.rejects(client.withMaintenance(async () => undefined), /Wait for TADX commands/);
    assert.equal(calls, 1);
    throw new Error('Synthetic installer failure');
  }), /Synthetic installer failure/);
  await client.version();
  assert.equal(calls, 2);
});

test('compatibility probes count as in-flight CLI commands during maintenance checks', async () => {
  let finish!: () => void;
  const pending = new Promise<void>(resolve => { finish = resolve; });
  const client = new TadxClient('tadx', { runner: async (_executable, args) => {
    await pending;
    return { stdout: JSON.stringify(args[0] === 'version' ? { version: '1.0.0' } : discovery()), stderr: '', exitCode: 0 };
  } });
  const check = client.checkCompatibility();
  await assert.rejects(client.withMaintenance(async () => undefined), /Wait for TADX commands/);
  finish();
  await check;
  assert.equal(await client.withMaintenance(async () => 'complete'), 'complete');
});
