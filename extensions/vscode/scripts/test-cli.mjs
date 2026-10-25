import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { promisify } from 'node:util';
import { build } from 'esbuild';

const extensionRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repository = resolve(extensionRoot, '..', '..');
const parent = resolve(tmpdir());
const root = await mkdtemp(join(parent, 'tadx-vscode-cli-'));
const child = relative(parent, root);
if (!child || isAbsolute(child) || child.startsWith('..') || !child.startsWith('tadx-vscode-cli-')) throw new Error('Invalid contract fixture root.');
const external = process.argv.find(arg => arg.startsWith('--executable='))?.slice('--executable='.length);
const expectedVersion = process.argv.find(arg => arg.startsWith('--expected-version='))?.slice('--expected-version='.length);
const executable = external ? resolve(external) : join(root, process.platform === 'win32' ? 'tadx.exe' : 'tadx');
const config = join(root, 'config.json');
const allowedEnvironment = new Set(['PATH', 'PATHEXT', 'SYSTEMROOT', 'WINDIR', 'TEMP', 'TMP', 'TMPDIR', 'LANG', 'LC_ALL']);
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => allowedEnvironment.has(key.toUpperCase())));
Object.assign(env, { HOME: root, USERPROFILE: root, XDG_CONFIG_HOME: join(root, 'config'), XDG_CACHE_HOME: join(root, 'cache'), APPDATA: join(root, 'appdata'), LOCALAPPDATA: join(root, 'localappdata') });
let passed = false;
try {
  await mkdir(join(root, 'appdata'));
  await writeFile(config, '{"version":1}\n');
  if (!external) await promisify(execFile)('go', ['build', '-trimpath', '-o', executable, './cmd/tadx'], { cwd: repository, windowsHide: true, timeout: 180_000 });
  const suite = join(root, 'suite.cjs');
  await build({ entryPoints: [join(extensionRoot, 'tests', 'real-cli.ts')], outfile: suite,
    bundle: true, platform: 'node', format: 'cjs', target: 'node20' });
  const { verify } = await import(pathToFileURL(suite).href);
  const runner = async (binary, args, options) => {
    try {
      const result = await promisify(execFile)(binary, ['--config', config, ...args], {
        cwd: root, env, windowsHide: true, timeout: options.timeoutMs ?? 30_000, maxBuffer: 32 * 1024 * 1024,
        signal: options.signal,
      });
      return { stdout: result.stdout, stderr: result.stderr, exitCode: 0 };
    } catch (error) {
      if (typeof error.code !== 'number') throw error;
      return { stdout: error.stdout, stderr: error.stderr, exitCode: error.code };
    }
  };
  const version = await verify(executable, root, runner);
  if (expectedVersion && version !== expectedVersion) throw new Error(`The CLI reports ${version}; expected reviewed version ${expectedVersion}.`);
  console.log(`Real CLI contract validation passed for TADX ${version}: version, capabilities, environments, workspaces, cached reads, and mutation status. No Tableau calls or credentials.`);
  passed = true;
} finally {
  if (passed) await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
  else console.error(`Contract fixture retained at ${root}`);
}
