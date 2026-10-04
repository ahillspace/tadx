'use strict';

const ACTION = 'policy.install';
const OUTPUT = '/etc/tadx';

function allowed(parsed, capability, state) {
  const guard = state?.guard;
  if (capability !== ACTION || guard?.family !== 'g9-policy-install' ||
      guard.action !== ACTION || guard.network_none !== true ||
      guard.machine_policy_absent !== true || guard.root_overlay_only !== true ||
      state.networkNone !== true ||
      state.baselinePresent !== true || state.baselineUnchanged !== true ||
      state.guardPresent !== true || !parsed || !Array.isArray(parsed.words) ||
      !Array.isArray(parsed.errors) || parsed.errors.length || !parsed.flags ||
      Array.isArray(parsed.flags)) return false;
  const flags = parsed.flags;
  if (Object.values(flags).some(value => Array.isArray(value) || value === null || typeof value === 'object')) return false;
  if (parsed.words.join(' ') !== 'policy install') return false;
  if (Object.keys(flags).sort().join(',') !== 'full,json,output,template') return false;
  return flags.full === true && flags.json === true &&
    flags.output === OUTPUT && flags.template === 'read-only';
}

module.exports = {allowed, ACTION, OUTPUT};
