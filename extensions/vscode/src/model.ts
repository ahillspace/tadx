/** The extension consumes public CLI output, never the cache database or private config. */
export type ArtifactKind = 'workbook' | 'datasource' | 'flow';
export type JsonObject = Record<string, unknown>;

export interface EnvironmentProfile {
  alias: string;
  default: boolean;
  serverUrl?: string;
  siteContentUrl?: string;
  defaultWorkspace?: string;
  status?: string;
  violations?: string[];
}

export interface Workspace {
  name: string;
  id?: string;
  root?: string;
  default: boolean;
  available: boolean;
  manifestValid: boolean;
  status?: string;
  violations?: string[];
}

export interface ReadSource {
  mode: string;
  coverage: string;
  stale: boolean;
  observedAt?: string;
  generationId?: string;
  coverageReason?: string;
  cacheWarning?: string;
}

export interface Project {
  id: string;
  name: string;
  parentId: string;
  description?: string;
}

export interface RemoteArtifact {
  id: string;
  kind: ArtifactKind;
  name: string;
  projectId: string;
  projectPath?: string;
  updatedAt?: string;
  contentUrl?: string;
  webpageUrl?: string;
  description?: string;
  source?: ReadSource;
}

export interface LocalArtifact {
  id: string;
  kind: string;
  name: string;
  path?: string;
  canonicalPath?: string;
  state: string;
  reason?: string;
  diagnostic?: string;
  source?: { serverOrigin?: string; siteLuid?: string; environment?: string; site?: string };
}

export interface CacheCoverage {
  scope: string;
  requested: boolean;
  complete: boolean;
  records: number;
}

export interface CacheStatus {
  status: string;
  generatedAt?: string;
  complete: boolean;
  stale: boolean;
  coverage: CacheCoverage[];
  warnings: string[];
}

export interface SiteInventory {
  environment: string;
  site?: string;
  cache: CacheStatus;
  projects: Project[];
  artifacts: RemoteArtifact[];
  complete: boolean;
  warnings: string[];
}

export interface WorkspaceInventory {
  workspace: Workspace;
  artifacts: LocalArtifact[];
  scanComplete: boolean;
  moreAvailable: boolean;
  warnings: string[];
}

export interface DownloadRequest {
  kind: ArtifactKind;
  environment: string;
  workspace: string;
  ids: readonly string[];
  overwrite?: boolean;
  includeExtract?: boolean;
  includePds?: boolean;
}

export interface UploadRequest {
  kind: ArtifactKind;
  environment: string;
  workspace: string;
  artifacts: readonly string[];
  projectId: string;
  mode: 'create' | 'overwrite';
}

/** One user selection has one source and destination, with resource-specific CLI groups. */
export interface DownloadBatchRequest {
  environment: string;
  workspace: string;
  items: readonly { kind: ArtifactKind; id: string }[];
  overwrite?: boolean;
  includeExtract?: boolean;
  includePds?: boolean;
}

export interface UploadBatchRequest {
  environment: string;
  workspace: string;
  projectId: string;
  mode: 'create' | 'overwrite';
  items: readonly { kind: ArtifactKind; path: string }[];
}

export interface MoveRequest {
  workspace: string;
  destination: string;
  artifact: string;
}

export interface DeleteRequest {
  workspace: string;
  artifact: string;
  force?: boolean;
}

/** Preserve operation-specific receipts and confirmed partial outcomes verbatim. */
export type OperationResult = JsonObject;

export function isArtifactKind(value: unknown): value is ArtifactKind {
  return value === 'workbook' || value === 'datasource' || value === 'flow';
}

/** These keys identify UI selections, not shared remote/local provenance. */
export function remoteArtifactKey(environment: string, artifact: Pick<RemoteArtifact, 'kind' | 'id'>): string {
  return JSON.stringify(['remote', environment, artifact.kind, artifact.id]);
}

export function localArtifactKey(workspace: string, artifact: Pick<LocalArtifact, 'kind' | 'id' | 'path'>): string {
  return JSON.stringify(['local', workspace, artifact.kind, artifact.path ?? artifact.id]);
}
