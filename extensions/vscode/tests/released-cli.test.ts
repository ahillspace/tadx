import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import test from 'node:test';

test('released CLI contract gate rejects altered bytes before extraction or execution', async () => {
  const root = await mkdtemp(join(tmpdir(), 'tadx-release-checksum-test-'));
  assert.equal(dirname(resolve(root)), resolve(tmpdir()));
  try {
    const archive = join(root, 'altered.zip');
    await writeFile(archive, 'This is not the reviewed release.');
    const result = spawnSync(process.execPath, [resolve(__dirname, '../scripts/test-release-cli.mjs'), `--archive=${archive}`], {
      encoding: 'utf8', windowsHide: true, timeout: 20_000,
    });
    assert.equal(result.status, 1);
    assert.match(result.stderr, /checksum mismatch.*not extracted or executed/);
    assert.equal(result.stdout, '');
  } finally { await rm(root, { recursive: true, force: true }); }
});
