import assert from 'node:assert/strict';
import { join, resolve } from 'node:path';
import type { CommandRunner } from '../src/cli.js';
import { TadxClient } from '../src/cli.js';

/** Tests the adapter against the executable's public contracts, using only isolated local state. */
export async function verify(executable: string, root: string, runner: CommandRunner): Promise<string> {
  const client = new TadxClient(executable, { runner });
  const compatibility = await client.checkCompatibility();
  assert.deepEqual(compatibility.issues, [], 'The CLI must implement every current extension contract.');
  assert.deepEqual(await client.environments(), []);
  assert.deepEqual(await client.workspaces(), []);
  const invoke = async (args: string[]): Promise<void> => {
    const result = await runner(executable, [...args, '--full', '--json'], {});
    assert.equal(result.exitCode, 0, `Local fixture command failed: ${args.slice(0, 2).join(' ')}`);
    assert.equal('error' in JSON.parse(result.stdout), false);
  };
  await invoke(['env', 'add', 'fixture', '--url', 'https://tableau.example.test', '--site', 'sandbox']);
  const environments = await client.environments();
  assert.equal(environments.length, 1);
  assert.equal(environments[0]?.alias, 'fixture');
  assert.equal(environments[0]?.serverUrl, 'https://tableau.example.test');
  await invoke(['workspace', 'create', 'work', '--path', join(root, 'workspace')]);
  const workspaces = await client.workspaces();
  assert.equal(workspaces.length, 1);
  assert.ok(workspaces[0]?.root);
  assert.equal(resolve(workspaces[0].root), resolve(root, 'workspace'));
  const inventory = await client.workspaceInventory('work');
  assert.equal(inventory.scanComplete, true);
  assert.deepEqual(inventory.artifacts, []);
  const cached = await client.siteInventory('fixture');
  assert.equal(cached.cache.status, 'uninitialized');
  assert.equal(cached.complete, false);
  assert.deepEqual(cached.artifacts, []);
  const consent = await client.mutationStatus('fixture');
  assert.ok(Array.isArray(consent.sites));
  assert.equal(consent.sites.length, 1);
  assert.ok(consent.sites.every(site => typeof site === 'object' && site !== null && 'enabled' in site && site.enabled === false));
  return compatibility.version;
}
