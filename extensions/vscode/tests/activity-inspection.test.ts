import assert from 'node:assert/strict';
import test from 'node:test';
import { TadxClient } from '../src/cli.js';
import { Operations } from '../src/operations.js';

test('checking a retained receipt displays the newly observed failure and preserves its evidence', async () => {
  const outcome = { operation_id: 'retained-operation', operation: 'workbook.publish', status: 'failed',
    error: { kind: 'operation', summary: 'The worker could not publish the workbook.' },
    tracking: { phase: 'finished', worker_alive: false } };
  const client = new TadxClient('tadx', { runner: async (_executable, args) => {
    assert.deepEqual(args, ['job', 'inspect', '--operation-id', 'retained-operation', '--full', '--json']);
    return { stdout: JSON.stringify(outcome), stderr: '', exitCode: 0 };
  } });
  const shown: unknown[] = [];
  const operations = new Operations(client, { confirm: async () => false, display: entry => shown.push(entry), changed: () => undefined });
  operations.restore([{ operationId: 'retained-operation', title: 'Upload workbook', startedAt: '2026-01-01T00:00:00Z' }]);
  const entry = operations.entries[0]!;
  assert.equal(entry.state, 'unknown');
  await operations.inspect(entry.key);
  assert.equal(entry.state, 'failed');
  assert.equal(entry.operationId, 'retained-operation');
  assert.deepEqual(entry.result, outcome);
  assert.equal(shown.length, 1);
  assert.match(JSON.stringify(shown[0]), /worker could not publish/);
  assert.equal(operations.receipts().length, 1);
});
