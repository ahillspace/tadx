// Optional local check: node scripts/tests/site_browser.mjs <chromium-browser-path>
// No Tableau calls, package downloads, or modification of the authored homepage.
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

const browser = process.argv[2];
if (!browser) throw new Error('Usage: node scripts/tests/site_browser.mjs <chromium-browser-path>');
const repo = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const html = readFileSync(join(repo, 'site/index.html'), 'utf8');
const folder = mkdtempSync(join(tmpdir(), 'tadx-site-browser-'));

function exercise() {
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  const id = value => document.getElementById(value);
  for (const demo of ['metric', 'publish', 'find']) {
    id(`tab-${demo}`).click();
    check(id(`tab-${demo}`).getAttribute('aria-selected') === 'true', `${demo} not selected`);
    check(id('demo-panel').getAttribute('aria-labelledby') === `tab-${demo}`, `${demo} panel inaccessible`);
    check(id('session-body').querySelector('.command').textContent.startsWith('tadx '), `${demo} lacks command`);
  }
  id('tab-publish').click();
  id('tab-publish').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
  check(id('tab-find').getAttribute('aria-selected') === 'true', 'Keyboard tab navigation failed');
  check(document.documentElement.scrollWidth <= innerWidth, 'Homepage overflows horizontally');
  document.querySelector('[data-install]').click();
  check(id('install-dialog').open, 'Installer did not open');
  id('os-windows').click();
  check(id('download-code').textContent === 'irm https://tadx.net/install.ps1 | iex', 'Windows command mismatch');
  id('os-unix').click();
  check(id('download-code').textContent === 'curl -fsSL https://tadx.net/install.sh | sh', 'Unix command mismatch');
  check(id('run-code').textContent === 'tadx update', 'Update command mismatch');
  check(id('install-dialog').scrollWidth <= id('install-dialog').clientWidth, 'Install dialog overflows horizontally');
  document.querySelector('.close-dialog').click();
  check(!id('install-dialog').open, 'Installer did not close');
  return { passed: true, width: innerWidth, demos: 3 };
}

try {
  const probe = `<script>try { document.body.dataset.siteTest = JSON.stringify((${exercise.toString()})()); } catch (error) { document.body.dataset.siteTest = JSON.stringify({passed:false,error:String(error)}); }</script>`;
  const path = join(folder, 'index.html');
  writeFileSync(path, html.replace('</body>', `${probe}</body>`));
  for (const width of [1280, 390]) {
    const result = spawnSync(browser, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', `--window-size=${width},1000`, `--user-data-dir=${join(folder, `profile-${width}`)}`, '--dump-dom', pathToFileURL(path).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024, windowsHide: true });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    const encoded = /data-site-test="([^"]+)"/.exec(result.stdout)?.[1];
    assert.ok(encoded, 'Browser did not run homepage checks');
    const report = JSON.parse(encoded.replaceAll('&quot;', '"').replaceAll('&amp;', '&'));
    assert.equal(report.passed, true, report.error);
    console.log(JSON.stringify(report));
  }
} finally {
  rmSync(folder, { recursive: true, force: true, maxRetries: 4, retryDelay: 200 });
}
