import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

const script = resolve(__dirname, '../scripts/release.mjs');
const sha256 = (value: string | Buffer): string => createHash('sha256').update(value).digest('hex');

async function fixture(version = '0.2.0'): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'tadx-release-test-'));
  await writeFile(join(root, 'package.json'), JSON.stringify({ name: 'tadx', publisher: 'ahillspace', version }));
  await writeFile(join(root, 'package-lock.json'), JSON.stringify({ name: 'tadx', version, packages: { '': { name: 'tadx', version } } }));
  return root;
}

function run(root: string, ...args: string[]): { status: number | null; output: string } {
  const result = spawnSync(process.execPath, [script, ...args, '--root', root], { encoding: 'utf8', windowsHide: true,
    timeout: args[0] === 'package' ? 60_000 : 20_000 });
  if (result.error) throw result.error;
  return { status: result.status, output: result.stdout + result.stderr };
}

test('release identity separates extension tags from CLI releases and matches both package versions', async () => {
  const root = await fixture();
  try {
    const result = run(root, 'check', '--tag', 'extension-v0.2.0');
    assert.equal(result.status, 0, result.output);
    const metadata = JSON.parse(result.output) as { tag: string; file: string; channel: string };
    assert.equal(metadata.tag, 'extension-v0.2.0');
    assert.equal(metadata.file, 'tadx-0.2.0.vsix');
    assert.equal(metadata.channel, 'stable');
    // The CLI release workflow listens for v*.*.* tags.
    assert.equal(/^v.*\..*\..*$/.test(metadata.tag), false);
    const mismatch = run(root, 'check', '--tag', 'extension-v0.4.0');
    assert.equal(mismatch.status, 1);
    assert.match(mismatch.output, /match the manifest exactly/);
    await writeFile(join(root, 'package-lock.json'), '{"name":"tadx","version":"0.1.0"}');
    const drift = run(root, 'check');
    assert.equal(drift.status, 1);
    assert.match(drift.output, /matching names and versions/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('release channels reject ambiguous suffixes and enforce distinct stable and prerelease version lines', async () => {
  for (const [version, channel, expected] of [
    ['0.2.0', 'stable', 0], ['0.3.0', 'prerelease', 0], ['0.2.0', 'prerelease', 1],
    ['0.3.0', 'stable', 1], ['0.2.0-beta.1', 'prerelease', 1], ['0.2.0+build', 'stable', 1],
    ['00.2.0', 'stable', 1], ['0.2.0', 'nightly', 1],
  ] as const) {
    const root = await fixture(version);
    try { const result = run(root, 'check', '--channel', channel); assert.equal(result.status, expected, `${version} ${channel}: ${result.output}`); }
    finally { await rm(root, { recursive: true, force: true }); }
  }
});

test('release packaging refuses to overwrite an existing version before building', async () => {
  const root = await fixture();
  try {
    await mkdir(join(root, 'dist/release'), { recursive: true });
    await writeFile(join(root, 'dist/release/tadx-0.2.0.vsix'), 'existing-release');
    const result = run(root, 'package');
    assert.equal(result.status, 1);
    assert.match(result.output, /already exists/);
    assert.equal(await readFile(join(root, 'dist/release/tadx-0.2.0.vsix'), 'utf8'), 'existing-release');
  } finally { await rm(root, { recursive: true, force: true }); }
});

async function releaseFixture(root: string, options: { embeddedVersion?: string; prerelease?: boolean; extraFile?: string } = {}): Promise<void> {
  const version = '0.2.0';
  const file = `tadx-${version}.vsix`;
  const manifest = { name: 'tadx', publisher: 'ahillspace', version: options.embeddedVersion ?? version };
  const entries: [string, string][] = [
    ['extension/package.json', JSON.stringify(manifest)],
    ['extension.vsixmanifest', `<PackageManifest><Metadata><Identity Id="tadx" Publisher="ahillspace" Version="${manifest.version}"/><Properties>${options.prerelease ? '<Property Id="Microsoft.VisualStudio.Code.PreRelease" Value="true"/>' : ''}</Properties></Metadata></PackageManifest>`],
    ['extension/dist/extension.js', 'exports.activate = () => {};'],
    ['extension/dist/installers/install.sh', '#!/bin/sh\nexit 0\n'], ['extension/dist/installers/install.ps1', 'exit 0\n'],
    ['extension/readme.md', '# Fixture'], ['extension/LICENSE.txt', 'Fixture license'], ['[Content_Types].xml', '<Types/>'],
  ];
  if (options.extraFile) entries.push([options.extraFile, 'unexpected']);
  const bytes = storedZip(entries);
  const directory = join(root, 'dist/release');
  await mkdir(directory, { recursive: true });
  await writeFile(join(directory, file), bytes);
  const release = `${JSON.stringify({ name: 'tadx', publisher: 'ahillspace', version, channel: 'stable', prerelease: false,
    tag: `extension-v${version}`, file, commit: 'a'.repeat(40), sha256: sha256(bytes) }, null, 2)}\n`;
  await writeFile(join(directory, 'release.json'), release);
  await writeFile(join(directory, 'SHA256SUMS'), `${sha256(bytes)}  ${file}\n${sha256(release)}  release.json\n`);
}

test('release verification inspects the actual VSIX identity, channel, contents, and digest', async () => {
  const root = await fixture();
  try {
    await releaseFixture(root);
    let result = run(root, 'verify');
    assert.equal(result.status, 0, result.output);
    await writeFile(join(root, 'dist/release/tadx-0.2.0.vsix'), 'corrupted');
    result = run(root, 'verify');
    assert.equal(result.status, 1);
    assert.match(result.output, /SHA-256/);
    await releaseFixture(root, { embeddedVersion: '0.4.0' });
    result = run(root, 'verify');
    assert.equal(result.status, 1);
    assert.match(result.output, /identity or version differs/);
    await releaseFixture(root, { prerelease: true });
    result = run(root, 'verify');
    assert.equal(result.status, 1);
    assert.match(result.output, /prerelease marker/);
    await releaseFixture(root, { extraFile: 'extension/node_modules/secret.txt' });
    result = run(root, 'verify');
    assert.equal(result.status, 1);
    assert.match(result.output, /Unexpected or duplicate/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('release packaging verifies the real vsce output including normalized README and license names', async () => {
  const root = await fixture();
  try {
    await writeFile(join(root, 'package.json'), JSON.stringify({ name: 'tadx', displayName: 'TADX release fixture',
      publisher: 'ahillspace', version: '0.2.0', description: 'Isolated release packaging contract fixture.',
      engines: { vscode: '^1.95.0' }, main: './dist/extension.js', activationEvents: ['onStartupFinished'], license: 'MIT',
      repository: { type: 'git', url: 'https://github.com/ahillspace/tadx.git' } }));
    await mkdir(join(root, 'dist/installers'), { recursive: true });
    await writeFile(join(root, 'dist/extension.js'), 'exports.activate = () => {};\n');
    await writeFile(join(root, 'dist/installers/install.sh'), '#!/bin/sh\nexit 0\n');
    await writeFile(join(root, 'dist/installers/install.ps1'), 'exit 0\n');
    await writeFile(join(root, 'README.md'), '# Release fixture\n\nIsolated packaging contract.\n');
    await writeFile(join(root, 'LICENSE'), 'MIT License\n');
    await writeFile(join(root, '.vscodeignore'), '**\n!package.json\n!README.md\n!LICENSE\n!dist/extension.js\n!dist/installers/*\n');
    for (const args of [
      ['init', '-q'], ['add', '--all'],
      ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test', '-c', 'commit.gpgsign=false',
        '-c', 'core.hooksPath=.git/no-hooks', 'commit', '-qm', 'Release fixture'],
    ]) {
      const result = spawnSync('git', args, { cwd: root, encoding: 'utf8', windowsHide: true });
      assert.equal(result.status, 0, result.stderr);
    }
    const packaged = run(root, 'package');
    assert.equal(packaged.status, 0, packaged.output);
    const verified = run(root, 'verify');
    assert.equal(verified.status, 0, verified.output);
  } finally { await rm(root, { recursive: true, force: true }); }
});

// A small standards-compliant stored ZIP makes corruption tests independent of vsce packaging.
function storedZip(entries: [string, string][]): Buffer {
  const files: Buffer[] = [];
  const directory: Buffer[] = [];
  let offset = 0;
  for (const [name, text] of entries) {
    const filename = Buffer.from(name);
    const body = Buffer.from(text);
    let crc = 0xffffffff;
    for (const byte of body) {
      crc ^= byte;
      for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
    crc = (crc ^ 0xffffffff) >>> 0;
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0); local.writeUInt16LE(20, 4); local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(body.length, 18); local.writeUInt32LE(body.length, 22); local.writeUInt16LE(filename.length, 26);
    files.push(local, filename, body);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6);
    central.writeUInt32LE(crc, 16); central.writeUInt32LE(body.length, 20); central.writeUInt32LE(body.length, 24);
    central.writeUInt16LE(filename.length, 28); central.writeUInt32LE(offset, 42);
    directory.push(central, filename);
    offset += local.length + filename.length + body.length;
  }
  const central = Buffer.concat(directory);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(entries.length, 8); end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(central.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...files, central, end]);
}
