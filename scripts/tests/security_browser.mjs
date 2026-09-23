// Optional check: node scripts/tests/security_browser.mjs <chromium-browser-path> [screenshot-directory]
// Isolated headless profiles; no Tableau calls, package downloads, or changes to authored pages.
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

const browser = process.argv[2];
if (!browser) throw new Error('Usage: node scripts/tests/security_browser.mjs <chromium-browser-path> [screenshot-directory]');
const screenshots = process.argv[3] ? resolve(process.argv[3]) : null;
if (screenshots) mkdirSync(screenshots, { recursive: true });
const repo = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const folder = mkdtempSync(join(tmpdir(), 'tadx-security-browser-'));
const html = readFileSync(join(repo, 'site/security.html'), 'utf8');

function exercise() {
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  const noOverflow = () => check(document.documentElement.scrollWidth <= innerWidth, 'Security page overflows horizontally');
  check(getComputedStyle(document.body).backgroundColor === 'rgb(243, 243, 235)', 'Homepage theme missing');
  check(document.querySelector('[aria-current="page"]').textContent === 'Security', 'Current page navigation missing');
  const checksTable = document.querySelector('.checks-table');
  check(checksTable.tBodies[0].rows.length === 3, 'Authorization checks are missing');
  check(checksTable.getBoundingClientRect().height < 350, 'Authorization table is not compact');
  for (const card of document.querySelectorAll('.control-card')) {
    check(card.querySelector('pre').getBoundingClientRect().height > 0, 'Status command is hidden');
    check(!card.closest('details'), 'Status command must be visible without expanding details');
  }
  const details = [...document.querySelectorAll('details')];
  check(details.length >= 10, 'Missing detail sections');
  noOverflow();
  for (const detail of details) {
    check(!detail.open, 'Details should start collapsed');
    const summary = detail.querySelector('summary');
    summary.focus();
    check(document.activeElement === summary, 'Detail summary is not focusable');
    summary.click();
    check(detail.open, 'Detail did not open');
    check(detail.querySelector('.detail-body').getBoundingClientRect().height > 0, 'Open detail is hidden');
    noOverflow();
    summary.click();
    check(!detail.open, 'Detail did not close');
  }
  const cards = [...document.querySelectorAll('.template-card')].map(card => card.getBoundingClientRect());
  if (innerWidth > 960) check(cards.every(card => card.top === cards[0].top), 'Desktop template comparison is not a row');
  else check(cards[1].top > cards[0].bottom, 'Narrow template comparison is not stacked');
  for (const anchor of document.querySelectorAll('a[href^="#"]')) {
    check(document.getElementById(anchor.getAttribute('href').slice(1)), 'In-page link has no target');
  }
  document.activeElement.blur();
  if (document.body.dataset.captureSection) {
    const target = document.getElementById(document.body.dataset.captureSection);
    window.scrollTo({ top: target.getBoundingClientRect().top + window.scrollY - 28, behavior: 'instant' });
    check(window.scrollY > 0, 'Section screenshot did not scroll');
  }
  else window.scrollTo({ top: 0, behavior: 'instant' });
  return { passed: true, width: innerWidth, details: details.length, overflow: false };
}

try {
  const probe = '<script>addEventListener("load", () => { let report; try { report = (' + exercise.toString() + ')(); } catch(error) { report = {passed:false,error:String(error)}; } parent.postMessage({securityTest:report}, "*"); });</script>';
  for (const [width, section] of [[1280, ''], [768, ''], [390, ''], [1280, 'controls'], [390, 'controls'], [1280, 'checks-title'], [390, 'checks-title']]) {
    const label = String(width) + (section ? '-' + section : '');
    // A sized frame supplies exact CSS viewports despite Chromium's minimum native window width.
    const positioned = html.replace('<body>', section ? '<body data-capture-section="' + section + '">' : '<body>').replace('</body>', probe + '</body>');
    const escaped = positioned.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
    const page = join(folder, 'security-' + label + '.html');
    const wrapper = '<!doctype html><html><head><meta charset="utf-8"><style>body{margin:0;background:#d9dcd0;display:flex;justify-content:center}iframe{border:0;flex:none;width:' + width + 'px;height:1100px}</style></head><body><script>addEventListener("message", event => { if(event.source === document.querySelector("iframe").contentWindow && event.data.securityTest) document.body.dataset.securityTest = JSON.stringify(event.data.securityTest); });</script><iframe title="Security page viewport" srcdoc="' + escaped + '"></iframe></body></html>';
    writeFileSync(page, wrapper);
    const args = ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--force-device-scale-factor=1', '--virtual-time-budget=1500', '--window-size=' + Math.max(width + 24, 560) + ',1124', '--user-data-dir=' + join(folder, 'profile-' + label), '--dump-dom'];
    if (screenshots) args.push('--screenshot=' + join(screenshots, 'security-' + label + '.png'));
    args.push(pathToFileURL(page).href);
    const result = spawnSync(browser, args, { encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024, windowsHide: true });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    const encoded = /data-security-test="([^"]+)"/.exec(result.stdout)?.[1];
    assert.ok(encoded, 'Browser did not run security checks');
    const report = JSON.parse(encoded.replaceAll('&quot;', '"').replaceAll('&amp;', '&'));
    assert.equal(report.passed, true, report.error);
    assert.equal(report.width, width, 'Browser did not honor requested CSS viewport width');
    console.log(JSON.stringify(report));
  }
} finally {
  rmSync(folder, { recursive: true, force: true, maxRetries: 4, retryDelay: 200 });
}
