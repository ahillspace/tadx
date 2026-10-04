// One root-only, memory-only credential initialization for a disposable CLI broker.
const fs = require('node:fs');
const net = require('node:net');
const path = require('node:path');

const DIRECTORY = '/tmp/tadx-private';
const SOCKET = path.posix.join(DIRECTORY, 'credential-init.sock');
const LIMIT = 16 * 1024;
let credentials = null;
let admitted = false;

function parse(bytes) {
  if (!Buffer.isBuffer(bytes) || bytes.length < 2 || bytes.length > LIMIT) {
    throw Error('Credential initialization has an invalid size');
  }
  const value = JSON.parse(bytes.toString('utf8'));
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw Error('Credential initialization is not a record');
  }
  const names = Object.keys(value);
  if (names.length < 2 || names.length > 16 ||
      !names.includes('TADX_BENCH_AGENT_PAT_NAME') ||
      !names.includes('TADX_BENCH_AGENT_PAT_SECRET')) {
    throw Error('Credential initialization has an invalid shape');
  }
  for (const name of names) {
    if (!/^TADX_BENCH_[A-Z0-9_]+_PAT_(NAME|SECRET)$/.test(name) ||
        typeof value[name] !== 'string' || !value[name] || value[name].length > 4096) {
      throw Error('Credential initialization has an invalid field');
    }
    const partner = name.endsWith('_NAME') ? name.slice(0, -5) + '_SECRET' :
      name.slice(0, -7) + '_NAME';
    if (!Object.hasOwn(value, partner)) {
      throw Error('Credential initialization has an incomplete pair');
    }
  }
  return Object.freeze(Object.assign(Object.create(null), value));
}

function start(directory = DIRECTORY) {
  if (credentials !== null) throw Error('Credential initialization already completed');
  fs.mkdirSync(directory, {mode: 0o700});
  const details = fs.lstatSync(directory);
  if (!details.isDirectory() || details.isSymbolicLink() ||
      (typeof process.getuid === 'function' && details.uid !== process.getuid())) {
    throw Error('Credential initialization directory is not owned by the broker');
  }
  fs.chmodSync(directory, 0o700);
  const socketPath = path.join(directory, 'credential-init.sock');
  const server = net.createServer(connection => {
    if (credentials !== null || admitted) {
      connection.end('refused');
      return;
    }
    admitted = true;
    let length = 0;
    const chunks = [];
    connection.setTimeout(5000, () => connection.destroy());
    connection.on('data', chunk => {
      length += chunk.length;
      if (length > LIMIT) connection.destroy();
      else chunks.push(chunk);
    });
    connection.on('end', () => {
      try {
        const accepted = parse(Buffer.concat(chunks));
        fs.unlinkSync(socketPath);
        credentials = accepted;
        connection.end('ready');
        server.close();
      } catch {
        connection.end('refused');
      }
    });
  });
  server.listen(socketPath, () => fs.chmodSync(socketPath, 0o600));
  return server;
}

function environment() {
  return credentials === null ? {} : {...credentials};
}

module.exports = {SOCKET, parse, start, environment};
