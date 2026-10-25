import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { lstat, realpath } from 'node:fs/promises';
import { homedir } from 'node:os';
import { basename, delimiter, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';

export interface VerifiedCliRelease { readonly version: string; readonly archiveSha256: string }
export interface SetupChoices {
  guidanceConsent: boolean;
  otherTransfersStopped: boolean;
  modifyPath: boolean;
  completion: boolean;
}
export interface CliInstallation { executablePath: string; version: string; managed: boolean }
export interface SetupPlan {
  readonly action: 'install' | 'update';
  readonly executablePath: string;
  readonly installDirectory: string;
  readonly currentVersion?: string;
  readonly release: VerifiedCliRelease;
}
export interface SetupCommandOptions {
  environment: NodeJS.ProcessEnv;
  signal?: AbortSignal;
  timeoutMs?: number;
  maxOutputBytes?: number;
}
export interface SetupCommandResult { stdout: string; stderr: string; exitCode: number }
export type SetupRunner = (executable: string, args: readonly string[], options: SetupCommandOptions) => Promise<SetupCommandResult>;

export class SetupError extends Error {
  constructor(message: string, readonly outcome: 'not_attempted' | 'unknown' = 'not_attempted') {
    super(message);
    this.name = 'SetupError';
  }
}

// Only transport, native credential-helper, platform, and profile-location variables reach installers.
// Arbitrarily named Tableau credential variables are excluded by the allowlist.
const allowedEnvironment = new Set(['PATH', 'PATHEXT', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'OS',
  'PROCESSOR_ARCHITECTURE', 'PROCESSOR_ARCHITEW6432', 'HOME', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH',
  'LOCALAPPDATA', 'APPDATA', 'TEMP', 'TMP', 'TMPDIR', 'SHELL', 'ZDOTDIR', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME',
  'XDG_RUNTIME_DIR', 'DBUS_SESSION_BUS_ADDRESS', 'LANG', 'LC_ALL', 'GH_TOKEN', 'GITHUB_TOKEN', 'GH_CONFIG_DIR',
  'HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY', 'SSL_CERT_FILE', 'SSL_CERT_DIR', 'CURL_CA_BUNDLE']);

export function setupEnvironment(source: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  const environment: NodeJS.ProcessEnv = {};
  for (const [key, value] of Object.entries(source)) {
    if (value !== undefined && allowedEnvironment.has(key.toUpperCase())) environment[key] = value;
  }
  environment.GH_PROMPT_DISABLED = '1';
  environment.GH_NO_UPDATE_NOTIFIER = '1';
  return environment;
}

/** Run a packaged installer directly, with bounded output and no command-string interpolation. */
export const runSetupCommand: SetupRunner = (executable, args, options) => new Promise((fulfill, reject) => {
  const timeoutMs = options.timeoutMs ?? 300_000;
  const maxBytes = options.maxOutputBytes ?? 1024 * 1024;
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || !Number.isSafeInteger(maxBytes) || maxBytes <= 0) {
    reject(new SetupError('Setup timeout and output limits must be positive integers.'));
    return;
  }
  if (options.signal?.aborted) { reject(new SetupError('Setup was canceled before the command started.')); return; }
  const child = spawn(executable, [...args], { shell: false, windowsHide: true, env: options.environment,
    stdio: ['ignore', 'pipe', 'pipe'], detached: process.platform !== 'win32' });
  const stdout: Buffer[] = [];
  const stderr: Buffer[] = [];
  let size = 0;
  let settled = false;
  const finish = (error?: SetupError, code = 1): void => {
    if (settled) return;
    settled = true;
    clearTimeout(timer);
    options.signal?.removeEventListener('abort', abort);
    if (error) {
      child.stdout.destroy();
      child.stderr.destroy();
      child.unref();
      reject(error);
    }
    else fulfill({ stdout: Buffer.concat(stdout).toString('utf8'), stderr: Buffer.concat(stderr).toString('utf8'), exitCode: code });
  };
  const stop = (message: string): void => {
    if (settled) return;
    if (child.pid && process.platform === 'win32') {
      const windows = Object.entries(options.environment).find(([key]) => key.toUpperCase() === 'SYSTEMROOT')?.[1];
      const taskkill = windows ? join(windows, 'System32', 'taskkill.exe') : 'taskkill.exe';
      const cleanup = spawn(taskkill, ['/PID', String(child.pid), '/T', '/F'], {
        shell: false, windowsHide: true, env: options.environment, stdio: 'ignore',
      });
      cleanup.once('error', () => { child.kill(); });
      cleanup.unref();
    } else {
      try { if (child.pid) process.kill(-child.pid, 'SIGTERM'); else child.kill(); }
      catch { child.kill(); }
    }
    finish(new SetupError(`${message} Installation state may be partial; rollback is not confirmed. Inspect the installed CLI before retrying.`, 'unknown'));
  };
  const abort = (): void => stop('Setup was canceled.');
  const timer = setTimeout(() => stop('Setup exceeded its time limit.'), timeoutMs);
  options.signal?.addEventListener('abort', abort, { once: true });
  if (options.signal?.aborted) abort();
  const collect = (destination: Buffer[], chunk: Buffer): void => {
    if (settled) return;
    size += chunk.length;
    if (size > maxBytes) { stop('Setup exceeded its output limit.'); return; }
    destination.push(chunk);
  };
  child.stdout.on('data', (chunk: Buffer) => collect(stdout, chunk));
  child.stderr.on('data', (chunk: Buffer) => collect(stderr, chunk));
  child.once('error', () => finish(new SetupError('The setup command could not start. Check the host shell and executable.')));
  child.once('close', code => finish(undefined, code ?? 1));
});

export function installerInvocation(platform: NodeJS.Platform, extensionPath: string, directory: string,
  release: VerifiedCliRelease, choices: SetupChoices, expectedCurrentSha256?: string): { executable: string; args: string[] } {
  validateRelease(release);
  if (expectedCurrentSha256 !== undefined && !/^(?:absent|[a-f0-9]{64})$/.test(expectedCurrentSha256)) {
    throw new SetupError('The installation target precondition must be absent or an exact SHA-256.');
  }
  if (platform === 'win32') {
    return { executable: 'powershell.exe', args: ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
      '-File', join(extensionPath, 'dist', 'installers', 'install.ps1'), '-Version', release.version,
      '-ExpectedSHA256', release.archiveSha256, '-InstallDir', directory, '-Target', 'auto',
      ...(expectedCurrentSha256 !== undefined ? ['-ExpectedCurrentSHA256', expectedCurrentSha256] : []),
      ...(!choices.modifyPath ? ['-NoModifyPath'] : []), ...(!choices.completion ? ['-NoCompletion'] : [])] };
  }
  if (platform !== 'linux' && platform !== 'darwin') throw new SetupError('Guided CLI setup supports Windows, Linux, and macOS hosts.');
  return { executable: '/bin/sh', args: [join(extensionPath, 'dist', 'installers', 'install.sh'),
    '--version', release.version, '--expected-sha256', release.archiveSha256, '--install-dir', directory,
    ...(expectedCurrentSha256 !== undefined ? ['--expected-current-sha256', expectedCurrentSha256] : []),
    '--target', 'auto', ...(!choices.modifyPath ? ['--no-modify-path'] : []), ...(!choices.completion ? ['--no-completion'] : [])] };
}

interface SetupOptions {
  extensionPath: string;
  home?: string;
  environment?: NodeJS.ProcessEnv;
  run?: SetupRunner;
}

/** Plans are previewed before confirmation and consumed exactly once by this service. */
export class SetupService {
  private readonly environment: NodeJS.ProcessEnv;
  private readonly home: string;
  private readonly run: SetupRunner;
  private readonly plans = new WeakMap<SetupPlan, { fingerprint: string | undefined }>();
  private readonly executableName = process.platform === 'win32' ? 'tadx.exe' : 'tadx';

  constructor(private readonly options: SetupOptions) {
    this.home = resolve(options.home ?? homedir());
    this.environment = setupEnvironment(options.environment ?? process.env);
    this.run = options.run ?? runSetupCommand;
  }

  async inspect(configuredExecutable: string, signal?: AbortSignal): Promise<CliInstallation> {
    return (await this.inspectInstallation(configuredExecutable, signal)).installation;
  }

  private async inspectInstallation(configuredExecutable: string, signal?: AbortSignal): Promise<{ installation: CliInstallation; fingerprint: string }> {
    const candidate = await this.findExecutable(configuredExecutable);
    const path = await realpath(candidate);
    const before = await fingerprint(path);
    if (before === undefined) throw new SetupError('The selected executable changed during inspection. Inspect it again.');
    const response = await this.run(path, ['version', '--full', '--json'], { environment: this.environment, signal, timeoutMs: 30_000 });
    let document: unknown;
    try { document = JSON.parse(response.stdout); } catch { throw new SetupError('The selected executable did not return TADX version JSON.'); }
    if (response.exitCode !== 0 || !document || typeof document !== 'object' || !('version' in document) || typeof document.version !== 'string'
      || !document.version.trim() || 'error' in document) throw new SetupError('The selected executable did not return a valid TADX version.');
    if (before !== await fingerprint(path)) throw new SetupError('The selected executable changed during inspection. Inspect it again.');
    let managed = false;
    try { managed = samePath(candidate, path) && samePath(path, await this.standardTarget()); }
    catch (error) { if (!(error instanceof SetupError)) throw error; }
    return { installation: { executablePath: path, version: document.version, managed }, fingerprint: before };
  }

  async prepareInstall(release: VerifiedCliRelease): Promise<SetupPlan> {
    validateRelease(release);
    const executablePath = await this.standardTarget();
    if (await fingerprint(executablePath) !== undefined) throw new SetupError('A CLI already exists at the shared user installation. Choose it or use Update TADX.');
    return this.remember({ action: 'install', executablePath, installDirectory: dirname(executablePath), release }, undefined);
  }

  async prepareUpdate(configuredExecutable: string, release: VerifiedCliRelease, signal?: AbortSignal): Promise<SetupPlan> {
    validateRelease(release);
    const { installation: installed, fingerprint: inspectedFingerprint } = await this.inspectInstallation(configuredExecutable, signal);
    if (!installed.managed || !samePath(basename(installed.executablePath), this.executableName)) {
      throw new SetupError('This is a custom or package manager installation. Update it through its owner or rebuild its source, then choose that executable.');
    }
    const installedVersion = stableVersion(installed.version);
    if (!installedVersion) throw new SetupError('This is a development or pre-release CLI. Update its source or installation owner; guided setup does not replace it.');
    const targetVersion = stableVersion(release.version)!;
    if (compareVersions(installedVersion, targetVersion) > 0) throw new SetupError('The installed CLI is newer than this extension\'s verified release. Keep it if compatible; guided setup does not downgrade it.');
    return this.remember({ action: 'update', executablePath: installed.executablePath, installDirectory: dirname(installed.executablePath),
      currentVersion: installed.version, release }, inspectedFingerprint);
  }

  async execute(plan: SetupPlan, choices: SetupChoices, signal?: AbortSignal): Promise<CliInstallation> {
    const observed = this.plans.get(plan);
    if (!observed) throw new SetupError('Create a new setup preview before continuing.');
    if (choices.guidanceConsent !== true) throw new SetupError('Confirm installation of the CLI and bundled agent Guidance before continuing.');
    if (choices.otherTransfersStopped !== true) throw new SetupError('Confirm that other TADX transfers and CLI activity have finished before continuing.');
    if (signal?.aborted) throw new SetupError('Setup was canceled before installation started.');
    if (!samePath(plan.executablePath, await this.standardTarget()) || observed.fingerprint !== await fingerprint(plan.executablePath)) {
      throw new SetupError('The installation target changed after preview. Create a new setup preview before continuing.');
    }
    const command = installerInvocation(process.platform, this.options.extensionPath, plan.installDirectory, plan.release, choices, observed.fingerprint ?? 'absent');
    const script = command.args[process.platform === 'win32' ? command.args.indexOf('-File') + 1 : 0]!;
    const scriptInfo = await lstat(script);
    if (!scriptInfo.isFile() || scriptInfo.isSymbolicLink()) throw new SetupError('The bundled installer is unavailable or redirected. Reinstall the extension.');
    if (process.platform === 'win32') {
      const windows = Object.entries(this.environment).find(([key]) => key.toUpperCase() === 'SYSTEMROOT')?.[1];
      if (windows) command.executable = join(windows, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
    }
    this.plans.delete(plan);
    let installerReturned = false;
    try {
      const result = await this.run(command.executable, command.args, { environment: this.environment, signal, timeoutMs: 300_000, maxOutputBytes: 1024 * 1024 });
      installerReturned = true;
      if (result.exitCode !== 0) throw new Error('The installer returned a failure.');
      const installed = await this.inspect(plan.executablePath, signal);
      if (installed.version.replace(/^v/, '') !== plan.release.version) throw new Error('The installed version does not match the selected release.');
      return installed;
    } catch (error) {
      if (!installerReturned && error instanceof SetupError && error.outcome === 'not_attempted') throw error;
      throw new SetupError('TADX setup did not complete or could not be verified. Installation state may be partial; rollback is not confirmed. Inspect the installed CLI and Guidance before retrying.', 'unknown');
    }
  }

  private remember(plan: SetupPlan, value: string | undefined): SetupPlan {
    const captured = Object.freeze({ ...plan, release: Object.freeze({ ...plan.release }) });
    this.plans.set(captured, { fingerprint: value });
    return captured;
  }

  private async standardTarget(): Promise<string> {
    const root = process.platform === 'win32'
      ? Object.entries(this.environment).find(([key]) => key.toUpperCase() === 'LOCALAPPDATA')?.[1] ?? join(this.home, 'AppData', 'Local')
      : this.home;
    if (!isAbsolute(root)) throw new SetupError('The host user installation directory must be absolute.');
    const directory = process.platform === 'win32' ? join(root, 'Programs', 'tadx', 'bin') : join(root, '.local', 'bin');
    const canonicalRoot = await canonicalPath(root);
    const canonicalDirectory = await canonicalPath(directory);
    const offset = relative(canonicalRoot, canonicalDirectory);
    if (offset === '..' || offset.startsWith(`..${sep}`) || isAbsolute(offset)) throw new SetupError('The shared installation directory is redirected outside its user location. Repair that location before setup.');
    const target = join(canonicalDirectory, this.executableName);
    try { if ((await lstat(target)).isSymbolicLink()) throw new SetupError('The CLI installation is redirected by a link. Use its installation owner to update it.'); }
    catch (error) { if (!missing(error)) throw error; }
    return target;
  }

  private async findExecutable(value: string): Promise<string> {
    if (!value.trim() || value.includes('\0')) throw new SetupError('Choose a TADX executable.');
    const candidates = isAbsolute(value) || value.includes('/') || value.includes('\\') ? [resolve(value)]
      : (Object.entries(this.environment).find(([key]) => key.toUpperCase() === 'PATH')?.[1] ?? '').split(delimiter)
        .filter(directory => isAbsolute(directory)).map(directory => join(directory, process.platform === 'win32' && !value.toLowerCase().endsWith('.exe') ? `${value}.exe` : value));
    for (const candidate of candidates) {
      try { if ((await lstat(await realpath(candidate))).isFile()) return candidate; }
      catch (error) { if (!missing(error)) throw error; }
    }
    throw new SetupError('TADX was not found on this workspace host. Install it or choose an existing executable.');
  }
}

function validateRelease(release: VerifiedCliRelease): void {
  if (!stableVersion(release.version) || release.version.startsWith('v') || !/^[a-f0-9]{64}$/.test(release.archiveSha256)) {
    throw new SetupError('A verified release with an exact stable version and archive SHA-256 is required.');
  }
}
function stableVersion(version: string): number[] | undefined {
  const match = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(version);
  if (!match) return undefined;
  const numbers = match.slice(1).map(Number);
  return numbers.every(Number.isSafeInteger) ? numbers : undefined;
}
function compareVersions(left: number[], right: number[]): number {
  for (let index = 0; index < 3; index++) { const difference = left[index]! - right[index]!; if (difference) return difference; }
  return 0;
}
function samePath(left: string, right: string): boolean { return process.platform === 'win32' ? left.toLowerCase() === right.toLowerCase() : left === right; }
function missing(error: unknown): boolean { return !!error && typeof error === 'object' && 'code' in error && (error.code === 'ENOENT' || error.code === 'ENOTDIR'); }
async function canonicalPath(path: string): Promise<string> {
  try { return await realpath(path); }
  catch (error) {
    if (!missing(error) || dirname(path) === path) throw error;
    return join(await canonicalPath(dirname(path)), basename(path));
  }
}
async function fingerprint(path: string): Promise<string | undefined> {
  let info;
  try { info = await lstat(path); } catch (error) { if (missing(error)) return undefined; throw error; }
  if (!info.isFile() || info.isSymbolicLink()) throw new SetupError('The installation target must be a regular executable file.');
  const maximum = 256 * 1024 * 1024;
  if (info.size > maximum) throw new SetupError('The executable exceeds the supported inspection size.');
  const hash = createHash('sha256');
  let bytes = 0;
  for await (const chunk of createReadStream(path)) {
    bytes += chunk.length;
    if (bytes > maximum) throw new SetupError('The executable changed or exceeded the supported inspection size.');
    hash.update(chunk);
  }
  return hash.digest('hex');
}
