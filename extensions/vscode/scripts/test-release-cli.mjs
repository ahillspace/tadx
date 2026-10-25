import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, lstat, mkdir, mkdtemp, open, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const extensionRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const run = promisify(execFile);
const limit = 128 * 1024 * 1024;

async function download(url, file) {
  const response = await globalThis.fetch(url, { signal: globalThis.AbortSignal.timeout(120_000) });
  if (!response.ok || !response.body) throw new Error(`Pinned release download failed (HTTP ${response.status}). Configure GH_TOKEN for private repository access.`);
  if (Number(response.headers.get('content-length')) > limit) throw new Error('Pinned release archive exceeds the size limit.');
  const target = await open(file, 'wx');
  let size = 0;
  try {
    for await (const chunk of response.body) {
      size += chunk.length;
      if (size > limit) throw new Error('Pinned release archive exceeds the size limit.');
      await target.writeFile(chunk);
    }
  } finally { await target.close(); }
}

async function main() {
  const extra = process.argv.slice(2);
  if (extra.length > 1 || extra.some(arg => !arg.startsWith('--archive=') || !arg.slice('--archive='.length))) throw new Error('Usage: test-release-cli.mjs [--archive=<already downloaded pinned archive>]');
  const policy = JSON.parse(await readFile(join(extensionRoot, 'cli-release.json'), 'utf8'));
  if (typeof policy.version !== 'string' || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(policy.version)) throw new Error('The reviewed CLI release version is invalid.');
  const platform = { win32: 'windows', linux: 'linux', darwin: 'darwin' }[process.platform];
  const architecture = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  const expected = policy.archives?.[`${process.platform}-${process.arch}`];
  if (!platform || !architecture || typeof expected !== 'string' || !/^[a-f0-9]{64}$/.test(expected)) throw new Error('No reviewed CLI archive is configured for this host.');
  const archiveName = `tadx_${policy.version}_${platform}_${architecture}.${process.platform === 'win32' ? 'zip' : 'tar.gz'}`;
  const parent = resolve(tmpdir());
  const root = await mkdtemp(join(parent, 'tadx-released-cli-'));
  const child = relative(parent, root);
  if (!child || isAbsolute(child) || child.startsWith('..') || !child.startsWith('tadx-released-cli-')) throw new Error('Invalid released CLI fixture root.');
  try {
    const archive = extra[0] ? resolve(extra[0].slice('--archive='.length)) : join(root, archiveName);
    if (!extra.length) {
      try {
        await run('gh', ['release', 'download', `v${policy.version}`, '--repo', 'ahillspace/tadx', '--pattern', archiveName, '--dir', root], {
          windowsHide: true, timeout: 120_000, maxBuffer: 1024 * 1024,
          env: { ...process.env, GH_PROMPT_DISABLED: '1', GH_NO_UPDATE_NOTIFIER: '1' },
        });
      } catch {
        await rm(archive, { force: true });
        await download(`https://github.com/ahillspace/tadx/releases/download/v${policy.version}/${archiveName}`, archive);
      }
    }
    if ((await stat(archive)).size > limit) throw new Error('Pinned release archive exceeds the size limit.');
    const actual = createHash('sha256').update(await readFile(archive)).digest('hex');
    if (actual !== expected) throw new Error('Pinned release archive checksum mismatch. The archive was not extracted or executed.');
    const extracted = join(root, 'extracted');
    await mkdir(extracted);
    const member = process.platform === 'win32' ? 'tadx.exe' : 'tadx';
    await run('tar', ['-xf', archive, '-C', extracted, member], { windowsHide: true, timeout: 30_000, maxBuffer: 1024 * 1024 });
    const executable = join(extracted, member);
    const file = await lstat(executable);
    if (!file.isFile() || file.isSymbolicLink() || file.nlink !== 1) throw new Error('The pinned release did not contain a regular CLI executable.');
    if (process.platform !== 'win32') await chmod(executable, 0o755);
    const result = await run(process.execPath, [join(extensionRoot, 'scripts', 'test-cli.mjs'), `--executable=${executable}`, `--expected-version=${policy.version}`], {
      cwd: extensionRoot, windowsHide: true, timeout: 180_000, maxBuffer: 1024 * 1024,
    });
    process.stdout.write(result.stdout);
    console.log(`Reviewed release v${policy.version} verified against ${process.platform}-${process.arch} SHA-256 policy.`);
  } finally { await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 }); }
}

try { await main(); }
catch (error) { console.error(error.message); process.exitCode = 1; }
