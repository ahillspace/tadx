import assert from 'node:assert/strict';
import test from 'node:test';
import type * as VSCode from 'vscode';
import { PanelManager, type PanelArtifact, type PanelSnapshot } from '../src/panels.js';

function panelHarness() {
  const messages = new Map<string, Record<string, unknown>[]>();
  const receivers = new Map<string, (message: unknown) => Promise<void>>();
  const uri = { toString: () => 'fixture:/media' };
  const api = {
    Uri: { joinPath: () => uri }, ViewColumn: { Active: 1 },
    window: { createWebviewPanel: (kind: string) => {
      const sent: Record<string, unknown>[] = [];
      messages.set(kind, sent);
      return { title: '', reveal() {}, dispose() {}, onDidDispose() {}, webview: {
        cspSource: 'fixture:', html: '', asWebviewUri: () => uri,
        postMessage: (message: Record<string, unknown>) => { sent.push(message); return Promise.resolve(true); },
        onDidReceiveMessage: (listener: (message: unknown) => Promise<void>) => {
          receivers.set(kind, listener); return { dispose() {} };
        },
      } };
    } },
  } as unknown as typeof VSCode;
  const requests: unknown[] = [];
  const manager = new PanelManager(api, uri as VSCode.Uri, request => { requests.push(request); });
  return { manager, messages, receivers, requests };
}

function largeSnapshot(): PanelSnapshot {
  const artifacts: PanelArtifact[] = Array.from({ length: 10_000 }, (_, index) => ({
    key: `row-${index}`, name: `Workbook ${index}`, kind: 'workbook', sourceKind: 'sites',
    sourceId: index % 2 ? 'other' : 'selected', location: 'Department/Operations',
    actions: ['download', 'openRemote'], metadata: [{ label: 'Observation', value: 'Synthetic cached inventory' }],
  }));
  artifacts[0]!.relatedKeys = ['row-2'];
  return { sources: { sites: [{ id: 'selected', label: 'Selected' }, { id: 'other', label: 'Other' }], workspaces: [] }, artifacts };
}

test('panels receive only their selected inventory projection and busy updates carry no rows', () => {
  const { manager, messages } = panelHarness();
  const snapshot = largeSnapshot();
  manager.update(snapshot);
  manager.showArtifact('row-0');
  manager.showInventory('sites', 'selected');
  manager.showOperation({ title: 'Download', state: 'pending', summary: 'Accepted operation', details: 'Retained result '.repeat(10_000) });
  manager.showArtifact('row-0');
  manager.showInventory('sites', 'selected');
  const latest = (view: string): Record<string, unknown> => messages.get(`tadx.${view}`)!.at(-1)!;
  const rows = (view: string): PanelArtifact[] => (latest(view).snapshot as PanelSnapshot).artifacts;
  assert.equal(rows('artifact').length, 2);
  assert.deepEqual(rows('artifact').map(row => row.key), ['row-0', 'row-2']);
  assert.equal(rows('inventory').length, 5_000);
  assert.equal(rows('inventory').every(row => row.sourceId === 'selected'), true);
  assert.equal(rows('operation').length, 0);
  assert.equal('operation' in latest('artifact'), false);
  assert.equal('operation' in latest('inventory'), false);
  assert.equal('artifactKey' in latest('operation'), false);
  assert.equal('inventorySource' in latest('operation'), false);

  manager.update({ ...snapshot, busy: true });
  const status = ['artifact', 'inventory', 'operation'].map(latest);
  const bytes = status.reduce((sum, value) => sum + Buffer.byteLength(JSON.stringify(value)), 0);
  console.log(`Busy-only update across three panels: ${bytes} bytes`);
  assert.equal(status.every(value => value.type === 'status' && value.busy === true && !('snapshot' in value)), true);
  assert.ok(bytes < 1_000, `busy update transferred ${bytes} bytes`);
  manager.dispose();
});

test('projected views still validate actions against the full current host inventory', async () => {
  const { manager, receivers, messages, requests } = panelHarness();
  const snapshot = largeSnapshot();
  manager.update(snapshot);
  manager.showInventory('sites', 'selected');
  const receive = receivers.get('tadx.inventory')!;
  await receive({ type: 'action', action: 'selectSource', sourceKind: 'sites', sourceId: 'other' });
  const message = messages.get('tadx.inventory')!.at(-1)!;
  assert.equal((message.snapshot as PanelSnapshot).artifacts.every(row => row.sourceId === 'other'), true);
  assert.deepEqual(requests, [{ action: 'selectSource', sourceKind: 'sites', sourceId: 'other' }]);
  manager.update({ ...snapshot, artifacts: snapshot.artifacts.filter(row => row.key !== 'row-1') });
  await receive({ type: 'action', action: 'download', keys: ['row-1'] });
  assert.equal(requests.length, 1);
  assert.equal(messages.get('tadx.inventory')!.at(-1)!.type, 'error');
  manager.dispose();
});
