import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { SetupError, SetupService, installerInvocation, runSetupCommand, setupEnvironment, type SetupRunner } from '../src/setup.js';

const release = { version: '1.0.0', archiveSha256: 'a'.repeat(64) };
const consent = { guidanceConsent: true, otherTransfersStopped: true, modifyPath: false, completion: false };

async function fixture(run?: SetupRunner) {
  const root = await mkdtemp(join(tmpdir(), 'tadx-extension-setup-'));
  const home = join(root, 'home');
  const extensionPath = join(root, 'extension');
  await mkdir(home);
  await mkdir(join(extensionPath, 'dist', 'installers'), { recursive: true });
  await writeFile(join(extensionPath, 'dist', 'installers', process.platform === 'win32' ? 'install.ps1' : 'install.sh'), 'fixture installer');
  const environment = { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot,
    LOCALAPPDATA: join(home, 'local-app-data'), HOME: home, USERPROFILE: home };
  const service = new SetupService({ extensionPath, home, environment, run });
  return { root, home, service, cleanup: () => rm(root, { recursive: true, force: true }) };
}

test('setup requires an exact release and archive pin before preparing an installation', async () => {
  const f = await fixture();
  try {
    for (const invalid of [{ ...release, version: 'latest' }, { ...release, version: '1.0.0; exit' },
      { ...release, archiveSha256: '' }, { ...release, archiveSha256: 'g'.repeat(64) }]) {
      await assert.rejects(f.service.prepareInstall(invalid), /verified release/i);
    }
    const plan = await f.service.prepareInstall(release);
    assert.equal(plan.release.version, release.version);
    assert.equal(plan.release.archiveSha256, release.archiveSha256);
    assert.equal(plan.action, 'install');
    assert.equal(Object.isFrozen(plan), true);
    assert.equal(Object.isFrozen(plan.release), true);
  } finally { await f.cleanup(); }
});

test('installation requires Guidance and detached-transfer consent and defaults to no profile changes', async () => {
  const calls: { executable: string; args: readonly string[] }[] = [];
  let target = '';
  const f = await fixture(async (executable, args) => {
    calls.push({ executable, args });
    if (args[0] === 'version') return { stdout: JSON.stringify({ version: release.version }), stderr: '', exitCode: 0 };
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, 'installed binary');
    return { stdout: 'installed', stderr: '', exitCode: 0 };
  });
  try {
    const plan = await f.service.prepareInstall(release);
    target = plan.executablePath;
    await assert.rejects(f.service.execute(plan, { ...consent, guidanceConsent: false }), /Guidance/);
    await assert.rejects(f.service.execute(plan, { ...consent, otherTransfersStopped: false }), /transfers/);
    assert.equal(calls.length, 0);
    const installed = await f.service.execute(plan, consent);
    assert.equal(installed.version, release.version);
    assert.equal(installed.executablePath, target);
    assert.ok(calls[0]!.args.includes(release.archiveSha256));
    const precondition = calls[0]!.args.indexOf(process.platform === 'win32' ? '-ExpectedCurrentSHA256' : '--expected-current-sha256');
    assert.equal(calls[0]!.args[precondition + 1], 'absent');
    assert.ok(calls[0]!.args.includes(process.platform === 'win32' ? '-NoModifyPath' : '--no-modify-path'));
    assert.ok(calls[0]!.args.includes(process.platform === 'win32' ? '-NoCompletion' : '--no-completion'));
    await assert.rejects(f.service.execute(plan, consent), /new setup preview/);
  } finally { await f.cleanup(); }
});

test('update passes the inspected executable fingerprint to the native replacement guard', async () => {
  let target = '';
  let guardedHash: string | undefined;
  const f = await fixture(async (_executable, args) => {
    if (args[0] === 'version') return { stdout: JSON.stringify({ version: release.version }), stderr: '', exitCode: 0 };
    const flag = process.platform === 'win32' ? '-ExpectedCurrentSHA256' : '--expected-current-sha256';
    guardedHash = args[args.indexOf(flag) + 1];
    await writeFile(target, 'replacement binary');
    return { stdout: '', stderr: '', exitCode: 0 };
  });
  try {
    target = (await f.service.prepareInstall(release)).executablePath;
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, 'inspected binary');
    const plan = await f.service.prepareUpdate(target, release);
    await f.service.execute(plan, consent);
    assert.equal(guardedHash, createHash('sha256').update('inspected binary').digest('hex'));
  } finally { await f.cleanup(); }
});

test('update rejects executable bytes changed while reporting their version', async () => {
  let target = '';
  const f = await fixture(async () => {
    await writeFile(target, 'changed during inspection');
    return { stdout: JSON.stringify({ version: '0.9.0' }), stderr: '', exitCode: 0 };
  });
  try {
    target = (await f.service.prepareInstall(release)).executablePath;
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, 'original binary');
    await assert.rejects(f.service.prepareUpdate(target, release), /changed during inspection/);
  } finally { await f.cleanup(); }
});

test('new installation refuses an existing target and prepared plans reject changes before execution', async () => {
  let invoked = false;
  const f = await fixture(async () => { invoked = true; return { stdout: '', stderr: '', exitCode: 0 }; });
  try {
    const plan = await f.service.prepareInstall(release);
    await mkdir(dirname(plan.executablePath), { recursive: true });
    await writeFile(plan.executablePath, 'another installation');
    await assert.rejects(f.service.execute(plan, consent), /changed/);
    await assert.rejects(f.service.prepareInstall(release), /already exists/);
    assert.equal(invoked, false);
    assert.equal(await readFile(plan.executablePath, 'utf8'), 'another installation');
  } finally { await f.cleanup(); }
});

test('update resolves the selected binary and refuses custom installation ownership', async () => {
  let version = '0.9.0';
  const f = await fixture(async (_executable, args) => {
    assert.equal(args[0], 'version');
    return { stdout: JSON.stringify({ version }), stderr: '', exitCode: 0 };
  });
  try {
    const initial = await f.service.prepareInstall(release);
    await mkdir(dirname(initial.executablePath), { recursive: true });
    await writeFile(initial.executablePath, 'old binary');
    const update = await f.service.prepareUpdate(initial.executablePath, release);
    assert.equal(update.currentVersion, '0.9.0');
    assert.equal(update.executablePath, initial.executablePath);
    assert.equal(update.action, 'update');
    version = '1.1.0';
    await assert.rejects(f.service.prepareUpdate(initial.executablePath, release), /newer/);
    version = '0.9.0';
    const custom = join(f.home, 'custom', process.platform === 'win32' ? 'tadx.exe' : 'tadx');
    await mkdir(dirname(custom));
    await writeFile(custom, 'custom binary');
    await assert.rejects(f.service.prepareUpdate(custom, release), /custom|package manager/);
    assert.equal(await readFile(custom, 'utf8'), 'custom binary');
  } finally { await f.cleanup(); }
});

test('update detects a binary replaced after preview and refuses foreign or replayed plans', async () => {
  const f = await fixture(async () => ({ stdout: JSON.stringify({ version: '0.9.0' }), stderr: '', exitCode: 0 }));
  try {
    const initial = await f.service.prepareInstall(release);
    await mkdir(dirname(initial.executablePath), { recursive: true });
    await writeFile(initial.executablePath, 'before');
    const plan = await f.service.prepareUpdate(initial.executablePath, release);
    await writeFile(initial.executablePath, 'after');
    await assert.rejects(f.service.execute(plan, consent), /changed/);
    await assert.rejects(f.service.execute({ ...plan }, consent), /new setup preview/);
  } finally { await f.cleanup(); }
});

test('setup does not follow a standard install directory redirected outside the user home', async () => {
  const f = await fixture(async () => ({ stdout: JSON.stringify({ version: '1.0.0' }), stderr: '', exitCode: 0 }));
  try {
    const plan = await f.service.prepareInstall(release);
    const directory = dirname(plan.executablePath);
    const outside = join(f.root, 'outside');
    await mkdir(outside);
    await mkdir(dirname(directory), { recursive: true });
    await symlink(outside, directory, process.platform === 'win32' ? 'junction' : 'dir');
    await assert.rejects(f.service.prepareInstall(release), /outside|redirected/);
    const custom = join(f.home, process.platform === 'win32' ? 'custom-tadx.exe' : 'custom-tadx');
    await writeFile(custom, 'selected custom CLI');
    assert.equal((await f.service.inspect(custom)).managed, false);
  } finally { await f.cleanup(); }
});

test('installer argv preserves exact values and only requested profile changes', () => {
  const invocation = installerInvocation('win32', 'C:\\extension with spaces', 'C:\\user\\bin & literal', release,
    { ...consent, modifyPath: true, completion: false });
  assert.equal(invocation.executable, 'powershell.exe');
  assert.ok(invocation.args.includes('C:\\user\\bin & literal'));
  assert.ok(invocation.args.includes('-ExpectedSHA256'));
  assert.ok(!invocation.args.includes('-NoModifyPath'));
  assert.ok(invocation.args.includes('-NoCompletion'));
  const unix = installerInvocation('linux', '/extension', '/home/user/bin $(literal)', release, consent);
  assert.equal(unix.executable, '/bin/sh');
  assert.ok(unix.args.includes('/home/user/bin $(literal)'));
  assert.ok(unix.args.includes('--expected-sha256'));
  assert.ok(installerInvocation('win32', 'extension', 'directory', release, consent, 'absent').args.includes('-ExpectedCurrentSHA256'));
  assert.ok(installerInvocation('linux', 'extension', 'directory', release, consent, 'a'.repeat(64)).args.includes('--expected-current-sha256'));
  assert.throws(() => installerInvocation('linux', 'extension', 'directory', release, consent, ''), /precondition/);
});

test('setup environment excludes arbitrary and conventional Tableau secrets but retains GitHub access', () => {
  const environment = setupEnvironment({ PATH: '/bin', HOME: '/home/user', TADX_PAT_SECRET: 'private',
    TADX_TEAM_PAT_NAME: 'private', SOME_CUSTOM_CREDENTIAL: 'private', TADX_CONFIG: '/private/config',
    GH_TOKEN: 'github-fixture', NODE_OPTIONS: '--require unsafe', ENV: '/unsafe', BASH_ENV: '/unsafe' });
  assert.deepEqual(Object.keys(environment).sort(), ['GH_NO_UPDATE_NOTIFIER', 'GH_PROMPT_DISABLED', 'GH_TOKEN', 'HOME', 'PATH']);
  assert.equal(environment.GH_TOKEN, 'github-fixture');
});

test('runner executes literal argv with filtered environment and bounds subprocess output', async () => {
  const result = await runSetupCommand(process.execPath, ['-e', 'process.stdout.write(JSON.stringify({argv:process.argv[1],secret:process.env.TADX_PAT_SECRET}))', 'a & $(literal)'],
    { environment: setupEnvironment({ PATH: process.env.PATH, TADX_PAT_SECRET: 'private' }), timeoutMs: 5000 });
  assert.deepEqual(JSON.parse(result.stdout), { argv: 'a & $(literal)' });
  await assert.rejects(runSetupCommand(process.execPath, ['-e', 'process.stdout.write("x".repeat(4096))'],
    { environment: {}, timeoutMs: 5000, maxOutputBytes: 100 }), /output limit/);
});

test('setup cancellation before start performs no command and installer failure retains unknown outcome', async () => {
  let calls = 0;
  const f = await fixture(async () => { calls++; throw new Error('installer interrupted'); });
  try {
    const plan = await f.service.prepareInstall(release);
    await assert.rejects(f.service.execute(plan, consent, AbortSignal.abort()), error => {
      assert.ok(error instanceof SetupError);
      assert.equal(error.outcome, 'not_attempted');
      return true;
    });
    assert.equal(calls, 0);
    await assert.rejects(f.service.execute(plan, consent), error => {
      assert.ok(error instanceof SetupError);
      assert.equal(error.outcome, 'unknown');
      assert.match(error.message, /rollback is not confirmed/);
      return true;
    });
  } finally { await f.cleanup(); }
});

test('runner timeout and cancellation return unknown outcomes without claiming rollback', async () => {
  const timeout = runSetupCommand(process.execPath, ['-e', 'setInterval(()=>{},1000)'],
    { environment: setupEnvironment(process.env), timeoutMs: 50 });
  await assert.rejects(timeout, error => {
    assert.ok(error instanceof SetupError);
    assert.equal(error.outcome, 'unknown');
    assert.match(error.message, /time limit/);
    return true;
  });
  const abort = new AbortController();
  const canceled = runSetupCommand(process.execPath, ['-e', 'setInterval(()=>{},1000)'],
    { environment: setupEnvironment(process.env), timeoutMs: 5000, signal: abort.signal });
  abort.abort();
  await assert.rejects(canceled, error => {
    assert.ok(error instanceof SetupError);
    assert.equal(error.outcome, 'unknown');
    assert.match(error.message, /rollback is not confirmed/);
    return true;
  });
});

test('setup preserves a confirmed installer launch failure as not attempted', async () => {
  const f = await fixture(async () => { throw new SetupError('The host shell could not start.'); });
  try {
    const plan = await f.service.prepareInstall(release);
    await assert.rejects(f.service.execute(plan, consent), error => {
      assert.ok(error instanceof SetupError);
      assert.equal(error.outcome, 'not_attempted');
      assert.match(error.message, /host shell could not start/);
      return true;
    });
    await assert.rejects(f.service.execute(plan, consent), /new setup preview/);
  } finally { await f.cleanup(); }
});
