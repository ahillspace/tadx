import assert from 'node:assert/strict';
import test from 'node:test';
import { escapeHtml, panelHtml, validatePanelRequest, type PanelArtifact, type PanelSnapshot } from '../src/panels';

function row(key: string, overrides: Partial<PanelArtifact> = {}): PanelArtifact {
  return { key, name: key, kind: 'workbook', sourceKind: 'sites', sourceId: 'production', location: 'Projects/Finance', actions: ['download', 'openRemote'], ...overrides };
}

function snapshot(artifacts: PanelArtifact[] = [row('remote-1')]): PanelSnapshot {
  return { sources: { sites: [{ id: 'production', label: 'Production' }, { id: 'sandbox', label: 'Sandbox' }], workspaces: [{ id: 'work', label: 'Work' }] }, artifacts };
}

test('the host accepts only known artifact actions and removes extra arguments', () => {
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'download', keys: ['remote-1'], argv: ['--force'], path: '../../other' }, snapshot()), { action: 'download', keys: ['remote-1'] });
  assert.equal(validatePanelRequest({ type: 'action', action: 'shell', key: 'remote-1', command: 'anything' }, snapshot()), undefined);
  assert.equal(validatePanelRequest({ type: 'action', action: 'delete', key: 'remote-1' }, snapshot()), undefined);
  assert.equal(validatePanelRequest({ type: 'action', action: 'download', keys: ['removed-row'] }, snapshot()), undefined);
  assert.equal(validatePanelRequest({ type: 'action', action: 'download', keys: [null] }, snapshot()), undefined);
});

test('batches accept mixed transferable types and reject duplicates, source changes, ineligible rows, and limits', () => {
  const data = snapshot([row('a'), row('b', { kind: 'flow' }), row('c', { sourceId: 'sandbox' }), row('d', { kind: 'datasource' }), row('e', { kind: 'pulse-definition' }), row('f', { actions: ['openRemote'] })]);
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'download', keys: ['d', 'a', 'b'] }, data), { action: 'download', keys: ['d', 'a', 'b'] });
  for (const keys of [[], ['a', 'a'], ['a', 'c'], ['a', 'e'], ['e'], ['a', 'f']]) {
    assert.equal(validatePanelRequest({ type: 'action', action: 'download', keys }, data), undefined);
  }
  const rows = Array.from({ length: 101 }, (_, index) => row(`row-${index}`, { kind: index % 2 ? 'flow' : 'workbook' }));
  assert.equal(validatePanelRequest({ type: 'action', action: 'download', keys: rows.map(item => item.key) }, snapshot(rows)), undefined);
  const keys = rows.slice(0, 100).map(item => item.key).reverse();
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'download', keys }, snapshot(rows)), { action: 'download', keys });
});

test('upload selects an exact eligible local copy and never a cached remote row', () => {
  const data = snapshot([row('remote-1'), row('copy-1', { sourceKind: 'workspaces', sourceId: 'work', path: 'artifacts/workbook/one/', actions: ['upload', 'openPayload'] }), row('copy-2', { kind: 'datasource', sourceKind: 'workspaces', sourceId: 'work', path: 'artifacts/datasource/two/', actions: ['upload'] }), row('copy-3', { kind: 'flow', sourceKind: 'workspaces', sourceId: 'work', path: 'artifacts/flow/three/', actions: ['upload'] })]);
  assert.equal(validatePanelRequest({ type: 'action', action: 'upload', keys: ['remote-1'] }, data), undefined);
  assert.equal(validatePanelRequest({ type: 'action', action: 'download', keys: ['copy-1'] }, data), undefined);
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'upload', keys: ['copy-1'] }, data), { action: 'upload', keys: ['copy-1'] });
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'upload', keys: ['copy-1', 'copy-2', 'copy-3'] }, data), { action: 'upload', keys: ['copy-1', 'copy-2', 'copy-3'] });
  assert.equal(validatePanelRequest({ type: 'action', action: 'upload', keys: ['copy-1', 'remote-1'] }, data), undefined);
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'openPayload', key: 'copy-1' }, data), { action: 'openPayload', key: 'copy-1' });
});

test('busy state blocks artifact changes while keeping inspection available', () => {
  const data = { ...snapshot([row('a', { actions: ['download', 'move', 'delete', 'openMetadata'] })]), busy: true };
  for (const action of ['move', 'delete']) assert.equal(validatePanelRequest({ type: 'action', action, key: 'a' }, data), undefined);
  assert.equal(validatePanelRequest({ type: 'action', action: 'download', keys: ['a'] }, data), undefined);
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'openMetadata', key: 'a' }, data), { action: 'openMetadata', key: 'a' });
});

test('source selection accepts only host-issued source identities', () => {
  assert.deepEqual(validatePanelRequest({ type: 'action', action: 'selectSource', sourceKind: 'sites', sourceId: 'sandbox' }, snapshot()), { action: 'selectSource', sourceKind: 'sites', sourceId: 'sandbox' });
  for (const sourceKind of ['sites', '__proto__', 'workspaces']) {
    assert.equal(validatePanelRequest({ type: 'action', action: 'refresh', sourceKind, sourceId: 'unknown' }, snapshot()), undefined);
  }
});

test('webview shell has a restrictive CSP, external assets, and escaped attribute values', () => {
  const html = panelHtml({ cspSource: 'vscode-webview://test', scriptUri: 'vscode-resource:/panels.js?x=" onerror="bad', styleUri: 'vscode-resource:/panels.css', nonce: 'test-nonce', view: 'artifact' });
  assert.match(html, /default-src 'none'/);
  assert.match(html, /script-src 'nonce-test-nonce'/);
  assert.match(html, /base-uri 'none'; form-action 'none'/);
  assert.match(html, /<script nonce="test-nonce" src="[^"]*"><\/script>/);
  assert.match(html, /&quot; onerror=&quot;bad/);
  assert.doesNotMatch(html, /unsafe-inline|unsafe-eval|<script>[^<]/);
  assert.equal(escapeHtml('<img src=x onerror="bad"> & \'x\''), '&lt;img src=x onerror=&quot;bad&quot;&gt; &amp; &#39;x&#39;');
  assert.match(panelHtml({ cspSource: 'test', scriptUri: 'test.js', styleUri: 'test.css', nonce: 'test', view: 'inventory' }), /<title>TADX Bulk Actions<\/title>/);
});
