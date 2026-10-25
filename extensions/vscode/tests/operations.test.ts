import assert from 'node:assert/strict';
import test from 'node:test';
import { TadxClient, TadxError } from '../src/cli.js';
import type { DownloadBatchRequest, DownloadRequest, OperationResult, UploadBatchRequest, UploadRequest } from '../src/model.js';
import { Operations, operationItems, operationState, type OperationUI } from '../src/operations.js';

test('bulk receipt display preserves ordered successes and failures', () => {
  assert.deepEqual(operationItems({ output: { items: [
    { status: 'succeeded', result: { plan: { workbook_name: 'Revenue' }, result: { status: 'published' } } },
    { selector: 'artifacts/workbook/Costs', status: 'failed', error: { summary: 'Permission denied.' } },
  ] } }), [
    { name: 'Revenue', state: 'succeeded', message: undefined },
    { name: 'artifacts/workbook/Costs', state: 'failed', message: 'Permission denied.' },
  ]);
});

test('real single-operation snapshots distinguish completion from interruption and unknown state', () => {
  assert.equal(operationState({ operation_id: 'download-1', status: 'pulled' }), 'succeeded');
  assert.equal(operationState({ plan: { mode: 'execute' }, result: { status: 'deleted' } }), 'succeeded');
  assert.equal(operationState({ plan: { mode: 'execute' }, result: { status: 'published' } }), 'succeeded');
  assert.equal(operationState({ operation_id: 'stopped-1', status: 'interrupted' }), 'unknown');
  assert.equal(operationState({ operation_id: 'unknown-1', status: 'new-future-state' }), 'unknown');
  assert.equal(operationState({ operation_id: 'starting-1', status: 'starting' }), 'pending');
});

test('canceling an open confirmation settles promptly and ignores a later Run choice', async () => {
  let confirm!: (value: boolean) => void;
  let opened!: () => void;
  const confirmationOpen = new Promise<void>(resolve => { opened = resolve; });
  let writes = 0;
  const h = harness({ upload: async () => { writes++; return { status: 'published' }; } }, () => {
    opened();
    return new Promise<boolean>(resolve => { confirm = resolve; });
  });
  const pending = h.operations.upload({ environment: 'dev', workspace: 'work', items: [{ kind: 'workbook', path: 'artifacts/workbook/A' }], projectId: 'p-1', mode: 'create' }, 'Ops');
  await confirmationOpen;
  h.operations.cancel();
  assert.equal(await pending, false);
  assert.equal(h.operations.busy, false);
  confirm(true);
  await Promise.resolve();
  assert.equal(writes, 0);
});

type Client = Pick<TadxClient, 'previewDownload' | 'download' | 'previewUpload' | 'upload' | 'previewMove' | 'move' | 'previewDelete' | 'deleteArtifact' | 'inspectOperation'>;

function harness(overrides: Partial<Client> = {}, confirm: OperationUI['confirm'] = async () => true) {
  const calls: string[] = [];
  const method = (name: string, result: OperationResult) => async () => { calls.push(name); return result; };
  const client: Client = {
    previewDownload: method('previewDownload', { status: 'preview' }), download: method('download', { operation_id: 'op-1', status: 'started' }),
    previewUpload: method('previewUpload', { plan: { mode: 'preview' } }), upload: method('upload', { operation_id: 'op-1', status: 'started' }),
    previewMove: method('previewMove', { status: 'preview' }), move: method('move', { status: 'moved' }),
    previewDelete: method('previewDelete', { plan: { mode: 'preview' } }), deleteArtifact: method('deleteArtifact', { status: 'deleted' }),
    inspectOperation: method('inspectOperation', { operation_id: 'op-1', status: 'succeeded' }), ...overrides,
  };
  const displayed: Parameters<OperationUI['display']>[0][] = [];
  const busy: boolean[] = [];
  let confirmations = 0;
  const ui: OperationUI = { confirm: async (...args) => { confirmations++; return confirm(...args); },
    display: entry => displayed.push(entry), changed: (_entries, value) => busy.push(value) };
  return { operations: new Operations(client, ui), calls, displayed, busy, confirmations: () => confirmations };
}

const download: DownloadBatchRequest = { environment: 'source', workspace: 'work', items: [{ kind: 'workbook', id: 'wb-1' }] };
const upload: UploadBatchRequest = { environment: 'destination', workspace: 'work',
  items: [{ kind: 'datasource', path: 'artifacts/datasource/Sales--identity' }], projectId: 'project-1', mode: 'create' };

test('failed preview never requests confirmation or runs the mutation', async () => {
  const failure = new TadxError('A destination collision exists.', 'cli', { error: { summary: 'A destination collision exists.' } }, 1);
  const h = harness({ previewUpload: async () => { throw failure; } });
  await assert.rejects(h.operations.upload(upload, 'Department/Ops'), error => error === failure);
  assert.equal(h.confirmations(), 0);
  assert.deepEqual(h.calls, []);
  assert.equal(h.operations.busy, false);
  assert.equal(h.operations.entries[0]?.state, 'failed');
  assert.match(h.operations.entries[0]?.summary ?? '', /destination collision/);
});

test('declining the preview performs no mutation and clears busy state', async () => {
  const h = harness({}, async () => false);
  assert.equal(await h.operations.download(download), false);
  assert.deepEqual(h.calls, ['previewDownload']);
  assert.equal(h.operations.entries.length, 0);
  assert.deepEqual(h.displayed.map(item => item.state), ['preview']);
  assert.equal(h.busy.at(-1), false);
});

test('cancellation while a preview runs never starts the accepted operation', async () => {
  const preview = deferred<OperationResult>();
  let signal: AbortSignal | undefined;
  const h = harness({ previewDownload: async (_request, value) => { signal = value; return preview.promise; } });
  const running = h.operations.download(download);
  h.operations.cancel();
  assert.equal(signal?.aborted, true);
  preview.resolve({ status: 'preview' });
  assert.equal(await running, false);
  assert.equal(h.confirmations(), 0);
  assert.equal(h.calls.includes('download'), false);
  assert.equal(h.operations.busy, false);
});

test('upload runs the exact destination and artifact set captured before the preview', async () => {
  const original: UploadRequest = { kind: 'datasource', environment: upload.environment, workspace: upload.workspace,
    projectId: upload.projectId, mode: upload.mode, artifacts: upload.items.map(item => item.path) };
  const request: UploadBatchRequest = { ...upload, items: upload.items.map(item => ({ ...item })) };
  let previewed: UploadRequest | undefined;
  let executed: UploadRequest | undefined;
  const h = harness({ previewUpload: async value => { previewed = structuredClone(value); return { plan: { target: { project_luid: value.projectId } } }; },
    upload: async value => { executed = structuredClone(value); return { operation_id: 'op-1', status: 'started' }; } }, async () => {
    request.environment = 'different-site';
    request.projectId = 'different-project';
    request.mode = 'overwrite';
    request.items[0]!.path = 'artifacts/datasource/Changed';
    (request.items as { kind: 'datasource'; path: string }[]).push({ kind: 'datasource', path: 'artifacts/datasource/Unexpected--identity' });
    return true;
  });
  assert.equal(await h.operations.upload(request, 'Department/Ops'), true);
  assert.deepEqual(previewed, original);
  assert.deepEqual(executed, original);
});

test('mixed batch errors retain accepted identity and both successful and failed item evidence', async () => {
  const receipt = { output: { operation_id: 'op-mixed', status: 'partial', items: [
    { status: 'succeeded', datasource_luid: 'ds-1' }, { status: 'failed', error: 'Permission denied.' },
  ] }, error: { kind: 'operation', summary: 'One upload failed.', outcome: 'partial' } };
  const h = harness({ upload: async () => { throw new TadxError('One upload failed.', 'cli', receipt, 1); } });
  await assert.rejects(h.operations.upload(upload, 'Ops'), TadxError);
  assert.equal(h.operations.entries[0]?.operationId, 'op-mixed');
  assert.equal(h.operations.entries[0]?.state, 'partial');
  assert.deepEqual(h.operations.entries[0]?.result, receipt);
  assert.equal(h.operations.receipts().length, 1);
  assert.match(h.displayed.at(-1)?.details ?? '', /Permission denied/);
});

test('unknown execution outcome preserves an accepted receipt and avoids automatic retry', async () => {
  let executions = 0;
  const receipt = { operation_id: 'op-unknown', status: 'started' };
  const h = harness({ download: async () => { executions++; throw new TadxError('Timed out.', 'timeout', receipt); } });
  await assert.rejects(h.operations.download(download), TadxError);
  assert.equal(executions, 1);
  assert.equal(h.operations.entries[0]?.operationId, 'op-unknown');
  assert.equal(h.operations.entries[0]?.state, 'unknown');
  assert.deepEqual(h.operations.entries[0]?.result, receipt);
});

test('failed status inspection preserves the prior receipt, identity, and state', async () => {
  let inspected: string | undefined;
  const h = harness({ inspectOperation: async operationId => { inspected = operationId; throw new Error('Status service unavailable.'); } });
  await h.operations.download(download);
  const entry = h.operations.entries[0]!;
  const priorResult = structuredClone(entry.result);
  await assert.rejects(h.operations.inspect(entry.key), /Status service unavailable/);
  assert.equal(inspected, 'op-1');
  assert.equal(entry.operationId, 'op-1');
  assert.equal(entry.state, 'pending');
  assert.deepEqual(entry.result, priorResult);
  assert.match(entry.summary, /Status check failed/);
});

test('concurrent operations cannot preview or mutate twice', async () => {
  const preview = deferred<OperationResult>();
  let previews = 0;
  const h = harness({ previewDownload: async () => { previews++; return preview.promise; } });
  const first = h.operations.download(download);
  await assert.rejects(h.operations.download(download), /current action/);
  assert.equal(previews, 1);
  preview.resolve({ status: 'preview' });
  await first;
  assert.deepEqual(h.calls, ['download']);
  assert.equal(h.operations.entries.length, 1);
});

test('concurrent status checks reuse the existing in-flight check', async () => {
  const status = deferred<OperationResult>();
  let reads = 0;
  const h = harness({ inspectOperation: async () => { reads++; return status.promise; } });
  await h.operations.download(download);
  const key = h.operations.entries[0]!.key;
  const first = h.operations.inspect(key);
  await h.operations.inspect(key);
  assert.equal(reads, 1);
  status.resolve({ operation_id: 'op-1', status: 'succeeded' });
  await first;
  assert.equal(h.operations.entries[0]?.state, 'succeeded');
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

test('mixed downloads preview every kind before one confirmation and keep one receipt per kind', async () => {
  const events: string[] = [];
  const previews: DownloadRequest[] = [];
  const executions: DownloadRequest[] = [];
  const h = harness({ previewDownload: async request => {
    events.push(`preview:${request.kind}`); previews.push(structuredClone(request)); return { status: 'preview' };
  }, download: async request => {
    events.push(`run:${request.kind}`); executions.push(structuredClone(request)); return { operation_id: `op-${request.kind}`, status: 'running' };
  } }, async () => { events.push('confirm'); return true; });
  const request: DownloadBatchRequest = { environment: 'source', workspace: 'one-target', overwrite: true, includeExtract: false, includePds: true,
    items: [{ kind: 'flow', id: 'f-1' }, { kind: 'workbook', id: 'w-1' }, { kind: 'datasource', id: 'd-1' }, { kind: 'workbook', id: 'w-2' }] };
  assert.equal(await h.operations.download(request), true);
  assert.deepEqual(events, ['preview:flow', 'preview:workbook', 'preview:datasource', 'confirm', 'run:flow', 'run:workbook', 'run:datasource']);
  assert.deepEqual(executions, previews);
  assert.equal(h.confirmations(), 1);
  assert.deepEqual(previews, [
    { kind: 'flow', environment: 'source', workspace: 'one-target', ids: ['f-1'], overwrite: true },
    { kind: 'workbook', environment: 'source', workspace: 'one-target', ids: ['w-1', 'w-2'], overwrite: true, includeExtract: false, includePds: true },
    { kind: 'datasource', environment: 'source', workspace: 'one-target', ids: ['d-1'], overwrite: true },
  ]);
  assert.deepEqual(h.operations.entries.map(entry => entry.operationId), ['op-datasource', 'op-workbook', 'op-flow']);
  assert.equal(h.operations.entries.every(entry => entry.summary.includes('one-target')), true);
  const restored = harness();
  restored.operations.restore(h.operations.receipts());
  assert.deepEqual(restored.operations.entries.map(entry => entry.operationId), ['op-datasource', 'op-workbook', 'op-flow']);
  assert.equal(restored.operations.entries.every(entry => entry.state === 'unknown'), true);
  assert.match(h.displayed.at(-1)?.summary ?? '', /accepted.*continue/i);
});

test('failed second-kind preview prevents every transfer and retains its error details', async () => {
  const events: string[] = [];
  const h = harness({ previewUpload: async request => {
    events.push(`preview:${request.kind}`);
    if (request.kind === 'flow') throw new TadxError('Flow destination denied.', 'cli', { error: { summary: 'Flow destination denied.' } });
    return { plan: { mode: 'preview' } };
  }, upload: async () => { events.push('run'); return {}; } });
  await assert.rejects(h.operations.upload({ ...upload, items: [upload.items[0]!, { kind: 'flow', path: 'artifacts/flow/F' }] }, 'Ops'), /Flow destination denied/);
  assert.deepEqual(events, ['preview:datasource', 'preview:flow']);
  assert.equal(h.confirmations(), 0);
  assert.match(h.operations.entries[0]?.summary ?? '', /No transfers were started/);
  assert.match(JSON.stringify(h.operations.entries[0]?.result) ?? '', /Flow destination denied/);
});

test('mixed upload preview-only uses the same grouping without confirmation or execution', async () => {
  const previews: UploadRequest[] = [];
  const h = harness({ previewUpload: async request => { previews.push(structuredClone(request)); return { plan: { project_luid: request.projectId } }; } });
  const result = await h.operations.previewUpload({ ...upload, items: [{ kind: 'workbook', path: 'artifacts/workbook/W' }, ...upload.items] });
  assert.equal(result.status, 'preview');
  assert.equal(result.total, 2);
  assert.equal((result.groups as unknown[]).length, 2);
  assert.deepEqual(previews.map(request => [request.kind, request.environment, request.workspace, request.projectId, request.mode]), [
    ['workbook', 'destination', 'work', 'project-1', 'create'], ['datasource', 'destination', 'work', 'project-1', 'create'],
  ]);
  assert.equal(h.confirmations(), 0);
  assert.deepEqual(h.calls, []);
});

test('mixed upload reaches the public CLI with one destination and resource-specific create flags', async () => {
  const calls: string[][] = [];
  let confirmations = 0;
  const client = new TadxClient('synthetic-cli', { runner: async (_executable, args) => {
    calls.push([...args]);
    return { stdout: JSON.stringify(args.includes('--preview') ? { status: 'preview' } : { operation_id: `op-${args[1]}`, status: 'running' }), stderr: '', exitCode: 0 };
  } });
  const operations = new Operations(client, { confirm: async () => { confirmations++; assert.equal(calls.length, 3); return true; }, display() {}, changed() {} });
  assert.equal(await operations.upload({ ...upload, items: [
    { kind: 'workbook', path: 'artifacts/workbook/Book' }, { kind: 'datasource', path: 'artifacts/datasource/Source' }, { kind: 'flow', path: 'artifacts/flow/Flow' },
  ] }, 'Destination'), true);
  assert.equal(confirmations, 1);
  assert.deepEqual(calls.map(args => args[1]), ['workbook', 'datasource', 'flow', 'workbook', 'datasource', 'flow']);
  for (const [index, args] of calls.entries()) {
    assert.equal(args[args.indexOf('--workspace') + 1], 'work');
    assert.equal(args[args.indexOf('--environment') + 1], 'destination');
    assert.equal(args[args.indexOf('--project-id') + 1], 'project-1');
    assert.equal(args.includes('--create'), args[1] === 'datasource');
    assert.equal(args.includes('--preview'), index < 3);
    assert.equal(args.includes('--no-wait'), index >= 3);
    assert.deepEqual(args.slice(-2), ['--full', '--json']);
  }
  assert.deepEqual(operations.entries.map(entry => entry.operationId), ['op-flow', 'op-datasource', 'op-workbook']);
});

test('second submission failure retains earlier receipts and explicitly skips later kinds', async () => {
  const executed: string[] = [];
  const failure = new TadxError('Second launch outcome unknown.', 'timeout', { operation_id: 'op-datasource', status: 'starting' });
  const h = harness({ download: async request => {
    executed.push(request.kind);
    if (request.kind === 'datasource') throw failure;
    return { operation_id: `op-${request.kind}`, status: 'running' };
  } });
  await assert.rejects(h.operations.download({ ...download, items: [...download.items, { kind: 'datasource', id: 'd-1' }, { kind: 'flow', id: 'f-1' }] }), error => error === failure);
  assert.deepEqual(executed, ['workbook', 'datasource']);
  assert.deepEqual(h.operations.entries.filter(entry => entry.operationId).map(entry => [entry.operationId, entry.state]), [['op-datasource', 'unknown'], ['op-workbook', 'pending']]);
  assert.match(h.displayed.at(-1)?.summary ?? '', /flow.*not submitted/i);
  assert.match(h.displayed.at(-1)?.summary ?? '', /accepted.*continue/i);
  assert.match(h.displayed.at(-1)?.details ?? '', /Second launch outcome unknown/);
});

test('cancellation after acceptance retains the receipt and submits no further groups', async () => {
  const executed: string[] = [];
  const h = harness({ download: async request => {
    executed.push(request.kind); h.operations.cancel(); return { operation_id: 'accepted-1', status: 'running' };
  } });
  assert.equal(await h.operations.download({ ...download, items: [...download.items, { kind: 'flow', id: 'f-1' }] }), false);
  assert.deepEqual(executed, ['workbook']);
  assert.equal(h.operations.entries.find(entry => entry.operationId)?.operationId, 'accepted-1');
  assert.match(h.displayed.at(-1)?.summary ?? '', /flow.*not submitted/i);
  assert.equal(h.operations.busy, false);
});

for (const status of ['partial_failure', 'failed', 'new-future-state']) {
  test(`resolved ${status} submission stops later kind submissions`, async () => {
    const executed: string[] = [];
    const h = harness({ download: async request => { executed.push(request.kind); return { operation_id: 'op-adverse', status }; } });
    assert.equal(await h.operations.download({ ...download, items: [...download.items, { kind: 'flow', id: 'f-1' }] }), false);
    assert.deepEqual(executed, ['workbook']);
    assert.equal(h.operations.entries.find(entry => entry.operationId)?.operationId, 'op-adverse');
    assert.match(h.displayed.at(-1)?.summary ?? '', /flow.*not submitted/i);
    assert.equal(h.displayed.at(-1)?.state, status === 'failed' ? 'failed' : status === 'partial_failure' ? 'partial' : 'unknown');
  });
}

test('an accepted group followed by a failed group reports partial submission', async () => {
  const h = harness({ download: async request => ({ operation_id: `op-${request.kind}`, status: request.kind === 'workbook' ? 'running' : 'failed' }) });
  assert.equal(await h.operations.download({ ...download, items: [...download.items, { kind: 'flow', id: 'f-1' }] }), false);
  assert.equal(h.displayed.at(-1)?.state, 'partial');
  assert.equal(h.operations.receipts().length, 2);
});

test('mixed selection validates the total bound and kind-qualified identities before any preview', async () => {
  const h = harness();
  await assert.rejects(h.operations.download({ ...download, items: [] }), /1 to 100/);
  await assert.rejects(h.operations.download({ ...download, items: Array.from({ length: 101 }, (_, index) => ({ kind: index < 51 ? 'workbook' : 'datasource', id: `id-${index}` })) }), /1 to 100/);
  await assert.rejects(h.operations.download({ ...download, items: [{ kind: 'workbook', id: ' w-1 ' }, { kind: 'workbook', id: 'w-1' }] }), /unique/);
  assert.deepEqual(h.calls, []);
  assert.equal(await h.operations.download({ ...download, items: [{ kind: 'workbook', id: 'same-id' }, { kind: 'flow', id: 'same-id' }] }), true);
});
