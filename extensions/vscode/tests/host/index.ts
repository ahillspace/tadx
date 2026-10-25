import assert from 'node:assert/strict';
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import * as vscode from 'vscode';
import type { PanelSnapshot } from '../../src/panels.js';

interface ExtensionAPI { ready: Promise<void>; getSnapshot(): PanelSnapshot }

function deadline<T>(promise: PromiseLike<T>, label: string, milliseconds = 15_000): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`Timed out: ${label}`)), milliseconds);
    Promise.resolve(promise).then(value => { clearTimeout(timer); resolve(value); }, error => { clearTimeout(timer); reject(error); });
  });
}

function waitForState(predicate: () => boolean, subscribe: (check: () => void) => vscode.Disposable, label: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { listener.dispose(); reject(new Error(`Timed out: ${label}`)); }, 10_000);
    const check = (): void => {
      if (!predicate()) return;
      clearTimeout(timer);
      listener.dispose();
      resolve();
    };
    const listener = subscribe(check);
    check();
  });
}

/** Runs inside the real VS Code extension host, without replacing any VS Code APIs. */
export async function run(): Promise<void> {
  const root = process.env.TADX_HOST_FIXTURE_ROOT;
  assert.ok(root, 'The isolated fixture root must be present.');
  const checks: string[] = [];
  const check = async (name: string, work: () => Promise<void>): Promise<void> => {
    await deadline(work(), name);
    checks.push(name);
    console.log(`PASS ${name}`);
  };
  const invoke = <T = unknown>(command: string, ...args: unknown[]): Promise<T> => deadline(vscode.commands.executeCommand<T>(command, ...args), command);
  const extension = vscode.extensions.getExtension<ExtensionAPI>('ahillspace.tadx');
  assert.ok(extension, 'The extension must be discovered from its real manifest.');
  const api = await deadline(extension.activate(), 'extension activation');
  await deadline(api.ready, 'initial inventory');
  const calls = async (): Promise<string[][]> => Promise.all((await readdir(join(root, 'calls'))).map(async name => {
    const value: unknown = JSON.parse(await readFile(join(root, 'calls', name), 'utf8'));
    assert.ok(Array.isArray(value) && value.every(arg => typeof arg === 'string'));
    return value as string[];
  }));
  const webviewTabs = (): vscode.Tab[] => vscode.window.tabGroups.all.flatMap(group => group.tabs)
    .filter(tab => tab.input instanceof vscode.TabInputWebview);

  try {
    await check('trusted activation and exact inventory', async () => {
      assert.equal(vscode.workspace.isTrusted, true);
      assert.equal(extension.isActive, true);
      assert.equal(vscode.workspace.workspaceFolders?.[0]?.uri.fsPath, vscode.Uri.file(join(root, 'workspace')).fsPath);
      const snapshot = api.getSnapshot();
      assert.deepEqual(snapshot.errors ?? [], []);
      assert.equal(snapshot.sources.sites.some(source => source.id === 'sandbox'), true);
      assert.equal(snapshot.sources.workspaces.some(source => source.id === 'work'), true);
      assert.equal(snapshot.artifacts.filter(row => row.sourceKind === 'sites').length, 3);
      assert.equal(snapshot.artifacts.filter(row => row.sourceKind === 'workspaces').length, 3);
      assert.ok(snapshot.artifacts.some(row => row.name === 'Finance local' && row.path === 'artifacts/workbook/Finance--fixture'));
      for (const kind of ['pulse-definition', 'lineage']) {
        const artifact = snapshot.artifacts.find(row => row.sourceKind === 'workspaces' && row.kind === kind);
        assert.ok(artifact, `Local ${kind} must remain in the inventory.`);
        assert.equal(artifact.actions.includes('upload'), false);
        assert.equal(artifact.actions.includes('download'), false);
        assert.ok(artifact.actions.includes('openMetadata'));
      }
      const initialCalls = await calls();
      assert.ok(initialCalls.length >= 8, 'Activation must obtain inventory through the configured executable.');
      assert.ok(initialCalls.some(args => args[0] === 'version'));
      assert.ok(initialCalls.some(args => args[0] === 'capability' && args[1] === 'list'));
      for (const args of initialCalls) {
        assert.ok(args.includes('--full') && args.includes('--json'));
        if (args[0] === 'content') assert.ok(args.includes('--cache') && args.includes('--all'));
        assert.equal(args.includes('publish') || args.includes('pull') || args.includes('refresh'), false);
      }
    });

    await check('three native views and contributed commands', async () => {
      const commands = await vscode.commands.getCommands(true);
      for (const command of ['tadx.refresh', 'tadx.chooseWorkspace', 'tadx.openInventory', 'tadx.openWorkspaceBulkActions', 'tadx.openArtifact', 'tadx.download', 'tadx.upload',
        'tadx.cliStatus', 'tadx.installCli', 'tadx.updateCli', 'tadx.chooseExecutable']) {
        assert.ok(commands.includes(command), `Missing command: ${command}`);
      }
      for (const id of ['tadx.sites', 'tadx.workspaces', 'tadx.activity']) {
        assert.ok(commands.includes(`${id}.focus`), `Missing native view: ${id}`);
        await invoke(`${id}.focus`);
      }
    });

    await check('Bulk Actions panel title and reuse, plus artifact editor webviews', async () => {
      await invoke('tadx.openInventory');
      await waitForState(() => webviewTabs().some(tab => tab.input instanceof vscode.TabInputWebview && tab.input.viewType.includes('tadx.inventory')),
        vscode.window.tabGroups.onDidChangeTabs, 'inventory editor tab');
      const bulkTabs = (): vscode.Tab[] => webviewTabs().filter(tab => tab.input instanceof vscode.TabInputWebview && tab.input.viewType.includes('tadx.inventory'));
      assert.equal(bulkTabs().length, 1);
      assert.equal(bulkTabs()[0]?.label, 'TADX Bulk Actions');
      await invoke('tadx.openWorkspaceBulkActions');
      await waitForState(() => bulkTabs()[0]?.isActive === true, vscode.window.tabGroups.onDidChangeTabs, 'workspace Bulk Actions panel');
      assert.equal(bulkTabs().length, 1, 'Workspace bulk actions must reuse the existing Bulk Actions panel.');
      assert.equal(bulkTabs()[0]?.label, 'TADX Bulk Actions');
      assert.equal(bulkTabs()[0]?.isActive, true);
      const remote = api.getSnapshot().artifacts.find(row => row.sourceKind === 'sites' && row.kind === 'workbook');
      assert.ok(remote);
      await invoke('tadx.openArtifact', remote.key);
      await waitForState(() => webviewTabs().some(tab => tab.label === remote.name && tab.input instanceof vscode.TabInputWebview),
        vscode.window.tabGroups.onDidChangeTabs, 'artifact editor tab');
      for (const kind of ['pulse-definition', 'lineage']) {
        const local = api.getSnapshot().artifacts.find(row => row.sourceKind === 'workspaces' && row.kind === kind);
        assert.ok(local);
        await invoke('tadx.openArtifact', local.key);
        await waitForState(() => webviewTabs().some(tab => tab.label === local.name && tab.input instanceof vscode.TabInputWebview),
          vscode.window.tabGroups.onDidChangeTabs, `${kind} artifact editor tab`);
      }
    });

    await check('local payload opens through the native editor', async () => {
      const local = api.getSnapshot().artifacts.find(row => row.sourceKind === 'workspaces' && row.kind === 'workbook');
      assert.ok(local);
      await invoke('tadx.openPayload', local.key);
      await waitForState(() => vscode.window.activeTextEditor?.document.uri.fsPath === vscode.Uri.file(join(root, 'workspace', 'artifacts', 'workbook', 'Finance--fixture', 'Finance.twb')).fsPath,
        vscode.window.onDidChangeActiveTextEditor, 'local payload editor');
      const document = vscode.window.activeTextEditor?.document;
      assert.equal(document?.uri.fsPath, vscode.Uri.file(join(root, 'workspace', 'artifacts', 'workbook', 'Finance--fixture', 'Finance.twb')).fsPath);
      assert.equal(document?.getText(), '<workbook/>\n');
    });

    await check('Bulk Actions loads its initial site when no default or previous source exists', async () => {
      await writeFile(join(root, 'scenario.json'), JSON.stringify({ noDefaultSites: true }));
      try {
        await invoke('tadx.refresh');
        assert.deepEqual(api.getSnapshot().sources.sites.map(source => source.id), ['alpha', 'beta']);
        assert.equal(api.getSnapshot().artifacts.filter(row => row.sourceKind === 'sites').length, 0);
        await invoke('tadx.openInventory');
        const remote = api.getSnapshot().artifacts.filter(row => row.sourceKind === 'sites');
        assert.equal(remote.length, 3, 'Opening Bulk Actions must load the site displayed by its initial selection.');
        assert.ok(remote.every(row => row.sourceId === 'alpha'));
      } finally {
        await writeFile(join(root, 'scenario.json'), '{}');
        await invoke('tadx.refresh');
      }
    });

    await check('native cache refresh site selection cancellation keeps inventory', async () => {
      const before = JSON.stringify(api.getSnapshot());
      const beforeCalls = (await calls()).length;
      const selection = invoke('tadx.refreshCache');
      // The public API does not expose another extension's QuickPick instance.
      // Close the native picker after the command has yielded to the workbench.
      await new Promise<void>(resolve => setTimeout(resolve, 300));
      await invoke('workbench.action.closeQuickOpen');
      await selection;
      assert.equal(JSON.stringify(api.getSnapshot()), before);
      assert.equal((await calls()).length, beforeCalls);
    });

    await check('cached inventory excludes unrelated permission warnings and retains inventory diagnostics', async () => {
      await writeFile(join(root, 'scenario.json'), JSON.stringify({ incompletePermissions: true, warningScope: 'workbook' }));
      await invoke('tadx.refresh');
      const snapshot = api.getSnapshot();
      const site = snapshot.sources.sites.find(source => source.id === 'sandbox');
      assert.ok(site);
      assert.deepEqual(snapshot.errors ?? [], []);
      assert.equal(snapshot.artifacts.filter(row => row.sourceKind === 'sites').length, 3);
      assert.equal(site.complete, true, 'Permission coverage must not mark artifact inventory incomplete.');
      assert.deepEqual(site.warnings, ['Synthetic workbook inventory diagnostic.']);
      assert.doesNotMatch(JSON.stringify(snapshot), /permission reads were denied|HTTP 403/);
      await writeFile(join(root, 'scenario.json'), '{}');
      await invoke('tadx.refresh');
      assert.doesNotMatch(JSON.stringify(api.getSnapshot()), /Synthetic workbook inventory diagnostic/);
    });

    await check('partial cached scope remains visible and recovers', async () => {
      await writeFile(join(root, 'scenario.json'), JSON.stringify({ failScope: 'flow' }));
      await invoke('tadx.refresh');
      const partial = api.getSnapshot();
      assert.equal(partial.artifacts.filter(row => row.sourceKind === 'sites').length, 2);
      assert.match(JSON.stringify(partial), /Synthetic flow scope unavailable/);
      assert.ok(partial.artifacts.some(row => row.sourceKind === 'workspaces'));
      await writeFile(join(root, 'scenario.json'), '{}');
      await invoke('tadx.refresh');
      assert.equal(api.getSnapshot().artifacts.filter(row => row.sourceKind === 'sites').length, 3);
      assert.doesNotMatch(JSON.stringify(api.getSnapshot()), /Synthetic flow scope unavailable/);
    });

    await check('CLI compatibility blocks unavailable commands and preserves supported inventory', async () => {
      await writeFile(join(root, 'scenario.json'), JSON.stringify({ missingCapability: 'flow.list' }));
      const beforeFlowCalls = (await calls()).filter(args => args[0] === 'content' && args[1] === 'flow').length;
      await invoke('tadx.refresh');
      const snapshot = api.getSnapshot();
      assert.equal(snapshot.artifacts.filter(row => row.sourceKind === 'sites').length, 2);
      assert.ok(snapshot.artifacts.some(row => row.sourceKind === 'workspaces'));
      assert.match(JSON.stringify(snapshot), /flow.list.*compatible CLI/i);
      const afterFlowCalls = (await calls()).filter(args => args[0] === 'content' && args[1] === 'flow').length;
      assert.equal(afterFlowCalls, beforeFlowCalls,
        'An unavailable command must not reach the executable.');
      await writeFile(join(root, 'scenario.json'), '{}');
      await invoke('tadx.refresh');
      assert.equal(api.getSnapshot().artifacts.filter(row => row.sourceKind === 'sites').length, 3);
    });

    await check('malformed compatibility metadata blocks inventory reads and recovers explicitly', async () => {
      await writeFile(join(root, 'scenario.json'), JSON.stringify({ malformedCapabilities: true }));
      const reads = (args: string[]): boolean => args[0] === 'env' || args[0] === 'workspace';
      const before = (await calls()).filter(reads).length;
      await invoke('tadx.refresh');
      const snapshot = api.getSnapshot();
      assert.equal(snapshot.artifacts.length, 0);
      assert.match(JSON.stringify(snapshot.errors), /complete capability metadata/);
      assert.equal((await calls()).filter(reads).length, before);
      await writeFile(join(root, 'scenario.json'), '{}');
      await invoke('tadx.refresh');
      assert.equal(api.getSnapshot().artifacts.length, 6);
    });

    await writeFile(join(root, 'report.json'), JSON.stringify({ vscodeVersion: vscode.version, checks }, null, 2));
  } finally {
    await invoke('workbench.action.closeAllEditors');
  }
}
