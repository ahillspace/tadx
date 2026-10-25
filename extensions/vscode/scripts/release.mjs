import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { open } from 'yauzl';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(import.meta.url);

function options(args) {
  const result = { command: args[0], root, channel: 'stable', requireTag: false };
  if (!['check', 'package', 'verify'].includes(result.command)) throw new Error('Use release.mjs check, package, or verify.');
  for (let i = 1; i < args.length; i++) {
    const arg = args[i];
    if (arg === '--require-tag') result.requireTag = true;
    else if (['--tag', '--channel', '--root'].includes(arg) && args[i + 1] && !args[i + 1].startsWith('--')) {
      result[arg.slice(2)] = args[++i];
    } else throw new Error(`Unknown or incomplete release option: ${arg}`);
  }
  result.root = resolve(result.root);
  return result;
}

function git(cwd, args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8', windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

async function metadata(settings) {
  const manifest = JSON.parse(await readFile(join(settings.root, 'package.json'), 'utf8'));
  const lock = JSON.parse(await readFile(join(settings.root, 'package-lock.json'), 'utf8'));
  const version = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(manifest.version);
  if (!version || version.slice(1).some(value => !Number.isSafeInteger(Number(value)))) {
    throw new Error('Extension versions must be numeric major.minor.patch without prerelease or build suffixes.');
  }
  if (manifest.name !== 'tadx' || manifest.publisher !== 'ahillspace') throw new Error('The extension identity must remain ahillspace.tadx.');
  if (lock.version !== manifest.version || lock.packages?.['']?.version !== manifest.version
      || lock.name !== manifest.name || lock.packages?.['']?.name !== manifest.name) {
    throw new Error('package.json and package-lock.json must have matching names and versions.');
  }
  if (!['stable', 'prerelease'].includes(settings.channel)) throw new Error('The release channel must be stable or prerelease.');
  const prerelease = settings.channel === 'prerelease';
  if (Number(version[2]) % 2 !== Number(prerelease)) {
    throw new Error('Stable releases use even minor versions; prereleases use odd minor versions.');
  }
  const tag = `extension-v${manifest.version}`;
  if (settings.tag && settings.tag !== tag) throw new Error(`The release tag must match the manifest exactly: ${tag}`);
  if (settings.requireTag) {
    const head = git(settings.root, ['rev-parse', 'HEAD']);
    if (git(settings.root, ['rev-parse', `refs/tags/${tag}^{commit}`]) !== head) {
      throw new Error('The release tag must identify the checked-out commit.');
    }
    if (git(settings.root, ['status', '--porcelain', '--untracked-files=all'])) throw new Error('Release source must have a clean working tree.');
  }
  return { version: manifest.version, publisher: manifest.publisher, name: manifest.name,
    channel: settings.channel, prerelease, tag, file: `tadx-${manifest.version}.vsix` };
}

function sha256(bytes) { return createHash('sha256').update(bytes).digest('hex'); }

async function inspectPackage(path, expected) {
  const entries = await new Promise((resolveZip, reject) => {
    open(path, { lazyEntries: true }, (error, zip) => {
      if (error) { reject(error); return; }
      const names = new Set();
      const selected = new Map();
      let total = 0;
      const fail = reason => { zip.close(); reject(reason); };
      zip.once('error', fail);
      zip.once('end', () => {
        for (const required of ['[content_types].xml', 'extension.vsixmanifest', 'extension/package.json', 'extension/readme.md', 'extension/license.txt', 'extension/dist/extension.js', 'extension/dist/installers/install.sh', 'extension/dist/installers/install.ps1']) {
          if (!names.has(required)) { reject(new Error(`Missing release package entry: ${required}`)); return; }
        }
        resolveZip(selected);
      });
      zip.on('entry', entry => {
        const name = entry.fileName;
        if (names.has(name.toLowerCase()) || names.size >= 100
            || !/^(?:\[Content_Types\]\.xml|extension\.vsixmanifest|extension\/(?:package\.json|readme\.md|LICENSE\.txt|dist\/extension\.js|dist\/installers\/install\.(?:sh|ps1)|media\/[^/]+))$/.test(name)) {
          fail(new Error(`Unexpected or duplicate release package entry: ${name}`)); return;
        }
        names.add(name.toLowerCase());
        total += entry.uncompressedSize;
        if (total > 16 * 1024 * 1024) { fail(new Error('Release package exceeds the 16 MiB uncompressed limit.')); return; }
        if (!['extension/package.json', 'extension.vsixmanifest'].includes(name)) { zip.readEntry(); return; }
        if (entry.uncompressedSize > 1024 * 1024) { fail(new Error('Release metadata exceeds 1 MiB.')); return; }
        zip.openReadStream(entry, (streamError, stream) => {
          if (streamError) { fail(streamError); return; }
          const chunks = [];
          stream.on('data', chunk => chunks.push(chunk));
          stream.once('error', fail);
          stream.once('end', () => { selected.set(name, Buffer.concat(chunks).toString('utf8')); zip.readEntry(); });
        });
      });
      zip.readEntry();
    });
  });
  const manifest = JSON.parse(entries.get('extension/package.json') ?? 'null');
  if (!manifest || manifest.name !== expected.name || manifest.publisher !== expected.publisher || manifest.version !== expected.version) {
    throw new Error('The VSIX identity or version differs from the release manifest.');
  }
  const xml = entries.get('extension.vsixmanifest') ?? '';
  const identities = [...xml.matchAll(/<Identity\b([^>]*)\/?\s*>/g)];
  const attributes = value => Object.fromEntries([...value.matchAll(/([\w.]+)="([^"]*)"/g)].map(match => [match[1], match[2]]));
  const identity = identities.length === 1 ? attributes(identities[0][1]) : {};
  if (identity.Id !== expected.name || identity.Publisher !== expected.publisher || identity.Version !== expected.version || identity.TargetPlatform) {
    throw new Error('The VSIX XML identity differs from this universal extension release.');
  }
  const pre = [...xml.matchAll(/<Property\b([^>]*)\/?\s*>/g)].map(match => attributes(match[1]))
    .filter(value => value.Id === 'Microsoft.VisualStudio.Code.PreRelease');
  if (pre.length > 1 || (pre.length === 1 && pre[0].Value !== 'true') || (pre.length === 1) !== expected.prerelease) {
    throw new Error('The VSIX prerelease marker differs from the selected release channel.');
  }
}

async function verify(directory, expected) {
  const release = JSON.parse(await readFile(join(directory, 'release.json'), 'utf8'));
  for (const key of ['version', 'publisher', 'name', 'channel', 'prerelease', 'tag', 'file']) {
    if (release[key] !== expected[key]) throw new Error(`Release metadata mismatch: ${key}`);
  }
  if (!/^[a-f0-9]{40}$/.test(release.commit)) throw new Error('Release metadata must identify its source commit.');
  const bytes = await readFile(join(directory, expected.file));
  if (sha256(bytes) !== release.sha256) throw new Error('The VSIX SHA-256 does not match release.json.');
  const sums = await readFile(join(directory, 'SHA256SUMS'), 'utf8');
  if (sums !== `${release.sha256}  ${expected.file}\n${sha256(await readFile(join(directory, 'release.json')))}  release.json\n`) {
    throw new Error('SHA256SUMS does not match the VSIX and release metadata.');
  }
  await inspectPackage(join(directory, expected.file), expected);
  return release;
}

async function main() {
  const settings = options(process.argv.slice(2));
  const expected = await metadata(settings);
  const directory = join(settings.root, 'dist', 'release');
  if (settings.command === 'package') {
    await mkdir(join(settings.root, 'dist'), { recursive: true });
    try { await mkdir(directory); }
    catch (error) {
      if (error.code === 'EEXIST') throw new Error('Release output already exists. Use a fresh output directory; never replace a released version.', { cause: error });
      throw error;
    }
    const path = join(directory, expected.file);
    const commit = git(settings.root, ['rev-parse', 'HEAD']);
    const { createVSIX } = require('@vscode/vsce');
    await createVSIX({ cwd: settings.root, packagePath: path, dependencies: false, preRelease: expected.prerelease,
      baseContentUrl: `https://github.com/ahillspace/tadx/blob/${commit}/extensions/vscode`,
      baseImagesUrl: `https://github.com/ahillspace/tadx/raw/${commit}/extensions/vscode` });
    await inspectPackage(path, expected);
    const release = { ...expected, commit,
      sourceDirty: !!git(settings.root, ['status', '--porcelain', '--untracked-files=all']),
      sha256: sha256(await readFile(path)) };
    const document = `${JSON.stringify(release, null, 2)}\n`;
    await writeFile(join(directory, 'release.json'), document);
    await writeFile(join(directory, 'SHA256SUMS'), `${release.sha256}  ${expected.file}\n${sha256(document)}  release.json\n`);
    await verify(directory, expected);
  } else if (settings.command === 'verify') {
    const release = await verify(directory, expected);
    if (settings.requireTag && (release.sourceDirty !== false || release.commit !== git(settings.root, ['rev-parse', 'HEAD']))) {
      throw new Error('The VSIX must come from the clean, tagged source commit.');
    }
  }
  console.log(JSON.stringify(expected));
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });
