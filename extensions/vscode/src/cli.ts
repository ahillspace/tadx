import { spawn } from 'node:child_process';
import { assessCompatibility, CompatibilityError, requireCompatibleCommand, type CLICompatibility } from './compatibility.js';
import {
  isArtifactKind, type ArtifactKind, type CacheStatus, type DeleteRequest, type DownloadRequest, type EnvironmentProfile,
  type JsonObject, type LocalArtifact, type OperationResult, type Project, type ReadSource,
  type MoveRequest, type RemoteArtifact, type SiteInventory, type UploadRequest, type Workspace, type WorkspaceInventory,
} from './model.js';

export interface CommandOptions {
  cwd?: string;
  signal?: AbortSignal;
  timeoutMs?: number;
  maxOutputBytes?: number;
}
export interface CommandResult { stdout: string; stderr: string; exitCode: number }
export type CommandRunner = (executable: string, args: readonly string[], options: CommandOptions) => Promise<CommandResult>;
export interface ClientOptions extends Omit<CommandOptions, 'signal'> { runner?: CommandRunner }
export type FailureReason = 'cli' | 'contract' | 'start' | 'aborted' | 'timeout' | 'output_limit' | 'busy';

export class TadxError extends Error {
  readonly details?: JsonObject;
  readonly output?: JsonObject;
  constructor(message: string, readonly reason: FailureReason, readonly document?: JsonObject,
    readonly exitCode?: number, readonly stderr?: string) {
    super(message);
    this.name = 'TadxError';
    if (isObject(document?.error)) this.details = document.error;
    if (isObject(document?.output)) this.output = document.output;
    else if (typeof document?.operation_id === 'string') this.output = document;
  }
}

/** Run argv directly. Never run a shell, a rendered help command, or webview-supplied command text. */
export const runCommand: CommandRunner = (executable, args, options) => new Promise((resolve, reject) => {
  const timeoutMs = options.timeoutMs ?? 300_000;
  const maxBytes = options.maxOutputBytes ?? 32 * 1024 * 1024;
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || !Number.isSafeInteger(maxBytes) || maxBytes <= 0) {
    reject(new TadxError('The CLI timeout and output limit must be positive integers.', 'contract'));
    return;
  }
  if (options.signal?.aborted) {
    reject(new TadxError('The TADX command was canceled before it started.', 'aborted'));
    return;
  }
  let child: ReturnType<typeof spawn>;
  try {
    child = spawn(executable, [...args], { cwd: options.cwd, shell: false, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  } catch {
    reject(new TadxError('TADX could not start. Check the configured executable.', 'start'));
    return;
  }
  let bytes = 0;
  const stdout: Buffer[] = [];
  const stderr: Buffer[] = [];
  let settled = false;
  const finish = (error?: TadxError, exitCode = 1): void => {
    if (settled) return;
    settled = true;
    clearTimeout(timer);
    options.signal?.removeEventListener('abort', abort);
    if (error) reject(error);
    else resolve({ stdout: Buffer.concat(stdout).toString('utf8'), stderr: Buffer.concat(stderr).toString('utf8'), exitCode });
  };
  const stop = (reason: FailureReason, message: string): void => {
    // Stopping the client process does not imply a remote operation was canceled.
    child.kill();
    let document: JsonObject | undefined;
    try {
      const value: unknown = JSON.parse(Buffer.concat(stdout).toString('utf8'));
      if (isObject(value)) document = value;
    } catch { /* Incomplete output is not a receipt. */ }
    finish(new TadxError(message, reason, document, undefined, Buffer.concat(stderr).toString('utf8')));
  };
  const abort = (): void => stop('aborted', 'The TADX command was canceled. Any accepted operation still needs its status checked.');
  const timer = setTimeout(() => stop('timeout', 'The TADX command timed out. Its operation outcome is unknown.'), timeoutMs);
  options.signal?.addEventListener('abort', abort, { once: true });
  // An abort can occur between the initial check and listener registration.
  if (options.signal?.aborted) abort();
  const collect = (destination: Buffer[], chunk: Buffer): void => {
    if (settled) return;
    const remaining = maxBytes - bytes;
    bytes += chunk.length;
    if (chunk.length <= remaining) destination.push(chunk);
    else {
      if (remaining > 0) destination.push(chunk.subarray(0, remaining));
      stop('output_limit', 'TADX output exceeded the configured limit. The result is incomplete.');
    }
  };
  child.stdout?.on('data', (chunk: Buffer) => collect(stdout, chunk));
  child.stderr?.on('data', (chunk: Buffer) => collect(stderr, chunk));
  child.once('error', () => finish(new TadxError('TADX could not start. Install TADX or select its executable in settings.', 'start')));
  child.once('close', (code) => finish(undefined, code ?? 1));
});

export class TadxClient {
  private readonly runner: CommandRunner;
  private compatibility?: Promise<CLICompatibility>;
  private report?: CLICompatibility;
  private running = 0;
  private maintenance = false;
  constructor(readonly executable: string, private readonly options: ClientOptions = {}) {
    requiredInput(executable, 'TADX executable');
    this.runner = options.runner ?? runCommand;
  }

  get compatibilityReport(): CLICompatibility | undefined { return this.report; }

  /** Excludes this client's subprocesses while an explicit installer changes its executable. */
  async withMaintenance<T>(task: () => Promise<T>): Promise<T> {
    if (this.maintenance || this.running > 0) throw new TadxError('Wait for TADX commands in this window to finish before changing the CLI installation.', 'busy');
    this.maintenance = true;
    try { return await task(); }
    finally { this.maintenance = false; }
  }

  /** Starts a new compatibility generation, including recovery after replacing the executable. */
  checkCompatibility(signal?: AbortSignal): Promise<CLICompatibility> {
    this.report = undefined;
    const pending = Promise.all([this.version(signal), this.capabilities(undefined, signal)])
      .then(([version, capabilities]) => assessCompatibility(version, capabilities))
      .catch((error: unknown) => {
        if (error instanceof CompatibilityError || error instanceof TadxError && error.reason === 'start') throw error;
        throw new CompatibilityError(`Could not verify TADX compatibility: ${error instanceof Error ? error.message : 'Capability discovery failed.'} Select a compatible CLI, then run TADX: Reload Inventory.`);
      });
    this.compatibility = pending;
    void pending.then(report => { if (this.compatibility === pending) this.report = report; }, () => undefined);
    return pending;
  }

  private async run(args: readonly string[], signal?: AbortSignal): Promise<OperationResult> {
    if (this.compatibility && args[0] !== 'version' && args[0] !== 'capability') {
      requireCompatibleCommand(await this.compatibility, args);
    }
    return this.execute(args, signal);
  }

  private async execute(args: readonly string[], signal?: AbortSignal): Promise<OperationResult> {
    if (this.maintenance) throw new TadxError('TADX CLI setup is in progress. Retry this command after setup finishes.', 'busy');
    this.running++;
    let result: CommandResult;
    try { result = await this.runner(this.executable, [...args, '--full', '--json'], { ...this.options, signal }); }
    finally { this.running--; }
    let document: unknown;
    try { document = JSON.parse(result.stdout); }
    catch { throw new TadxError('TADX did not return a valid JSON document. Check the installed CLI version.', 'contract', undefined, result.exitCode, result.stderr); }
    if (!isObject(document)) throw new TadxError('TADX returned an unexpected JSON document.', 'contract', undefined, result.exitCode, result.stderr);
    const inspectedFailure = args[0] === 'job' && args[1] === 'inspect' && args[2] === '--operation-id'
      && document.operation_id === args[3] && typeof document.operation === 'string' && document.operation.length > 0
      && typeof document.status === 'string' && ['failed', 'partial', 'unknown'].includes(document.status)
      && isObject(document.error);
    if (result.exitCode !== 0 || 'error' in document && !inspectedFailure) {
      const summary = isObject(document.error) && typeof document.error.summary === 'string'
        ? document.error.summary : 'The TADX command failed.';
      throw new TadxError(summary, 'cli', document, result.exitCode, result.stderr);
    }
    return document;
  }

  version(signal?: AbortSignal): Promise<OperationResult> { return this.run(['version'], signal); }
  capabilities(environment?: string, signal?: AbortSignal): Promise<OperationResult> {
    return this.run(['capability', 'list', '--all', ...(environment ? ['--environment', environment] : [])], signal);
  }
  mutationStatus(environment: string, signal?: AbortSignal): Promise<OperationResult> {
    return this.run(['mutation', 'status', '--environment', requiredInput(environment, 'environment')], signal);
  }
  async environments(signal?: AbortSignal): Promise<EnvironmentProfile[]> {
    const result = await this.run(['env', 'list', '--all'], signal);
    requireCompletePage(result, 'environment');
    return rows(result, 'environments').map(row => ({ alias: requiredString(row, 'alias'), default: requiredBoolean(row, 'default'),
      ...stringFields(row, { server_url: 'serverUrl', site_content_url: 'siteContentUrl', default_workspace: 'defaultWorkspace', status: 'status' }),
      ...optionalStrings(row, 'violations') }));
  }
  async workspaces(signal?: AbortSignal): Promise<Workspace[]> {
    const result = await this.run(['workspace', 'list', '--all'], signal);
    requireCompletePage(result, 'workspace');
    return rows(result, 'workspaces').map(row => normalizeWorkspace(row, true));
  }
  async workspaceInventory(workspace: string, signal?: AbortSignal): Promise<WorkspaceInventory> {
    const result = await this.run(['workspace', 'status', '--workspace', requiredInput(workspace, 'workspace'), '--all'], signal);
    const inventory = objectField(result, 'artifacts');
    const warnings = stringArray(result.warnings, 'warnings');
    const artifacts: LocalArtifact[] = [];
    for (const row of rows(inventory, 'artifacts')) {
      artifacts.push({ id: requiredString(row, 'luid', true), name: requiredString(row, 'name', true), kind: requiredString(row, 'kind'),
        state: requiredString(row, 'state'), ...stringFields(row, { path: 'path', canonical_path: 'canonicalPath', reason: 'reason', diagnostic: 'diagnostic' }),
        ...localSource(row) });
    }
    return { workspace: normalizeWorkspace(objectField(result, 'workspace'), false), artifacts,
      scanComplete: requiredBoolean(inventory, 'scan_complete'), moreAvailable: requiredBoolean(inventory, 'more_available'), warnings };
  }
  async siteInventory(environment: string, signal?: AbortSignal): Promise<SiteInventory> {
    requiredInput(environment, 'environment');
    const kinds = ['project', 'workbook', 'datasource', 'flow'] as const;
    const [status, ...results] = await Promise.allSettled([
      this.run(['cache', 'status', '--environment', environment], signal),
      ...kinds.map(kind => this.run(['content', kind, 'list', '--environment', environment, '--cache', '--all'], signal)),
    ]);
    if (!status || status.status === 'rejected') throw status?.status === 'rejected' ? status.reason : contract('Missing cache status.');
    const cache = normalizeCache(status.value);
    // Cache status covers scopes this view does not read, including permissions.
    // Display diagnostics from the requested inventories; retain global evidence on cache.
    const warnings: string[] = [];
    const projects: Project[] = [];
    const artifacts: RemoteArtifact[] = [];
    let complete = cache.status !== 'uninitialized';
    let site: string | undefined;
    for (const [index, result] of results.entries()) {
      const kind = kinds[index]!;
      if (result.status === 'rejected') {
        if (signal?.aborted) throw result.reason;
        complete = false;
        warnings.push(`${kind}: ${result.reason instanceof Error ? result.reason.message : 'Inventory could not be read.'}`);
        continue;
      }
      const document = result.value;
      const source = normalizeReadSource(document.source);
      const page = objectField(document, 'page');
      if (requiredBoolean(page, 'more_available') || source?.coverage !== 'complete') {
        complete = false;
        warnings.push(`${kind} inventory is incomplete. Refresh the cache to check its coverage.`);
      }
      if (source?.stale) warnings.push(`${kind} inventory is stale.`);
      if (source?.cacheWarning) warnings.push(`${kind}: ${source.cacheWarning}`);
      if (typeof document.site === 'string') site = document.site;
      warnings.push(...stringArray(document.warnings, 'warnings'));
      if (kind === 'project') {
        projects.push(...rows(document, 'projects').map(row => ({ id: requiredString(row, 'luid'), name: requiredString(row, 'name'),
          parentId: requiredString(row, 'parent_luid', true), ...stringFields(row, { description: 'description' }) })));
      } else {
        artifacts.push(...rows(document, `${kind}s`).map(row => ({ id: requiredString(row, 'luid'), name: requiredString(row, 'name'), kind,
          projectId: requiredString(row, 'project_luid', true),
          ...stringFields(row, { project_path: 'projectPath', updated_at: 'updatedAt', content_url: 'contentUrl', webpage_url: 'webpageUrl', description: 'description' }),
          ...(source ? { source } : {}) })));
      }
    }
    return { environment, ...(site !== undefined ? { site } : {}), cache, projects, artifacts, complete, warnings: [...new Set(warnings)] };
  }
  refreshCache(environment: string, signal?: AbortSignal): Promise<OperationResult> {
    return this.run(['cache', 'refresh', '--environment', requiredInput(environment, 'environment'), '--scope', 'projects,workbooks,datasources,flows'], signal);
  }
  previewDownload(request: DownloadRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildDownloadArgs(request, true), signal); }
  download(request: DownloadRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildDownloadArgs(request, false), signal); }
  previewUpload(request: UploadRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildUploadArgs(request, true), signal); }
  upload(request: UploadRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildUploadArgs(request, false), signal); }
  previewMove(request: MoveRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildMoveArgs(request, true), signal); }
  move(request: MoveRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildMoveArgs(request, false), signal); }
  previewDelete(request: DeleteRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildDeleteArgs(request, true), signal); }
  deleteArtifact(request: DeleteRequest, signal?: AbortSignal): Promise<OperationResult> { return this.run(buildDeleteArgs(request, false), signal); }
  inspectOperation(operationId: string, signal?: AbortSignal): Promise<OperationResult> {
    return this.run(['job', 'inspect', '--operation-id', requiredInput(operationId, 'operation ID')], signal);
  }
}

export function buildDownloadArgs(request: DownloadRequest, preview: boolean): string[] {
  const args = selectionArgs(request.kind, 'pull', request.environment, request.workspace);
  for (const id of batch(request.ids, 'content IDs')) args.push('--id', requiredInput(id, 'content ID'));
  if (request.overwrite) args.push('--overwrite');
  if (request.includeExtract !== undefined) {
    if (request.kind !== 'workbook') throw contract('The extract download option applies only to workbooks.');
    args.push(`--include-extract=${request.includeExtract}`);
  }
  if (request.includePds) {
    if (request.kind !== 'workbook') throw contract('Published datasource dependencies apply only to workbooks.');
    args.push('--include-pds');
  }
  args.push(preview ? '--preview' : '--no-wait');
  return args;
}

export function buildUploadArgs(request: UploadRequest, preview: boolean): string[] {
  const args = selectionArgs(request.kind, 'publish', request.environment, request.workspace);
  args.push('--project-id', requiredInput(request.projectId, 'destination project ID'));
  for (const path of batch(request.artifacts, 'artifact paths')) {
    args.push('--artifact', artifactPath(path));
  }
  if (request.mode === 'overwrite') args.push('--overwrite');
  else if (request.mode !== 'create') throw contract('Upload mode must be create or overwrite.');
  else if (request.kind === 'datasource') args.push('--create');
  args.push(preview ? '--preview' : '--no-wait');
  return args;
}

export function buildMoveArgs(request: MoveRequest, preview: boolean): string[] {
  const args = ['workspace', 'artifact', 'move', '--source', requiredInput(request.workspace, 'source workspace'),
    '--destination', requiredInput(request.destination, 'destination workspace'), '--artifact', artifactPath(request.artifact)];
  if (preview) args.push('--preview');
  return args;
}

export function buildDeleteArgs(request: DeleteRequest, preview: boolean): string[] {
  const args = ['workspace', 'artifact', 'delete', '--workspace', requiredInput(request.workspace, 'workspace'),
    '--artifact', artifactPath(request.artifact)];
  if (request.force) args.push('--force');
  if (preview) args.push('--preview');
  return args;
}

function artifactPath(path: string): string {
  requiredInput(path, 'artifact path');
  if (path.startsWith('/') || /[\\:]/u.test(path) || path.split('/').some(part => part === '..' || part === '.' || part === '')) {
    throw contract('Artifact paths must be relative to their workspace and use forward slashes.');
  }
  return path;
}

function selectionArgs(kind: ArtifactKind, verb: string, environment: string, workspace: string): string[] {
  if (!isArtifactKind(kind)) throw contract('Unsupported artifact kind.');
  return ['content', kind, verb, '--environment', requiredInput(environment, 'environment'), '--workspace', requiredInput(workspace, 'workspace')];
}
function batch(values: readonly string[], name: string): readonly string[] {
  if (values.length < 1 || values.length > 100 || new Set(values).size !== values.length) throw contract(`Select 1 to 100 unique ${name}.`);
  return values;
}
function requiredInput(value: string, name: string): string {
  if (typeof value !== 'string' || !value.trim() || Array.from(value).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) {
    throw contract(`A valid ${name} is required.`);
  }
  return value;
}
function isObject(value: unknown): value is JsonObject { return value !== null && typeof value === 'object' && !Array.isArray(value); }
function contract(message: string): TadxError { return new TadxError(message, 'contract'); }
function objectField(row: JsonObject, key: string): JsonObject {
  if (!isObject(row[key])) throw contract(`TADX output is missing the ${key} object.`);
  return row[key];
}
function requireCompletePage(document: JsonObject, label: string): void {
  if (requiredBoolean(objectField(document, 'page'), 'more_available')) {
    throw contract(`The ${label} list exceeds the CLI result limit. The extension cannot display a complete list.`);
  }
}
function rows(row: JsonObject, key: string): JsonObject[] {
  const value = row[key];
  if (!Array.isArray(value) || !value.every(isObject)) throw contract(`TADX output has an invalid ${key} list.`);
  return value;
}
function requiredString(row: JsonObject, key: string, allowEmpty = false): string {
  const value = row[key];
  if (typeof value !== 'string' || (!allowEmpty && !value)) throw contract(`TADX output has an invalid ${key} field.`);
  return value;
}
function requiredBoolean(row: JsonObject, key: string): boolean {
  if (typeof row[key] !== 'boolean') throw contract(`TADX output has an invalid ${key} field.`);
  return row[key];
}
function stringFields(row: JsonObject, fields: Record<string, string>): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [source, target] of Object.entries(fields)) {
    if (row[source] !== undefined) result[target] = requiredString(row, source, true);
  }
  return result;
}
function stringArray(value: unknown, field: string): string[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || !value.every(item => typeof item === 'string')) throw contract(`TADX output has an invalid ${field} list.`);
  return [...value] as string[];
}
function optionalStrings(row: JsonObject, field: string): { violations?: string[] } {
  return row[field] === undefined ? {} : { violations: stringArray(row[field], field) };
}
function normalizeWorkspace(row: JsonObject, listed: boolean): Workspace {
  return { name: requiredString(row, 'name'), ...stringFields(row, { id: 'id', root: 'root', status: 'status' }),
    ...optionalStrings(row, 'violations'), default: listed ? requiredBoolean(row, 'default') : false,
    available: listed ? requiredBoolean(row, 'available') : true, manifestValid: listed ? requiredBoolean(row, 'manifest_valid') : true };
}
function localSource(row: JsonObject): Pick<LocalArtifact, 'source'> {
  const source = stringFields(row, { source_server_origin: 'serverOrigin', source_site_luid: 'siteLuid', source_environment: 'environment', source_site: 'site' });
  return Object.keys(source).length > 0 ? { source } : {};
}
function normalizeReadSource(value: unknown): ReadSource | undefined {
  if (value === undefined) return undefined;
  if (!isObject(value)) throw contract('TADX returned invalid read source metadata.');
  return { mode: requiredString(value, 'mode'), coverage: requiredString(value, 'coverage'), stale: requiredBoolean(value, 'stale'),
    ...stringFields(value, { observed_at: 'observedAt', generation_id: 'generationId', coverage_reason: 'coverageReason', cache_warning: 'cacheWarning' }) };
}
function normalizeCache(document: JsonObject): CacheStatus {
  const status = requiredString(document, 'status');
  const warnings = stringArray(document.warnings, 'warnings');
  if (status === 'uninitialized') return { status, complete: false, stale: false, coverage: [], warnings };
  const generation = objectField(document, 'generation');
  return { status, complete: requiredBoolean(generation, 'complete'), stale: requiredBoolean(generation, 'stale'),
    ...stringFields(generation, { generated_at: 'generatedAt' }), warnings,
    coverage: generation.coverage === undefined ? [] : rows(generation, 'coverage').map(row => {
      if (typeof row.records !== 'number' || !Number.isSafeInteger(row.records) || row.records < 0) throw contract('TADX returned an invalid cache record count.');
      return { scope: requiredString(row, 'scope'), requested: requiredBoolean(row, 'requested'), complete: requiredBoolean(row, 'complete'), records: row.records };
    }) };
}
