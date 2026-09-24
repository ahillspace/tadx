#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const defaults = JSON.parse(fs.readFileSync(path.join(scriptDirectory, '../defaults.json'), 'utf8'));
const output = '.tadx-explore';
const instructionPath = '.agents/skills/tadx-explore/references/worker.md';
const statuses = ['pending', 'completed', 'blocked', 'interrupted', 'uncertain'];
const kinds = ['bug', 'friction', 'unexpected_behavior', 'documentation', 'token_waste', 'edge_case'];
const reportFields = ['schema_version', 'run_id', 'pass_id', 'status', 'tadx_version', 'repository_revision', 'call_count', 'coverage', 'findings', 'blockers', 'cleanup'];
const coverageFields = ['action', 'variations', 'outcome', 'note'];
const findingFields = ['kind', 'title', 'severity', 'confidence', 'observed', 'expected', 'impact', 'reproducer', 'recommendation', 'error'];
const errorFields = ['exit_code', 'code', 'phase', 'outcome', 'excerpt'];
const resourceFields = ['key', 'kind', 'intent', 'status', 'identity', 'location', 'job_id'];
const outcomes = ['success', 'failure', 'blocked', 'skipped'];
const severities = ['low', 'medium', 'high', 'critical'];
const confidenceLevels = ['low', 'medium', 'high'];
const cleanupStatuses = ['not_needed', 'complete', 'incomplete', 'uncertain'];
const resourceStatuses = ['planned', 'created', 'removed', 'not_created', 'uncertain'];
const credentialPattern = /(?:authorization["']?\s*:\s*["']?(?:bearer|basic)\s+\S+|(?:x-tableau-auth|personal[ _-]?access[ _-]?token(?:[ _-]?(?:secret|name))?|access[ _-]?token|session[ _-]?token|password|client[ _-]?secret|pat)["']?\s*[:=]\s*["']?[^\s"']+|--(?:token|pat|password|token-secret|personal-access-token)\s+\S+|https?:\/\/[^\s/:]+:[^\s/@]+@)/i;

function requireThat(condition, message) {
  if (!condition) throw new Error(message);
}

function object(value, keys, label) {
  requireThat(value !== null && typeof value === 'object' && !Array.isArray(value), `${label}: expected an object.`);
  requireThat(Object.keys(value).every(key => keys.includes(key)), `${label}: unsupported field.`);
}

function string(value, label, max = 4000, nullable = false) {
  if (nullable && value === null) return;
  requireThat(typeof value === 'string' && value.trim().length > 0 && value.length <= max, `${label}: expected bounded nonempty text.`);
  requireThat(!credentialPattern.test(value), `${label}: possible credential material; remove it from the local artifact.`);
}

function array(value, label, max = 200) {
  requireThat(Array.isArray(value) && value.length <= max, `${label}: expected a bounded array.`);
}

function integer(value, label, min = 0) {
  requireThat(Number.isSafeInteger(value) && value >= min, `${label}: expected an integer of at least ${min}.`);
}

function member(value, choices, label) {
  requireThat(choices.includes(value), `${label}: unsupported value.`);
}

function scope(value) {
  requireThat(typeof value === 'string' && /^[a-z][a-z0-9-]*(?: +[a-z][a-z0-9-]*){0,2}$/.test(value.trim()), 'Scope must contain one to three canonical lowercase command words.');
  return value.trim().split(/ +/).join(' ');
}

function settings(value) {
  object(value, Object.keys(defaults), 'Settings');
  for (const key of ['environment', 'workspace', 'project_id']) string(value[key], `Settings ${key}`, 200, true);
  for (const key of ['max_calls', 'max_calls_per_pass', 'max_minutes', 'max_passes', 'stop_after_no_new_passes']) integer(value[key], `Settings ${key}`, 1);
  integer(value.cleanup_reserve, 'Settings cleanup_reserve');
  requireThat(value.cleanup_reserve < Math.min(value.max_calls, value.max_calls_per_pass), 'Settings cleanup_reserve must leave at least one exploration call.');
  return value;
}

function findingKey(runScope, finding) {
  return JSON.stringify([runScope, finding.kind, finding.title.trim().replace(/\s+/g, ' ').toLowerCase()]);
}

function validateReport(report, runId, passId) {
  const label = 'Report';
  object(report, reportFields, label);
  requireThat(report.schema_version === 1 && report.run_id === runId && report.pass_id === passId, 'Report identity or schema version does not match the pass.');
  member(report.status, statuses, 'Report status');
  string(report.tadx_version, 'Report tadx_version', 200, true);
  string(report.repository_revision, 'Report repository_revision', 200, true);
  integer(report.call_count, 'Report call_count');
  array(report.coverage, 'Report coverage');
  for (const coverage of report.coverage) {
    object(coverage, coverageFields, 'Report coverage');
    string(coverage.action, 'Report coverage action', 200);
    array(coverage.variations, 'Report variations', 50);
    for (const variation of coverage.variations) string(variation, 'Report variation', 1000);
    member(coverage.outcome, outcomes, 'Report coverage outcome');
    string(coverage.note, 'Report coverage note');
  }
  array(report.findings, 'Report findings', 100);
  for (const finding of report.findings) {
    object(finding, findingFields, 'Report finding');
    member(finding.kind, kinds, 'Report finding kind');
    member(finding.severity, severities, 'Report finding severity');
    member(finding.confidence, confidenceLevels, 'Report finding confidence');
    for (const key of ['title', 'observed', 'expected', 'impact', 'reproducer', 'recommendation']) string(finding[key], `Report finding ${key}`, key === 'title' ? 200 : 4000);
    if (finding.error !== undefined) {
      object(finding.error, errorFields, 'Report error');
      for (const [key, value] of Object.entries(finding.error)) {
        if (key === 'exit_code') requireThat(Number.isSafeInteger(value), 'Report error exit_code must be an integer.');
        else string(value, `Report error ${key}`);
      }
    }
  }
  array(report.blockers, 'Report blockers', 50);
  for (const blocker of report.blockers) string(blocker, 'Report blocker');
  object(report.cleanup, ['status', 'remaining'], 'Report cleanup');
  member(report.cleanup.status, cleanupStatuses, 'Report cleanup status');
  array(report.cleanup.remaining, 'Report cleanup remaining');
  for (const resource of report.cleanup.remaining) {
    object(resource, ['identity', 'location', 'note'], 'Report remaining resource');
    string(resource.identity, 'Report remaining identity', 1000, true);
    string(resource.location, 'Report remaining location', 1000, true);
    string(resource.note, 'Report remaining note');
  }
  return report;
}

function validateJournal(journal, runId, passId) {
  object(journal, ['schema_version', 'run_id', 'pass_id', 'resources'], 'Journal');
  requireThat(journal.schema_version === 1 && journal.run_id === runId && journal.pass_id === passId, 'Journal identity or schema version does not match the pass.');
  array(journal.resources, 'Journal resources');
  const keys = new Set();
  for (const resource of journal.resources) {
    object(resource, resourceFields, 'Journal resource');
    for (const key of ['key', 'kind', 'intent']) string(resource[key], `Journal ${key}`);
    requireThat(!keys.has(resource.key), 'Journal resource keys must be unique.');
    keys.add(resource.key);
    member(resource.status, resourceStatuses, 'Journal resource status');
    for (const key of ['identity', 'location', 'job_id']) string(resource[key], `Journal ${key}`, 1000, true);
  }
  return journal;
}

function parse(args) {
  const [command, ...rest] = args;
  member(command, ['init', 'prepare', 'next', 'summarize'], 'Command');
  const options = {};
  const settingOptions = Object.keys(defaults).map(key => key.replaceAll('_', '-'));
  const allowed = command === 'init' ? settingOptions : command === 'prepare' ? [...settingOptions, 'scope', 'allow-fixture-writes'] : ['run'];
  for (let index = 0; index < rest.length; index++) {
    const argument = rest[index];
    requireThat(argument.startsWith('--') && allowed.includes(argument.slice(2)), 'Unsupported option for this command.');
    const key = argument.slice(2).replaceAll('-', '_');
    requireThat(options[key] === undefined, 'Duplicate option.');
    if (key === 'allow_fixture_writes') options[key] = true;
    else {
      const value = rest[++index];
      requireThat(value !== undefined && !value.startsWith('--'), 'Option requires a value.');
      options[key] = typeof defaults[key] === 'number' ? Number(value) : value;
    }
  }
  return { command, options };
}

export function createLauncher(root, { now = () => Date.now(), revision = null } = {}) {
  root = fs.realpathSync(root);

  // Reject redirected output paths before either reading or replacing artifacts.
  function safe(relative) {
    requireThat(typeof relative === 'string' && relative.startsWith(`${output}/`) && !relative.includes('\\'), 'Artifact path must remain inside the exploration directory.');
    const parts = relative.split('/');
    requireThat(parts.every(part => part && part !== '.' && part !== '..'), 'Artifact path contains an invalid component.');
    let current = root;
    for (const part of parts) {
      current = path.join(current, part);
      try { requireThat(!fs.lstatSync(current).isSymbolicLink(), 'Artifact path contains a symbolic link.'); }
      catch (error) { if (error.code !== 'ENOENT') throw error; }
    }
    return current;
  }

  function read(relative) {
    const target = safe(relative);
    requireThat(fs.statSync(target).size <= 2_000_000, 'Artifact exceeds the two-megabyte read limit.');
    try { return JSON.parse(fs.readFileSync(target, 'utf8')); }
    catch (error) {
      if (error instanceof SyntaxError) throw new Error('Artifact contains invalid JSON.');
      throw error;
    }
  }

  function write(relative, value, exclusive = false) {
    const target = safe(relative);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    safe(relative);
    const body = typeof value === 'string' ? value : `${JSON.stringify(value, null, 2)}\n`;
    if (exclusive) fs.writeFileSync(target, body, { flag: 'wx', mode: 0o600 });
    else {
      const temporary = `${relative}.${randomUUID()}.tmp`;
      fs.writeFileSync(safe(temporary), body, { flag: 'wx', mode: 0o600 });
      try { safe(relative); fs.renameSync(safe(temporary), target); }
      finally { if (fs.existsSync(safe(temporary))) fs.unlinkSync(safe(temporary)); }
    }
  }

  function runPath(id) {
    requireThat(typeof id === 'string' && /^\d{14}-[a-f0-9]{12}$/.test(id), 'Invalid run ID.');
    return `${output}/runs/${id}`;
  }

  function paths(id, passId) {
    const base = runPath(id);
    const pass = `${base}/passes/${passId}`;
    return { run_manifest: `${base}/manifest.json`, pass_manifest: `${pass}/manifest.json`, report_contract: `${base}/report-contract.json`, report: `${pass}/report.json`, journal: `${pass}/journal.json`, evidence: `${pass}/evidence`, summary_json: `${base}/summary.json`, summary_md: `${base}/summary.md` };
  }

  function loadRun(id) {
    const run = read(`${runPath(id)}/manifest.json`);
    requireThat(run.schema_version === 1 && run.run_id === id && Number.isSafeInteger(run.active_pass) && run.active_pass >= 1, 'Invalid run manifest.');
    settings(run.settings);
    scope(run.scope);
    array(run.processed, 'Run processed passes', run.settings.max_passes);
    member(run.status, ['active', 'stopped'], 'Run status');
    requireThat(run.active_pass <= run.settings.max_passes && run.processed.length === run.active_pass - (run.status === 'active' ? 1 : 0), 'Run pass accounting is inconsistent.');
    integer(run.no_new_passes, 'Run no_new_passes');
    for (let index = 0; index < run.processed.length; index++) {
      const item = run.processed[index];
      object(item, ['pass_id', 'status', 'call_count', 'finding_keys'], 'Run processed pass');
      requireThat(item.pass_id === String(index + 1).padStart(2, '0'), 'Run processed pass sequence is inconsistent.');
      member(item.status, statuses.filter(status => status !== 'pending'), 'Run processed status');
      integer(item.call_count, 'Run processed call_count');
      array(item.finding_keys, 'Run finding identities', 100);
      for (const key of item.finding_keys) string(key, 'Run finding identity', 1000);
    }
    requireThat(Number.isFinite(Date.parse(run.started_at)) && Number.isFinite(Date.parse(run.deadline)), 'Invalid run time bounds.');
    return run;
  }

  function counts(run) {
    return { completed_passes: run.processed.filter(item => item.status === 'completed').length, reported_passes: run.processed.length, reported_calls: run.processed.reduce((sum, item) => sum + item.call_count, 0), findings: new Set(run.processed.flatMap(item => item.finding_keys)).size };
  }

  function brief(run, spawn = false) {
    const passId = String(run.active_pass).padStart(2, '0');
    const result = { run_id: run.run_id, pass_id: passId, status: run.status === 'stopped' ? 'stopped' : 'pending', stop_reason: run.stop_reason, counts: counts(run), paths: paths(run.run_id, passId) };
    if (!spawn && run.status === 'active') result.stop_reason = 'awaiting_report';
    if (spawn) result.spawn = {
      task_name: `explore_${run.run_id.replaceAll('-', '_')}_${passId}`,
      model: 'gpt-6-luna', reasoning_effort: 'medium', fork_turns: 'none',
      message: `Read ${instructionPath}, then ${result.paths.run_manifest} and ${result.paths.pass_manifest}. Authority is limited to this run; fixture writes require its explicit authorization. Never change consent, auth, policy, shared content, or installed tools.`,
    };
    return result;
  }

  function makePass(run, previousBrief) {
    const passId = String(run.active_pass).padStart(2, '0');
    const locations = paths(run.run_id, passId);
    const remaining = Math.max(0, run.settings.max_calls - counts(run).reported_calls);
    const limit = Math.min(remaining, run.settings.max_calls_per_pass);
    if (run.active_pass === 1) write(locations.report_contract, {
      schema_version: 1,
      guidance: 'Edit the generated report and journal drafts. All fields listed as required must appear. Omit optional fields when unavailable. Use null only for explicitly nullable fields. Do not include credentials. Coverage and call counts are worker accounts, not telemetry.',
      report: {
        required_fields: reportFields, status: statuses,
        nullable_fields: ['tadx_version', 'repository_revision'], call_count: 'Nonnegative integer, including every TADX invocation for setup, help, exploration, reproduction, and cleanup.',
        coverage: { required_fields: coverageFields, action: 'Command words', variations: 'Array of strings', outcome: outcomes, note: 'Observed outcome and coverage limits' },
        findings: { required_fields: findingFields.filter(field => field !== 'error'), optional_fields: ['error'], kind: kinds, severity: severities, confidence: confidenceLevels, text_fields: ['title', 'observed', 'expected', 'impact', 'reproducer', 'recommendation'], error: { optional_fields: errorFields, exit_code: 'Integer', other_fields: 'Strings; sanitized excerpt at most 4000 characters' } },
        blockers: 'Array of strings, including why version or revision evidence is unavailable.',
        cleanup: { required_fields: ['status', 'remaining'], status: cleanupStatuses, remaining: { required_fields: ['identity', 'location', 'note'], nullable_fields: ['identity', 'location'] } },
      },
      journal: { required_fields: ['schema_version', 'run_id', 'pass_id', 'resources'], resources: { required_fields: resourceFields, status: resourceStatuses, nullable_fields: ['identity', 'location', 'job_id'], guidance: 'Record intent before creation; preserve confirmed identities and jobs; only removed or not_created entries permit continuation.' } },
      limits: { text_characters: 4000, title_and_action_characters: 200, version_and_revision_characters: 200, variation_characters: 1000, identity_and_location_characters: 1000, coverage_items: 200, variations_per_item: 50, findings: 100, blockers: 50, resources: 200, report_bytes: 2000000 },
    }, true);
    write(locations.pass_manifest, {
      schema_version: 1, run_id: run.run_id, pass_id: passId, scope: run.scope,
      budget: { remaining_calls: remaining, call_limit: limit, cleanup_reserve: run.settings.cleanup_reserve, exploration_calls: Math.max(0, limit - run.settings.cleanup_reserve), deadline: run.deadline },
      paths: locations, previous_brief: previousBrief,
    }, true);
    write(locations.report, {
      schema_version: 1, run_id: run.run_id, pass_id: passId, status: 'pending', tadx_version: null,
      repository_revision: run.repository_revision, call_count: 0, coverage: [], findings: [], blockers: [],
      cleanup: { status: 'not_needed', remaining: [] },
    }, true);
    write(locations.journal, { schema_version: 1, run_id: run.run_id, pass_id: passId, resources: [] }, true);
    fs.mkdirSync(safe(locations.evidence), { recursive: true });
  }

  function mergeSettings(options) {
    const filename = `${output}/settings.json`;
    const saved = fs.existsSync(safe(filename)) ? read(filename) : {};
    object(saved, Object.keys(defaults), 'Settings');
    return settings({ ...defaults, ...saved, ...Object.fromEntries(Object.entries(options).filter(([key]) => key in defaults)) });
  }

  function summarize(selectedId) {
    let ids;
    if (selectedId) ids = [selectedId];
    else {
      const directory = safe(`${output}/runs`);
      ids = fs.existsSync(directory) ? fs.readdirSync(directory).filter(id => /^\d{14}-[a-f0-9]{12}$/.test(id)).sort() : [];
    }
    const summary = { schema_version: 1, accounting: 'Worker self-reported calls and coverage; not telemetry or exhaustive testing.', runs: [], findings: [] };
    const grouped = new Map();
    for (const id of ids) {
      const run = loadRun(id);
      const entry = { run_id: id, scope: run.scope, status: run.status === 'stopped' ? 'stopped' : 'pending', stop_reason: run.stop_reason, counts: counts(run), passes: [] };
      for (let number = 1; number <= run.active_pass; number++) {
        const passId = String(number).padStart(2, '0');
        const locations = paths(id, passId);
        try {
          const report = validateReport(read(locations.report), id, passId);
          const journal = validateJournal(read(locations.journal), id, passId);
          entry.passes.push({ pass_id: passId, status: report.status, report: locations.report, tadx_version: report.tadx_version, repository_revision: report.repository_revision, reported_calls: report.call_count, coverage: report.coverage, blockers: report.blockers, cleanup: report.cleanup, unresolved_resources: journal.resources.filter(resource => !['removed', 'not_created'].includes(resource.status)) });
          if (number === run.active_pass && run.status !== 'stopped') entry.status = report.status === 'pending' ? 'pending' : 'awaiting_next';
          for (const finding of report.findings) {
            const key = findingKey(run.scope, finding);
            if (!grouped.has(key)) grouped.set(key, { scope: run.scope, kind: finding.kind, title: finding.title, sources: [] });
            grouped.get(key).sources.push({ run_id: id, pass_id: passId, report: locations.report, report_status: report.status, ...finding });
          }
        } catch (error) {
          entry.status = 'invalid_report';
          entry.passes.push({ pass_id: passId, status: 'invalid_report', report: locations.report, issue: 'Report or journal is missing, invalid, unsafe, or contains possible credential material.' });
        }
      }
      summary.runs.push(entry);
    }
    summary.findings = [...grouped.values()];
    const base = selectedId ? runPath(selectedId) : output;
    const summaryPaths = { summary_json: `${base}/summary.json`, summary_md: `${base}/summary.md` };
    const lines = ['# Exploration summary', '', summary.accounting, '', '## Runs', ''];
    for (const run of summary.runs) {
      lines.push(`- ${run.run_id}: ${run.scope}; ${run.status}; ${run.stop_reason ?? 'no terminal reason'}.`);
      for (const pass of run.passes) {
        lines.push(`  - Pass ${pass.pass_id}: ${pass.status}; report: ${pass.report}.`);
        if (pass.cleanup) lines.push(`    Cleanup: ${pass.cleanup.status}; remaining resources: ${pass.cleanup.remaining.length + pass.unresolved_resources.length}.`);
        if (pass.blockers?.length) lines.push(...pass.blockers.map(blocker => `    Blocker: ${blocker}`));
      }
    }
    lines.push('', '## Findings', '');
    for (const finding of summary.findings) {
      lines.push(`### ${finding.title}`, '', `Scope: ${finding.scope}. Kind: ${finding.kind}.`, '');
      for (const source of finding.sources) {
        lines.push(`Source: ${source.report} (${source.severity}, ${source.confidence} confidence).`, '', `Observed: ${source.observed}`, '', `Expected: ${source.expected}`, '', `Impact: ${source.impact}`, '', `Reproducer: ${source.reproducer}`, '', `Recommendation: ${source.recommendation}`, '');
        if (source.error) lines.push('Error excerpt and fields:', '', '```json', JSON.stringify(source.error, null, 2), '```', '');
      }
    }
    // Check both targets before replacing either summary.
    safe(summaryPaths.summary_json); safe(summaryPaths.summary_md);
    write(summaryPaths.summary_json, summary);
    write(summaryPaths.summary_md, `${lines.join('\n')}\n`);
    return { status: 'summarized', counts: { runs: summary.runs.length, findings: summary.findings.length }, paths: summaryPaths };
  }

  return args => {
    const { command, options } = parse(args);
    if (command === 'init') {
      const filename = `${output}/settings.json`;
      if (fs.existsSync(safe(filename))) return { status: 'existing', paths: { settings: filename } };
      write(filename, settings({ ...defaults, ...options }), true);
      return { status: 'initialized', paths: { settings: filename } };
    }
    if (command === 'summarize') return summarize(options.run);
    if (command === 'prepare') {
      const selectedScope = scope(options.scope);
      const selectedSettings = mergeSettings(options);
      requireThat(!options.allow_fixture_writes || selectedSettings.environment !== null, 'Fixture writes require an explicitly selected environment.');
      const timestamp = now();
      const id = `${new Date(timestamp).toISOString().replace(/\D/g, '').slice(0, 14)}-${randomUUID().replaceAll('-', '').slice(0, 12)}`;
      const run = { schema_version: 1, run_id: id, scope: selectedScope, settings: selectedSettings, authority: { allow_fixture_writes: options.allow_fixture_writes === true }, repository_revision: revision, started_at: new Date(timestamp).toISOString(), deadline: new Date(timestamp + selectedSettings.max_minutes * 60000).toISOString(), active_pass: 1, processed: [], no_new_passes: 0, status: 'active', stop_reason: null };
      write(`${runPath(id)}/manifest.json`, run, true);
      makePass(run, null);
      return brief(run, true);
    }
    const run = loadRun(options.run);
    if (run.status === 'stopped') return brief(run);
    const passId = String(run.active_pass).padStart(2, '0');
    const locations = paths(run.run_id, passId);
    const report = validateReport(read(locations.report), run.run_id, passId);
    const journal = validateJournal(read(locations.journal), run.run_id, passId);
    if (report.status === 'pending') return brief(run);
    const known = new Set(run.processed.flatMap(item => item.finding_keys));
    const keys = [...new Set(report.findings.map(finding => findingKey(run.scope, finding)))];
    run.no_new_passes = keys.some(key => !known.has(key)) ? 0 : run.no_new_passes + 1;
    run.processed.push({ pass_id: passId, status: report.status, call_count: report.call_count, finding_keys: keys });
    const total = counts(run).reported_calls;
    let reason = null;
    if (['blocked', 'interrupted', 'uncertain'].includes(report.status)) reason = report.status;
    else if (report.blockers.length) reason = 'blocker';
    else if (!['not_needed', 'complete'].includes(report.cleanup.status) || report.cleanup.remaining.length || journal.resources.some(resource => !['removed', 'not_created'].includes(resource.status))) reason = 'cleanup_incomplete';
    else if (report.tadx_version === null || report.repository_revision === null) reason = 'missing_version_evidence';
    else if (total >= run.settings.max_calls) reason = 'max_calls';
    else if (report.call_count > run.settings.max_calls_per_pass) reason = 'max_calls_per_pass';
    else if (run.settings.max_calls - total <= run.settings.cleanup_reserve) reason = 'cleanup_reserve';
    else if (now() >= Date.parse(run.deadline)) reason = 'max_minutes';
    else if (run.active_pass >= run.settings.max_passes) reason = 'max_passes';
    else if (run.no_new_passes >= run.settings.stop_after_no_new_passes) reason = 'no_new_findings';
    if (reason) {
      run.status = 'stopped'; run.stop_reason = reason;
    } else {
      run.active_pass++;
      makePass(run, { coverage: report.coverage.map(({ action, variations, outcome }) => ({ action, variations, outcome })), findings: report.findings.map(({ kind, title }) => ({ kind, title })) });
    }
    write(locations.run_manifest, run);
    summarize(run.run_id);
    return brief(run, !reason);
  };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const root = path.resolve(scriptDirectory, '../../../..');
    let revision = null;
    try { revision = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim(); } catch { /* A worker can report missing repository metadata. */ }
    process.stdout.write(`${JSON.stringify(createLauncher(root, { revision })(process.argv.slice(2)))}\n`);
  } catch (error) {
    process.stderr.write(`${JSON.stringify({ error: error.message })}\n`);
    process.exitCode = 1;
  }
}
