import assert from 'node:assert/strict';
import { mkdtemp, mkdir, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { relativeArtifactPath, tableauUrl, workspaceFile } from '../src/paths.js';

test('artifact paths reject absolute paths, traversal, Windows alternate streams, and malformed segments', () => {
  for (const value of ['', '..', '../outside', 'artifact/../../outside', './artifact', 'artifact/./file', '/outside',
    'C:/outside', 'C:\\outside', '\\\\server\\share', 'artifact/file:stream', 'artifact//file', `artifact/${String.fromCharCode(0)}file`]) {
    assert.throws(() => relativeArtifactPath(value), /inside its registered workspace/, value);
  }
  assert.equal(relativeArtifactPath('artifacts/workbook/Finance & Ops--identity/payload.twbx'), 'artifacts/workbook/Finance & Ops--identity/payload.twbx');
});

test('workspace file resolution opens a real contained file and rejects unavailable roots', async t => {
  const fixture = await temporaryWorkspace();
  t.after(() => rm(fixture.directory, { recursive: true, force: true }));
  const payload = path.join(fixture.root, 'payload.twb');
  await writeFile(payload, '<workbook/>');
  assert.equal(await workspaceFile(fixture.root, 'payload.twb'), await realpath(payload));
  await assert.rejects(workspaceFile(fixture.root, '../outside/payload.twb'), /inside its registered workspace/);
  await assert.rejects(workspaceFile(undefined, 'payload.twb'), /unavailable/);
  await assert.rejects(workspaceFile('relative-root', 'payload.twb'), /unavailable/);
  await assert.rejects(workspaceFile(fixture.root, 'missing.twb'));
});

test('workspace file resolution rejects Windows junction or directory symlink escape', async t => {
  const fixture = await temporaryWorkspace();
  t.after(() => rm(fixture.directory, { recursive: true, force: true }));
  const outside = path.join(fixture.directory, 'outside');
  await mkdir(outside);
  await writeFile(path.join(outside, 'payload.twb'), '<workbook/>');
  await symlink(outside, path.join(fixture.root, 'linked'), process.platform === 'win32' ? 'junction' : 'dir');
  await assert.rejects(workspaceFile(fixture.root, 'linked/payload.twb'), /resolves outside/);
  assert.equal(await realpath(path.join(outside, 'payload.twb')), path.join(await realpath(outside), 'payload.twb'));
});

test('workspace file resolution accepts a contained directory symlink', async t => {
  const fixture = await temporaryWorkspace();
  t.after(() => rm(fixture.directory, { recursive: true, force: true }));
  const contained = path.join(fixture.root, 'actual');
  await mkdir(contained);
  await writeFile(path.join(contained, 'payload.twb'), '<workbook/>');
  await symlink(contained, path.join(fixture.root, 'linked'), process.platform === 'win32' ? 'junction' : 'dir');
  assert.equal(await workspaceFile(fixture.root, 'linked/payload.twb'), await realpath(path.join(contained, 'payload.twb')));
});

test('Tableau links require an authoritative URL rather than inventing one from a content ID', () => {
  const server = 'https://tableau.example.test';
  assert.equal(tableauUrl(`${server}/#/site/sandbox/datasources/42`, server).href, `${server}/#/site/sandbox/datasources/42`);
  assert.throws(() => tableauUrl(undefined, server), /does not expose a browser URL/);
  assert.throws(() => tableauUrl('workbook-luid', server));
  assert.throws(() => tableauUrl(`${server}/#/workbooks/42`, undefined), /does not expose a browser URL/);
  for (const url of ['https://unrelated.example.test/#/workbooks/42', 'https://tableau.example.test.evil.invalid/#/workbooks/42',
    'https://user:secret@tableau.example.test/#/workbooks/42', 'javascript:alert(1)', 'file:///tmp/payload']) {
    assert.throws(() => tableauUrl(url, server), /does not belong/);
  }
});

async function temporaryWorkspace(): Promise<{ directory: string; root: string }> {
  const directory = await mkdtemp(path.join(tmpdir(), 'tadx-extension-paths-'));
  const root = path.join(directory, 'workspace');
  await mkdir(root);
  return { directory, root };
}
