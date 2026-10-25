import * as vscode from 'vscode';
import { TadxClient, TadxError } from './cli.js';
import { CompatibilityError, upgradeGuide, type CLICompatibility } from './compatibility.js';
import { Inventory, message, projectPaths, type ArtifactSelection } from './inventory.js';
import { isArtifactKind, type DownloadBatchRequest, type UploadBatchRequest, type EnvironmentProfile, type Workspace } from './model.js';
import { Operations } from './operations.js';
import { PanelManager, type PanelRequest, type PanelSnapshot } from './panels.js';
import { tableauUrl, workspaceFile } from './paths.js';
import { ActivityTree, InventoryTree } from './trees.js';
import { SetupService, type VerifiedCliRelease } from './setup.js';
import cliRelease from '../cli-release.json';

const back = Symbol('back');
interface Choice<T> extends vscode.QuickPickItem { value: T }

/** Coordinates native inputs with host-owned identities. TADX owns every filesystem mutation. */
export class Controller implements vscode.Disposable {
  readonly inventory: Inventory;
  readonly operations: Operations;
  readonly panels: PanelManager;
  readonly ready: Promise<void>;
  private readonly client: TadxClient;
  private readonly setup: SetupService;
  private readonly output = vscode.window.createOutputChannel('TADX');
  private readonly resources: vscode.Disposable[] = [];
  private readonly sites: InventoryTree;
  private readonly workspaces: InventoryTree;
  private readonly activity = new ActivityTree();
  private choosing = false;
  private disposed = false;

  constructor(private readonly context: vscode.ExtensionContext) {
    const configuration = vscode.workspace.getConfiguration('tadx');
    const executable = configuration.get<string>('executablePath', 'tadx');
    const timeout = configuration.get<number>('commandTimeoutSeconds', 120);
    this.client = new TadxClient(executable, { timeoutMs: Math.round(Math.min(1200, Math.max(5, timeout)) * 1000) });
    this.setup = new SetupService({ extensionPath: context.extensionUri.fsPath });
    this.inventory = new Inventory(this.client, context.workspaceState.get('selection'));
    this.panels = new PanelManager(vscode, context.extensionUri, request => this.run(() => this.handle(request)));
    this.sites = new InventoryTree(this.inventory, 'sites');
    this.workspaces = new InventoryTree(this.inventory, 'workspaces');
    this.operations = new Operations(this.client, {
      confirm: async title => {
        const result = await vscode.window.showWarningMessage(`${title}: review the preview in the editor, then select Run.`, 'Run', 'Cancel');
        return result === 'Run' && !this.disposed;
      },
      display: operation => this.panels.showOperation(operation),
      changed: entries => {
        this.activity.entries = entries;
        this.activity.refresh();
        this.update();
        void context.workspaceState.update('operations', this.operations.receipts());
      },
    });
    this.resources.push(this.output, this.inventory, this.panels, this.operations, this.sites, this.workspaces, this.activity);
    this.resources.push(
      vscode.window.createTreeView('tadx.sites', { treeDataProvider: this.sites, showCollapseAll: true }),
      vscode.window.createTreeView('tadx.workspaces', { treeDataProvider: this.workspaces, showCollapseAll: true }),
      vscode.window.createTreeView('tadx.activity', { treeDataProvider: this.activity }),
      this.inventory.onChange(() => this.update()),
      vscode.workspace.onDidChangeConfiguration(event => {
        if (event.affectsConfiguration('tadx')) void vscode.window.showInformationMessage('Reload the VS Code window to apply the new TADX executable settings.', 'Reload Window')
          .then(action => action === 'Reload Window' ? vscode.commands.executeCommand('workbench.action.reloadWindow') : undefined);
      }),
    );
    this.registerCommands();
    this.operations.restore(context.workspaceState.get('operations'));
    this.ready = this.refreshCompatibility();
  }

  getSnapshot(): PanelSnapshot { return this.inventory.snapshot(this.operations.busy || this.choosing); }
  getCompatibility(): CLICompatibility | undefined { return this.client.compatibilityReport; }

  async refreshCompatibility(): Promise<void> {
    if (!vscode.workspace.isTrusted || this.disposed) return;
    try {
      const report = await this.client.checkCompatibility();
      this.output.appendLine(`TADX ${report.version}: ${report.issues.length ? report.issues.join('; ') : 'extension command capabilities available'}.`);
      if (report.issues.length) {
        void vscode.window.showWarningMessage(`TADX ${report.version} has limited extension support: ${report.issues.join('; ')}. Supported commands remain available.`, 'TADX CLI Setup', 'Upgrade Guide')
          .then(action => action === 'TADX CLI Setup' ? vscode.commands.executeCommand('tadx.cliStatus')
            : action === 'Upgrade Guide' ? vscode.env.openExternal(vscode.Uri.parse(upgradeGuide)) : undefined);
      }
    } catch (error) {
      this.output.appendLine(message(error));
      if (error instanceof TadxError && error.reason === 'start') {
        void vscode.window.showErrorMessage('TADX could not start on this workspace host. Install it or choose an existing executable.', 'Install TADX', 'Choose Executable')
          .then(action => action === 'Install TADX' ? vscode.commands.executeCommand('tadx.installCli')
            : action === 'Choose Executable' ? vscode.commands.executeCommand('tadx.chooseExecutable') : undefined);
      }
    }
    // Each inventory read retains its own error, including failed capability discovery.
    await this.inventory.refresh();
  }
  dispose(): void { this.disposed = true; for (const resource of this.resources.reverse()) resource.dispose(); }
  private update(): void {
    if (this.disposed) return;
    this.panels.update(this.getSnapshot());
    this.sites.refresh();
    this.workspaces.refresh();
    void this.context.workspaceState.update('selection', { site: this.inventory.selectedSite, workspace: this.inventory.selectedWorkspace });
  }
  private registerCommands(): void {
    const register = (name: string, handler: (arg?: unknown) => unknown | Promise<unknown>): void => {
      this.resources.push(vscode.commands.registerCommand(`tadx.${name}`, (arg?: unknown) => this.run(async () => { await handler(arg); })));
    };
    register('refresh', () => this.refreshCompatibility());
    register('cliStatus', () => this.exclusive(() => this.cliStatus()));
    register('installCli', () => this.exclusive(() => this.setupCli('install')));
    register('updateCli', () => this.exclusive(() => this.setupCli('update')));
    register('chooseExecutable', () => this.exclusive(() => this.chooseExecutable()));
    register('chooseWorkspace', () => this.chooseWorkspace());
    register('refreshCache', arg => this.refreshCache(sourceArgument(arg, 'site')));
    register('filter', async () => {
      const filter = await vscode.window.showInputBox({ title: 'Filter TADX artifacts', prompt: 'Match an artifact name, path, or ID. Leave empty to clear.', value: this.inventory.filter });
      if (filter !== undefined) { this.inventory.filter = filter; this.update(); }
    });
    register('openInventory', arg => this.openBulkActions('sites', sourceArgument(arg, 'site')));
    register('openWorkspaceBulkActions', arg => this.openBulkActions('workspaces', sourceArgument(arg, 'workspace')));
    register('openArtifact', async arg => { this.panels.showArtifact(await this.resolveKey(arg)); });
    for (const action of ['download', 'upload', 'openRemote', 'openPayload', 'openMetadata', 'reveal', 'move', 'delete'] as const) {
      register(action, async arg => {
        const key = await this.resolveKey(arg, action === 'download' ? 'remote' : action === 'openRemote' ? 'remote' : 'local');
        if (action === 'download' || action === 'upload') await this.handle({ action, keys: [key] });
        else await this.handle({ action, key });
      });
    }
    register('inspectOperation', async arg => {
      let key = keyArgument(arg);
      if (!key) key = (await vscode.window.showQuickPick(this.operations.entries.map(entry => ({ label: entry.title, description: entry.state, key: entry.key })), { title: 'Check TADX operation' }))?.key;
      const entry = this.operations.entries.find(item => item.key === key);
      if (!entry) return;
      if (entry.operationId) {
        await this.withProgress('Checking TADX operation', () => this.operations.inspect(entry.key));
        if (entry.state === 'succeeded' || entry.state === 'partial') await this.inventory.refresh();
      } else this.operations.display(entry);
    });
    register('showOutput', () => this.output.show());
  }

  private verifiedRelease(): VerifiedCliRelease {
    const key = `${process.platform}-${process.arch}`;
    const archiveSha256 = (cliRelease.archives as Record<string, string>)[key];
    if (!archiveSha256) throw new Error(`Guided CLI installation is unavailable for ${key}. Choose an existing executable on this host.`);
    return { version: cliRelease.version, archiveSha256 };
  }

  private async cliStatus(): Promise<void> {
    let detail: string;
    try {
      const installed = await this.setup.inspect(this.client.executable);
      const report = this.getCompatibility();
      detail = `TADX ${installed.version} at ${installed.executablePath}. ${installed.managed ? 'Shared user installation.' : 'Custom installation; updates remain with its owner.'}`;
      if (report?.issues.length) detail += ` Limited support: ${report.issues.join('; ')}.`;
    } catch (error) { detail = message(error); }
    const host = vscode.env.remoteName ? `Remote workspace host (${vscode.env.remoteName})` : 'Local workspace host';
    const choice = await vscode.window.showQuickPick([
      { label: 'Choose Existing Executable', id: 'choose', detail },
      { label: 'Install TADX', id: 'install', detail: `Install verified CLI ${cliRelease.version} and bundled agent Guidance in the shared user location.` },
      { label: 'Update TADX', id: 'update', detail: `Update or repair the shared user installation to verified CLI ${cliRelease.version}. Newer compatible versions remain usable.` },
      { label: 'Upgrade Guide', id: 'guide', detail: 'Extension updates, custom CLI installations, and release compatibility.' },
      { label: 'Show Output', id: 'output', detail: 'View CLI compatibility and operation diagnostics.' },
    ], { title: `TADX CLI - ${host}`, placeHolder: detail, matchOnDetail: true });
    if (choice?.id === 'choose') await this.chooseExecutable();
    else if (choice?.id === 'install' || choice?.id === 'update') await this.setupCli(choice.id);
    else if (choice?.id === 'guide') await vscode.env.openExternal(vscode.Uri.parse(upgradeGuide));
    else if (choice?.id === 'output') this.output.show();
  }

  private async chooseExecutable(): Promise<void> {
    const picked = await vscode.window.showOpenDialog({ title: 'Choose TADX on this workspace host', openLabel: 'Use TADX',
      canSelectFiles: true, canSelectFolders: false, canSelectMany: false });
    if (!picked?.[0]) return;
    const installation = await this.setup.inspect(picked[0].fsPath);
    const report = await new TadxClient(installation.executablePath).checkCompatibility();
    if (report.issues.length) {
      const choice = await vscode.window.showWarningMessage(`TADX ${report.version} has limited support: ${report.issues.join('; ')}.`, 'Use Supported Features', 'Cancel');
      if (choice !== 'Use Supported Features') return;
    }
    await vscode.workspace.getConfiguration('tadx').update('executablePath', installation.executablePath, vscode.ConfigurationTarget.Global);
  }

  private async setupCli(action: 'install' | 'update'): Promise<void> {
    const release = this.verifiedRelease();
    const plan = action === 'install' ? await this.setup.prepareInstall(release)
      : await this.setup.prepareUpdate(this.client.executable, release);
    const choices = await vscode.window.showQuickPick([
      { label: 'Add the CLI directory to PATH', id: 'path', detail: 'Update the user PATH or shell profile so new terminals can find TADX.' },
      { label: 'Enable shell completion', id: 'completion', detail: 'Update the shell profile with TADX completion.' },
    ], { title: 'Optional terminal integration', canPickMany: true, placeHolder: 'Leave both unchecked to preserve PATH and shell profiles.' });
    if (!choices) return;
    const stopped = await vscode.window.showWarningMessage('Finish all TADX commands and background transfers on this host before changing its CLI, including work started outside this window.',
      { modal: true }, 'All TADX Work Has Finished');
    if (stopped !== 'All TADX Work Has Finished' || this.disposed) return;
    const verb = action === 'install' ? 'Install' : plan.currentVersion?.replace(/^v/, '') === release.version ? 'Repair' : 'Update';
    const approved = await vscode.window.showWarningMessage(`${verb} TADX ${release.version} at ${plan.executablePath}?`,
      { modal: true, detail: `This installs the verified CLI archive and refreshes bundled agent Guidance for detected supported tools on this workspace host.${plan.currentVersion ? ` Current version: ${plan.currentVersion}.` : ''}\nPATH changes: ${choices.some(item => item.id === 'path') ? 'enabled' : 'disabled'}. Shell completion changes: ${choices.some(item => item.id === 'completion') ? 'enabled' : 'disabled'}.` },
      `${verb} CLI and Guidance`);
    if (approved !== `${verb} CLI and Guidance` || this.disposed) return;
    const installed = await this.client.withMaintenance(() => this.withProgress(`${verb} TADX ${release.version}`, signal => this.setup.execute(plan, {
      guidanceConsent: true, otherTransfersStopped: true, modifyPath: choices.some(item => item.id === 'path'), completion: choices.some(item => item.id === 'completion'),
    }, signal)));
    const report = await new TadxClient(installed.executablePath).checkCompatibility();
    this.output.appendLine(`Installed TADX ${installed.version}: ${report.issues.length ? report.issues.join('; ') : 'extension command capabilities available'}.`);
    await vscode.workspace.getConfiguration('tadx').update('executablePath', installed.executablePath, vscode.ConfigurationTarget.Global);
    if (this.client.executable === installed.executablePath) await this.refreshCompatibility();
    await vscode.window.showInformationMessage(`TADX ${installed.version} and bundled Guidance are installed.${report.issues.length ? ' Some extension capabilities remain unavailable; see TADX Output.' : ''}`);
  }

  private async resolveKey(arg: unknown, location?: 'local' | 'remote'): Promise<string> {
    const key = keyArgument(arg);
    if (key) return key;
    await this.ready;
    const choices = this.getSnapshot().artifacts.filter(row => !location || row.sourceKind === (location === 'remote' ? 'sites' : 'workspaces'))
      .map(row => ({ label: row.name, description: `${row.kind} · ${row.sourceId}`, detail: row.path ?? row.location, key: row.key }));
    const selected = await vscode.window.showQuickPick(choices, { title: 'Choose an exact artifact', matchOnDescription: true, matchOnDetail: true });
    return selected?.key ?? '';
  }
  private async openBulkActions(kind: 'sites' | 'workspaces', source?: string): Promise<void> {
    await this.ready;
    source ??= kind === 'sites'
      ? this.inventory.selectedSite ?? this.inventory.environments[0]?.alias
      : this.inventory.selectedWorkspace ?? this.inventory.workspaces[0]?.name;
    if (source) {
      if (kind === 'sites') await this.inventory.selectSite(source);
      else await this.inventory.selectWorkspace(source);
    }
    this.panels.showInventory(kind, source);
  }
  private selection(key: string): ArtifactSelection {
    const selection = this.inventory.selection(key);
    if (!selection) throw new Error('The selected artifact is unavailable. Reload the inventory and select it again.');
    return selection;
  }
  private async handle(request: PanelRequest): Promise<void> {
    if (request.action === 'selectSource') {
      if (request.sourceKind === 'sites') await this.inventory.selectSite(request.sourceId);
      else await this.inventory.selectWorkspace(request.sourceId);
      return;
    }
    if (request.action === 'refresh') {
      if (request.sourceKind === 'sites') await this.refreshCache(request.sourceId);
      else await this.inventory.loadWorkspace(request.sourceId, true);
      return;
    }
    if (request.action === 'download' || request.action === 'upload') {
      if (request.keys.every(key => !key)) return;
      await this.exclusive(async () => {
        if (request.action === 'download') await this.download(request.keys);
        else await this.upload(request.keys);
      });
      return;
    }
    if (!('key' in request) || !request.key) return;
    const selection = this.selection(request.key);
    if (request.action === 'showArtifact') { this.panels.showArtifact(request.key); return; }
    if (request.action === 'openRemote') {
      if (selection.location !== 'remote') throw new Error('Select a remote artifact.');
      const url = tableauUrl(selection.artifact.webpageUrl, selection.environment.serverUrl);
      await vscode.env.openExternal(vscode.Uri.parse(url.toString()));
      return;
    }
    if (selection.location !== 'local') throw new Error('Select an exact local artifact.');
    if (request.action === 'move' || request.action === 'delete') {
      await this.exclusive(() => this.localMutation(request.action as 'move' | 'delete', selection));
      return;
    }
    const relative = request.action === 'openPayload' ? selection.artifact.canonicalPath : request.action === 'openMetadata'
      ? selection.artifact.path && `${selection.artifact.path}/metadata.json` : selection.artifact.path;
    const target = vscode.Uri.file(await workspaceFile(selection.workspace.root, relative));
    if (request.action === 'reveal') await vscode.commands.executeCommand('revealInExplorer', target);
    else await vscode.commands.executeCommand('vscode.open', target);
  }

  private async download(keys: string[]): Promise<void> {
    const selected = this.inventory.transferSelection(keys, 'remote');
    const first = selected[0];
    if (!first || first.location !== 'remote') return;
    const workspace = await this.workspacePicker('Download to workspace');
    if (!workspace) return;
    const options: (vscode.QuickPickItem & { id: string })[] = [
      { id: 'overwrite', label: 'Replace existing local artifacts', detail: 'Replace the local copy, including local changes.' },
      ...(selected.some(item => item.artifact.kind === 'workbook') ? [
        { id: 'extract', label: 'Include extracts', detail: 'Download embedded extracts with the workbook.', picked: true },
        { id: 'pds', label: 'Include referenced published datasources', detail: 'Download direct datasource dependencies as sibling artifacts.' },
      ] : []),
    ];
    const chosen = await vscode.window.showQuickPick(options, { canPickMany: true, title: 'Download options', placeHolder: 'Select options, then press Enter to preview.' });
    if (!chosen) return;
    const request: DownloadBatchRequest = { environment: first.environment.alias, workspace: workspace.name,
      items: selected.map(item => ({ kind: item.artifact.kind, id: item.artifact.id })), overwrite: chosen.some(item => item.id === 'overwrite'),
      ...(selected.some(item => item.artifact.kind === 'workbook') ? { includeExtract: chosen.some(item => item.id === 'extract'), includePds: chosen.some(item => item.id === 'pds') } : {}) };
    try { await this.withProgress('Preparing TADX download', () => this.operations.download(request), () => this.operations.cancel()); }
    finally { await this.inventory.loadWorkspace(workspace.name, true); }
  }

  private async upload(keys: string[]): Promise<void> {
    const selected = this.inventory.transferSelection(keys, 'local');
    const first = selected[0];
    if (!first || first.location !== 'local') return;
    const items = selected.map(item => {
      if (!isArtifactKind(item.artifact.kind) || !item.artifact.path) throw new Error('Select valid workbooks, datasources, or flows to upload.');
      return { kind: item.artifact.kind, path: item.artifact.path };
    });
    let environment: EnvironmentProfile | undefined;
    let project: { id: string; path: string } | undefined;
    let step = 1;
    while (true) {
      if (step === 1) {
        const picked = await pick('Upload: choose destination site', this.environmentChoices(), 1, 3, false);
        if (!picked || picked === back) return;
        environment = picked;
        project = undefined;
        step = 2;
      }
      if (step === 2 && environment) {
        await this.inventory.loadSite(environment.alias, true);
        let inventory = this.inventory.sites.get(environment.alias);
        if (!inventory?.projects.length) {
          const action = await vscode.window.showInformationMessage('No destination projects are cached for this site.', 'Refresh Site Cache');
          if (action !== 'Refresh Site Cache') return;
          await this.refreshCache(environment.alias);
          inventory = this.inventory.sites.get(environment.alias);
          if (!inventory?.projects.length) return;
        }
        const paths = projectPaths(inventory.projects);
        const choices = inventory.projects.map(item => ({ label: paths.get(item.id) ?? item.name, description: item.id, value: { id: item.id, path: paths.get(item.id) ?? item.name } }))
          .sort((a, b) => a.label.localeCompare(b.label));
        const picked = await pick(`Upload to ${environment.alias}: choose project`, choices, 2, 3, true);
        if (picked === back) { step = 1; continue; }
        if (!picked) return;
        project = picked;
      }
      if (!environment || !project) return;
      const mode = await pick<'create' | 'overwrite'>('Upload: choose conflict behavior', [
        { label: 'Create new content', description: 'Stop if the name already exists in this project.', value: 'create' },
        { label: 'Overwrite matching content', description: 'Replace an exact same-name match in this project.', value: 'overwrite' },
      ], 3, 3, true);
      if (mode === back) { step = 2; continue; }
      if (!mode) return;
      const request: UploadBatchRequest = { environment: environment.alias, workspace: first.workspace.name, items, projectId: project.id, mode };
      const status = await this.client.mutationStatus(environment.alias);
      const sites = Array.isArray(status.sites) ? status.sites : [];
      const consent = sites.find((item: unknown) => isObject(item) && item.environment === environment!.alias);
      if (!isObject(consent) || consent.enabled !== true || typeof status.restriction === 'string') {
        const preview = await this.withProgress('Preparing read-only upload preview', signal => this.operations.previewUpload(request, signal));
        this.panels.showOperation({ title: 'Upload preview', state: 'preview', summary: typeof status.restriction === 'string' ? status.restriction : 'Remote mutations are disabled for this site. This extension does not change saved consent.', details: JSON.stringify(preview, null, 2) });
        return;
      }
      try { await this.withProgress('Preparing TADX upload', () => this.operations.upload(request, project!.path), () => this.operations.cancel()); }
      finally { await this.inventory.loadSite(environment.alias, true); }
      return;
    }
  }

  private async localMutation(action: 'move' | 'delete', selection: Extract<ArtifactSelection, { location: 'local' }>): Promise<void> {
    if (!selection.artifact.path || !['clean', 'dirty'].includes(selection.artifact.state)) throw new Error('Reload the workspace and select a valid managed artifact.');
    if (action === 'move') {
      const destination = await this.workspacePicker('Move artifact to workspace', selection.workspace.name);
      if (!destination) return;
      const ran = await this.operations.move({ workspace: selection.workspace.name, destination: destination.name, artifact: selection.artifact.path });
      if (ran) await Promise.all([this.inventory.loadWorkspace(selection.workspace.name, true), this.inventory.loadWorkspace(destination.name, true)]);
    } else {
      let force = false;
      if (selection.artifact.state === 'dirty') {
        const answer = await vscode.window.showWarningMessage('This artifact contains local changes.', { modal: true, detail: 'Deleting it also removes those local changes. A deletion preview follows.' }, 'Include local changes');
        if (answer !== 'Include local changes') return;
        force = true;
      }
      const ran = await this.operations.delete({ workspace: selection.workspace.name, artifact: selection.artifact.path, force });
      if (ran) await this.inventory.loadWorkspace(selection.workspace.name, true);
    }
  }

  private environmentChoices(): Choice<EnvironmentProfile>[] {
    return this.inventory.environments.filter(environment => environment.status !== 'invalid').map(environment => ({
      label: environment.alias, description: environment.default ? 'CLI default' : '', detail: `${environment.serverUrl ?? ''} / ${environment.siteContentUrl || 'Default site'}`, value: environment,
    }));
  }
  private async chooseWorkspace(): Promise<void> {
    await this.ready;
    const workspace = await this.workspacePicker('Choose TADX workspace');
    if (workspace) await this.inventory.selectWorkspace(workspace.name);
  }
  private async workspacePicker(title: string, exclude?: string): Promise<Workspace | undefined> {
    const choices = this.inventory.workspaces.filter(workspace => workspace.available && workspace.manifestValid && workspace.name !== exclude)
      .map(workspace => ({ label: workspace.name, description: workspace.default ? 'Default' : '', value: workspace }));
    if (!choices.length) { await vscode.window.showInformationMessage('No available TADX workspace is registered. Create or register one with the TADX CLI, then reload inventory.'); return; }
    const selected = await pick(title, choices);
    return selected === back ? undefined : selected;
  }
  private async refreshCache(alias?: string): Promise<void> {
    await this.ready;
    if (!alias) {
      const environment = await pick('Refresh site cache', this.environmentChoices());
      if (!environment || environment === back) return;
      alias = environment.alias;
    }
    if (!alias) return;
    if (!this.inventory.environments.some(environment => environment.alias === alias)) throw new Error('The selected environment is no longer configured. Reload the inventory.');
    await this.withProgress(`Refreshing ${alias} inventory from Tableau`, signal => this.client.refreshCache(alias!, signal));
    await this.inventory.loadSite(alias, true);
  }
  private async exclusive(task: () => Promise<void>): Promise<void> {
    if (this.choosing || this.operations.busy) throw new Error('Finish or cancel the current action before starting another.');
    this.choosing = true;
    this.update();
    try { await task(); } finally { this.choosing = false; this.update(); }
  }
  private async withProgress<T>(title: string, task: (signal: AbortSignal) => Promise<T>, cancel?: () => void): Promise<T> {
    return vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title, cancellable: true }, async (_progress, token) => {
      const abort = new AbortController();
      const disposable = token.onCancellationRequested(() => { abort.abort(); cancel?.(); });
      try { return await task(abort.signal); } finally { disposable.dispose(); }
    });
  }
  private async run(task: () => Promise<void>): Promise<void> {
    if (!vscode.workspace.isTrusted) { await vscode.window.showInformationMessage('Trust this workspace to run TADX.'); return; }
    try { await this.ready; await task(); }
    catch (error) {
      this.output.appendLine(message(error));
      if (error instanceof TadxError && error.document) this.output.appendLine(JSON.stringify(error.document, null, 2));
      const recovery = error instanceof CompatibilityError || error instanceof TadxError && error.reason === 'start';
      const action = await vscode.window.showErrorMessage(message(error), 'Show Output', ...(recovery ? ['TADX CLI Setup', 'Upgrade Guide'] : []));
      if (action === 'Show Output') this.output.show();
      else if (action === 'TADX CLI Setup') await vscode.commands.executeCommand('tadx.cliStatus');
      else if (action === 'Upgrade Guide') await vscode.env.openExternal(vscode.Uri.parse(upgradeGuide));
    }
  }
}

function keyArgument(value: unknown): string | undefined {
  if (typeof value === 'string') return value;
  if (isObject(value) && typeof value.key === 'string') return value.key;
  return undefined;
}
function sourceArgument(value: unknown, type: 'site' | 'workspace'): string | undefined {
  return isObject(value) && value.type === type && typeof value.source === 'string' ? value.source : undefined;
}
function isObject(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value); }

/** Native Quick Pick steps support Back without preserving a preview for an old destination. */
function pick<T>(title: string, items: Choice<T>[], step?: number, totalSteps?: number, goBack = false): Promise<T | typeof back | undefined> {
  return new Promise(resolve => {
    const input = vscode.window.createQuickPick<Choice<T>>();
    input.title = title;
    input.items = items;
    input.matchOnDescription = true;
    input.matchOnDetail = true;
    input.step = step;
    input.totalSteps = totalSteps;
    input.ignoreFocusOut = true;
    input.buttons = goBack ? [vscode.QuickInputButtons.Back] : [];
    const finish = (value?: T | typeof back): void => { resolve(value); input.dispose(); };
    input.onDidAccept(() => { const item = input.selectedItems[0]; if (item) finish(item.value); });
    input.onDidTriggerButton(() => finish(back));
    input.onDidHide(() => finish());
    input.show();
  });
}
