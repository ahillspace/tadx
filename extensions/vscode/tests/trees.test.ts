import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import test from 'node:test';
import { runInNewContext } from 'node:vm';
import { buildSync } from 'esbuild';
import { Inventory } from '../src/inventory.js';
import type { Project, RemoteArtifact, SiteInventory } from '../src/model.js';
import type { ExplorerNode } from '../src/trees.js';

// Only VS Code's presentation primitives are replaced; tree projection and inventory loading run unchanged.
const vscode = {
  TreeItem: class { constructor(readonly label: string, readonly collapsibleState: number) {} },
  ThemeIcon: class { constructor(readonly id: string) {} },
  TreeItemCollapsibleState: { None: 0, Collapsed: 1, Expanded: 2 },
  EventEmitter: class {
    event = (): { dispose(): void } => ({ dispose() {} });
    fire(): void {}
    dispose(): void {}
  },
};
const moduleExports = { exports: {} };
const bundle = buildSync({ entryPoints: [resolve(__dirname, '../src/trees.ts')], bundle: true, platform: 'node', format: 'cjs', external: ['vscode'], write: false });
runInNewContext(bundle.outputFiles[0]!.text, {
  module: moduleExports, exports: moduleExports.exports,
  require: (name: string) => { assert.equal(name, 'vscode'); return vscode; },
});
const { InventoryTree } = moduleExports.exports as typeof import('../src/trees.js');

const project = (id: string, name: string, parentId = ''): Project => ({ id, name, parentId });
const artifact = (id: string, name: string, projectId: string, kind: RemoteArtifact['kind'] = 'workbook'): RemoteArtifact => ({ id, name, projectId, kind });
const labels = (nodes: readonly ExplorerNode[]): string[] => Array.from(nodes, node => String(node.label));

function harness(projects: Project[], artifacts: RemoteArtifact[] = []) {
  const reads: string[] = [];
  const site: SiteInventory = { environment: 'dev', projects, artifacts, complete: true, warnings: [],
    cache: { status: 'current', complete: true, stale: false, coverage: [], warnings: [] } };
  const inventory = new Inventory({
    environments: async () => [], workspaces: async () => [],
    siteInventory: async alias => { reads.push(alias); return { ...site, environment: alias }; },
    workspaceInventory: async () => { throw new Error('Unexpected workspace read.'); },
  });
  inventory.environments = [{ alias: 'dev', default: true }, { alias: 'review', default: false }];
  inventory.selectedSite = 'dev';
  inventory.sites.set('dev', site);
  const tree = new InventoryTree(inventory, 'sites');
  return { tree, inventory, site, reads };
}

test('all sites are collapsed roots and expansion loads only that site without changing selection', async () => {
  const { tree, inventory, reads } = harness([project('ops', 'Operations')]);
  const roots = await tree.getChildren();
  assert.deepEqual(labels(roots), ['dev', 'review']);
  assert.ok(roots.every(root => root.collapsibleState === vscode.TreeItemCollapsibleState.Collapsed));
  assert.ok(roots.every(root => root.contextValue === 'site'));
  assert.deepEqual(reads, []);
  assert.deepEqual(labels(await tree.getChildren(roots[1])), ['Operations']);
  assert.deepEqual(reads, ['review']);
  assert.equal(inventory.selectedSite, 'dev');
  await tree.getChildren(roots[1]);
  assert.deepEqual(reads, ['review']);
});

test('workspace roots expose their source to contextual bulk actions', async () => {
  const { inventory } = harness([]);
  inventory.workspaces = [{ name: 'work', default: true, available: true, manifestValid: true }];
  const tree = new InventoryTree(inventory, 'workspaces');
  const [root] = await tree.getChildren();
  assert.equal(root?.contextValue, 'workspace');
  assert.equal(root?.source, 'work');
  assert.equal(root?.collapsibleState, vscode.TreeItemCollapsibleState.Collapsed);
});

test('nested projects retain empty ancestors, short names, full paths, and exact identities', async () => {
  const { tree } = harness([project('root', 'Department'), project('child', 'Operations', 'root'), project('empty', 'Empty', 'child')],
    [artifact('wb-1', 'Revenue', 'child'), artifact('flow-1', 'Refresh', 'root', 'flow')]);
  const [site] = await tree.getChildren();
  const roots = await tree.getChildren(site);
  assert.deepEqual(labels(roots), ['Department']);
  const children = await tree.getChildren(roots[0]);
  assert.deepEqual(labels(children), ['Operations', 'Flows']);
  const child = children[0]!;
  assert.equal(child.projectId, 'child');
  assert.match(String(child.tooltip), /Department\/Operations/);
  assert.match(String(child.tooltip), /Project ID: child/);
  const nested = await tree.getChildren(child);
  assert.deepEqual(labels(nested), ['Empty', 'Workbooks']);
  assert.equal(nested[0]?.collapsibleState, vscode.TreeItemCollapsibleState.None);
  const [workbook] = await tree.getChildren(nested[1]);
  assert.equal(workbook?.label, 'Revenue');
  assert.equal(workbook?.key, JSON.stringify(['remote', 'dev', 'workbook', 'wb-1']));
});

test('empty projects remain visible even when the site has no artifacts', async () => {
  const { tree } = harness([project('root', 'Department'), project('child', 'Empty', 'root')]);
  const [site] = await tree.getChildren();
  const roots = await tree.getChildren(site);
  assert.deepEqual(labels(roots), ['Department']);
  assert.deepEqual(labels(await tree.getChildren(roots[0])), ['Empty']);
});

test('artifact filters preserve ancestors and remove unrelated branches', async () => {
  const { tree, inventory } = harness([project('root', 'Department'), project('child', 'Operations', 'root'), project('other', 'Other')],
    [artifact('wb-1', 'Revenue', 'child'), artifact('wb-2', 'Costs', 'other')]);
  inventory.filter = 'revenue';
  const [site] = await tree.getChildren();
  const roots = await tree.getChildren(site);
  assert.deepEqual(labels(roots), ['Department']);
  const children = await tree.getChildren(roots[0]);
  assert.deepEqual(labels(children), ['Operations']);
  const groups = await tree.getChildren(children[0]);
  assert.deepEqual(labels(groups), ['Workbooks']);
  assert.deepEqual(labels(await tree.getChildren(groups[0])), ['Revenue']);
});

test('full project path filters match descendants without flattening them', async () => {
  const { tree, inventory } = harness([project('root', 'Department'), project('child', 'Operations', 'root')], [artifact('wb-1', 'Revenue', 'child')]);
  inventory.filter = 'department/operations';
  const [site] = await tree.getChildren();
  const [root] = await tree.getChildren(site);
  assert.equal(root?.label, 'Department');
  const [child] = await tree.getChildren(root);
  assert.equal(child?.label, 'Operations');
  const [group] = await tree.getChildren(child);
  assert.deepEqual(labels(await tree.getChildren(group)), ['Revenue']);
});

test('missing parents, missing projects, and cycles remain reachable exactly once', async () => {
  const { tree } = harness([project('a', 'Cycle A', 'b'), project('b', 'Cycle B', 'a'), project('self', 'Self', 'self'),
    project('orphan', 'Orphan', 'missing'), project('leaf', 'Leaf', 'b')], [artifact('wb-1', 'Unassigned', 'absent'), artifact('wb-2', 'Nested', 'leaf')]);
  const [site] = await tree.getChildren();
  const pending = await tree.getChildren(site);
  const projects = new Set<string>();
  const artifacts = new Set<string>();
  while (pending.length) {
    const node = pending.shift()!;
    if (node.type === 'project') {
      assert.equal(projects.has(node.projectId!), false, 'Every project must have only one parent in the displayed tree.');
      projects.add(node.projectId!);
    }
    if (node.type === 'artifact') artifacts.add(String(node.label));
    pending.push(...await tree.getChildren(node));
  }
  assert.deepEqual([...projects].sort(), ['a', 'absent', 'b', 'leaf', 'orphan', 'self']);
  assert.deepEqual([...artifacts].sort(), ['Nested', 'Unassigned']);
});

test('site read failures stay visible and do not hide other configured roots', async () => {
  const { tree, inventory } = harness([]);
  inventory.errors.set('site:review', 'Cache cannot be read.');
  const roots = await tree.getChildren();
  assert.deepEqual(labels(roots), ['dev', 'review']);
  assert.deepEqual(labels(await tree.getChildren(roots[1])), ['Cache cannot be read.']);
});
