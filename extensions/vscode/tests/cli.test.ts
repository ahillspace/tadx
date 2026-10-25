import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import test from 'node:test';
import {
  TadxClient, TadxError, buildDeleteArgs, buildDownloadArgs, buildMoveArgs, buildUploadArgs, runCommand,
  type CommandRunner,
} from '../src/cli.js';

const fixture = async (name: string): Promise<string> => readFile(join(__dirname, 'fixtures', 'cli', `${name}.json`), 'utf8');

test('normalizes the public workspace status shape without inventing remote provenance', async () => {
  const stdout = await fixture('workspace-status');
  const client = new TadxClient('tadx', { runner: async () => ({ stdout, stderr: '', exitCode: 0 }) });
  const inventory = await client.workspaceInventory('work');
  assert.equal(inventory.workspace.root, '/work/tableau');
  assert.equal(inventory.scanComplete, true);
  assert.deepEqual(inventory.artifacts[0], {
    kind: 'workbook', id: 'wb-1', name: 'Finance', path: 'artifacts/workbook/Finance--identity',
    canonicalPath: 'artifacts/workbook/Finance--identity/Finance.twbx', state: 'dirty',
  });
  assert.equal('source' in inventory.artifacts[0]!, false);
});

test('retains partial operation evidence even when the CLI exits unsuccessfully', async () => {
  const stdout = await fixture('partial-error');
  const client = new TadxClient('tadx', { runner: async () => ({ stdout, stderr: '', exitCode: 1 }) });
  await assert.rejects(client.inspectOperation('operation-1'), (error: unknown) => {
    assert.ok(error instanceof TadxError);
    assert.equal(error.exitCode, 1);
    assert.equal(error.output?.operation_id, 'operation-1');
    assert.deepEqual(error.output?.items, [{ status: 'succeeded', workbook_luid: 'wb-1' }, { status: 'failed' }]);
    assert.equal(error.details?.outcome, 'partial');
    return true;
  });
});

test('builds explicit destination argv and treats shell metacharacters as literal values', () => {
  const upload = buildUploadArgs({ kind: 'datasource', environment: 'dev & exit', workspace: 'work space',
    artifacts: ['artifacts/datasource/Sales $(echo bad)'], projectId: 'project-1', mode: 'create' }, true);
  assert.deepEqual(upload, ['content', 'datasource', 'publish', '--environment', 'dev & exit',
    '--workspace', 'work space', '--project-id', 'project-1', '--artifact',
    'artifacts/datasource/Sales $(echo bad)', '--create', '--preview']);
  const download = buildDownloadArgs({ kind: 'workbook', environment: 'dev', workspace: 'work', ids: ['wb-1', 'wb-2'] }, false);
  assert.deepEqual(download, ['content', 'workbook', 'pull', '--environment', 'dev', '--workspace', 'work',
    '--id', 'wb-1', '--id', 'wb-2', '--no-wait']);
  assert.equal(download.includes('--preview'), false);
});

test('rejects ambiguous batches and artifact paths that escape their workspace', () => {
  const base = { kind: 'workbook' as const, environment: 'dev', workspace: 'work', projectId: 'p-1', mode: 'create' as const };
  for (const path of ['../outside', '/outside', 'C:/outside', 'artifacts/../../outside', 'artifacts\\workbook\\A']) {
    assert.throws(() => buildUploadArgs({ ...base, artifacts: [path] }, true));
  }
  assert.throws(() => buildUploadArgs({ ...base, artifacts: [] }, true));
  assert.throws(() => buildUploadArgs({ ...base, artifacts: ['same', 'same'] }, true));
  assert.throws(() => buildDownloadArgs({ kind: 'flow', environment: '', workspace: 'work', ids: ['f-1'] }, true));
  assert.throws(() => buildDownloadArgs({ kind: 'flow', environment: 'dev', workspace: 'work', ids: Array.from({ length: 101 }, (_, i) => `f-${i}`) }, true));
});

test('local move and delete preserve exact paths and execute in the foreground', () => {
  const artifact = 'artifacts/workbook/Finance--identity';
  assert.deepEqual(buildMoveArgs({ workspace: 'work', destination: 'review', artifact }, true),
    ['workspace', 'artifact', 'move', '--source', 'work', '--destination', 'review', '--artifact', artifact, '--preview']);
  assert.deepEqual(buildDeleteArgs({ workspace: 'work', artifact, force: true }, false),
    ['workspace', 'artifact', 'delete', '--workspace', 'work', '--artifact', artifact, '--force']);
  assert.throws(() => buildMoveArgs({ workspace: 'work', destination: 'review', artifact: '../escape' }, false));
  assert.throws(() => buildDeleteArgs({ workspace: 'work', artifact: '/escape' }, true));
});

test('environment and workspace discovery reject truncated lists explicitly', async () => {
  const runner: CommandRunner = async () => ({ stdout: JSON.stringify({ page: { more_available: true }, environments: [], workspaces: [] }), stderr: '', exitCode: 0 });
  const client = new TadxClient('tadx', { runner });
  await assert.rejects(client.environments(), /exceeds the CLI result limit/);
  await assert.rejects(client.workspaces(), /exceeds the CLI result limit/);
});

test('normalizes optional source identity only when the CLI provides it', async () => {
  const document = JSON.parse(await fixture('workspace-status'));
  Object.assign(document.artifacts.artifacts[0], { source_server_origin: 'https://tableau.example.test',
    source_site_luid: 'site-1', source_environment: 'dev', source_site: 'sandbox' });
  const client = new TadxClient('tadx', { runner: async () => ({ stdout: JSON.stringify(document), stderr: '', exitCode: 0 }) });
  assert.deepEqual((await client.workspaceInventory('work')).artifacts[0]?.source, {
    serverOrigin: 'https://tableau.example.test', siteLuid: 'site-1', environment: 'dev', site: 'sandbox',
  });
});

test('preserves local Pulse, lineage, and unknown artifact kinds without dropping scan evidence', async () => {
  const document = JSON.parse(await fixture('workspace-status'));
  document.artifacts.artifacts = [
    { kind: 'pulse-definition', luid: 'definition-1', name: 'Sales', path: 'artifacts/pulse-definition/Sales', state: 'dirty',
      source_server_origin: 'https://tableau.example.test', source_site_luid: 'site-1', source_environment: 'dev', source_site: 'sandbox' },
    { kind: 'lineage', luid: 'wb-1', name: 'Dependencies', path: 'artifacts/lineage/Dependencies', state: 'clean' },
    { kind: 'future-kind', luid: '', name: '', path: 'artifacts/future-kind/Example', state: 'invalid', reason: 'Unreadable metadata' },
  ];
  document.artifacts.scan_complete = false;
  document.artifacts.more_available = true;
  document.warnings = ['One directory could not be scanned.'];
  const client = new TadxClient('tadx', { runner: async () => ({ stdout: JSON.stringify(document), stderr: '', exitCode: 0 }) });
  const inventory = await client.workspaceInventory('work');
  assert.deepEqual(inventory.artifacts.map(item => item.kind), ['pulse-definition', 'lineage', 'future-kind']);
  assert.deepEqual(inventory.artifacts.map(item => item.state), ['dirty', 'clean', 'invalid']);
  assert.equal(inventory.artifacts[0]?.source?.siteLuid, 'site-1');
  assert.equal(inventory.artifacts[2]?.reason, 'Unreadable metadata');
  assert.equal(inventory.scanComplete, false);
  assert.equal(inventory.moreAvailable, true);
  assert.deepEqual(inventory.warnings, ['One directory could not be scanned.']);
});

test('rejects a missing local artifact kind instead of claiming a complete scan with omitted rows', async () => {
  const document = JSON.parse(await fixture('workspace-status'));
  document.artifacts.artifacts[0].kind = '';
  const client = new TadxClient('tadx', { runner: async () => ({ stdout: JSON.stringify(document), stderr: '', exitCode: 0 }) });
  await assert.rejects(client.workspaceInventory('work'), /invalid kind/);
});

test('reads cached inventory only and reports partial scope without calling Tableau', async () => {
  const calls: string[][] = [];
  const runner: CommandRunner = async (_executable, args) => {
    calls.push([...args]);
    const command = args.slice(0, 3).join(' ');
    const page = { returned: 1, total: 1, limit: 10000, more_available: false };
    let document: unknown;
    if (command.startsWith('cache status')) {
      document = { status: 'current', generation: { complete: false, stale: false,
        generated_at: '2026-10-01T00:00:00Z', coverage: [] } };
    } else {
      const kind = args[1]!;
      const source = { mode: 'cache', coverage: kind === 'flow' ? 'partial' : 'complete', stale: false };
      document = { status: 'listed', site: 'sandbox', page, source,
        [`${kind}s`]: kind === 'project' ? [{ luid: 'p-1', name: 'Ops', parent_luid: '' }]
          : [{ luid: `${kind}-1`, name: 'Sample', project_luid: 'p-1',
            ...(kind === 'datasource' ? { webpage_url: 'https://tableau.example.test/#/site/sandbox/datasources/42' } : {}) }] };
    }
    return { stdout: JSON.stringify(document), stderr: '', exitCode: 0 };
  };
  const inventory = await new TadxClient('tadx', { runner }).siteInventory('dev');
  assert.equal(inventory.artifacts.length, 3);
  assert.equal(inventory.artifacts.find(item => item.kind === 'datasource')?.webpageUrl, 'https://tableau.example.test/#/site/sandbox/datasources/42');
  assert.equal(inventory.artifacts.find(item => item.kind === 'workbook')?.webpageUrl, undefined);
  assert.equal(inventory.complete, false);
  assert.match(inventory.warnings.join(' '), /flow/);
  assert.equal(calls.length, 5);
  assert.ok(calls.filter(args => args[0] === 'content').every(args => args.includes('--cache') && args.includes('--all')));
  assert.ok(calls.every(args => args.slice(-2).join(' ') === '--full --json'));
  assert.equal(calls.some(args => args.includes('refresh')), false);
});

test('a failed scope remains explicit and preserves other inventory', async () => {
  const runner: CommandRunner = async (_executable, args) => {
    if (args[1] === 'flow') return { stdout: JSON.stringify({ error: { kind: 'operation', summary: 'Flow scope is unavailable.' } }), stderr: '', exitCode: 1 };
    const document = args[0] === 'cache' ? { status: 'uninitialized', environment: 'dev', site: 'sandbox' }
      : { status: 'listed', page: { more_available: false }, source: { mode: 'cache', coverage: 'complete', stale: false }, [`${args[1]}s`]: [] };
    return { stdout: JSON.stringify(document), stderr: '', exitCode: 0 };
  };
  const inventory = await new TadxClient('tadx', { runner }).siteInventory('dev');
  assert.equal(inventory.complete, false);
  assert.match(inventory.warnings.join(' '), /Flow scope is unavailable/);
});

test('artifact inventory scopes diagnostics to its lists while retaining global permission coverage', async () => {
  const permissionWarning = 'Cache permission coverage is incomplete because some workbook permission reads were denied (HTTP 403). Missing rules are unknown, not empty permissions.';
  const runner: CommandRunner = async (_executable, args) => {
    const kind = args[1]!;
    const document = args[0] === 'cache'
      ? { status: 'partial', generation: { complete: false, stale: false, coverage: [
        { scope: 'permissions', requested: true, complete: false, records: 0 },
      ] }, warnings: [permissionWarning] }
      : { status: 'listed', page: { more_available: false },
        source: { mode: 'cache', coverage: 'complete', stale: kind === 'flow',
          ...(kind === 'workbook' ? { cache_warning: 'Retained workbook observation.' } : {}) },
        warnings: kind === 'datasource' ? ['Datasource diagnostic.'] : [],
        [`${kind}s`]: kind === 'project' ? [{ luid: 'p-1', name: 'Ops', parent_luid: '' }]
          : [{ luid: `${kind}-1`, name: 'Sample', project_luid: 'p-1' }] };
    return { stdout: JSON.stringify(document), stderr: '', exitCode: 0 };
  };
  const inventory = await new TadxClient('tadx', { runner }).siteInventory('dev');
  assert.equal(inventory.complete, true);
  assert.equal(inventory.artifacts.length, 3);
  assert.deepEqual(inventory.cache.warnings, [permissionWarning]);
  assert.equal(inventory.cache.coverage[0]?.complete, false);
  assert.deepEqual(inventory.warnings, [
    'workbook: Retained workbook observation.', 'Datasource diagnostic.', 'flow inventory is stale.',
  ]);
});

test('does not treat malformed or missing required JSON fields as empty successful inventories', async () => {
  for (const stdout of ['not json', '[]', '{}', '{"environments":[{"alias":3}]}']) {
    const client = new TadxClient('tadx', { runner: async () => ({ stdout, stderr: '', exitCode: 0 }) });
    await assert.rejects(client.environments(), TadxError);
  }
});

test('operation status output retains the flattened full snapshot', async () => {
  const document = { operation_id: 'op-1', operation: 'workbook.publish', status: 'succeeded',
    result: { workbook_luid: 'wb-1' }, tracking: { phase: 'finished', worker_alive: false } };
  const client = new TadxClient('tadx', { runner: async () => ({ stdout: JSON.stringify(document), stderr: '', exitCode: 0 }) });
  assert.deepEqual(await client.inspectOperation('op-1'), document);
});

test('successful inspection retains an exact operation failure instead of rejecting it as a command failure', async () => {
  const document = { operation_id: 'op-1', operation: 'workbook.publish', status: 'failed',
    error: { kind: 'operation', summary: 'Publication failed after acceptance.' },
    tracking: { phase: 'finished', worker_alive: false } };
  const client = new TadxClient('tadx', { runner: async () => ({ stdout: JSON.stringify(document), stderr: '', exitCode: 0 }) });
  assert.deepEqual(await client.inspectOperation('op-1'), document);
});

test('inspection errors still reject mismatched identities, malformed outcomes, and unsuccessful commands', async () => {
  const base = { operation_id: 'op-1', operation: 'workbook.publish', status: 'failed', error: { summary: 'Failure.' } };
  for (const [document, exitCode] of [[{ ...base, operation_id: 'another' }, 0], [{ ...base, status: 'not-a-state' }, 0],
    [{ ...base, operation: undefined }, 0], [base, 1]] as const) {
    const client = new TadxClient('tadx', { runner: async () => ({ stdout: JSON.stringify(document), stderr: '', exitCode }) });
    await assert.rejects(client.inspectOperation('op-1'), TadxError);
  }
});

test('runner invokes an executable directly and captures structured output', async () => {
  const result = await runCommand(process.execPath, ['-e', 'process.stdout.write(JSON.stringify(process.argv.slice(1)))', 'a & b', '$(literal)'], {});
  assert.deepEqual(JSON.parse(result.stdout), ['a & b', '$(literal)']);
  assert.equal(result.exitCode, 0);
});

test('runner bounds combined output, timeout, and cancellation', async () => {
  await assert.rejects(runCommand(process.execPath, ['-e', 'process.stdout.write("x".repeat(10000))'], { maxOutputBytes: 100 }),
    (error: unknown) => error instanceof TadxError && error.reason === 'output_limit');
  await assert.rejects(runCommand(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { timeoutMs: 50 }),
    (error: unknown) => error instanceof TadxError && error.reason === 'timeout');
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(runCommand(process.execPath, ['-e', 'process.exit(0)'], { signal: controller.signal }),
    (error: unknown) => error instanceof TadxError && error.reason === 'aborted');
});
