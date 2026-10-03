// Exact owned-project reads; this policy grants no remote mutation.
const scalar = v => typeof v === 'string' && v.length > 0 && !/[\x00-\x1f\x7f]/.test(v);
const own = (v, key) => Object.hasOwn(v, key);
function validGuard(g) {
  return g && g.family === 'g9-project-read' && scalar(g.environment)
    && ['project.list', 'project.inspect'].includes(g.capability)
    && Array.isArray(g.owned_ids) && g.owned_ids.length > 0 && g.owned_ids.every(scalar)
    && new Set(g.owned_ids).size === g.owned_ids.length
    && g.owned_ids.includes(g.parent_id) && g.owned_ids.includes(g.target_id)
    && g.paths && typeof g.paths === 'object' && !Array.isArray(g.paths)
    && Object.entries(g.paths).every(([id, path]) => g.owned_ids.includes(id) && scalar(path));
}
function allowed(parsed, cap, state) {
  if (!state || !state.guardPresent || !state.baselinePresent || !state.baselineUnchanged
      || !validGuard(state.guard) || !parsed || !Array.isArray(parsed.words)
      || !Array.isArray(parsed.errors) || parsed.errors.length) return false;
  const g = state.guard, f = parsed.flags, words = parsed.words.join(' ');
  if (!f || typeof f !== 'object' || Array.isArray(f)
      || Object.values(f).some(v => v === null || typeof v === 'object')) return false;
  if (own(f, 'environment') ? f.environment !== g.environment :
      state.defaultEnvironment !== g.environment) return false;
  for (const key of ['json', 'full', 'help']) if (own(f, key) && typeof f[key] !== 'boolean') return false;
  const only = keys => Object.keys(f).every(k => ['environment', 'json', 'full', ...keys].includes(k));
  if (f.help === true && only(['help'])) return true;
  if (cap === 'version.get') return words === 'version' && only([]);
  if (cap === 'capability.list') return words === 'capability list' && only([]);
  if (cap === 'capability.get') return parsed.words.length === 3
    && parsed.words.slice(0, 2).join(' ') === 'capability get' && only([]);
  if (['auth.status', 'mutation.status'].includes(cap)) return words === cap.replace('.', ' ') && only([]);
  if (cap === 'project.list') return words === 'content project list' && only(['parent-id', 'limit'])
    && f['parent-id'] === g.parent_id
    && (!own(f, 'limit') || /^[1-9][0-9]*$/.test(String(f.limit)) && Number(f.limit) <= 25);
  if (cap === 'project.inspect') return words === 'content project inspect' && only(['id', 'project'])
    && (own(f, 'id') ? !own(f, 'project') && g.owned_ids.includes(f.id) :
      scalar(f.project) && Object.values(g.paths).includes(f.project));
  return false;
}
module.exports = {validGuard, allowed};
