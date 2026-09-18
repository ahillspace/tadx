import assert from 'node:assert/strict';
import { mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { buildSite, publicFiles } from '../build-site.mjs';

const repo = dirname(dirname(dirname(fileURLToPath(import.meta.url))));

test('public build contains only reviewed pages, capability data, and exact installer sources', async () => {
  const folder = await mkdtemp(join(tmpdir(), 'tadx-site-test-'));
  try {
    const destination = join(folder, 'public');
    await buildSite(repo, destination);
    assert.deepEqual((await readdir(destination)).sort(), ['.nojekyll', 'capabilities.html', 'capabilities.json', 'index.html', 'install.ps1', 'install.sh']);
    for (const [source, target] of publicFiles) {
      assert.deepEqual(await readFile(join(destination, target)), await readFile(join(repo, source)));
    }
    await assert.rejects(buildSite(repo, destination), /empty real directory/);
    await writeFile(join(folder, 'not-a-directory'), 'preserve me');
    await assert.rejects(buildSite(repo, join(folder, 'not-a-directory')), /empty real directory/);
    assert.equal(await readFile(join(folder, 'not-a-directory'), 'utf8'), 'preserve me');
  } finally { await rm(folder, { recursive: true, force: true }); }
});

test('Docs opens the hosted command browser with current registry data and return navigation', async () => {
  const home = await readFile(join(repo, 'site/index.html'), 'utf8');
  const map = await readFile(join(repo, 'docs/reference/capability-map.html'), 'utf8');
  const data = JSON.parse(await readFile(join(repo, 'docs/reference/capabilities.json'), 'utf8'));
  const links = [...home.matchAll(/<a\b[^>]*href="([^"]+)"[^>]*>Docs<\/a>/g)];
  assert.equal(links.length, 2);
  for (const link of links) assert.equal(link[1], 'capabilities.html');
  assert.match(map, /href="https:\/\/tadx\.net\/"/);
  assert.match(map, /aria-current="page"/);
  assert.match(map, /--page: #f3f3eb/);
  assert.match(map, /--ink: #202720/);
  const embedded = /<script id="capability-data" type="application\/json">([\s\S]*?)<\/script>/.exec(map);
  assert.deepEqual(JSON.parse(embedded[1]), data);
  assert.doesNotMatch(map, /TADX_ENABLE_MUTATIONS|disabled-policy snapshot baseline|Execution enabled now/);
  assert.match(map, /machine authority not evaluated/);
  assert.match(map, /canonical server and exact site/);
  assert.match(map, /managed policy permission/);
  assert.match(map, /<dt>Administrative<\/dt>/);
  assert.match(map, /value="administrative"/);
  for (const capability of data) {
    assert.ok((capability.command_path || []).length <= 3, `Too many command levels: ${capability.surface}`);
  }
  for (const route of ['tadx catalog label', 'tadx admin label-value', 'tadx admin label-category']) {
    assert.ok(map.includes(`<code>${route}</code>`), `Missing current route: ${route}`);
  }
  assert.ok(!map.includes('<code>tadx content label</code>'));
  for (const match of map.matchAll(/href="([^"]+)"/g)) {
    assert.ok(match[1].startsWith('#') || match[1].startsWith('https://') || match[1] === 'capabilities.json', `Unpublished map link: ${match[1]}`);
  }
});

test('homepage has both one-line installers, current setup, and the supplied demos', async () => {
  const html = await readFile(join(repo, 'site/index.html'), 'utf8');
  const targetSource = await readFile(join(repo, 'internal/agenttarget/targets.go'), 'utf8');
  const labels = {claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode', cursor: 'Cursor', pi: 'Pi', hermes: 'Hermes', copilot: 'GitHub Copilot', gemini: 'Gemini CLI', cline: 'Cline'};
  const advertised = [...html.matchAll(/class="agent-name">([^<]+)<\/span>/g)].map(match => match[1]).sort();
  const supported = [...targetSource.matchAll(/name: "([^"]+)"/g)].map(match => match[1]).filter(name => name !== 'generic');
  for (const name of supported) assert.ok(labels[name], `Add homepage label for supported harness: ${name}`);
  assert.deepEqual(advertised, supported.map(name => labels[name]).sort());
  for (const text of ['Tableau.', 'At your command.', 'irm https://tadx.net/install.ps1 | iex', 'curl -fsSL https://tadx.net/install.sh | sh', 'tadx update', 'tab-find', 'tab-metric', 'tab-publish', 'Agent Guidance']) {
    assert.ok(html.includes(text), `Missing ${text}`);
  }
  assert.doesNotMatch(html, /https?:\/\/[^"'<>\s]+\.(?:js|css)(?:["'<>\s]|$)/i, 'Keep the homepage self-contained');
  assert.doesNotMatch(html, /C:[/\\]Users|releases\/latest\/download\/install|a62e3d7/);
  assert.doesNotMatch(html, /#install-tadx["']/);
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
