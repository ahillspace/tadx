import assert from 'node:assert/strict';
import { mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { after, before, test } from 'node:test';
import { buildSite, publicFiles } from '../build-site.mjs';

const repo = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const pages = ['index.html', 'capabilities.html', 'security.html'];
const origin = 'https://tadx.net';
let folder;
let destination;

before(async () => {
  folder = await mkdtemp(join(tmpdir(), 'tadx-site-test-'));
  destination = await buildSite(repo, join(folder, 'public'));
});

after(async () => {
  if (folder) await rm(folder, { recursive: true, force: true });
});

test('public build contains only reviewed pages, assets, capability data, and exact installer sources', async () => {
  assert.deepEqual((await readdir(destination)).sort(), [
    '.nojekyll', 'capabilities.css', 'capabilities.html', 'capabilities.js', 'capabilities.json', 'home.css',
    'home.js', 'index.html', 'install.ps1', 'install.sh', 'security.css', 'security.html', 'shared.css',
    'space-grotesk-OFL.txt', 'space-grotesk.woff2',
  ]);
  for (const [source, target] of publicFiles) {
    assert.deepEqual(await readFile(join(destination, target)), await readFile(join(repo, source)));
  }
  await assert.rejects(buildSite(repo, destination), /empty real directory/);
  await writeFile(join(folder, 'not-a-directory'), 'preserve me');
  await assert.rejects(buildSite(repo, join(folder, 'not-a-directory')), /empty real directory/);
  assert.equal(await readFile(join(folder, 'not-a-directory'), 'utf8'), 'preserve me');
});

test('public pages cross-link and packaged local navigation resolves', async () => {
  for (const page of pages) {
    const html = await readFile(join(destination, page), 'utf8');
    const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
    assert.equal(new Set(ids).size, ids.length, `Duplicate identifiers in ${page}`);
    for (const match of html.matchAll(/\baria-(?:controls|labelledby)="([^"]+)"/g)) {
      for (const id of match[1].split(' ')) assert.ok(ids.includes(id), `Missing accessible target ${id} in ${page}`);
    }

    const navigation = new Set();
    for (const [tag, href] of html.matchAll(/<(?:a|link|script)\b[^>]*?\s(?:href|src)="([^"]+)"/g)) {
      const url = new URL(href, `${origin}/${page}`);
      if (url.origin !== origin) {
        assert.match(tag, /^<a\b/, `Expected a locally packaged stylesheet or script in ${page}`);
        const repoLink = /^https:\/\/github\.com\/ahillspace\/tadx\/blob\/main\/([^#]+)$/.exec(url.origin + url.pathname);
        if (repoLink) await readFile(join(repo, repoLink[1]));
        continue;
      }
      const file = url.pathname.slice(1) || 'index.html';
      await readFile(join(destination, file));
      if (file === page && url.hash) assert.ok(ids.includes(url.hash.slice(1)), `Invalid anchor ${href} in ${page}`);
      if (tag.startsWith('<a ')) navigation.add(file);
    }
    for (const otherPage of pages.filter(other => other !== page)) {
      assert.ok(navigation.has(otherPage), `${page} should link to ${otherPage}`);
    }
  }
});

test('the public capability inventory has current CLI command paths and unique IDs', async () => {
  const registry = JSON.parse(await readFile(join(destination, 'capabilities.json'), 'utf8'));
  assert.equal(new Set(registry.map(capability => capability.id)).size, registry.length);
  const commands = new Set();
  for (const capability of registry.filter(capability => capability.owner === 'cli')) {
    assert.ok((capability.command_path || []).length <= 3, `Too many command levels: ${capability.surface}`);
    commands.add(capability.surface);
  }
  const hasRoute = route => [...commands].some(command => command === route || command.startsWith(route + ' '));
  for (const route of ['tadx catalog label', 'tadx admin label-value', 'tadx admin label-category']) {
    assert.ok(hasRoute(route), `Missing current route: ${route}`);
  }
  assert.ok(!hasRoute('tadx content label'));
});

test('the public homepage lists the supported agents', async () => {
  const html = await readFile(join(destination, 'index.html'), 'utf8');
  const targetSource = await readFile(join(repo, 'internal/agenttarget/targets.go'), 'utf8');
  const labels = {claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode', cursor: 'Cursor', pi: 'Pi', hermes: 'Hermes', copilot: 'GitHub Copilot', gemini: 'Gemini CLI', cline: 'Cline'};
  const advertised = [...html.matchAll(/class="agent-name">([^<]+)<\/span>/g)].map(match => match[1]).sort();
  const supported = [...targetSource.matchAll(/name: "([^"]+)"/g)].map(match => match[1]).filter(name => name !== 'generic');
  for (const name of supported) assert.ok(labels[name], `Add homepage label for supported harness: ${name}`);
  assert.deepEqual(advertised, supported.map(name => labels[name]).sort());
});

test('the public security overview keeps its native disclosures and stable section links', async () => {
  const html = await readFile(join(destination, 'security.html'), 'utf8');
  assert.doesNotMatch(html, /<script\b|<iframe\b|<form\b/);
  assert.ok([...html.matchAll(/<details>/g)].length >= 10, 'Missing progressive disclosure');
  assert.doesNotMatch(html, /<details\b[^>]*\bopen\b/, 'Details should start collapsed');
  for (const id of ['controls', 'templates', 'credentials', 'policy-installation', 'local-data', 'boundaries']) {
    assert.ok(html.includes('href="#' + id + '"'), 'Missing section navigation ' + id);
  }
  for (const text of ['tadx mutation status', 'tadx policy status', 'tadx policy install --template read-only', 'no enforced approval prompt', 'Any failed check blocks the change', 'not a sandbox', 'checksum', 'Secret Service']) {
    assert.ok(html.includes(text), 'Missing public security guidance: ' + text);
  }
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
