// Strict local-policy admission for a disposable non-privileged worker.
const fs = require('node:fs');
const crypto = require('node:crypto');
const actions = new Set(['policy.samples', 'policy.validate', 'policy.status']);
const scalar = value => typeof value === 'string' && value.length > 0 &&
  !/[\x00-\x1f\x7f]/.test(value);
const hash = value => typeof value === 'string' && /^[0-9a-f]{64}$/.test(value);

function validGuard(guard) {
  return guard && guard.family === 'g9-local-policy' && actions.has(guard.action) &&
    scalar(guard.run_id) && guard.case_id === 'P-' + guard.action.replace('.', '-') &&
    guard.network_none === true && guard.preflight_verified === true &&
    guard.machine_policy_absent === true && hash(guard.sentinel_sha256) &&
    (guard.action !== 'policy.validate' || hash(guard.candidate_sha256)) &&
    (guard.action !== 'policy.samples' || guard.output_path === '/cli-state/g9-policy-private/candidates') &&
    (guard.action !== 'policy.validate' || guard.candidate_path === '/cli-state/g9-policy-private/candidate.json');
}

function regular(io, name, expected) {
  const info = io.lstatSync(name);
  return info.isFile() && !info.isSymbolicLink() &&
    crypto.createHash('sha256').update(io.readFileSync(name)).digest('hex') === expected;
}

function absent(io, name) {
  try { io.lstatSync(name); return false; }
  catch (error) { return error.code === 'ENOENT'; }
}

function fileBoundary(guard, io) {
  try {
    const privateRoot = io.lstatSync('/cli-state/g9-policy-private');
    if (!privateRoot.isDirectory() || privateRoot.isSymbolicLink() ||
        io.realpathSync('/cli-state/g9-policy-private') !== '/cli-state/g9-policy-private' ||
        !regular(io, '/cli-state/g9-policy-private/sentinel.txt', guard.sentinel_sha256) ||
        !absent(io, '/etc/tadx-policy-location.json') ||
        !absent(io, '/etc/tadx/managed-policy.json')) return false;
    if (guard.action === 'policy.samples') return absent(io, guard.output_path);
    if (guard.action === 'policy.validate')
      return regular(io, guard.candidate_path, guard.candidate_sha256);
    return true;
  } catch { return false; }
}

function allowed(parsed, capability, state, io = fs) {
  if (!state || state.guardPresent !== true || state.baselinePresent !== true ||
      state.baselineUnchanged !== true || state.networkNone !== true ||
      !validGuard(state.guard) || !fileBoundary(state.guard, io) ||
      !parsed || !Array.isArray(parsed.errors) ||
      parsed.errors.length || !Array.isArray(parsed.words)) return false;
  const guard = state.guard;
  if (capability !== guard.action) return false;
  const words = parsed.words;
  const flags = parsed.flags;
  if (!flags || typeof flags !== 'object' || Array.isArray(flags) ||
      flags.json !== true || flags.full !== true) return false;
  if (capability === 'policy.samples')
    return words.length === 2 && words[0] === 'policy' && words[1] === 'samples' &&
      Object.keys(flags).sort().join(',') === 'full,json,output' &&
      flags.output === guard.output_path;
  if (capability === 'policy.validate')
    return words.length === 3 && words[0] === 'policy' && words[1] === 'validate' &&
      words[2] === guard.candidate_path &&
      Object.keys(flags).sort().join(',') === 'full,json';
  return words.length === 2 && words[0] === 'policy' && words[1] === 'status' &&
    Object.keys(flags).sort().join(',') === 'full,json';
}

module.exports = {validGuard, fileBoundary, allowed};
