import type { TadxClient } from './cli.js';
import { isArtifactKind, localArtifactKey, remoteArtifactKey, type EnvironmentProfile, type LocalArtifact, type Project, type RemoteArtifact, type SiteInventory, type Workspace, type WorkspaceInventory } from './model.js';
import type { PanelArtifact, PanelSnapshot } from './panels.js';

export type ArtifactSelection =
  | { location: 'remote'; environment: EnvironmentProfile; artifact: RemoteArtifact }
  | { location: 'local'; workspace: Workspace; artifact: LocalArtifact };

type InventoryClient = Pick<TadxClient, 'environments' | 'workspaces' | 'siteInventory' | 'workspaceInventory'>;

/** Owns selection and loaded snapshots. Reading inventory never refreshes the remote cache. */
export class Inventory {
  environments: EnvironmentProfile[] = [];
  workspaces: Workspace[] = [];
  readonly sites = new Map<string, SiteInventory>();
  readonly locals = new Map<string, WorkspaceInventory>();
  readonly errors = new Map<string, string>();
  selectedSite?: string;
  selectedWorkspace?: string;
  busy = false;
  filter = '';
  private generation = 0;
  private abort = new AbortController();
  private disposed = false;
  private readonly loading = new Map<string, { promise: Promise<void>; controller: AbortController }>();
  private readonly listeners = new Set<() => void>();

  constructor(private readonly client: InventoryClient, selected?: { site?: string; workspace?: string }) {
    this.selectedSite = selected?.site;
    this.selectedWorkspace = selected?.workspace;
  }

  onChange(listener: () => void): { dispose(): void } {
    this.listeners.add(listener);
    return { dispose: () => this.listeners.delete(listener) };
  }
  changed(): void { for (const listener of this.listeners) listener(); }
  dispose(): void {
    this.disposed = true;
    this.generation++;
    this.abort.abort();
    this.loading.clear();
    this.busy = false;
    this.listeners.clear();
  }

  async refresh(): Promise<void> {
    if (this.disposed) return;
    const generation = ++this.generation;
    this.abort.abort();
    this.abort = new AbortController();
    this.loading.clear();
    this.busy = true;
    this.changed();
    const results = await Promise.allSettled([this.client.environments(this.abort.signal), this.client.workspaces(this.abort.signal)]);
    if (generation !== this.generation) return;
    this.errors.clear();
    this.sites.clear();
    this.locals.clear();
    const [environments, workspaces] = results;
    this.environments = environments.status === 'fulfilled' ? environments.value : [];
    this.workspaces = workspaces.status === 'fulfilled' ? workspaces.value : [];
    if (environments.status === 'rejected') this.errors.set('sites', message(environments.reason));
    if (workspaces.status === 'rejected') this.errors.set('workspaces', message(workspaces.reason));
    if (!this.environments.some(item => item.alias === this.selectedSite)) {
      this.selectedSite = this.environments.find(item => item.default)?.alias ?? (this.environments.length === 1 ? this.environments[0]?.alias : undefined);
    }
    if (!this.workspaces.some(item => item.name === this.selectedWorkspace)) {
      this.selectedWorkspace = this.workspaces.find(item => item.default)?.name ?? this.workspaces.find(item => item.available && item.manifestValid)?.name;
    }
    await Promise.allSettled([
      ...(this.selectedSite ? [this.loadSite(this.selectedSite)] : []),
      ...(this.selectedWorkspace ? [this.loadWorkspace(this.selectedWorkspace)] : []),
    ]);
    if (generation !== this.generation) return;
    this.busy = false;
    this.changed();
  }

  async selectSite(alias: string): Promise<void> {
    if (!this.environments.some(item => item.alias === alias)) throw new Error('The selected environment is no longer configured. Reload the inventory.');
    this.selectedSite = alias;
    this.changed();
    await this.loadSite(alias);
  }
  async selectWorkspace(name: string): Promise<void> {
    if (!this.workspaces.some(item => item.name === name)) throw new Error('The selected workspace is no longer registered. Reload the inventory.');
    this.selectedWorkspace = name;
    this.changed();
    await this.loadWorkspace(name);
  }
  loadSite(alias: string, reload = false): Promise<void> {
    const key = `site:${alias}`;
    if (!reload && this.sites.has(alias) && !this.loading.has(key)) return Promise.resolve();
    return this.load(key, reload, async signal => {
      const inventory = await this.client.siteInventory(alias, signal);
      return () => this.sites.set(alias, inventory);
    });
  }
  loadWorkspace(name: string, reload = false): Promise<void> {
    const key = `workspace:${name}`;
    if (!reload && this.locals.has(name) && !this.loading.has(key)) return Promise.resolve();
    return this.load(key, reload, async signal => {
      const inventory = await this.client.workspaceInventory(name, signal);
      return () => this.locals.set(name, inventory);
    });
  }
  private load(key: string, reload: boolean, read: (signal: AbortSignal) => Promise<() => unknown>): Promise<void> {
    if (this.disposed) return Promise.resolve();
    const existing = this.loading.get(key);
    if (existing && !reload) return existing.promise;
    existing?.controller.abort();
    const generation = this.generation;
    const controller = new AbortController();
    const signal = AbortSignal.any([this.abort.signal, controller.signal]);
    // Only the latest read for this source may publish, including when an old client ignores cancellation.
    const current = (): boolean => generation === this.generation && this.loading.get(key)?.promise === pending;
    const pending = read(signal).then(commit => {
      if (current()) {
        commit();
        this.errors.delete(key);
      }
    }, error => {
      if (current()) this.errors.set(key, message(error));
    }).finally(() => {
      if (current()) {
        this.loading.delete(key);
        this.changed();
      }
    });
    this.loading.set(key, { promise: pending, controller });
    return pending;
  }

  selection(key: string): ArtifactSelection | undefined {
    for (const environment of this.environments) {
      const artifact = this.sites.get(environment.alias)?.artifacts.find(item => remoteArtifactKey(environment.alias, item) === key);
      if (artifact) return { location: 'remote', environment, artifact };
    }
    for (const workspace of this.workspaces) {
      const artifact = this.locals.get(workspace.name)?.artifacts.find(item => localArtifactKey(workspace.name, item) === key);
      if (artifact) return { location: 'local', workspace: this.locals.get(workspace.name)?.workspace ?? workspace, artifact };
    }
    return undefined;
  }

  transferSelection(keys: readonly string[], location: 'remote'): Extract<ArtifactSelection, { location: 'remote' }>[];
  transferSelection(keys: readonly string[], location: 'local'): Extract<ArtifactSelection, { location: 'local' }>[];
  transferSelection(keys: readonly string[], location: 'local' | 'remote'): ArtifactSelection[] {
    if (keys.length < 1 || keys.length > 100 || new Set(keys).size !== keys.length) throw new Error('Select 1 to 100 unique artifacts.');
    const selections = keys.map(key => {
      const selected = this.selection(key);
      if (!selected) throw new Error('The selected artifact is unavailable. Reload the inventory and select it again.');
      return selected;
    });
    const source = (item: ArtifactSelection): string => item.location === 'remote' ? item.environment.alias : item.workspace.name;
    if (selections.some(item => item.location !== location || source(item) !== source(selections[0]!))) throw new Error('Select artifacts from one source.');
    for (const item of selections) {
      if (!isArtifactKind(item.artifact.kind)) throw new Error('Transfers support workbooks, datasources, and flows.');
      if (item.location === 'local' && (!item.artifact.path || !['clean', 'dirty'].includes(item.artifact.state))) throw new Error('Only valid local artifacts can be uploaded.');
    }
    return selections;
  }

  snapshot(operationBusy = false): PanelSnapshot {
    const artifacts: PanelArtifact[] = [];
    for (const environment of this.environments) {
      const inventory = this.sites.get(environment.alias);
      if (!inventory) continue;
      const paths = projectPaths(inventory.projects);
      for (const artifact of inventory.artifacts) {
        const stale = artifact.source?.stale || inventory.cache.stale;
        artifacts.push({
          key: remoteArtifactKey(environment.alias, artifact), name: artifact.name, kind: artifact.kind,
          sourceKind: 'sites', sourceId: environment.alias,
          location: artifact.projectPath ?? paths.get(artifact.projectId) ?? `Unknown project (${artifact.projectId || 'none'})`,
          state: stale ? 'Cached, stale' : 'Cached', description: artifact.description, updatedAt: artifact.updatedAt,
          artifactId: artifact.id, actions: artifact.webpageUrl ? ['download', 'openRemote'] : ['download'],
          metadata: [{ label: 'Environment', value: environment.alias }, { label: 'Observation', value: artifact.source?.observedAt ?? inventory.cache.generatedAt ?? 'Unknown' },
            { label: 'Local copies', value: 'Not correlated. Cached inventory does not expose authoritative site identity.' }],
        });
      }
    }
    for (const workspace of this.workspaces) {
      const inventory = this.locals.get(workspace.name);
      if (!inventory) continue;
      for (const artifact of inventory.artifacts) {
        const manageable = !!artifact.path && ['clean', 'dirty'].includes(artifact.state);
        const actions: PanelArtifact['actions'] = [];
        if (manageable) {
          if (isArtifactKind(artifact.kind)) actions.push('upload');
          actions.push('move', 'delete', 'reveal', 'openMetadata');
        }
        if (manageable && artifact.canonicalPath) actions.push('openPayload');
        const metadata = [{ label: 'Workspace', value: workspace.name },
          { label: 'Baseline', value: artifact.state === 'clean' ? 'Unchanged since download. Remote equality is unknown.' : artifact.state === 'dirty' ? 'Changed since download. Remote equality is unknown.' : artifact.reason ?? artifact.state }];
        if (artifact.source?.serverOrigin) metadata.push({ label: 'Source server', value: artifact.source.serverOrigin });
        if (artifact.source?.siteLuid) metadata.push({ label: 'Source site ID', value: artifact.source.siteLuid });
        if (artifact.source?.environment) metadata.push({ label: 'Source environment', value: artifact.source.environment });
        if (artifact.source?.site !== undefined) metadata.push({ label: 'Source site', value: artifact.source.site || 'Default site' });
        artifacts.push({ key: localArtifactKey(workspace.name, artifact), name: artifact.name || artifact.path || 'Unidentified artifact', kind: artifact.kind,
          sourceKind: 'workspaces', sourceId: workspace.name, location: workspace.name, path: artifact.path,
          state: localStateLabel(artifact.state), artifactId: artifact.id, description: artifact.diagnostic ?? artifact.reason, actions, metadata });
      }
    }
    return {
      sources: {
        sites: this.environments.map(environment => {
          const inventory = this.sites.get(environment.alias);
          return { id: environment.alias, label: environment.alias + (environment.default ? ' (default)' : ''),
            description: environment.serverUrl, status: this.errors.has(`site:${environment.alias}`) ? 'Unavailable' : inventory ? cacheLabel(inventory) : 'Not loaded',
            complete: inventory?.complete, warnings: inventory?.warnings };
        }),
        workspaces: this.workspaces.map(workspace => {
          const inventory = this.locals.get(workspace.name);
          return { id: workspace.name, label: workspace.name + (workspace.default ? ' (default)' : ''),
            status: !workspace.available || !workspace.manifestValid ? 'Unavailable' : inventory ? inventory.scanComplete && !inventory.moreAvailable ? 'Scanned' : 'Partial scan' : 'Not loaded',
            complete: inventory ? inventory.scanComplete && !inventory.moreAvailable : undefined,
            warnings: inventory?.warnings ?? workspace.violations };
        }),
      }, artifacts, busy: this.busy || operationBusy, errors: [...this.errors.values()],
    };
  }
}

export function projectPaths(projects: readonly Project[]): Map<string, string> {
  const byId = new Map(projects.map(project => [project.id, project]));
  const paths = new Map<string, string>();
  for (const project of projects) {
    const names: string[] = [];
    const visited = new Set<string>();
    let current: Project | undefined = project;
    while (current) {
      if (visited.has(current.id)) { names.unshift('[cyclic hierarchy]'); break; }
      visited.add(current.id);
      names.unshift(current.name);
      if (!current.parentId) break;
      const parent = byId.get(current.parentId);
      if (!parent) { names.unshift(`[unknown parent ${current.parentId}]`); break; }
      current = parent;
    }
    paths.set(project.id, names.join('/'));
  }
  return paths;
}

export function cacheLabel(inventory: SiteInventory): string {
  if (inventory.cache.status === 'uninitialized') return 'Cache not initialized';
  return [inventory.cache.stale ? 'Stale cache' : 'Cached', ...(!inventory.complete ? ['partial'] : [])].join(', ');
}
export function localStateLabel(state: string): string {
  return state === 'clean' ? 'Unchanged locally' : state === 'dirty' ? 'Changed locally' : state;
}
export function message(error: unknown): string { return error instanceof Error ? error.message : 'TADX could not complete the request.'; }
