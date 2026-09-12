import assert from 'node:assert/strict';
import { mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { buildSite, publicFiles } from '../build-site.mjs';

const repo = dirname(dirname(dirname(fileURLToPath(import.meta.url))));

test('public build contains only reviewed homepage and exact installer sources', async () => {
  const folder = await mkdtemp(join(tmpdir(), 'tadx-site-test-'));
  try {
    const destination = join(folder, 'public');
    await buildSite(repo, destination);
    assert.deepEqual((await readdir(destination)).sort(), ['.nojekyll', 'index.html', 'install.ps1', 'install.sh']);
    for (const [source, target] of publicFiles) {
      assert.deepEqual(await readFile(join(destination, target)), await readFile(join(repo, source)));
    }
    await assert.rejects(buildSite(repo, destination), /empty real directory/);
    await writeFile(join(folder, 'not-a-directory'), 'preserve me');
    await assert.rejects(buildSite(repo, join(folder, 'not-a-directory')), /empty real directory/);
    assert.equal(await readFile(join(folder, 'not-a-directory'), 'utf8'), 'preserve me');
  } finally { await rm(folder, { recursive: true, force: true }); }
});

test('homepage has both one-line installers, current setup, and a metadata example', async () => {
  const html = await readFile(join(repo, 'site/index.html'), 'utf8');
  for (const text of ['Tableau.', 'At your command.', 'irm https://tadx.net/install.ps1 | iex', 'curl -fsSL https://tadx.net/install.sh | sh', 'tadx update', 'tadx catalog column update', '--table-id', 'agent Guidance']) {
    assert.ok(html.includes(text), `Missing ${text}`);
  }
  assert.doesNotMatch(html, /https?:\/\/[^"'<>\s]+\.(?:js|css)(?:["'<>\s]|$)/i, 'Keep the homepage self-contained');
  assert.doesNotMatch(html, /C:[/\\]Users|releases\/latest\/download\/install|a62e3d7/);
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(ids.length, new Set(ids).size, 'Duplicate DOM identifiers');
  for (const match of html.matchAll(/aria-(?:controls|labelledby)="([^"]+)"/g)) {
    for (const id of match[1].split(' ')) assert.ok(ids.includes(id), `Missing accessible target ${id}`);
  }
  const script = /<script>([\s\S]*?)<\/script>/.exec(html)?.[1];
  assert.ok(script);
  new Function(script);
});

test('Pages is manual and publishes only the allowlisted build', async () => {
  const workflow = await readFile(join(repo, '.github/workflows/pages.yml'), 'utf8');
  assert.match(workflow, /workflow_dispatch:/);
  assert.doesNotMatch(workflow, /^\s+(push|pull_request|schedule):/m);
  assert.match(workflow, /path: _site/);
  assert.match(workflow, /needs: build/);
  assert.match(workflow, /inputs\.publish/);
  assert.match(workflow, /github\.event\.repository\.private == false/);
});
