import type { JsonObject } from './model.js';

export const upgradeGuide = 'https://github.com/ahillspace/tadx/blob/main/docs/vscode-extension-release.md';

export interface CLIRequirement {
  readonly id: string;
  readonly command: string;
  readonly preview?: boolean;
  readonly batch?: boolean;
}

/** The extension's public command contract, independent of release numbering and site consent. */
export const cliRequirements: readonly CLIRequirement[] = [
  { id: 'env.profile.list', command: 'env list' },
  { id: 'workspace.list', command: 'workspace list' },
  { id: 'workspace.status', command: 'workspace status' },
  { id: 'cache.status', command: 'cache status' },
  { id: 'cache.refresh', command: 'cache refresh' },
  ...['project', 'workbook', 'datasource', 'flow'].map(kind => ({ id: `${kind}.list`, command: `content ${kind} list` })),
  ...['workbook', 'datasource', 'flow'].flatMap(kind => ['pull', 'publish'].map(verb => ({
    id: `${kind}.${verb}`, command: `content ${kind} ${verb}`, preview: true, batch: true,
  }))),
  { id: 'mutation.status', command: 'mutation status' },
  { id: 'job.inspect', command: 'job inspect' },
  { id: 'workspace.move', command: 'workspace artifact move', preview: true },
  { id: 'workspace.artifact.delete', command: 'workspace artifact delete', preview: true },
];

export interface CLICapability {
  readonly implemented: boolean;
  readonly preview: boolean;
  readonly batch: boolean;
}

export interface CLICompatibility {
  readonly version: string;
  readonly capabilities: ReadonlyMap<string, CLICapability>;
  readonly issues: readonly string[];
}

export class CompatibilityError extends Error {
  constructor(message: string) { super(message); this.name = 'CompatibilityError'; }
}

export function assessCompatibility(version: JsonObject, discovery: JsonObject): CLICompatibility {
  if (typeof version.version !== 'string' || !version.version.trim() || /[\r\n]/.test(version.version)) {
    throw new CompatibilityError('TADX did not report its installed version. Select a compatible CLI in TADX: Executable Path.');
  }
  if (!object(discovery.page) || discovery.page.more_available !== false || !Array.isArray(discovery.capabilities)) {
    throw new CompatibilityError('TADX did not return complete capability metadata. Select a compatible CLI, then run TADX: Reload Inventory.');
  }
  const capabilities = new Map<string, CLICapability>();
  for (const entry of discovery.capabilities) {
    if (!object(entry) || typeof entry.id !== 'string' || !/^[a-z][a-z0-9.-]*$/.test(entry.id)
      || capabilities.has(entry.id) || typeof entry.implementation_state !== 'string') {
      throw new CompatibilityError('TADX returned invalid capability metadata. Select a compatible CLI, then run TADX: Reload Inventory.');
    }
    capabilities.set(entry.id, {
      implemented: entry.implementation_state === 'implemented',
      preview: entry.supports_preview === true,
      batch: entry.supports_batch === true,
    });
  }
  const issues = cliRequirements.flatMap(requirement => {
    const capability = capabilities.get(requirement.id);
    if (!capability?.implemented) return [`${requirement.id} is unavailable`];
    return [
      ...(requirement.preview && !capability.preview ? [`${requirement.id} does not support previews`] : []),
      ...(requirement.batch && !capability.batch ? [`${requirement.id} does not support batches`] : []),
    ];
  });
  return { version: version.version, capabilities, issues };
}

export function requireCompatibleCommand(report: CLICompatibility, args: readonly string[]): void {
  const requirement = cliRequirements.find(item => item.command.split(' ').every((part, index) => args[index] === part));
  if (!requirement) throw new CompatibilityError('This command has no extension compatibility contract. Reload or update the extension.');
  const capability = report.capabilities.get(requirement.id);
  const batch = ['--id', '--artifact'].some(flag => args.filter(arg => arg === flag).length > 1);
  const reason = !capability?.implemented ? 'is unavailable'
    : requirement.preview && !capability.preview ? 'does not support the required preview'
    : requirement.batch && batch && !capability.batch ? 'does not support this batch' : undefined;
  if (reason) throw new CompatibilityError(`TADX ${report.version}: ${requirement.id} ${reason}. Select a compatible CLI or update your installation, then run TADX: Reload Inventory.`);
}

function object(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value); }
