import assert from 'node:assert/strict';
import test from 'node:test';
import type { TadxClient } from '../src/cli.js';
import { Inventory, projectPaths } from '../src/inventory.js';
import { localArtifactKey, remoteArtifactKey, type EnvironmentProfile, type SiteInventory, type Workspace, type WorkspaceInventory } from '../src/model.js';

type Client = Pick<TadxClient, 'environments' | 'workspaces' | 'siteInventory' | 'workspaceInventory'>;
const environment: EnvironmentProfile = { alias: 'dev', default: true, serverUrl: 'https://tableau.example.test' };
const workspace: Workspace = { name: 'work', id: 'ws-1', root: '/work', default: true, available: true, manifestValid: true };
const site = (name = 'Current'): SiteInventory => ({ environment: 'dev', cache: { status: 'current', complete: true, stale: false, coverage: [], warnings: [] },
  projects: [{ id: 'p-1', name: 'Ops', parentId: '' }], artifacts: [{ id: 'wb-1', kind: 'workbook', name, projectId: 'p-1' }], complete: true, warnings: [] });
const local = (): WorkspaceInventory => ({ workspace, artifacts: [{ id: 'wb-1', kind: 'workbook', name: 'Current', state: 'clean', path: 'artifacts/workbook/Current--identity' }],
  scanComplete: true, moreAvailable: false, warnings: [] });
function client(overrides: Partial<Client> = {}): Client {
  return { environments: async () => [environment], workspaces: async () => [workspace],
    siteInventory: async () => site(), workspaceInventory: async () => local(), ...overrides };
}

test('a previous refresh cannot overwrite the current generation after resolving late', async () => {
  const oldRead = deferred<SiteInventory>();
  const firstStarted = deferred<void>();
  let reads = 0;
  let oldSignal: AbortSignal | undefined;
  const inventory = new Inventory(client({ siteInventory: async (_alias, signal) => {
    if (++reads === 1) { oldSignal = signal; firstStarted.resolve(); return oldRead.promise; }
    return site('Fresh generation');
  } }));
  const first = inventory.refresh();
  await firstStarted.promise;
  await inventory.refresh();
  assert.equal(oldSignal?.aborted, true);
  assert.equal(inventory.sites.get('dev')?.artifacts[0]?.name, 'Fresh generation');
  oldRead.resolve(site('Stale generation'));
  await first;
  assert.equal(inventory.sites.get('dev')?.artifacts[0]?.name, 'Fresh generation');
  assert.equal(inventory.busy, false);
  assert.equal(inventory.snapshot().artifacts.find(row => row.sourceKind === 'sites')?.name, 'Fresh generation');
});

test('list failures remain visible while independent inventory still loads', async () => {
  const inventory = new Inventory(client({ environments: async () => { throw new Error('Environment config is unavailable.'); } }));
  await inventory.refresh();
  assert.deepEqual(inventory.environments, []);
  assert.equal(inventory.locals.get('work')?.artifacts.length, 1);
  assert.match(inventory.errors.get('sites') ?? '', /Environment config/);
  assert.match(inventory.snapshot().errors?.join(' ') ?? '', /Environment config/);
  assert.equal(inventory.busy, false);
});

test('a failed site read is not represented as an empty successful site', async () => {
  const inventory = new Inventory(client({ siteInventory: async () => { throw new Error('Cache generation cannot be read.'); } }));
  await inventory.refresh();
  assert.equal(inventory.sites.has('dev'), false);
  assert.equal(inventory.snapshot().sources.sites[0]?.status, 'Unavailable');
  assert.match(inventory.snapshot().errors?.join(' ') ?? '', /Cache generation/);
});

test('duplicate site loads share one request and source switching preserves explicit selection', async () => {
  const loading = deferred<SiteInventory>();
  let reads = 0;
  const inventory = new Inventory(client({ siteInventory: async () => { reads++; return loading.promise; } }));
  inventory.environments = [environment, { alias: 'review', default: false }];
  const first = inventory.loadSite('review');
  const second = inventory.selectSite('review');
  assert.equal(reads, 1);
  loading.resolve({ ...site(), environment: 'review' });
  await Promise.all([first, second]);
  assert.equal(inventory.selectedSite, 'review');
  assert.equal(inventory.sites.has('review'), true);
  await assert.rejects(inventory.selectSite('missing'), /no longer configured/);
});

test('project hierarchy names cycles and unknown parents instead of hiding them', () => {
  const paths = projectPaths([
    { id: 'root', name: 'Department', parentId: '' }, { id: 'nested', name: 'Ops', parentId: 'root' },
    { id: 'orphan', name: 'Detached', parentId: 'missing' },
    { id: 'cycle-a', name: 'A', parentId: 'cycle-b' }, { id: 'cycle-b', name: 'B', parentId: 'cycle-a' },
    { id: 'self', name: 'Self', parentId: 'self' },
  ]);
  assert.equal(paths.get('nested'), 'Department/Ops');
  assert.equal(paths.get('orphan'), '[unknown parent missing]/Detached');
  assert.match(paths.get('cycle-a') ?? '', /\[cyclic hierarchy\].*B\/A/);
  assert.match(paths.get('cycle-b') ?? '', /\[cyclic hierarchy\].*A\/B/);
  assert.equal(paths.get('self'), '[cyclic hierarchy]/Self');
});

test('identical content IDs in separate sites and local copies remain distinct selections', async () => {
  const inventory = new Inventory(client());
  await inventory.refresh();
  const remote = site().artifacts[0]!;
  const artifact = local().artifacts[0]!;
  inventory.environments.push({ alias: 'review', default: false });
  inventory.sites.set('review', { ...site('Other site'), environment: 'review' });
  assert.equal(inventory.selection(remoteArtifactKey('review', remote))?.artifact.name, 'Other site');
  assert.equal(inventory.selection(remoteArtifactKey('dev', remote))?.location, 'remote');
  assert.equal(inventory.selection(localArtifactKey('work', artifact))?.location, 'local');
  assert.equal(inventory.snapshot().artifacts.some(item => item.relatedKeys?.length), false);
});

test('transfer selection accepts mixed supported kinds within one source and rejects cross-source or invalid rows', async () => {
  const cached = site();
  cached.artifacts.push({ id: 'ds-1', kind: 'datasource', name: 'Sales', projectId: 'p-1' });
  const scanned = local();
  scanned.artifacts.push({ id: 'flow-1', kind: 'flow', name: 'Prepare', path: 'artifacts/flow/Prepare', state: 'dirty' },
    { id: 'pulse-1', kind: 'pulse-definition', name: 'Metric', path: 'artifacts/pulse-definition/Metric', state: 'clean' });
  const inventory = new Inventory(client({ siteInventory: async () => cached, workspaceInventory: async () => scanned }));
  await inventory.refresh();
  const remoteKeys = cached.artifacts.map(item => remoteArtifactKey('dev', item));
  const localKeys = scanned.artifacts.map(item => localArtifactKey('work', item));
  assert.deepEqual(inventory.transferSelection(remoteKeys, 'remote').map(item => item.artifact.kind), ['workbook', 'datasource']);
  assert.deepEqual(inventory.transferSelection(localKeys.slice(0, 2), 'local').map(item => item.artifact.kind), ['workbook', 'flow']);
  inventory.environments.push({ alias: 'review', default: false });
  inventory.sites.set('review', { ...cached, environment: 'review' });
  assert.throws(() => inventory.transferSelection([remoteKeys[0]!, remoteArtifactKey('review', cached.artifacts[1]!)], 'remote'), /one source/);
  assert.throws(() => inventory.transferSelection([remoteKeys[0]!, localKeys[0]!], 'remote'), /one source/);
  assert.throws(() => inventory.transferSelection([localKeys[2]!], 'local'), /workbooks, datasources, and flows/);
  assert.throws(() => inventory.transferSelection([remoteKeys[0]!, remoteKeys[0]!], 'remote'), /unique/);
  assert.throws(() => inventory.transferSelection(['missing'], 'remote'), /unavailable/);
  scanned.artifacts[0]!.state = 'invalid';
  assert.throws(() => inventory.transferSelection([localKeys[0]!], 'local'), /valid local/);
});

test('partial scan and cache freshness remain visible without claiming remote equality', async () => {
  const cached = site();
  cached.complete = false;
  cached.cache.stale = true;
  const scanned = local();
  scanned.scanComplete = false;
  scanned.moreAvailable = true;
  const inventory = new Inventory(client({ siteInventory: async () => cached, workspaceInventory: async () => scanned }));
  await inventory.refresh();
  const snapshot = inventory.snapshot();
  assert.equal(snapshot.sources.sites[0]?.complete, false);
  assert.match(snapshot.sources.sites[0]?.status ?? '', /Stale.*partial/);
  assert.equal(snapshot.sources.workspaces[0]?.status, 'Partial scan');
  assert.equal(snapshot.sources.workspaces[0]?.complete, false);
  const row = snapshot.artifacts.find(item => item.sourceKind === 'workspaces');
  assert.match(row?.metadata?.map(item => item.value).join(' ') ?? '', /Remote equality is unknown/);
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

for (const location of ['site', 'workspace'] as const) {
  test(`forced ${location} reload acquires updated inventory while ordinary loads share its request`, async () => {
    const old = deferred<SiteInventory | WorkspaceInventory>();
    const fresh = deferred<SiteInventory | WorkspaceInventory>();
    const signals: AbortSignal[] = [];
    const read = async (_source: string, signal?: AbortSignal) => {
      signals.push(signal!);
      return signals.length === 1 ? old.promise : fresh.promise;
    };
    const inventory = new Inventory(client(location === 'site'
      ? { siteInventory: read as Client['siteInventory'] }
      : { workspaceInventory: read as Client['workspaceInventory'] }));
    inventory.environments = [environment];
    inventory.workspaces = [workspace];
    const load = (reload = false) => location === 'site' ? inventory.loadSite('dev', reload) : inventory.loadWorkspace('work', reload);
    const original = load();
    // The cache refresh or local mutation finishes while its previous scan is still pending.
    const reloaded = load(true);
    assert.equal(signals.length, 2);
    assert.equal(signals[0]?.aborted, true);
    assert.equal(load(), reloaded);
    const updated = location === 'site' ? site('Updated after refresh') : local();
    updated.artifacts[0]!.name = 'Updated after refresh';
    fresh.resolve(updated);
    await reloaded;
    old.resolve(location === 'site' ? site('Old observation') : local());
    await original;
    const rows = inventory.snapshot().artifacts;
    assert.equal(rows[0]?.name, 'Updated after refresh');
    assert.deepEqual(inventory.snapshot().errors, []);
    assert.equal(inventory.selection(rows[0]!.key)?.location, location === 'site' ? 'remote' : 'local');
  });
}

test('superseded success cannot clear a current read failure, and a later reload can recover', async () => {
  const old = deferred<SiteInventory>();
  const latest = deferred<SiteInventory>();
  let reads = 0;
  const inventory = new Inventory(client({ siteInventory: async () => {
    reads++;
    return reads === 1 ? old.promise : reads === 2 ? latest.promise : site('Recovered');
  } }));
  const original = inventory.loadSite('dev');
  const reloaded = inventory.loadSite('dev', true);
  latest.reject(new Error('Current cache read failed.'));
  await reloaded;
  old.resolve(site('Old observation'));
  await original;
  assert.equal(inventory.sites.has('dev'), false);
  assert.equal(inventory.errors.get('site:dev'), 'Current cache read failed.');
  await inventory.loadSite('dev', true);
  assert.equal(reads, 3);
  assert.equal(inventory.sites.get('dev')?.artifacts[0]?.name, 'Recovered');
  assert.deepEqual(inventory.snapshot().errors, []);
});

test('each forced reload supersedes earlier reads without disturbing another source', async () => {
  const reads = [deferred<SiteInventory>(), deferred<SiteInventory>(), deferred<SiteInventory>()];
  const signals: AbortSignal[] = [];
  const inventory = new Inventory(client({ siteInventory: async (alias, signal) => {
    if (alias === 'review') return { ...site('Independent source'), environment: alias };
    signals.push(signal!);
    return reads[signals.length - 1]!.promise;
  } }));
  const first = inventory.loadSite('dev');
  const second = inventory.loadSite('dev', true);
  const latest = inventory.loadSite('dev', true);
  assert.equal(signals.length, 3);
  assert.equal(signals[0]?.aborted, true);
  assert.equal(signals[1]?.aborted, true);
  assert.equal(signals[2]?.aborted, false);
  await inventory.loadSite('review');
  reads[2]!.resolve(site('Latest observation'));
  await latest;
  reads[1]!.reject(new Error('Superseded read failed late.'));
  reads[0]!.reject(new Error('First read failed late.'));
  await Promise.all([first, second]);
  assert.equal(inventory.sites.get('dev')?.artifacts[0]?.name, 'Latest observation');
  assert.equal(inventory.sites.get('review')?.artifacts[0]?.name, 'Independent source');
  assert.deepEqual(inventory.snapshot().errors, []);
});

test('a global refresh cancels forced source reads and ignores their later completion', async () => {
  const old = deferred<SiteInventory>();
  const forced = deferred<SiteInventory>();
  const signals: AbortSignal[] = [];
  const inventory = new Inventory(client({ siteInventory: async (_alias, signal) => {
    signals.push(signal!);
    return signals.length === 1 ? old.promise : signals.length === 2 ? forced.promise : site('Global refresh');
  } }));
  const original = inventory.loadSite('dev');
  const reloaded = inventory.loadSite('dev', true);
  assert.equal(signals.length, 2);
  await inventory.refresh();
  assert.equal(signals[0]?.aborted, true);
  assert.equal(signals[1]?.aborted, true);
  forced.reject(new Error('Canceled source read finished late.'));
  old.resolve(site('Old observation'));
  await Promise.all([original, reloaded]);
  assert.equal(inventory.sites.get('dev')?.artifacts[0]?.name, 'Global refresh');
  assert.deepEqual(inventory.snapshot().errors, []);
  assert.equal(inventory.busy, false);
});

test('disposal cancels reads and prevents late inventory commits or new reads', async () => {
  const pending = deferred<SiteInventory>();
  let signal: AbortSignal | undefined;
  let reads = 0;
  const inventory = new Inventory(client({ siteInventory: async (_alias, value) => { reads++; signal = value; return pending.promise; } }));
  const loading = inventory.loadSite('dev');
  inventory.dispose();
  assert.equal(signal?.aborted, true);
  pending.resolve(site('After disposal'));
  await loading;
  await inventory.loadSite('dev', true);
  await inventory.refresh();
  assert.equal(inventory.sites.size, 0);
  assert.equal(reads, 1);
});

test('disposal during source discovery prevents its late results from restarting inventory reads', async () => {
  const environments = deferred<EnvironmentProfile[]>();
  let sourceReads = 0;
  const inventory = new Inventory(client({ environments: async () => environments.promise,
    siteInventory: async () => { sourceReads++; return site(); }, workspaceInventory: async () => { sourceReads++; return local(); } }));
  const refreshing = inventory.refresh();
  inventory.dispose();
  environments.resolve([environment]);
  await refreshing;
  assert.deepEqual(inventory.environments, []);
  assert.equal(sourceReads, 0);
  assert.equal(inventory.busy, false);
});

test('nontransferable local kinds remain inspectable without publishing actions', async () => {
  const scanned = local();
  scanned.artifacts = ['pulse-definition', 'lineage', 'future-kind'].map(kind => ({
    kind, id: `id-${kind}`, name: kind, path: `artifacts/${kind}/Example`, canonicalPath: `artifacts/${kind}/Example/payload.json`, state: 'clean',
  }));
  const inventory = new Inventory(client({ workspaceInventory: async () => scanned }));
  await inventory.refresh();
  const rows = inventory.snapshot().artifacts.filter(row => row.sourceKind === 'workspaces');
  assert.equal(rows.length, 3);
  for (const row of rows) {
    assert.equal(row.actions.includes('upload'), false);
    assert.equal(row.actions.includes('download'), false);
    assert.equal(row.actions.includes('openPayload'), true);
    assert.equal(row.actions.includes('move'), true);
    assert.equal(row.actions.includes('delete'), true);
  }
});
