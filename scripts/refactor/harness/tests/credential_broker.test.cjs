const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const broker = require('../source/integration/credential_broker.cjs');

const secret = 'SENTINEL-NOT-A-REAL-PAT';
const payload = Buffer.from(JSON.stringify({
  TADX_BENCH_AGENT_PAT_NAME: 'fixture-name',
  TADX_BENCH_AGENT_PAT_SECRET: secret,
}));

test('bounded PAT pair accepts only credential fields', () => {
  assert.equal(broker.parse(payload).TADX_BENCH_AGENT_PAT_SECRET, secret);
  assert.throws(() => broker.parse(Buffer.from('{}')));
  assert.throws(() => broker.parse(Buffer.alloc(16 * 1024 + 1)));
  assert.throws(() => broker.parse(Buffer.from(JSON.stringify({
    TADX_BENCH_AGENT_PAT_NAME: 'fixture-name',
    TADX_BENCH_AGENT_PAT_SECRET: secret,
    PATH: '/unsafe',
  }))));
  assert.throws(() => broker.parse(Buffer.from(JSON.stringify({
    TADX_BENCH_AGENT_PAT_NAME: 'fixture-name',
    TADX_BENCH_AGENT_PAT_SECRET: secret,
    TADX_BENCH_DEST_PAT_NAME: 'unpaired',
  }))));
});

test('one root-owned socket moves a fake PAT into memory only',
  {skip: process.platform === 'win32' ? 'POSIX socket mode is verified on Linux CI' : false},
  async () => {
    const parent = fs.mkdtempSync(path.join(os.tmpdir(), 'tadx-credential-test-'));
    const directory = path.join(parent, 'private');
    try {
      const server = broker.start(directory);
      await new Promise(resolve => server.once('listening', resolve));
      const socketPath = path.join(directory, 'credential-init.sock');
      assert.equal(fs.statSync(directory).mode & 0o777, 0o700);
      assert.equal(fs.statSync(socketPath).mode & 0o777, 0o600);
      const reply = await new Promise((resolve, reject) => {
        const client = net.createConnection(socketPath);
        let answer = '';
        client.on('connect', () => client.end(payload));
        client.on('data', bytes => { answer += bytes.toString('utf8'); });
        client.on('error', reject);
        client.on('end', () => resolve(answer));
      });
      assert.equal(reply, 'ready');
      assert.equal(fs.existsSync(socketPath), false);
      assert.deepEqual(broker.environment(), {
        TADX_BENCH_AGENT_PAT_NAME: 'fixture-name',
        TADX_BENCH_AGENT_PAT_SECRET: secret,
      });
      assert.equal(fs.readdirSync(directory).length, 0);
      assert.throws(() => broker.start(path.join(parent, 'again')));
    } finally {
      fs.rmSync(parent, {recursive: true, force: true});
    }
  });
