// Fixed offline transport for one candidate-built TADX update fixture.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');

const repository = 'ahillspace/tadx';
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');

function appendAudit(context, row) {
  if (context.audit) fs.appendFileSync(context.audit, JSON.stringify(row) + '\n');
}

function verifiedManifest(context) {
  if (context.networkMode !== 'none') throw Error('offline boundary missing');
  if (!context.assets || fs.lstatSync(context.assets).isSymbolicLink()) throw Error('asset root invalid');
  const bytes = fs.readFileSync(path.join(context.assets, 'manifest.json'));
  if (!/^[0-9a-f]{64}$/.test(context.manifestSha256 || '') || sha(bytes) !== context.manifestSha256) {
    throw Error('manifest digest differs');
  }
  const manifest = JSON.parse(bytes);
  if (manifest.schema_version !== 1 || manifest.kind !== 'candidate-built-offline-fixture' ||
      manifest.published !== false || !/^[0-9a-f]{40}$/.test(context.sourceCommit || '') ||
      manifest.source_commit !== context.sourceCommit ||
      manifest.target_version !== `0.1.4-fixture.${context.sourceCommit.slice(0, 7)}` ||
      manifest.target_version !== context.targetVersion ||
      !manifest.files || typeof manifest.files !== 'object') {
    throw Error('candidate provenance differs');
  }
  return manifest;
}

function outputPath(destination, asset) {
  if (!path.isAbsolute(destination) || path.basename(destination) !== asset || fs.existsSync(destination)) {
    throw Error('destination is not a new exact asset path');
  }
  const parent = path.dirname(destination);
  const entry = fs.lstatSync(parent);
  if (!entry.isDirectory() || entry.isSymbolicLink() ||
      !/^tadx-install\.[A-Za-z0-9]+$/.test(path.basename(parent))) {
    throw Error('destination is not the installer temporary directory');
  }
  const relative = path.relative(os.tmpdir(), parent);
  if (!relative || relative.startsWith('..') || path.isAbsolute(relative)) {
    throw Error('destination escaped the isolated temporary root');
  }
}

function handle(kind, args, context) {
  let operation = 'denied';
  try {
    if (!Array.isArray(args) || args.some(arg => typeof arg !== 'string')) throw Error('invalid argv');
    const manifest = verifiedManifest(context);
    const version = manifest.target_version;
    const archive = `tadx_${version}_linux_amd64.tar.gz`;
    if (kind === 'gh' && JSON.stringify(args) === JSON.stringify(['release', 'view', '--repo', repository, '--json', 'tagName,url'])) {
      operation = 'release-view';
      appendAudit(context, {operation, status: 'served', tag: `v${version}`});
      return {status: 0, stdout: JSON.stringify({tagName: `v${version}`,
        url: `https://github.com/${repository}/releases/tag/v${version}`})};
    }
    if (kind === 'gh' && JSON.stringify(args) === JSON.stringify(['auth', 'status', '--hostname', 'github.com'])) {
      operation = 'auth-refused';
      appendAudit(context, {operation, status: 'refused'});
      return {status: 1, stdout: ''};
    }
    if (kind !== 'curl' || args.length !== 15 ||
        JSON.stringify(args.slice(0, 10)) !== JSON.stringify(['-sSL', '--proto', '=https', '--tlsv1.2',
          '--connect-timeout', '15', '--max-time', '120', '--max-filesize', '268435456']) ||
        args[10] !== '-o' || args[12] !== '-w' || args[13] !== '%{http_code}') {
      throw Error('unsupported transport request');
    }
    const asset = path.basename(args[11]);
    if (!['checksums.txt', archive].includes(asset)) throw Error('asset not allowlisted');
    const urls = [`https://github.com/${repository}/releases/download/${version}/${asset}`,
      `https://github.com/${repository}/releases/download/v${version}/${asset}`];
    if (!urls.includes(args[14])) throw Error('URL is not the pinned candidate');
    outputPath(args[11], asset);
    const source = path.join(context.assets, asset);
    if (fs.lstatSync(source).isSymbolicLink()) throw Error('asset symlink refused');
    const bytes = fs.readFileSync(source);
    const digest = sha(bytes);
    if (manifest.files[asset] !== digest) throw Error('asset bytes differ');
    const fd = fs.openSync(args[11], fs.constants.O_CREAT | fs.constants.O_EXCL |
      fs.constants.O_WRONLY | (fs.constants.O_NOFOLLOW || 0), 0o600);
    try { fs.writeFileSync(fd, bytes); } finally { fs.closeSync(fd); }
    operation = 'asset-served';
    appendAudit(context, {operation, status: 'served', asset, sha256: digest, url: args[14]});
    return {status: 0, stdout: '200'};
  } catch {
    appendAudit(context, {operation, status: 'refused'});
    return {status: 22, stdout: ''};
  }
}

function main(kind) {
  const outcome = handle(kind, process.argv.slice(2), {
    assets: process.env.BENCH_RELEASE_ASSETS_DIR,
    audit: process.env.BENCH_RELEASE_TRANSPORT_LOG,
    manifestSha256: process.env.BENCH_RELEASE_MANIFEST_SHA,
    sourceCommit: process.env.BENCH_RELEASE_SOURCE_COMMIT,
    targetVersion: process.env.BENCH_RELEASE_TARGET_VERSION,
    networkMode: process.env.BENCH_NETWORK_MODE,
  });
  process.stdout.write(outcome.stdout);
  process.exitCode = outcome.status;
}

module.exports = {handle, main};
