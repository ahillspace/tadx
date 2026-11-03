import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { mkdtemp, mkdir, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { SetupError, SetupService, installerInvocation, runSetupCommand, setupEnvironment, type SetupRunner } from '../src/setup.js';
import { TadxClient } from '../src/cli.js';

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

for (const trigger of ['cancel', 'timeout', 'output'] as const) for (const stubborn of [false, true]) {
  test(`${trigger} retains maintenance until its ${stubborn ? 'stubborn' : 'responsive'} native process tree has stopped`, async () => {
    const root = await mkdtemp(join(tmpdir(), 'tadx-setup-tree-'));
    const parentPath = join(root, 'installer.cjs');
    const childPath = join(root, 'guidance.cjs');
    const parentReady = join(root, 'installer.pid');
    const childReady = join(root, 'guidance.pid');
    const abort = new AbortController();
    const client = new TadxClient(process.execPath);
    let parentPID: number | undefined;
    let childPID: number | undefined;
    try {
      await writeFile(childPath, `const fs = require('node:fs');
        ${stubborn ? "process.on('SIGTERM', () => {});" : ''}
        fs.writeFileSync(process.argv[2], String(process.pid));
        setInterval(() => {}, 1000);`);
      await writeFile(parentPath, `const fs = require('node:fs');
        const { spawn } = require('node:child_process');
        spawn(process.execPath, [process.argv[2], process.argv[4]], { stdio: 'inherit', windowsHide: true });
        process.on('SIGTERM', () => setTimeout(() => process.exit(0), 100));
        fs.writeFileSync(process.argv[3], String(process.pid));
        ${trigger === 'output' ? `const ready = setInterval(() => {
          if (fs.existsSync(process.argv[4])) { clearInterval(ready); process.stdout.write('x'.repeat(4096)); }
        }, 10);` : ''}
        setInterval(() => {}, 1000);`);
      const stopped = client.withMaintenance(() => runSetupCommand(process.execPath,
        [parentPath, childPath, parentReady, childReady],
        { environment: setupEnvironment(process.env), signal: abort.signal,
          timeoutMs: trigger === 'timeout' ? 1000 : 10_000, maxOutputBytes: trigger === 'output' ? 100 : undefined }));
      const failure = assert.rejects(stopped, trigger === 'cancel' ? /Setup was canceled/ : trigger === 'timeout' ? /time limit/ : /output limit/);
      parentPID = await readPID(parentReady);
      childPID = await readPID(childReady);
      if (trigger === 'cancel') abort.abort();
      await failure;
      assert.equal(processAlive(parentPID), false, 'installer must stop before maintenance is released');
      assert.equal(processAlive(childPID), false, 'Guidance descendant must stop before maintenance is released');
      assert.equal(await client.withMaintenance(async () => 'retry'), 'retry');
    } finally {
      for (const pid of [parentPID, childPID]) {
        if (pid && processAlive(pid)) { try { process.kill(pid, 'SIGKILL'); } catch { /* Already exited. */ } }
      }
      await rm(root, { recursive: true, force: true });
    }
  });
}

test('unconfirmed setup termination keeps CLI commands and installer retries excluded', async () => {
  const client = new TadxClient(process.execPath);
  await assert.rejects(client.withMaintenance(async () => {
    throw new SetupError('The installer process tree could not be confirmed stopped.', 'unknown', true);
  }), /could not be confirmed stopped/);
  await assert.rejects(client.withMaintenance(async () => undefined), /Wait for TADX commands/);
  await assert.rejects(client.version(), /setup is in progress/);
});

test('Unix cancellation stops a redirected stubborn descendant after its parent closes',
  { skip: process.platform === 'win32' }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'tadx-setup-orphan-'));
    const parentPath = join(root, 'installer.cjs');
    const childPath = join(root, 'guidance.cjs');
    const ready = join(root, 'guidance.pid');
    const heartbeat = join(root, 'guidance-progress');
    const abort = new AbortController();
    const client = new TadxClient(process.execPath);
    let pid: number | undefined;
    try {
      await writeFile(childPath, `const fs = require('node:fs');
        process.on('SIGTERM', () => {});
        let count = 0;
        fs.writeFileSync(process.argv[3], String(count));
        fs.writeFileSync(process.argv[2], String(process.pid));
        setInterval(() => fs.writeFileSync(process.argv[3], String(++count)), 10);`);
      await writeFile(parentPath, `const { spawn } = require('node:child_process');
        spawn(process.execPath, [process.argv[2], process.argv[3], process.argv[4]], { stdio: 'ignore' });
        process.on('SIGTERM', () => process.exit(0));
        setInterval(() => {}, 1000);`);
      const stopped = client.withMaintenance(() => runSetupCommand(process.execPath,
        [parentPath, childPath, ready, heartbeat],
        { environment: setupEnvironment(process.env), signal: abort.signal, timeoutMs: 10_000 }));
      const failed = assert.rejects(stopped, error => {
        assert.ok(error instanceof SetupError);
        assert.equal(error.retainExclusion, false);
        return true;
      });
      pid = await readPID(ready);
      abort.abort();
      await failed;
      const lastWrite = await readFile(heartbeat, 'utf8');
      await new Promise(resolve => setTimeout(resolve, 100));
      assert.equal(await readFile(heartbeat, 'utf8'), lastWrite, 'a closed parent must not leave a writer behind');
      assert.equal(await client.withMaintenance(async () => 'retry'), 'retry');
    } finally {
      if (pid && processAlive(pid)) { try { process.kill(pid, 'SIGKILL'); } catch { /* Already exited. */ } }
      await rm(root, { recursive: true, force: true });
    }
  });

test('Windows termination-helper launch failure retains maintenance while the installer is alive',
  { skip: process.platform !== 'win32' }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'tadx-setup-termination-failure-'));
    const ready = join(root, 'installer.pid');
    const abort = new AbortController();
    const client = new TadxClient(process.execPath);
    let pid: number | undefined;
    try {
      const environment = setupEnvironment(process.env);
      const stopped = client.withMaintenance(() => runSetupCommand(process.execPath,
        ['-e', "require('node:fs').writeFileSync(process.argv[1], String(process.pid)); setInterval(() => {}, 1000)", ready],
        { environment, signal: abort.signal, timeoutMs: 10_000 }));
      const failed = stopped.then(result => {
        assert.fail(`Termination unexpectedly returned success: ${JSON.stringify(result)}`);
      }, error => {
        assert.ok(error instanceof SetupError);
        assert.equal(error.retainExclusion, true);
        assert.match(error.message, /could not be confirmed stopped/);
      });
      pid = await readPID(ready);
      // Make the native termination helper unavailable after Node has started;
      // Windows Node itself requires a valid SystemRoot during initialization.
      for (const key of Object.keys(environment)) if (key.toUpperCase() === 'SYSTEMROOT') delete environment[key];
      environment.SystemRoot = join(root, 'unavailable-system-root');
      abort.abort();
      await failed;
      assert.equal(processAlive(pid), true);
      await assert.rejects(client.withMaintenance(async () => undefined), /Wait for TADX commands/);
      await assert.rejects(client.version(), /setup is in progress/);
    } finally {
      if (pid && processAlive(pid)) process.kill(pid, 'SIGKILL');
      await rm(root, { recursive: true, force: true });
    }
  });

async function readPID(path: string): Promise<number> {
  const deadline = Date.now() + 5000;
  for (;;) {
    try {
      const pid = Number(await readFile(path, 'utf8'));
      if (!Number.isSafeInteger(pid) || pid <= 0) throw new Error('PID has not been written.');
      return pid;
    }
    catch (error) {
      if (Date.now() >= deadline) throw error;
      await new Promise(resolve => setTimeout(resolve, 10));
    }
  }
}

function processAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    if (process.platform === 'linux' && /\) [ZX] /.test(readFileSync(`/proc/${pid}/stat`, 'utf8'))) return false;
    return true;
  }
  catch (error) {
    if (error && typeof error === 'object' && 'code' in error && (error.code === 'ESRCH' || error.code === 'ENOENT')) return false;
    throw error;
  }
}
