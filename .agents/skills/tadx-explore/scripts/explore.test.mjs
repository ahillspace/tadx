import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { createLauncher } from './explore.mjs';

function fixture(t, overrides = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'tadx-explore-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  let time = Date.UTC(2026, 0, 1);
  const run = createLauncher(root, { now: () => time, revision: 'a'.repeat(40), ...overrides });
  const read = relative => JSON.parse(fs.readFileSync(path.join(root, relative), 'utf8'));
  const write = (relative, value) => fs.writeFileSync(path.join(root, relative), `${JSON.stringify(value, null, 2)}\n`);
  run(['init', '--environment', 'offline']);
  return { root, run, read, write, advance: ms => { time += ms; } };
}

function complete(f, result, changes = {}) {
  const report = f.read(result.paths.report);
  Object.assign(report, { status: 'completed', tadx_version: '0.0.0-test', call_count: 2 }, changes);
  f.write(result.paths.report, report);
  return report;
}

function finding(title = 'Scoped output loses an identity') {
  return {
    kind: 'bug', title, severity: 'medium', confidence: 'high',
    observed: 'The result omits the known fixture identity.', expected: 'The result retains that identity.',
    impact: 'Cleanup requires the resource journal.', reproducer: 'Use only the disposable fixture from this run.',
    recommendation: 'Retain the confirmed identity.',
  };
}

test('preparation isolates runs and returns only a compact fresh-worker payload', t => {
  const f = fixture(t);
  const first = f.run(['prepare', '--scope', 'content workbook move']);
  const second = f.run(['prepare', '--scope', 'content workbook move']);
  assert.notEqual(first.run_id, second.run_id);
  assert.notEqual(first.paths.report, second.paths.report);
  assert.deepEqual(Object.keys(first.spawn).sort(), ['fork_turns', 'message', 'model', 'reasoning_effort', 'task_name']);
  assert.equal(first.spawn.model, 'gpt-6-luna');
  assert.equal(first.spawn.reasoning_effort, 'medium');
  assert.equal(first.spawn.fork_turns, 'none');
  assert.ok(first.spawn.message.length < 1000);
  assert.match(first.spawn.message, /references\/worker\.md/);
  assert.equal(f.read(first.paths.pass_manifest).previous_brief, null);
  assert.equal(f.read(first.paths.run_manifest).authority.allow_fixture_writes, false);
  assert.equal(f.read(first.paths.report).status, 'pending');
  const contract = f.read(f.read(first.paths.pass_manifest).paths.report_contract);
  assert.ok(contract.report.findings.required_fields.includes('reproducer'));
  assert.ok(contract.report.findings.kind.includes('unexpected_behavior'));
  assert.ok(contract.journal.resources.required_fields.includes('job_id'));
});

test('settings persist, flags override them, and settings never grant fixture authority', t => {
  const f = fixture(t);
  const before = fs.readFileSync(path.join(f.root, '.tadx-explore/settings.json'), 'utf8');
  assert.equal(f.run(['init', '--environment', 'different']).status, 'existing');
  assert.equal(fs.readFileSync(path.join(f.root, '.tadx-explore/settings.json'), 'utf8'), before);
  const prepared = f.run(['prepare', '--scope', 'workspace', '--environment', 'override', '--workspace', 'sample', '--allow-fixture-writes']);
  const manifest = f.read(prepared.paths.run_manifest);
  assert.equal(manifest.settings.environment, 'override');
  assert.equal(manifest.settings.workspace, 'sample');
  assert.equal(manifest.authority.allow_fixture_writes, true);
  const settings = f.read('.tadx-explore/settings.json');
  settings.allow_fixture_writes = true;
  f.write('.tadx-explore/settings.json', settings);
  assert.throws(() => f.run(['prepare', '--scope', 'content']), /setting/i);
});

test('invalid scope, budgets, run selectors, and flags fail before artifact creation', t => {
  const f = fixture(t);
  for (const scope of ['content; echo bad', '../content', 'content --help', 'Content', 'one two three four', '']) {
    assert.throws(() => f.run(['prepare', '--scope', scope]), /scope/i);
  }
  for (const args of [['--max-calls', '0'], ['--cleanup-reserve', '100'], ['--max-minutes', 'NaN'], ['--max-passes', '-1'], ['--unknown', 'value']]) {
    assert.throws(() => f.run(['prepare', '--scope', 'content', ...args]));
  }
  assert.throws(() => f.run(['next', '--run', '../outside']), /run/i);
  assert.throws(() => f.run(['init', '--allow-fixture-writes']), /option/i);
});

test('local read-only exploration accepts no environment while fixture writes require one', t => {
  const f = fixture(t);
  const settings = f.read('.tadx-explore/settings.json');
  settings.environment = null;
  f.write('.tadx-explore/settings.json', settings);
  const prepared = f.run(['prepare', '--scope', 'workspace']);
  assert.equal(f.read(prepared.paths.run_manifest).settings.environment, null);
  assert.throws(() => f.run(['prepare', '--scope', 'workspace', '--allow-fixture-writes']), /environment/i);
});

test('pending reports and repeated next calls never invent or skip a pass', t => {
  const f = fixture(t);
  const prepared = f.run(['prepare', '--scope', 'content workbook']);
  const pending = f.run(['next', '--run', prepared.run_id]);
  assert.equal(pending.status, 'pending');
  assert.equal(pending.spawn, undefined);
  assert.equal(pending.stop_reason, 'awaiting_report');
  assert.equal(pending.counts.completed_passes, 0);
  const completedReport = complete(f, prepared, { findings: [finding()] });
  const second = f.run(['next', '--run', prepared.run_id]);
  assert.equal(second.pass_id, '02');
  assert.ok(second.spawn);
  assert.equal(f.read(second.paths.pass_manifest).budget.remaining_calls, 98);
  assert.equal(f.read(second.paths.pass_manifest).budget.call_limit, 35);
  assert.equal(f.read(second.paths.pass_manifest).budget.exploration_calls, 15);
  assert.equal(f.read(second.paths.pass_manifest).previous_brief.findings.length, 1);
  const repeated = f.run(['next', '--run', prepared.run_id]);
  assert.equal(repeated.pass_id, '02');
  assert.equal(repeated.spawn, undefined);
  assert.equal(repeated.counts.reported_calls, 2);
  assert.deepEqual(f.read(prepared.paths.report), completedReport);
});

test('the generated contract describes valid report and journal item shapes', t => {
  const f = fixture(t);
  const prepared = f.run(['prepare', '--scope', 'content']);
  const contract = f.read(prepared.paths.report_contract);
  const report = complete(f, prepared, {
    findings: [{ ...finding(), error: { exit_code: -1, code: 'example_code', phase: 'verification', outcome: 'unknown', excerpt: 'Sanitized meaningful evidence.' } }],
    coverage: [{ action: 'content workbook list', variations: ['empty project'], outcome: 'success', note: 'One scenario.' }],
  });
  assert.deepEqual(Object.keys(report).sort(), contract.report.required_fields.toSorted());
  assert.deepEqual(Object.keys(report.coverage[0]).sort(), contract.report.coverage.required_fields.toSorted());
  assert.deepEqual(Object.keys(report.findings[0]).sort(), [...contract.report.findings.required_fields, ...contract.report.findings.optional_fields].sort());
  assert.deepEqual(Object.keys(report.findings[0].error).sort(), contract.report.findings.error.optional_fields.toSorted());
  assert.ok(f.run(['next', '--run', prepared.run_id]).spawn);
});

test('reports validate identity, structure, unknown fields, and bounded evidence', t => {
  const f = fixture(t);
  const prepared = f.run(['prepare', '--scope', 'content']);
  const good = complete(f, prepared, { coverage: [{ action: 'content workbook list', variations: ['empty result'], outcome: 'success', note: 'Reported by worker.' }] });
  for (const changed of [
    { run_id: 'other' }, { call_count: -1 }, { coverage: [{}] }, { findings: [{ ...finding(), kind: 'invented' }] },
    { cleanup: { status: 'complete', remaining: 'none' } }, { arbitrary: 'extra' },
    { findings: [{ ...finding(), error: { excerpt: 'x'.repeat(4001) } }] },
  ]) {
    f.write(prepared.paths.report, { ...good, ...changed });
    assert.throws(() => f.run(['next', '--run', prepared.run_id]), /report/i);
    assert.equal(f.read(prepared.paths.run_manifest).processed.length, 0);
  }
  f.write(prepared.paths.report, good);
  assert.ok(f.run(['next', '--run', prepared.run_id]).spawn);
});

test('all terminal statuses, blockers, and cleanup uncertainty stop new workers', t => {
  const f = fixture(t);
  const cases = [
    [{ status: 'blocked', blockers: ['Site consent requires separate permission.'] }, 'blocked'],
    [{ status: 'interrupted' }, 'interrupted'],
    [{ status: 'uncertain' }, 'uncertain'],
    [{ tadx_version: null }, 'missing_version_evidence'],
    [{ repository_revision: null }, 'missing_version_evidence'],
    [{ blockers: ['Unavailable prerequisite.'] }, 'blocker'],
    [{ cleanup: { status: 'incomplete', remaining: [{ identity: 'fixture-id', location: null, note: 'Cleanup failed.' }] } }, 'cleanup_incomplete'],
    [{ cleanup: { status: 'uncertain', remaining: [] } }, 'cleanup_incomplete'],
  ];
  for (const [changes, reason] of cases) {
    const prepared = f.run(['prepare', '--scope', 'content']);
    complete(f, prepared, changes);
    const result = f.run(['next', '--run', prepared.run_id]);
    assert.equal(result.stop_reason, reason);
    assert.equal(result.spawn, undefined);
    assert.deepEqual(f.run(['next', '--run', prepared.run_id]), result);
  }
});

test('unresolved journal creation intents prevent continuation even with claimed cleanup', t => {
  const f = fixture(t);
  for (const status of ['planned', 'created', 'uncertain']) {
    const prepared = f.run(['prepare', '--scope', 'content', '--allow-fixture-writes']);
    complete(f, prepared, { cleanup: { status: 'complete', remaining: [] } });
    const journal = f.read(prepared.paths.journal);
    journal.resources.push({ key: 'fixture', kind: 'workbook', intent: 'Create a disposable fixture.', status, identity: null, location: null, job_id: null });
    f.write(prepared.paths.journal, journal);
    assert.equal(f.run(['next', '--run', prepared.run_id]).stop_reason, 'cleanup_incomplete');
  }
});

test('journal validation rejects malformed records and confirmed cleanup permits continuation', t => {
  const f = fixture(t);
  const prepared = f.run(['prepare', '--scope', 'content']);
  complete(f, prepared, { cleanup: { status: 'complete', remaining: [] } });
  const journal = f.read(prepared.paths.journal);
  journal.resources.push({ key: 'scratch', kind: 'local file', intent: 'Create an isolated local fixture.', status: 'removed', identity: null, location: 'scratch/example.json', job_id: null });
  f.write(prepared.paths.journal, { ...journal, resources: [{ ...journal.resources[0], status: 'claimed' }] });
  assert.throws(() => f.run(['next', '--run', prepared.run_id]), /journal/i);
  f.write(prepared.paths.journal, journal);
  assert.ok(f.run(['next', '--run', prepared.run_id]).spawn);
});

test('invalid run accounting cannot bypass bounded pass progression', t => {
  const f = fixture(t);
  const prepared = f.run(['prepare', '--scope', 'content']);
  const manifest = f.read(prepared.paths.run_manifest);
  manifest.active_pass = 100000;
  f.write(prepared.paths.run_manifest, manifest);
  assert.throws(() => f.run(['summarize']), /accounting/i);
  assert.throws(() => f.run(['next', '--run', prepared.run_id]), /accounting/i);
});

test('call, cleanup reserve, wall time, pass, and no-new-finding limits remain bounded', t => {
  const f = fixture(t);
  for (const [args, reportCalls, reason] of [
    [['--max-calls', '22'], 2, 'cleanup_reserve'],
    [['--max-calls', '22'], 22, 'max_calls'],
    [['--max-calls', '22'], 23, 'max_calls'],
    [[], 36, 'max_calls_per_pass'],
    [['--max-passes', '1'], 2, 'max_passes'],
    [['--stop-after-no-new-passes', '1'], 2, 'no_new_findings'],
  ]) {
    const prepared = f.run(['prepare', '--scope', 'content', ...args]);
    complete(f, prepared, { call_count: reportCalls });
    assert.equal(f.run(['next', '--run', prepared.run_id]).stop_reason, reason);
  }
  const timed = f.run(['prepare', '--scope', 'content', '--max-minutes', '1']);
  complete(f, timed);
  f.advance(60001);
  assert.equal(f.run(['next', '--run', timed.run_id]).stop_reason, 'max_minutes');
});

test('summaries retain incomplete runs and deduplicate exact normalized identities with provenance', t => {
  const f = fixture(t);
  const first = f.run(['prepare', '--scope', 'content workbook', '--max-passes', '1']);
  complete(f, first, { findings: [finding()] });
  f.run(['next', '--run', first.run_id]);
  const second = f.run(['prepare', '--scope', 'content workbook', '--max-passes', '1']);
  complete(f, second, { findings: [finding('  SCOPED output loses an identity  ')] });
  f.run(['next', '--run', second.run_id]);
  const third = f.run(['prepare', '--scope', 'content datasource']);
  const summary = f.run(['summarize']);
  const data = f.read(summary.paths.summary_json);
  assert.equal(data.findings.length, 1);
  assert.equal(data.findings[0].sources.length, 2);
  assert.equal(data.runs.find(run => run.run_id === third.run_id).status, 'pending');
  assert.match(fs.readFileSync(path.join(f.root, summary.paths.summary_md), 'utf8'), /self-reported/i);
  assert.equal(f.read(third.paths.report).status, 'pending');
});

test('secret-like material is rejected and never copied into summaries or errors', t => {
  const f = fixture(t);
  const prepared = f.run(['prepare', '--scope', 'content']);
  for (const secret of ['Authorization: Bearer SECRET-VALUE', '{"Authorization":"Bearer SECRET-VALUE"}', 'X-Tableau-Auth: SECRET-VALUE', 'personalAccessTokenSecret=SECRET-VALUE', '--token SECRET-VALUE']) {
    complete(f, prepared, { findings: [{ ...finding(), observed: secret }] });
    assert.throws(() => f.run(['next', '--run', prepared.run_id]), error => !error.message.includes('SECRET-VALUE'));
    const summary = f.run(['summarize']);
    assert.doesNotMatch(fs.readFileSync(path.join(f.root, summary.paths.summary_json), 'utf8'), /SECRET-VALUE/);
    assert.equal(f.read(summary.paths.summary_json).runs[0].status, 'invalid_report');
  }
});

test('symlinked artifact files cannot write outside the run tree', t => {
  const f = fixture(t);
  const outside = fs.mkdtempSync(path.join(os.tmpdir(), 'tadx-explore-outside-'));
  t.after(() => fs.rmSync(outside, { recursive: true, force: true }));
  f.run(['prepare', '--scope', 'content']);
  const summaryPath = path.join(f.root, '.tadx-explore/summary.json');
  const victim = path.join(outside, 'victim.json');
  fs.writeFileSync(victim, 'untouched');
  try {
    fs.symlinkSync(victim, summaryPath, 'file');
  } catch (error) {
    if (error.code === 'EPERM') { t.skip('Creating file symlinks requires platform permission.'); return; }
    throw error;
  }
  assert.throws(() => f.run(['summarize']), /symbolic|symlink/i);
  assert.equal(fs.readFileSync(victim, 'utf8'), 'untouched');
});

test('symlinked output roots cannot redirect artifact writes or reads', t => {
  const f = fixture(t);
  const outside = fs.mkdtempSync(path.join(os.tmpdir(), 'tadx-explore-outside-'));
  t.after(() => fs.rmSync(outside, { recursive: true, force: true }));
  const prepared = f.run(['prepare', '--scope', 'content']);
  fs.renameSync(path.join(f.root, '.tadx-explore'), path.join(f.root, 'saved'));
  fs.symlinkSync(outside, path.join(f.root, '.tadx-explore'), process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => f.run(['init']), /symbolic|symlink/i);
  assert.throws(() => f.run(['next', '--run', prepared.run_id]), /symbolic|symlink/i);
});
