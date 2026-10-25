import * as vscode from 'vscode';
import { cacheLabel, Inventory, localStateLabel, projectPaths } from './inventory.js';
import { isArtifactKind, localArtifactKey, remoteArtifactKey, type ArtifactKind, type SiteInventory } from './model.js';
import type { ActivityEntry } from './operations.js';

const kinds: ArtifactKind[] = ['workbook', 'datasource', 'flow'];
const labels: Record<ArtifactKind, string> = { workbook: 'Workbooks', datasource: 'Datasources', flow: 'Flows' };
const icons: Record<ArtifactKind, string> = { workbook: 'graph', datasource: 'database', flow: 'git-merge' };

export class ExplorerNode extends vscode.TreeItem {
  constructor(label: string, readonly type: 'site' | 'workspace' | 'project' | 'kind' | 'artifact' | 'message' | 'operation',
    readonly source?: string, readonly key?: string, readonly kind?: string, readonly projectId?: string) {
    super(label, type === 'site' || type === 'workspace' || type === 'project' || type === 'kind' ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None);
    this.id = JSON.stringify([type, source, key, kind, projectId]);
  }
}

export class InventoryTree implements vscode.TreeDataProvider<ExplorerNode>, vscode.Disposable {
  private readonly event = new vscode.EventEmitter<ExplorerNode | undefined>();
  readonly onDidChangeTreeData = this.event.event;
  constructor(private readonly inventory: Inventory, private readonly scope: 'sites' | 'workspaces') {}
  refresh(): void { this.event.fire(undefined); }
  dispose(): void { this.event.dispose(); }
  getTreeItem(node: ExplorerNode): vscode.TreeItem { return node; }

  async getChildren(node?: ExplorerNode): Promise<ExplorerNode[]> {
    if (!node) return this.roots();
    if (!node.source) return [];
    if (node.type === 'site') {
      if (!this.inventory.sites.has(node.source) && !this.inventory.errors.has(`site:${node.source}`)) await this.inventory.loadSite(node.source);
      const inventory = this.inventory.sites.get(node.source);
      const error = this.inventory.errors.get(`site:${node.source}`);
      if (error) return [notice(error, 'warning')];
      if (!inventory) return [notice('Loading cached inventory...')];
      const projects = this.projects(node.source, inventory);
      const messages = inventory.warnings.slice(0, 3).map(warning => notice(warning, 'warning'));
      if (!projects.length) messages.push(notice(inventory.cache.status === 'uninitialized' ? 'Refresh Site Cache to load inventory.' : this.inventory.filter ? 'No artifacts match the filter.' : 'No cached artifacts.'));
      return [...messages, ...projects];
    }
    if (node.type === 'workspace') {
      if (!this.inventory.locals.has(node.source) && !this.inventory.errors.has(`workspace:${node.source}`)) await this.inventory.loadWorkspace(node.source);
      const error = this.inventory.errors.get(`workspace:${node.source}`);
      if (error) return [notice(error, 'warning')];
      const inventory = this.inventory.locals.get(node.source);
      if (!inventory) return [];
      const localKinds = [...new Set(inventory.artifacts.map(item => item.kind))].sort();
      const groups = localKinds.filter(kind => inventory.artifacts.some(item => item.kind === kind && this.matches(item.name, item.path ?? '', item.id)))
        .map(kind => this.group(node.source!, kind));
      return [...inventory.warnings.slice(0, 3).map(warning => notice(warning, 'warning')), ...groups,
        ...(!groups.length ? [notice(this.inventory.filter ? 'No artifacts match the filter.' : 'No managed artifacts.')] : [])];
    }
    if (node.type === 'project') {
      const inventory = this.inventory.sites.get(node.source);
      if (!inventory) return [];
      const path = projectPaths(inventory.projects).get(node.projectId ?? '') ?? String(node.label);
      return [...this.projects(node.source, inventory, node.projectId),
        ...kinds.filter(kind => inventory.artifacts.some(item => item.projectId === node.projectId && item.kind === kind && this.matches(item.name, item.id, item.projectId, path)))
          .map(kind => this.group(node.source!, kind, node.projectId))];
    }
    if (node.type === 'kind') {
      if (this.scope === 'sites') {
        const inventory = this.inventory.sites.get(node.source);
        const paths = projectPaths(inventory?.projects ?? []);
        return (inventory?.artifacts ?? []).filter(item => item.kind === node.kind && item.projectId === node.projectId && this.matches(item.name, item.id, item.projectId, paths.get(item.projectId) ?? `Unknown project (${item.projectId || 'none'})`))
          .map(item => this.artifact(item.name, remoteArtifactKey(node.source!, item), item.kind, item.webpageUrl ? 'remoteArtifactWithUrl' : 'remoteArtifact', item.source?.stale ? 'Stale cache' : 'Cached', item.id));
      }
      return (this.inventory.locals.get(node.source)?.artifacts ?? []).filter(item => item.kind === node.kind && this.matches(item.name, item.id, item.path ?? ''))
        .map(item => this.artifact(item.name || item.path || 'Unidentified artifact', localArtifactKey(node.source!, item), item.kind,
          item.path && ['clean', 'dirty'].includes(item.state) ? 'localArtifact' : 'invalidArtifact', localStateLabel(item.state), item.path ?? item.id));
    }
    return [];
  }
  private roots(): ExplorerNode[] {
    const error = this.inventory.errors.get(this.scope);
    if (error) return [notice(error, 'error')];
    if (this.scope === 'sites') {
      return this.inventory.environments.map(environment => {
        const node = new ExplorerNode(environment.alias, 'site', environment.alias);
        node.iconPath = new vscode.ThemeIcon('server-environment');
        node.contextValue = 'site';
        const inventory = this.inventory.sites.get(environment.alias);
        const unavailable = this.inventory.errors.has(`site:${environment.alias}`);
        node.description = [environment.default ? 'Default' : '', unavailable ? 'Unavailable' : inventory ? cacheLabel(inventory) : 'Not loaded'].filter(Boolean).join(', ');
        node.tooltip = [environment.serverUrl, environment.siteContentUrl || 'Default site', inventory?.cache.generatedAt && `Observed: ${inventory.cache.generatedAt}`].filter(Boolean).join('\n');
        return node;
      });
    }
    return this.inventory.workspaces.map(workspace => {
      const node = new ExplorerNode(workspace.name, 'workspace', workspace.name);
      node.contextValue = 'workspace';
      node.iconPath = new vscode.ThemeIcon(!workspace.available || !workspace.manifestValid ? 'warning' : 'folder-opened');
      node.description = !workspace.available || !workspace.manifestValid ? 'Unavailable' : workspace.default ? 'Default' : undefined;
      node.tooltip = workspace.violations?.join('\n') || workspace.name;
      return node;
    });
  }
  private projects(source: string, inventory: SiteInventory, parentId?: string): ExplorerNode[] {
    const projects = new Map(inventory.projects.map(project => [project.id, project]));
    const paths = projectPaths(inventory.projects);
    for (const artifact of inventory.artifacts) {
      if (!projects.has(artifact.projectId)) projects.set(artifact.projectId, {
        id: artifact.projectId, name: `Unknown project (${artifact.projectId || 'none'})`, parentId: '',
      });
    }
    const parents = new Map<string, string | undefined>();
    for (const project of projects.values()) parents.set(project.id, project.parentId && projects.has(project.parentId) ? project.parentId : undefined);
    // Promote one node in each cycle so every project has a finite, reachable display path.
    const resolved = new Set<string>();
    for (const id of projects.keys()) {
      const chain = new Set<string>();
      let current: string | undefined = id;
      while (current !== undefined && !resolved.has(current)) {
        if (chain.has(current)) { parents.set(current, undefined); break; }
        chain.add(current);
        current = parents.get(current);
      }
      for (const item of chain) resolved.add(item);
    }
    const visible = new Set<string>();
    for (const project of projects.values()) {
      if (this.matches(project.name, project.id, paths.get(project.id) ?? project.name)) visible.add(project.id);
    }
    const artifactProjects = new Set<string>();
    for (const artifact of inventory.artifacts) {
      if (this.matches(artifact.name, artifact.id, artifact.projectId, paths.get(artifact.projectId) ?? projects.get(artifact.projectId)?.name ?? '')) {
        visible.add(artifact.projectId);
        artifactProjects.add(artifact.projectId);
      }
    }
    for (const id of visible) {
      let parent = parents.get(id);
      while (parent !== undefined && !visible.has(parent)) {
        visible.add(parent);
        parent = parents.get(parent);
      }
    }
    const branchProjects = new Set([...parents].filter(([id]) => visible.has(id)).map(([, parent]) => parent));
    return [...projects.values()].filter(project => parents.get(project.id) === parentId && visible.has(project.id)).map(project => {
      const node = new ExplorerNode(project.name, 'project', source, undefined, undefined, project.id);
      const path = paths.get(project.id) ?? project.name;
      node.iconPath = new vscode.ThemeIcon('folder');
      node.tooltip = `${path}\nProject ID: ${project.id || 'not reported'}`;
      if (project.parentId && parents.get(project.id) === undefined) node.description = projects.has(project.parentId) ? 'Cyclic hierarchy' : 'Parent unavailable';
      if (!branchProjects.has(project.id) && !artifactProjects.has(project.id)) node.collapsibleState = vscode.TreeItemCollapsibleState.None;
      return node;
    }).sort((a, b) => String(a.label).localeCompare(String(b.label)));
  }
  private group(source: string, kind: string, projectId?: string): ExplorerNode {
    const label = isArtifactKind(kind) ? labels[kind] : kind === 'pulse-definition' ? 'Pulse definitions' : kind === 'lineage' ? 'Lineage' : kind;
    const node = new ExplorerNode(label, 'kind', source, undefined, kind, projectId);
    node.iconPath = new vscode.ThemeIcon(isArtifactKind(kind) ? icons[kind] : 'file-code');
    return node;
  }
  private artifact(name: string, key: string, kind: string, context: string, state: string, detail: string): ExplorerNode {
    const node = new ExplorerNode(name, 'artifact', undefined, key, kind);
    node.description = state;
    node.tooltip = `${name}\n${detail}\n${state}`;
    node.iconPath = new vscode.ThemeIcon(isArtifactKind(kind) ? icons[kind] : 'file-code');
    node.contextValue = context === 'localArtifact' && !isArtifactKind(kind) ? 'localOtherArtifact' : context;
    node.command = { command: 'tadx.openArtifact', title: 'Open artifact', arguments: [key] };
    return node;
  }
  private matches(...values: string[]): boolean {
    return !this.inventory.filter || values.some(value => value.toLocaleLowerCase().includes(this.inventory.filter.toLocaleLowerCase()));
  }
}

export class ActivityTree implements vscode.TreeDataProvider<ExplorerNode>, vscode.Disposable {
  private readonly event = new vscode.EventEmitter<ExplorerNode | undefined>();
  readonly onDidChangeTreeData = this.event.event;
  entries: readonly ActivityEntry[] = [];
  refresh(): void { this.event.fire(undefined); }
  getTreeItem(node: ExplorerNode): vscode.TreeItem { return node; }
  getChildren(): ExplorerNode[] {
    return this.entries.map(entry => {
      const node = new ExplorerNode(entry.title, 'operation', undefined, entry.key);
      node.description = entry.state;
      node.tooltip = `${entry.startedAt}\n${entry.summary}`;
      node.iconPath = new vscode.ThemeIcon(entry.state === 'succeeded' ? 'pass' : entry.state === 'failed' ? 'error' : entry.state === 'pending' ? 'sync~spin' : 'warning');
      node.contextValue = entry.operationId ? 'operation' : 'operationResult';
      node.command = { command: 'tadx.inspectOperation', title: 'Show operation', arguments: [entry.key] };
      return node;
    });
  }
  dispose(): void { this.event.dispose(); }
}
function notice(text: string, icon = 'info'): ExplorerNode {
  const node = new ExplorerNode(text, 'message', undefined, text);
  node.id = undefined;
  node.iconPath = new vscode.ThemeIcon(icon);
  node.tooltip = text;
  return node;
}
