// Native before/after observation inside the private, offline CLI container.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const cp = require('node:child_process');

const binary = '/cli-state/update/tadx';
const guidanceRoot = '/tmp/cli-home/.codex/skills';
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');

function guidance(guidanceRootPath = guidanceRoot) {
  const files = {};
  if (!fs.existsSync(guidanceRootPath)) return files;
  function visit(dir) {
    for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
      const full = path.join(dir, entry.name);
      if (entry.isSymbolicLink()) throw Error('Guidance symlink is not accepted');
      if (entry.isDirectory()) visit(full);
      else if (entry.isFile()) files[path.relative(guidanceRootPath, full).split(path.sep).join('/')] = sha(fs.readFileSync(full));
      else throw Error('Guidance contains a non-file entry');
    }
  }
  visit(guidanceRootPath);
  return files;
}

function observe() {
  if (process.env.BENCH_NETWORK_MODE !== 'none') throw Error('Native observation needs the offline CLI boundary');
  const binarySha256 = sha(fs.readFileSync(binary));
  const execution = cp.spawnSync(binary, ['version', '--json'], {
    cwd: '/work', encoding: 'utf8', timeout: 30000,
    env: {PATH: '/usr/local/bin:/usr/bin:/bin', HOME: '/tmp/cli-home',
      XDG_CONFIG_HOME: '/cli-state', XDG_DATA_HOME: '/cli-state/data', TADX_FEEDBACK_MODE: 'off'},
  });
  let document = {};
  try { document = JSON.parse(execution.stdout || '{}'); } catch {}
  for (let depth = 0; depth < 4 && document && typeof document === 'object'; depth++) {
    if (typeof document.version === 'string') break;
    document = document.output || document.result || {};
  }
  return {binary_sha256: binarySha256, exit_status: execution.status,
    version: document.version, guidance: guidance(),
    native_stdout_sha256: sha(Buffer.from(execution.stdout || ''))};
}

if (require.main === module) {
  try { process.stdout.write(JSON.stringify(observe())); }
  catch (error) { process.stderr.write(String(error)); process.exitCode = 1; }
}
module.exports = {observe, guidance};
