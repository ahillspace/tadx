import { randomBytes } from 'node:crypto';
import type * as VSCode from 'vscode';

export type PanelSourceKind = 'sites' | 'workspaces';
export type PanelAction = 'download' | 'upload' | 'openRemote' | 'openPayload' | 'openMetadata' | 'reveal' | 'move' | 'delete';

export interface PanelSource {
  id: string;
  label: string;
  status?: string;
  description?: string;
  complete?: boolean;
  warnings?: string[];
}

/** A host-owned projection. Keys identify exact rows, never executable arguments. */
export interface PanelArtifact {
  key: string;
  name: string;
  kind: string;
  sourceKind: PanelSourceKind;
  sourceId: string;
  location: string;
  state?: string;
  description?: string;
  updatedAt?: string;
  artifactId?: string;
  path?: string;
  relatedKeys?: string[];
  actions: PanelAction[];
  metadata?: { label: string; value: string }[];
}

export interface PanelSnapshot {
  sources: Record<PanelSourceKind, PanelSource[]>;
  artifacts: PanelArtifact[];
  busy?: boolean;
  errors?: string[];
}

export interface PanelOperation {
  title: string;
  state: 'preview' | 'pending' | 'succeeded' | 'partial' | 'failed' | 'unknown';
  summary: string;
  details?: string;
  items?: { name: string; state: string; message?: string }[];
}

export type PanelRequest =
  | { action: 'download' | 'upload'; keys: string[] }
  | { action: Exclude<PanelAction, 'download' | 'upload'> | 'showArtifact'; key: string }
  | { action: 'refresh' | 'selectSource'; sourceKind: PanelSourceKind; sourceId: string };

const singleActions = new Set<string>(['openRemote', 'openPayload', 'openMetadata', 'reveal', 'move', 'delete']);
const transferKinds = new Set<string>(['workbook', 'datasource', 'flow']);

/** Treat every webview message as untrusted, including messages from an old render. */
export function validatePanelRequest(message: unknown, snapshot: PanelSnapshot): PanelRequest | undefined {
  if (!message || typeof message !== 'object' || Array.isArray(message)) return undefined;
  const value = message as Record<string, unknown>;
  if (value.type !== 'action' || typeof value.action !== 'string') return undefined;
  if (value.action === 'refresh' || value.action === 'selectSource') {
    if (value.sourceKind !== 'sites' && value.sourceKind !== 'workspaces') return undefined;
    if (typeof value.sourceId !== 'string' || !snapshot.sources[value.sourceKind].some(source => source.id === value.sourceId)) return undefined;
    return { action: value.action, sourceKind: value.sourceKind, sourceId: value.sourceId };
  }
  if (value.action === 'download' || value.action === 'upload') {
    if (snapshot.busy || !Array.isArray(value.keys) || value.keys.length === 0 || value.keys.length > 100) return undefined;
    if (!value.keys.every((key): key is string => typeof key === 'string') || new Set(value.keys).size !== value.keys.length) return undefined;
    const rows = value.keys.map(key => snapshot.artifacts.find(row => row.key === key));
    const first = rows[0];
    if (!first) return undefined;
    const sourceKind = value.action === 'download' ? 'sites' : 'workspaces';
    const action = value.action;
    if (rows.some(row => !row || row.sourceKind !== sourceKind || row.sourceId !== first.sourceId || !transferKinds.has(row.kind) || !row.actions.includes(action))) return undefined;
    return { action, keys: [...value.keys] };
  }
  if (typeof value.key !== 'string') return undefined;
  const row = snapshot.artifacts.find(artifact => artifact.key === value.key);
  if (!row) return undefined;
  if (value.action === 'showArtifact') return { action: 'showArtifact', key: row.key };
  if (!singleActions.has(value.action) || !row.actions.includes(value.action as PanelAction)) return undefined;
  if (snapshot.busy && (value.action === 'move' || value.action === 'delete')) return undefined;
  return { action: value.action as Exclude<PanelAction, 'download' | 'upload'>, key: row.key };
}

export function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character] ?? character);
}

export function panelHtml(options: { cspSource: string; scriptUri: string; styleUri: string; nonce: string; view: 'artifact' | 'inventory' | 'operation' }): string {
  const { cspSource, scriptUri, styleUri, nonce, view } = options;
  return `<!doctype html>
<html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${escapeHtml(cspSource)}; script-src 'nonce-${escapeHtml(nonce)}'; base-uri 'none'; form-action 'none';">
<title>TADX ${view === 'inventory' ? 'Bulk Actions' : view === 'operation' ? 'operation' : 'artifact'}</title>
<link rel="stylesheet" href="${escapeHtml(styleUri)}"></head>
<body data-view="${view}"><main id="app" aria-label="TADX"><p class="loading" role="status">Loading TADX inventory…</p></main>
<p id="announcement" class="visually-hidden" role="status" aria-live="polite"></p>
<script nonce="${escapeHtml(nonce)}" src="${escapeHtml(scriptUri)}"></script></body></html>`;
}

type PanelView = 'artifact' | 'inventory' | 'operation';

/** Owns editor panels only. All filesystem and CLI actions stay in the coordinator. */
export class PanelManager implements VSCode.Disposable {
  private readonly panels = new Map<PanelView, VSCode.WebviewPanel>();
  private snapshot: PanelSnapshot = { sources: { sites: [], workspaces: [] }, artifacts: [] };
  private artifactKey?: string;
  private inventorySource?: { sourceKind: PanelSourceKind; sourceId?: string };
  private operation?: PanelOperation;

  constructor(
    private readonly api: typeof VSCode,
    private readonly extensionUri: VSCode.Uri,
    private readonly onRequest: (request: PanelRequest) => void | Promise<void>,
  ) {}

  update(snapshot: PanelSnapshot): void {
    this.snapshot = snapshot;
    for (const [view, panel] of this.panels) this.sendState(view, panel);
  }

  showArtifact(key: string): void {
    if (!this.snapshot.artifacts.some(row => row.key === key)) return;
    this.artifactKey = key;
    const panel = this.open('artifact');
    panel.title = this.snapshot.artifacts.find(row => row.key === key)?.name ?? 'TADX artifact';
    this.sendState('artifact', panel, true);
  }

  showInventory(sourceKind?: PanelSourceKind, sourceId?: string): void {
    this.inventorySource = sourceKind ? { sourceKind, ...(sourceId !== undefined ? { sourceId } : {}) } : undefined;
    const panel = this.open('inventory');
    this.sendState('inventory', panel, true);
  }

  showOperation(operation: PanelOperation): void {
    this.operation = operation;
    const panel = this.open('operation');
    panel.title = operation.title;
    this.sendState('operation', panel, true);
  }

  dispose(): void {
    for (const panel of [...this.panels.values()]) panel.dispose();
    this.panels.clear();
  }

  private open(view: PanelView): VSCode.WebviewPanel {
    const existing = this.panels.get(view);
    if (existing) {
      existing.reveal(this.api.ViewColumn.Active);
      return existing;
    }
    const media = this.api.Uri.joinPath(this.extensionUri, 'media');
    const panel = this.api.window.createWebviewPanel(`tadx.${view}`, view === 'inventory' ? 'TADX Bulk Actions' : 'TADX artifact', this.api.ViewColumn.Active, {
      enableScripts: true,
      localResourceRoots: [media],
    });
    this.panels.set(view, panel);
    const receive = panel.webview.onDidReceiveMessage(async (message: unknown) => {
      if (message && typeof message === 'object' && (message as Record<string, unknown>).type === 'ready') {
        this.sendState(view, panel);
        return;
      }
      const request = validatePanelRequest(message, this.snapshot);
      if (!request) {
        void panel.webview.postMessage({ type: 'error', message: 'This action is unavailable. Refresh the inventory and select the artifacts again.' });
        return;
      }
      if (request.action === 'showArtifact') {
        this.showArtifact(request.key);
        return;
      }
      if (request.action === 'selectSource') {
        this.inventorySource = { sourceKind: request.sourceKind, sourceId: request.sourceId };
      }
      try {
        await this.onRequest(request);
      } catch (error) {
        void panel.webview.postMessage({ type: 'error', message: error instanceof Error ? error.message : 'The action failed. Check TADX output for details.' });
      }
    });
    panel.onDidDispose(() => {
      receive.dispose();
      this.panels.delete(view);
    });
    panel.webview.html = panelHtml({
      cspSource: panel.webview.cspSource,
      scriptUri: panel.webview.asWebviewUri(this.api.Uri.joinPath(media, 'panels.js')).toString(),
      styleUri: panel.webview.asWebviewUri(this.api.Uri.joinPath(media, 'panels.css')).toString(),
      nonce: randomBytes(24).toString('base64'),
      view,
    });
    return panel;
  }

  private sendState(view: PanelView, panel: VSCode.WebviewPanel, reveal = false): void {
    void panel.webview.postMessage({ type: 'state', snapshot: this.snapshot, artifactKey: this.artifactKey, inventorySource: this.inventorySource, operation: this.operation, reveal, view });
  }
}
