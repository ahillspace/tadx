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
    assert.deepEqual((await readdir(destination)).sort(), ['.nojekyll', 'capabilities.html', 'capabilities.json', 'index.html', 'install.ps1', 'install.sh', 'security-controls.svg', 'security.html']);
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
  assert.doesNotMatch(html, /#install-tadx|One-line installation is coming soon/);
  assert.match(html, /README\.md#install["']/);
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(ids.length, new Set(ids).size, 'Duplicate DOM identifiers');
  for (const match of html.matchAll(/aria-(?:controls|labelledby)="([^"]+)"/g)) {
    for (const id of match[1].split(' ')) assert.ok(ids.includes(id), `Missing accessible target ${id}`);
  }
  const script = /<script>([\s\S]*?)<\/script>/.exec(html)?.[1];
  assert.ok(script);
  new Function(script);
});

test('security page is linked, self-contained, and usable without application JavaScript', async () => {
  const html = await readFile(join(repo, 'site/security.html'), 'utf8');
  const home = await readFile(join(repo, 'site/index.html'), 'utf8');
  assert.equal([...home.matchAll(/href="security\.html"/g)].length, 2);
  assert.match(html, /aria-current="page">Security/);
  assert.match(html, /--paper: #f3f3eb/);
  assert.match(html, /--ink: #202720/);
  assert.doesNotMatch(html, /<script\b|<iframe\b|<form\b|https?:\/\/[^"'<> ]+\.(?:js|css)\b/i);
  assert.doesNotMatch(html, /C:[/\\]Users|TADX_ENABLE_MUTATIONS|impossib|tamper-proof/i);
  assert.ok([...html.matchAll(/<details>/g)].length >= 10, 'Missing progressive disclosure');
  assert.doesNotMatch(html, /<details\b[^>]*\bopen\b/, 'Details should start collapsed');
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(ids.length, new Set(ids).size, 'Duplicate security page identifiers');
  for (const id of ['controls', 'templates', 'credentials', 'policy-installation', 'local-data', 'boundaries']) {
    assert.ok(ids.includes(id), 'Missing security section ' + id);
    assert.ok(html.includes('href="#' + id + '"'), 'Missing section navigation ' + id);
  }
  for (const match of html.matchAll(/(?:aria-labelledby|aria-controls)="([^"]+)"/g)) {
    for (const id of match[1].split(' ')) assert.ok(ids.includes(id), 'Missing accessible target ' + id);
  }
  for (const match of html.matchAll(/href="#([^"]+)"/g)) assert.ok(ids.includes(match[1]), 'Broken anchor ' + match[1]);
  for (const match of html.matchAll(/href="https:\/\/github\.com\/ahillspace\/tadx\/blob\/main\/([^"#]+)(?:#[^"]*)?"/g)) {
    await readFile(join(repo, match[1]));
  }
  for (const name of ['read-only', 'read-write-no-admin', 'superuser']) assert.ok(html.includes('<h3>' + name + '</h3>'));
  for (const text of ['tadx policy install --template read-only', '--output', 'current development source', 'not a sandbox', 'checksum', 'Secret Service']) {
    assert.ok(html.includes(text), 'Missing security fact ' + text);
  }
  const controls = /<section id="controls"[\s\S]*?<div class="detail-group">/.exec(html)?.[0];
  assert.ok(controls, 'Missing visible controls introduction');
  assert.match(controls, /tadx mutation status/);
  assert.match(controls, /tadx policy status/);
  assert.match(controls, /no enforced approval prompt/);
  assert.match(controls, /src="security-controls\.svg"[^>]+alt="[^"]+"/);
  const diagram = await readFile(join(repo, 'site/security-controls.svg'), 'utf8');
  assert.ok(/<svg\b/.test(diagram), 'Missing rendered diagram');
  assert.ok(/<desc[^>]*>[^<]*Managed policy[^<]*site's write switch/.test(diagram), 'Missing accessible explanation of both gates');
  assert.ok(!/<script\b|<foreignObject\b|(?:href|src)="https?:/i.test(diagram), 'Diagram must be self-contained SVG');
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
