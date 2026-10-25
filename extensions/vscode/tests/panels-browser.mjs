/* global window, document, getComputedStyle */
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdir, readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { build } from 'esbuild';
import { chromium } from 'playwright';

const root = resolve(import.meta.dirname, '..');
const output = resolve(root, 'dist', 'ui');
await mkdir(output, { recursive: true });
const bundled = await build({ entryPoints: [resolve(root, 'src', 'panels.ts')], bundle: true, platform: 'node', format: 'esm', write: false });
const { panelHtml } = await import(`data:text/javascript;base64,${Buffer.from(bundled.outputFiles[0].text).toString('base64')}`);
const media = new Map(await Promise.all(['panels.css', 'panels.js'].map(async name => [name, await readFile(resolve(root, 'media', name), 'utf8')])));

// The VS Code host normally supplies these tokens and the acquireVsCodeApi bridge.
function themeCss(light) {
  const colors = light ? {
    background: '#ffffff', foreground: '#333333', muted: '#616161', surface: '#f3f3f3', border: '#d4d4d4', link: '#006ab1',
    input: '#ffffff', button: '#007acc', secondary: '#e5e5e5', selected: '#e4e6f1', hover: '#f0f0f0', quote: '#f2f2f2', focus: '#0090f1',
  } : {
    background: '#1f1f1f', foreground: '#cccccc', muted: '#9d9d9d', surface: '#252526', border: '#3c3c3c', link: '#4daafc',
    input: '#313131', button: '#0078d4', secondary: '#313131', selected: '#37373d', hover: '#2a2d2e', quote: '#282828', focus: '#007fd4',
  };
  return `:root {
    --vscode-font-family: "Segoe UI", sans-serif; --vscode-font-size: 13px; --vscode-editor-font-family: Consolas, monospace;
    --vscode-editor-background: ${colors.background}; --vscode-editor-foreground: ${colors.foreground}; --vscode-descriptionForeground: ${colors.muted};
    --vscode-panel-border: ${colors.border}; --vscode-button-border: transparent; --vscode-editorWidget-background: ${colors.surface};
    --vscode-button-background: ${colors.button}; --vscode-button-foreground: #ffffff; --vscode-button-hoverBackground: #1682ce;
    --vscode-button-secondaryBackground: ${colors.secondary}; --vscode-button-secondaryForeground: ${colors.foreground}; --vscode-button-secondaryHoverBackground: ${colors.hover};
    --vscode-input-background: ${colors.input}; --vscode-input-foreground: ${colors.foreground}; --vscode-input-border: ${colors.border}; --vscode-input-placeholderForeground: ${colors.muted};
    --vscode-dropdown-background: ${colors.input}; --vscode-dropdown-foreground: ${colors.foreground}; --vscode-dropdown-border: ${colors.border};
    --vscode-textLink-foreground: ${colors.link}; --vscode-textLink-activeForeground: ${colors.link}; --vscode-focusBorder: ${colors.focus};
    --vscode-list-inactiveSelectionBackground: ${colors.selected}; --vscode-list-inactiveSelectionForeground: ${colors.foreground}; --vscode-list-hoverBackground: ${colors.hover};
    --vscode-textBlockQuote-background: ${colors.quote}; --vscode-textCodeBlock-background: ${colors.surface};
    --vscode-editorInfo-foreground: ${colors.link}; --vscode-editorWarning-foreground: #bf8803; --vscode-errorForeground: #d13438;
  }`;
}

const server = createServer((request, response) => {
  const url = new URL(request.url || '/', 'http://localhost');
  if (url.pathname === '/theme.css') {
    response.writeHead(200, { 'Content-Type': 'text/css; charset=utf-8' }).end(themeCss(url.searchParams.get('theme') === 'light'));
    return;
  }
  const file = url.pathname.replace('/media/', '');
  if (media.has(file)) {
    response.writeHead(200, { 'Content-Type': file.endsWith('.css') ? 'text/css; charset=utf-8' : 'text/javascript; charset=utf-8' }).end(media.get(file));
    return;
  }
  if (['/artifact', '/inventory', '/operation'].includes(url.pathname)) {
    const theme = url.searchParams.get('theme') === 'light' ? 'light' : 'dark';
    const html = panelHtml({ cspSource: "'self'", scriptUri: '/media/panels.js', styleUri: '/media/panels.css', nonce: 'browser-test-nonce', view: url.pathname.slice(1) })
      .replace('</head>', `<link rel="stylesheet" href="/theme.css?theme=${theme}"></head>`)
      .replace('<body ', `<body class="vscode-${theme}" `);
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' }).end(html);
    return;
  }
  response.writeHead(404).end();
});
await new Promise(resolveListen => server.listen(0, '127.0.0.1', resolveListen));
const address = server.address();
const origin = `http://127.0.0.1:${address.port}`;
let browser;
try {
  browser = await chromium.launch({ headless: true });
} catch (error) {
  await new Promise(resolveClose => server.close(resolveClose));
  throw error;
}
const pageErrors = [];
const context = await browser.newContext({ viewport: { width: 1200, height: 900 }, reducedMotion: 'reduce' });
await context.addInitScript(() => {
  window.tadxMessages = [];
  window.tadxStateRevision = 0;
  let savedState;
  window.acquireVsCodeApi = () => ({
    postMessage: message => window.tadxMessages.push(message),
    getState: () => savedState,
    setState: value => { savedState = value; window.tadxStateRevision++; },
  });
});

const hostileName = '<img src=x onerror="window.tadxInjected=true"> & Revenue';
const remote = Array.from({ length: 105 }, (_, index) => ({
  key: `remote-${index}`, name: index === 0 ? 'Revenue overview' : `Workbook ${String(index).padStart(3, '0')}`,
  kind: 'workbook', sourceKind: 'sites', sourceId: 'production', location: 'Finance/Reports', state: 'Cached',
  actions: ['download', 'openRemote'], artifactId: `workbook-${index}`, updatedAt: '2026-10-22T16:00:00Z',
}));
remote[0].relatedKeys = ['local-1'];
const local = {
  key: 'local-1', name: 'Revenue overview', kind: 'workbook', sourceKind: 'workspaces', sourceId: 'work',
  location: 'artifacts/workbook/revenue/', path: 'artifacts/workbook/revenue/', state: 'Changed locally',
  actions: ['upload', 'openPayload', 'openMetadata', 'reveal', 'move', 'delete'], relatedKeys: ['remote-0'],
};
const snapshot = {
  sources: {
    sites: [
      { id: 'production', label: 'Production', status: 'Stale', complete: false, description: 'Cached 2 days ago', warnings: ['Flow coverage is incomplete. Refresh the cache to collect current inventory.'] },
      { id: 'sandbox', label: 'Sandbox', status: 'Not loaded', complete: false },
    ],
    workspaces: [{ id: 'work', label: 'Analytics workspace', status: 'Scanned', complete: true, description: 'Local files scanned just now' }],
  },
  artifacts: [...remote,
    { key: 'flow-1', name: 'Orders preparation', kind: 'flow', sourceKind: 'sites', sourceId: 'production', location: 'Finance/Preparation', state: 'Cached', actions: ['download'] },
    { key: 'hostile', name: hostileName, kind: 'datasource', sourceKind: 'sites', sourceId: 'production', location: '<script>window.tadxInjected=true</script>', state: 'Cached', actions: ['download'] },
    local,
    { ...local, key: 'local-flow', name: 'Local preparation', kind: 'flow', path: 'artifacts/flow/preparation/', actions: ['upload', 'openMetadata'] },
    { ...local, key: 'local-datasource', name: 'Local revenue source', kind: 'datasource', path: 'artifacts/datasource/revenue/', actions: ['upload', 'openMetadata'] },
    { ...local, key: 'local-pulse', name: 'Revenue Pulse definition', kind: 'pulse-definition', path: 'artifacts/pulse-definition/revenue/', actions: ['openMetadata'] },
  ],
};

async function open(view, theme = 'dark') {
  const page = await context.newPage();
  page.on('pageerror', error => pageErrors.push(error));
  await page.goto(`${origin}/${view}?theme=${theme}`);
  await page.waitForFunction(() => window.tadxMessages.some(message => message.type === 'ready'));
  return page;
}

async function sendState(page, values = {}) {
  const revision = await page.evaluate(() => window.tadxStateRevision);
  await page.evaluate(message => window.postMessage(message, '*'), { type: 'state', snapshot, ...values });
  await page.waitForFunction(previousRevision => window.tadxStateRevision > previousRevision, revision);
  await page.locator('.page-header').waitFor();
}

async function messages(page, action) {
  return page.evaluate(selectedAction => window.tadxMessages.filter(message => message.action === selectedAction), action);
}

async function check(label, run) {
  await run();
  console.log(`PASS ${label}`);
}

try {
  const artifact = await open('artifact');
  await check('single-artifact actions use host keys and exact local copies', async () => {
    await sendState(artifact, { artifactKey: 'remote-0' });
    assert.equal(await artifact.getByRole('heading', { name: 'Revenue overview', exact: true }).count(), 1);
    assert.equal(await artifact.getByRole('button', { name: 'Upload…', exact: true }).isDisabled(), true);
    await artifact.getByRole('button', { name: 'Download…', exact: true }).click();
    assert.deepEqual((await messages(artifact, 'download')).at(-1), { type: 'action', action: 'download', keys: ['remote-0'] });
    await artifact.getByRole('button', { name: 'Analytics workspace · Revenue overview', exact: true }).click();
    assert.deepEqual((await messages(artifact, 'showArtifact')).at(-1), { type: 'action', action: 'showArtifact', key: 'local-1' });
    await sendState(artifact, { artifactKey: 'local-1' });
    assert.equal(await artifact.getByText('artifacts/workbook/revenue/', { exact: true }).count(), 1);
    await artifact.getByRole('button', { name: 'Upload…', exact: true }).click();
    assert.deepEqual((await messages(artifact, 'upload')).at(-1), { type: 'action', action: 'upload', keys: ['local-1'] });
    await artifact.getByText('Local file actions', { exact: true }).click();
    await artifact.getByRole('button', { name: 'Open metadata', exact: true }).click();
    assert.deepEqual((await messages(artifact, 'openMetadata')).at(-1), { type: 'action', action: 'openMetadata', key: 'local-1' });
    await artifact.screenshot({ path: resolve(output, 'artifact-dark.png'), fullPage: true });
  });

  const inventory = await open('inventory');
  await check('mixed bulk actions, keyboard selection, filters, and pagination preserve intended keys', async () => {
    await sendState(inventory);
    assert.equal(await inventory.title(), 'TADX Bulk Actions');
    assert.equal(await inventory.getByRole('heading', { name: 'Bulk Actions', exact: true }).count(), 1);
    assert.equal(await inventory.getByRole('button', { name: 'Upload from workspace', exact: true }).isEnabled(), true);
    assert.equal(await inventory.evaluate(() => getComputedStyle(document.body).colorScheme), 'dark');
    assert.equal(await inventory.getByText('Coverage is incomplete. These rows do not represent the entire source.', { exact: true }).count(), 1);
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select visible eligible artifacts, up to 100', exact: true }).isDisabled(), false);
    await inventory.getByRole('checkbox', { name: 'Select Revenue overview', exact: true }).check();
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select Orders preparation', exact: true }).isEnabled(), true);
    await inventory.getByRole('checkbox', { name: 'Select Orders preparation', exact: true }).check();
    await inventory.getByLabel('Type', { exact: true }).selectOption('datasource');
    await inventory.getByRole('checkbox', { name: `Select ${hostileName}`, exact: true }).check();
    await inventory.getByRole('button', { name: 'Download selected…', exact: true }).click();
    assert.deepEqual((await messages(inventory, 'download')).at(-1).keys, ['remote-0', 'flow-1', 'hostile']);
    await sendState(inventory);
    await inventory.getByRole('button', { name: 'Download from site', exact: true }).click();
    assert.equal(await inventory.getByText('1 matching · 3 selected (2 outside this page)', { exact: true }).count(), 1);
    await inventory.getByLabel('Type', { exact: true }).selectOption('workbook');
    const first = inventory.getByRole('checkbox', { name: 'Select Revenue overview', exact: true });
    assert.equal(await first.isChecked(), true);
    await first.focus();
    await inventory.keyboard.press('Space');
    assert.equal(await first.isChecked(), false);
    await inventory.keyboard.press('Space');
    assert.equal(await first.isChecked(), true);
    assert.equal(await first.evaluate(node => node === document.activeElement), true);
    await inventory.getByLabel('Filter inventory', { exact: true }).fill('Workbook 001');
    assert.equal(await inventory.getByText('1 matching · 3 selected (3 outside this page)', { exact: true }).count(), 1);
    await inventory.getByRole('button', { name: 'Download selected…', exact: true }).click();
    assert.deepEqual((await messages(inventory, 'download')).at(-1).keys, ['remote-0', 'flow-1', 'hostile']);
    await inventory.getByLabel('Filter inventory', { exact: true }).fill('');
    await inventory.getByRole('button', { name: 'Next', exact: true }).click();
    assert.equal(await inventory.locator('tbody tr').count(), 5);
    assert.equal(await inventory.getByRole('button', { name: 'Previous', exact: true }).evaluate(node => node === document.activeElement), true);
    await inventory.getByRole('checkbox', { name: 'Select Workbook 104', exact: true }).check();
    await inventory.getByRole('button', { name: 'Download selected…', exact: true }).click();
    assert.deepEqual((await messages(inventory, 'download')).at(-1).keys, ['remote-0', 'remote-104', 'flow-1', 'hostile']);
    await inventory.getByRole('button', { name: 'Clear selection', exact: true }).click();
    await inventory.getByRole('button', { name: 'Previous', exact: true }).click();
    await inventory.getByRole('checkbox', { name: 'Select visible eligible artifacts, up to 100', exact: true }).check();
    await inventory.getByRole('button', { name: 'Next', exact: true }).click();
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select Workbook 104', exact: true }).isDisabled(), true);
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select visible eligible artifacts, up to 100', exact: true }).isDisabled(), true);
    await inventory.getByRole('button', { name: 'Download selected…', exact: true }).click();
    assert.equal((await messages(inventory, 'download')).at(-1).keys.length, 100);
    await inventory.getByLabel('Type', { exact: true }).selectOption('flow');
    assert.equal(await inventory.getByRole('button', { name: 'Download selected…', exact: true }).isEnabled(), true);
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select Orders preparation', exact: true }).isDisabled(), true);
    assert.equal(await inventory.getByText('1 matching · 100 selected (100 outside this page)', { exact: true }).count(), 1);
    await inventory.getByRole('button', { name: 'Clear selection', exact: true }).click();
    await inventory.getByRole('checkbox', { name: 'Select Orders preparation', exact: true }).check();
    await inventory.getByRole('button', { name: 'Upload from workspace', exact: true }).click();
    assert.deepEqual((await messages(inventory, 'selectSource')).at(-1), { type: 'action', action: 'selectSource', sourceKind: 'workspaces', sourceId: 'work' });
    assert.equal(await inventory.getByRole('button', { name: 'Upload selected…', exact: true }).isDisabled(), true);
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select Revenue Pulse definition', exact: true }).isDisabled(), true);
    await inventory.getByRole('checkbox', { name: 'Select visible eligible artifacts, up to 100', exact: true }).check();
    await inventory.getByRole('button', { name: 'Upload selected…', exact: true }).click();
    assert.deepEqual((await messages(inventory, 'upload')).at(-1), { type: 'action', action: 'upload', keys: ['local-1', 'local-flow', 'local-datasource'] });
    await inventory.screenshot({ path: resolve(output, 'bulk-upload-dark.png'), fullPage: true });
  });

  await check('stale rows and busy state disable writes while preserving browsing', async () => {
    await sendState(inventory, { snapshot: { ...snapshot, busy: true } });
    assert.equal(await inventory.getByRole('button', { name: 'Upload selected…', exact: true }).isDisabled(), true);
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select visible eligible artifacts, up to 100', exact: true }).isDisabled(), true);
    assert.equal(await inventory.getByRole('checkbox', { name: 'Select Revenue overview', exact: true }).isDisabled(), true);
    assert.equal(await inventory.getByRole('button', { name: 'Revenue overview', exact: true }).isEnabled(), true);
    await sendState(inventory, { snapshot: { ...snapshot, artifacts: snapshot.artifacts.filter(row => !['local-flow', 'local-datasource'].includes(row.key)).map(row => row.key === 'local-1' ? { ...row, actions: ['openPayload'] } : row) } });
    assert.equal(await inventory.getByRole('button', { name: 'Upload selected…', exact: true }).isDisabled(), true);
    await sendState(artifact, { artifactKey: 'local-1', snapshot: { ...snapshot, busy: true } });
    assert.equal(await artifact.getByRole('button', { name: 'Upload…', exact: true }).isDisabled(), true);
    assert.equal(await artifact.getByRole('button', { name: 'Open payload', exact: true }).isEnabled(), true);
  });

  await check('hostile inventory text stays text and the script CSP remains enforced', async () => {
    await sendState(artifact, { artifactKey: 'hostile' });
    assert.equal(await artifact.getByRole('heading', { name: hostileName, exact: true }).count(), 1);
    assert.equal(await artifact.locator('img').count(), 0);
    assert.equal(await artifact.evaluate(() => window.tadxInjected), undefined);
    const csp = await artifact.locator('meta[http-equiv="Content-Security-Policy"]').getAttribute('content');
    assert.match(csp, /default-src 'none'/);
    assert.match(csp, /script-src 'nonce-browser-test-nonce'/);
    await artifact.evaluate(() => {
      const script = document.createElement('script');
      script.textContent = 'window.tadxInjected = true;';
      document.body.appendChild(script);
    });
    assert.equal(await artifact.evaluate(() => window.tadxInjected), undefined);
  });

  await check('source switching and unavailable inventory remain explicit', async () => {
    await sendState(inventory);
    await inventory.getByRole('button', { name: 'Download from site', exact: true }).click();
    await inventory.getByLabel('Site', { exact: true }).selectOption('sandbox');
    assert.deepEqual((await messages(inventory, 'selectSource')).at(-1), { type: 'action', action: 'selectSource', sourceKind: 'sites', sourceId: 'sandbox' });
    assert.equal(await inventory.getByRole('heading', { name: 'No inventory rows', exact: true }).count(), 1);
    await inventory.getByRole('button', { name: 'Refresh site cache', exact: true }).click();
    assert.deepEqual((await messages(inventory, 'refresh')).at(-1), { type: 'action', action: 'refresh', sourceKind: 'sites', sourceId: 'sandbox' });
    await inventory.getByLabel('Site', { exact: true }).selectOption('production');
    await inventory.getByLabel('Type', { exact: true }).selectOption('workbook');
    await inventory.screenshot({ path: resolve(output, 'inventory-dark.png'), fullPage: false });
  });

  await check('light theme and narrow layouts keep controls reachable without page overflow', async () => {
    const light = await open('inventory', 'light');
    await sendState(light);
    assert.equal(await light.evaluate(() => getComputedStyle(document.body).colorScheme), 'light');
    await light.getByLabel('Type', { exact: true }).selectOption('flow');
    await light.screenshot({ path: resolve(output, 'inventory-light.png'), fullPage: true });
    await light.setViewportSize({ width: 360, height: 800 });
    assert.equal(await light.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    const scroll = light.getByRole('region', { name: 'Scrollable artifact inventory' });
    await scroll.focus();
    assert.equal(await scroll.evaluate(node => node === document.activeElement), true);
    await light.screenshot({ path: resolve(output, 'inventory-narrow.png'), fullPage: true });
    await light.emulateMedia({ forcedColors: 'active' });
    assert.equal(await light.evaluate(() => window.matchMedia('(forced-colors: active)').matches), true);
    await light.screenshot({ path: resolve(output, 'inventory-high-contrast.png'), fullPage: true });
    await artifact.setViewportSize({ width: 360, height: 800 });
    await sendState(artifact, { artifactKey: 'local-1' });
    assert.equal(await artifact.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    await artifact.screenshot({ path: resolve(output, 'artifact-narrow.png'), fullPage: true });
    await light.close();
  });

  await check('operation results expose ordered partial outcomes without executable controls', async () => {
    const operation = await open('operation');
    await sendState(operation, { operation: { title: 'Upload result', state: 'partial', summary: 'One artifact uploaded. One artifact needs attention.', items: [{ name: 'Revenue overview', state: 'succeeded' }, { name: 'Expenses', state: 'unknown', message: 'Operation ID: operation-123' }], details: '{"operation_id":"operation-123","verification":"unknown"}' } });
    assert.deepEqual(await operation.locator('.operation-items strong').allTextContents(), ['Revenue overview', 'Expenses']);
    assert.match(await operation.locator('pre').textContent(), /operation-123/);
    assert.equal(await operation.getByRole('button').count(), 0);
    await operation.screenshot({ path: resolve(output, 'operation-dark.png'), fullPage: true });
    await operation.close();
  });
  assert.deepEqual(pageErrors, [], 'No browser script errors');
  console.log(`Screenshots: ${output}`);
} finally {
  await context.close();
  await browser.close();
  await new Promise(resolveClose => server.close(resolveClose));
}
