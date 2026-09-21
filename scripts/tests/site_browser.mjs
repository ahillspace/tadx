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

async function exercise() {
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
  const installCopyButton = document.querySelector('[data-copy="download-code"]');
  const installCopyRect = installCopyButton.getBoundingClientRect();
  check(installCopyButton.contains(document.elementFromPoint(installCopyRect.x + installCopyRect.width / 2, installCopyRect.y + installCopyRect.height / 2)), 'Windows install copy button is obscured');
  let clipboardText = '';
  Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: async text => { clipboardText = text; } }
  });
  for (const copyCase of [
    { os: 'windows', target: 'download-code', command: 'irm https://tadx.net/install.ps1 | iex' },
    { os: 'unix', target: 'download-code', command: 'curl -fsSL https://tadx.net/install.sh | sh' },
    { os: 'windows', target: 'run-code', command: 'tadx update' }
  ]) {
    id(`os-${copyCase.os}`).click();
    const button = document.querySelector(`[data-copy="${copyCase.target}"]`);
    clipboardText = '';
    button.click();
    await new Promise(resolve => setTimeout(resolve));
    check(clipboardText === copyCase.command, `${copyCase.os} ${copyCase.target} was not sent to the clipboard API`);
    check(button.textContent.trim() === 'Copied', `${copyCase.os} ${copyCase.target} copy success was not reported`);
  }

  id('os-windows').click();
  const copyButton = document.querySelector('[data-copy="download-code"]');
  const originalExecCommand = document.execCommand;
  let fallbackText = '';
  document.execCommand = command => {
    check(command === 'copy', `Unexpected editing command: ${command}`);
    fallbackText = document.activeElement.value;
    return true;
  };
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: () => new Promise(() => {}) }
  });
  copyButton.click();
  await new Promise(resolve => setTimeout(resolve, 600));
  check(fallbackText === id('download-code').textContent, 'Pending clipboard request did not use the fallback');
  check(copyButton.textContent.trim() === 'Copied', 'Pending clipboard fallback success was not reported');

  fallbackText = '';
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: async () => { throw new Error('clipboard denied'); } }
  });
  copyButton.click();
  await new Promise(resolve => setTimeout(resolve));
  check(fallbackText === id('download-code').textContent, 'Rejected clipboard request did not use the fallback');
  check(copyButton.textContent.trim() === 'Copied', 'Rejected clipboard fallback success was not reported');

  fallbackText = '';
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: text => text === 'tadx update' ? Promise.resolve() : new Promise(() => {}) }
  });
  copyButton.click();
  check(copyButton.textContent.trim() === 'Copying', 'Pending clipboard request did not show progress');
  id('os-unix').click();
  const updateButton = document.querySelector('[data-copy="run-code"]');
  updateButton.click();
  await new Promise(resolve => setTimeout(resolve, 600));
  check(fallbackText === '', 'Stale Windows copy request used the fallback after a newer copy');
  check(copyButton.textContent.trim() === 'Copy', 'OS switch did not clear stale copy feedback');
  check(updateButton.textContent.trim() === 'Copied', 'Newer update copy was not reported');

  id('os-windows').click();
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
  fallbackText = '';
  copyButton.click();
  await new Promise(resolve => setTimeout(resolve));
  check(fallbackText === id('download-code').textContent, 'Windows command was not selected for fallback copy');
  check(copyButton.textContent.trim() === 'Copied', 'Fallback copy success was not reported');

  document.execCommand = () => false;
  copyButton.click();
  await new Promise(resolve => setTimeout(resolve));
  check(copyButton.textContent.trim() === 'Select to copy', 'Copy failure was not reported');
  check(getSelection().toString() === id('download-code').textContent, 'Copy failure did not select the Windows command');
  document.execCommand = originalExecCommand;
  id('os-unix').click();
  check(id('download-code').textContent === 'curl -fsSL https://tadx.net/install.sh | sh', 'Unix command mismatch');
  check(id('run-code').textContent === 'tadx update', 'Update command mismatch');
  check(id('install-dialog').scrollWidth <= id('install-dialog').clientWidth, 'Install dialog overflows horizontally');
  document.querySelector('.close-dialog').click();
  check(!id('install-dialog').open, 'Installer did not close');
  return { passed: true, width: innerWidth, demos: 3, copyPaths: 3 };
}

try {
  const probe = `<script>(${exercise.toString()})().then(report => { document.body.dataset.siteTest = JSON.stringify(report); }, error => { document.body.dataset.siteTest = JSON.stringify({passed:false,error:String(error)}); });</script>`;
  const path = join(folder, 'index.html');
  writeFileSync(path, html.replace('</body>', `${probe}</body>`));
  for (const width of [1280, 390]) {
    const result = spawnSync(browser, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--virtual-time-budget=2500', `--window-size=${width},1000`, `--user-data-dir=${join(folder, `profile-${width}`)}`, '--dump-dom', pathToFileURL(path).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024, windowsHide: true });
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
