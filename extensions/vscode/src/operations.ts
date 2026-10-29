import { randomUUID } from 'node:crypto';
import { TadxError, type TadxClient } from './cli.js';
import { isArtifactKind, type ArtifactKind, type DeleteRequest, type DownloadBatchRequest, type DownloadRequest, type MoveRequest, type OperationResult, type UploadBatchRequest, type UploadRequest } from './model.js';
import type { PanelOperation } from './panels.js';

export interface ActivityEntry {
  key: string;
  title: string;
  state: PanelOperation['state'];
  summary: string;
  startedAt: string;
  operationId?: string;
  result?: OperationResult;
}
export interface OperationUI {
  confirm(title: string, summary: string, preview: OperationResult): Promise<boolean>;
  display(operation: PanelOperation): void;
  changed(entries: readonly ActivityEntry[], busy: boolean): void;
}
type Client = Pick<TadxClient, 'previewDownload' | 'download' | 'previewUpload' | 'upload' | 'previewMove' | 'move' | 'previewDelete' | 'deleteArtifact' | 'inspectOperation'>;
type TransferRequest = DownloadRequest | UploadRequest;
interface TransferOutcome {
  kind: ArtifactKind;
  count: number;
  state: ActivityEntry['state'] | 'skipped';
  result?: OperationResult;
  message?: string;
}

/** Explicit operation methods share only preview, confirmation, and receipt handling. */
export class Operations {
  readonly entries: ActivityEntry[] = [];
  busy = false;
  private controller?: AbortController;
  private checking = new Set<string>();

  constructor(private readonly client: Client, private readonly ui: OperationUI) {}

  async download(request: DownloadBatchRequest): Promise<boolean> {
    const groups = downloadGroups(request);
    return this.transfer('Download', groups,
      `${request.environment} → ${request.workspace}\n${request.overwrite ? 'Replace existing local artifacts, including local changes.' : 'Preserve existing local changes.'}`);
  }
  async upload(request: UploadBatchRequest, projectPath: string): Promise<boolean> {
    const groups = uploadGroups(request);
    return this.transfer('Upload', groups,
      `${request.workspace} → ${request.environment} / ${projectPath}\nProject ID: ${request.projectId}\nMode: ${request.mode === 'overwrite' ? 'Overwrite matching remote content' : 'Create new content'}\nDependencies and credentials are not migrated.`);
  }
  async previewUpload(request: UploadBatchRequest, signal?: AbortSignal): Promise<OperationResult> {
    return this.previewTransfers(uploadGroups(request), signal);
  }

  private async previewTransfers(requests: readonly TransferRequest[], signal?: AbortSignal): Promise<OperationResult> {
    const groups: { kind: ArtifactKind; count: number; result: OperationResult }[] = [];
    for (const request of requests) {
      if (signal?.aborted) throw new TadxError('The transfer preview was canceled. No transfers were started.', 'aborted');
      const result = 'ids' in request ? await this.client.previewDownload(request, signal) : await this.client.previewUpload(request, signal);
      groups.push({ kind: request.kind, count: transferCount(request), result });
    }
    return { status: 'preview', total: groups.reduce((sum, group) => sum + group.count, 0), groups };
  }

  private async transfer(action: 'Download' | 'Upload', requests: readonly TransferRequest[], summary: string): Promise<boolean> {
    if (this.busy) throw new Error('Finish or cancel the current action before starting another.');
    const total = requests.reduce((sum, request) => sum + transferCount(request), 0);
    const title = requests.length === 1 ? transferTitle(action, requests[0]!) : `${action} ${total} artifacts`;
    const context = `${summary}\n${requests.map(request => `${transferCount(request)} ${request.kind}${transferCount(request) === 1 ? '' : 's'}`).join(', ')}`;
    this.busy = true;
    this.controller = new AbortController();
    const signal = this.controller.signal;
    this.notify();
    const outcomes: TransferOutcome[] = [];
    let executing = false;
    let attempted = 0;
    try {
      const preview = await this.previewTransfers(requests, signal);
      if (signal.aborted) return false;
      this.ui.display({ title: `${title} preview`, state: 'preview', summary: context, details: JSON.stringify(preview, null, 2) });
      if (!await this.confirm(title, context, preview, signal)) return false;
      executing = true;
      for (const request of requests) {
        if (signal.aborted) break;
        attempted++;
        try {
          const result = 'ids' in request ? await this.client.download(request, signal) : await this.client.upload(request, signal);
          const entry = this.record(transferTitle(action, request), context, result);
          outcomes.push({ kind: request.kind, count: transferCount(request), state: entry.state, result });
          if (!['pending', 'succeeded'].includes(entry.state)) break;
        } catch (error) {
          const result = error instanceof TadxError ? error.document : undefined;
          const state = failureState(error, true);
          const message = `${error instanceof Error ? error.message : 'The action failed.'}${state === 'unknown' ? '\nThe outcome is unknown. Check Activity before retrying.' : ''}`;
          const entry = this.record(transferTitle(action, request), `${context}\n${message}`, result, state);
          outcomes.push({ kind: request.kind, count: transferCount(request), state: entry.state, result, message });
          throw error;
        }
      }
      return !signal.aborted && attempted === requests.length && outcomes.every(outcome => ['pending', 'succeeded'].includes(outcome.state));
    } catch (error) {
      if (!executing) {
        if (signal.aborted) return false;
        this.record(`${title} preview`, `${context}\n${error instanceof Error ? error.message : 'The preview failed.'}\nNo transfers were started.`,
          error instanceof TadxError ? error.document : undefined, 'failed');
      }
      throw error;
    } finally {
      if (executing) {
        for (const request of requests.slice(attempted)) {
          outcomes.push({ kind: request.kind, count: transferCount(request), state: 'skipped', message: 'Not submitted because an earlier submission stopped or the action was canceled.' });
        }
        if (requests.length > 1) this.displayTransfers(title, context, outcomes);
      }
      this.busy = false;
      this.controller = undefined;
      this.notify();
    }
  }

  private displayTransfers(title: string, summary: string, outcomes: readonly TransferOutcome[]): void {
    const unresolved = outcomes.some(outcome => outcome.state === 'unknown');
    const adverse = outcomes.some(outcome => ['failed', 'partial', 'skipped'].includes(outcome.state));
    const progressed = outcomes.some(outcome => ['succeeded', 'pending', 'partial'].includes(outcome.state));
    const state = unresolved ? 'unknown' : adverse ? progressed ? 'partial' : 'failed' : outcomes.some(outcome => outcome.state === 'pending') ? 'pending' : 'succeeded';
    const skipped = outcomes.filter(outcome => outcome.state === 'skipped').map(outcome => `${outcome.kind}: not submitted (${outcome.count})`);
    this.ui.display({ title, state,
      summary: `${summary}\n${skipped.join('\n')}${skipped.length ? '\n' : ''}Accepted operations continue independently. Check each operation in Activity before retrying.`,
      items: outcomes.map(outcome => ({ name: `${outcome.count} ${outcome.kind}${outcome.count === 1 ? '' : 's'}`, state: outcome.state, message: outcome.message })),
      details: JSON.stringify({ status: state, groups: outcomes }, null, 2) });
  }

  private async confirm(title: string, summary: string, preview: OperationResult, signal: AbortSignal): Promise<boolean> {
    if (signal.aborted) return false;
    let onAbort!: () => void;
    const canceled = new Promise<boolean>(resolve => { onAbort = () => resolve(false); signal.addEventListener('abort', onAbort, { once: true }); });
    try { return await Promise.race([this.ui.confirm(title, summary, preview), canceled]) && !signal.aborted; }
    finally { signal.removeEventListener('abort', onAbort); }
  }
  move(request: MoveRequest): Promise<boolean> {
    request = { ...request };
    return this.perform('Move local artifact', `${request.artifact}\n${request.workspace} → ${request.destination}`,
      signal => this.client.previewMove(request, signal), signal => this.client.move(request, signal));
  }
  delete(request: DeleteRequest): Promise<boolean> {
    request = { ...request };
    return this.perform('Delete local artifact', `${request.workspace} / ${request.artifact}\nThis removes the managed artifact files.${request.force ? '\nLocal changes are also removed.' : ''}`,
      signal => this.client.previewDelete(request, signal), signal => this.client.deleteArtifact(request, signal));
  }

  private async perform(title: string, summary: string, preview: (signal: AbortSignal) => Promise<OperationResult>, run: (signal: AbortSignal) => Promise<OperationResult>): Promise<boolean> {
    if (this.busy) throw new Error('Finish or cancel the current action before starting another.');
    this.busy = true;
    this.controller = new AbortController();
    this.notify();
    let executing = false;
    try {
      const plan = await preview(this.controller.signal);
      if (this.controller.signal.aborted) return false;
      this.ui.display({ title: `${title} preview`, state: 'preview', summary, details: JSON.stringify(plan, null, 2) });
      const signal = this.controller.signal;
      if (!await this.confirm(title, summary, plan, signal)) return false;
      executing = true;
      const result = await run(this.controller.signal);
      this.record(title, summary, result);
      return true;
    } catch (error) {
      const result = error instanceof TadxError ? error.document : undefined;
      const state = failureState(error, executing);
      this.record(title, `${error instanceof Error ? error.message : 'The action failed.'}${state === 'unknown' ? '\nThe outcome is unknown. Check Activity before retrying.' : ''}`, result, state);
      throw error;
    } finally {
      this.busy = false;
      this.controller = undefined;
      this.notify();
    }
  }

  async inspect(key: string): Promise<void> {
    const entry = this.entries.find(item => item.key === key);
    if (!entry?.operationId || this.checking.has(entry.key)) return;
    this.checking.add(entry.key);
    try {
      const result = await this.client.inspectOperation(entry.operationId);
      entry.result = result;
      entry.state = operationState(result);
      entry.summary = resultSummary(result, entry.summary);
      this.display(entry);
    } catch (error) {
      // An inspection failure cannot erase the accepted operation identity or its last confirmed result.
      entry.summary = `Status check failed: ${error instanceof Error ? error.message : 'Unknown error'}. Last known state: ${entry.state}.`;
      this.display(entry);
      throw error;
    } finally { this.checking.delete(entry.key); this.notify(); }
  }

  restore(entries: unknown): void {
    if (!Array.isArray(entries)) return;
    for (const value of entries.slice(0, 100)) {
      if (!object(value) || typeof value.operationId !== 'string' || !value.operationId || value.operationId.length > 256 ||
          typeof value.title !== 'string' || typeof value.startedAt !== 'string') continue;
      this.entries.push({ key: randomUUID(), operationId: value.operationId, title: value.title.slice(0, 200), startedAt: value.startedAt,
        state: 'unknown', summary: 'Restored operation receipt. Check its current status.' });
    }
    this.notify();
  }
  receipts(): unknown[] {
    return this.entries.filter(entry => entry.operationId).slice(0, 100)
      .map(({ operationId, title, startedAt }) => ({ operationId, title, startedAt }));
  }
  cancel(): void { this.controller?.abort(); }
  dispose(): void { this.cancel(); }
  display(entry: ActivityEntry): void {
    this.ui.display({ title: entry.title, state: entry.state, summary: entry.summary,
      items: entry.result ? operationItems(entry.result) : undefined,
      details: entry.result ? JSON.stringify(entry.result, null, 2) : entry.operationId ? `Operation ID: ${entry.operationId}` : undefined });
  }
  private record(title: string, summary: string, result?: OperationResult, fallback?: ActivityEntry['state']): ActivityEntry {
    const operationId = findOperationId(result);
    const entry: ActivityEntry = { key: randomUUID(), title, summary: result ? resultSummary(result, summary) : summary,
      state: fallback ?? (result ? operationState(result) : 'failed'), startedAt: new Date().toISOString(), operationId, result };
    this.entries.unshift(entry);
    this.entries.splice(100);
    this.display(entry);
    this.notify();
    return entry;
  }
  private notify(): void { this.ui.changed(this.entries, this.busy); }
}

function failureState(error: unknown, executing: boolean): ActivityEntry['state'] {
  if (!executing || !(error instanceof TadxError)) return 'failed';
  if (error.details?.outcome === 'unknown') return 'unknown';
  if (error.details?.outcome === 'not_attempted' || error.details?.outcome === 'not_applied') return 'failed';
  if (['timeout', 'aborted', 'output_limit', 'contract'].includes(error.reason)) return 'unknown';
  if (error.document) {
    const state = operationState(error.document);
    if (state === 'partial') return 'partial';
    if (state === 'unknown' && operationStatus(error.document) !== '') return 'unknown';
    if (state === 'pending' && findOperationId(error.document)) return 'unknown';
  }
  return 'failed';
}

function downloadGroups(request: DownloadBatchRequest): DownloadRequest[] {
  const grouped = groupItems(request.items, item => item.id);
  return [...grouped].map(([kind, ids]) => ({ kind, environment: request.environment, workspace: request.workspace, ids,
    ...(request.overwrite !== undefined ? { overwrite: request.overwrite } : {}),
    ...(kind === 'workbook' && request.includeExtract !== undefined ? { includeExtract: request.includeExtract } : {}),
    ...(kind === 'workbook' && request.includePds !== undefined ? { includePds: request.includePds } : {}) }));
}

function uploadGroups(request: UploadBatchRequest): UploadRequest[] {
  const grouped = groupItems(request.items, item => item.path);
  return [...grouped].map(([kind, artifacts]) => ({ kind, environment: request.environment, workspace: request.workspace,
    artifacts, projectId: request.projectId, mode: request.mode }));
}

function groupItems<T extends { kind: ArtifactKind }>(items: readonly T[], selector: (item: T) => string): Map<ArtifactKind, string[]> {
  if (items.length < 1 || items.length > 100) throw new Error('Select 1 to 100 unique artifacts.');
  const groups = new Map<ArtifactKind, string[]>();
  const identities = new Set<string>();
  for (const item of items) {
    if (!isArtifactKind(item.kind)) throw new Error('Transfers support workbooks, datasources, and flows.');
    const value = selector(item);
    if (typeof value !== 'string' || !value.trim()) throw new Error('Every selected artifact needs an exact identity.');
    const identity = JSON.stringify([item.kind, value.trim()]);
    if (identities.has(identity)) throw new Error('Select 1 to 100 unique artifacts.');
    identities.add(identity);
    const group = groups.get(item.kind) ?? [];
    group.push(value);
    groups.set(item.kind, group);
  }
  return groups;
}

function transferCount(request: TransferRequest): number { return 'ids' in request ? request.ids.length : request.artifacts.length; }
function transferTitle(action: string, request: TransferRequest): string {
  const count = transferCount(request);
  return `${action} ${count} ${request.kind}${count === 1 ? '' : 's'}`;
}

export function findOperationId(value: unknown, depth = 0): string | undefined {
  if (!object(value) || depth > 4) return undefined;
  if (typeof value.operation_id === 'string' && value.operation_id.length > 0) return value.operation_id;
  for (const key of ['output', 'tracking', 'result', 'operation']) {
    const id = findOperationId(value[key], depth + 1);
    if (id) return id;
  }
  return undefined;
}
export function operationState(result: OperationResult): ActivityEntry['state'] {
  const raw = operationStatus(result);
  if (raw.includes('partial')) return 'partial';
  if (['interrupted', 'unknown', 'indeterminate'].includes(raw)) return 'unknown';
  if (['failed', 'error', 'canceled', 'cancelled'].includes(raw)) return 'failed';
  if (['queued', 'running', 'accepted', 'pending', 'started', 'starting', 'requested', 'remote_pending', 'in_progress', 'submitted'].includes(raw)) return 'pending';
  if (['completed', 'complete', 'succeeded', 'success', 'pulled', 'downloaded', 'published', 'moved', 'deleted', 'finished'].includes(raw)) return 'succeeded';
  if ('error' in result) return 'failed';
  return 'unknown';
}
function operationStatus(result: OperationResult): string {
  const source = object(result.output) ? result.output : result;
  const nested = object(source.plan) && object(source.result) ? source.result : undefined;
  return String(source.status ?? nested?.status ?? source.state ?? '').toLowerCase();
}
function resultSummary(result: OperationResult, fallback: string): string {
  const id = findOperationId(result);
  return `${fallback}${id && !fallback.includes(id) ? `\nOperation ID: ${id}\nUse Check Operation in Activity to read its current status.` : ''}`;
}
/** Project the public batch items in their original order; keep the complete receipt alongside them. */
export function operationItems(result: OperationResult): PanelOperation['items'] {
  const source = object(result.output) ? result.output : result;
  if (!Array.isArray(source.items)) return undefined;
  return source.items.slice(0, 100).map((item, index) => {
    if (!object(item)) return { name: `Item ${index + 1}`, state: 'unknown' };
    const details = object(item.result) ? item.result : {};
    const plan = object(details.plan) ? details.plan : {};
    const receipt = object(details.result) ? details.result : details;
    const text = (...values: unknown[]): string | undefined => values.find((value): value is string => typeof value === 'string' && value.length > 0);
    const failure = object(item.error) ? item.error : {};
    return { name: text(item.name, receipt.workbook_name, receipt.datasource_name, receipt.flow_name, plan.workbook_name, plan.datasource_name, plan.flow_name, item.selector, item.key) ?? `Item ${index + 1}`,
      state: text(item.status, receipt.status) ?? 'unknown', message: text(failure.summary, item.error, receipt.verification) };
  });
}
function object(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value); }
