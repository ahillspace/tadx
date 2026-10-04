// Narrow broker admission for one independently owned Tableau job.
const actions = new Set(['job.inspect', 'job.wait', 'job.cancel']);
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const hash = /^[0-9a-f]{64}$/;
const scalar = value => typeof value === 'string' && value.length > 0 &&
  !/[\x00-\x1f\x7f]/.test(value);

function validGuard(guard) {
  return guard && guard.family === 'job' && actions.has(guard.action) &&
    uuid.test(guard.job_id) && uuid.test(guard.site_luid) &&
    scalar(guard.environment) && scalar(guard.site) && scalar(guard.run_id) &&
    guard.case_id === 'P-' + guard.action.replace('.', '-') &&
    hash.test(guard.ownership_sha256) && hash.test(guard.baseline_sha256);
}

function extendParser(parser) {
  parser.values.add('site');
}

function allowed(parsed, capability, state) {
  if (!state || state.guardPresent !== true || state.baselinePresent !== true ||
      state.baselineUnchanged !== true || state.jobOwned !== true ||
      !validGuard(state.guard) || !parsed || !Array.isArray(parsed.words) ||
      !Array.isArray(parsed.errors) || parsed.errors.length) return false;
  const guard = state.guard;
  if (!actions.has(capability)) return false;
  if (capability !== guard.action &&
      !(guard.action === 'job.wait' && capability === 'job.inspect') &&
      !(guard.action === 'job.cancel' && ['job.inspect', 'job.wait'].includes(capability))) return false;
  if (parsed.words.length !== 2 || parsed.words[0] !== 'job' ||
      parsed.words[1] !== capability.split('.')[1]) return false;
  const flags = parsed.flags;
  if (!flags || typeof flags !== 'object' || Array.isArray(flags) ||
      Object.keys(flags).some(key => !['environment','site','id','json','full','preview'].includes(key))) return false;
  if (flags.environment !== guard.environment || flags.site !== guard.site ||
      flags.id !== guard.job_id || flags.json !== true || flags.full !== true) return false;
  if (Object.hasOwn(flags, 'preview') &&
      (capability !== 'job.cancel' || flags.preview !== true)) return false;
  if (capability === 'job.cancel' &&
      (state.jobFresh !== true ||
       (flags.preview !== true && state.cancelAttempted !== false))) return false;
  return true;
}

module.exports = {validGuard, extendParser, allowed};
