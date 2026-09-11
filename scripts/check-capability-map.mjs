// Optional local browser regression: node scripts/check-capability-map.mjs <chrome-path>
// No Tableau access, npm dependencies, or changes to the authored page.
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

const browser = process.argv[2];
if (!browser) throw new Error('Usage: node scripts/check-capability-map.mjs <chrome-path>');
const root = dirname(dirname(fileURLToPath(import.meta.url)));
const page = readFileSync(join(root, 'docs/reference/capability-map.html'), 'utf8');
const folder = mkdtempSync(join(tmpdir(), 'tadx-capability-map-'));

function checkPage() {
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  const control = id => document.getElementById(id);
  const card = value => [...document.querySelectorAll('[data-card-filter]')].find(button => button.dataset.cardValue === value);
  const press = value => card(value).click();
  const clear = () => control('clearFilters').click();
  const definitions = JSON.parse(control('capability-data').textContent);
  check(Number(control('sourceTotal').textContent) === definitions.length, 'Registry total failed to render');
  for (const id of ['catalog.database.update', 'catalog.table.update', 'catalog.column.update', 'catalog.audit', 'content.label.update', 'admin.label.category.update']) {
    check(definitions.some(item => item.id === id), `Missing capability ${id}`);
  }
  for (const state of ['implemented', 'planned', 'external/delegated']) {
    clear();
    press('blocked');
    check(control('blockerFilter').value === 'blocked', 'Blocked card did not apply its filter');
    press(state);
    check(control('blockerFilter').value === '', `Blocked filter survived clicking ${state}`);
    check(control('stateFilter').value === state, `State filter not set for ${state}`);
    check(card('blocked').getAttribute('aria-pressed') === 'false', 'Blocked card stayed selected');
    check(card(state).getAttribute('aria-pressed') === 'true', 'Selected card not announced');
    check(document.querySelectorAll('[data-card-filter][aria-pressed="true"]').length === 1, 'Multiple top cards selected');
    press('blocked');
    check(control('stateFilter').value === '', 'State filter survived clicking Blocked');
    press('blocked');
    check(control('blockerFilter').value === '', 'Second click did not clear Blocked');
    press(state);
    press(state);
    check(control('stateFilter').value === '', `Second click did not clear ${state}`);
  }
  clear();
  control('searchInput').value = 'catalog';
  control('searchInput').dispatchEvent(new Event('input', { bubbles: true }));
  press('blocked');
  press('implemented');
  check(control('searchInput').value === 'catalog', 'Top card erased independent search');
  check(!control('capability-tree').hidden, 'Implemented catalog results stayed hidden');
  return { passed: true, capabilities: definitions.length, scenarios: 4 };
}

try {
  const probe = `<script>try { document.body.dataset.mapTest = JSON.stringify((${checkPage.toString()})()); } catch (error) { document.body.dataset.mapTest = JSON.stringify({passed:false,error:String(error)}); }</script>`;
  const path = join(folder, 'map.html');
  writeFileSync(path, page.replace('</body>', `${probe}</body>`));
  const result = spawnSync(browser, ['--headless=new', '--no-first-run', '--no-default-browser-check', `--user-data-dir=${join(folder, 'profile')}`, '--dump-dom', pathToFileURL(path).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024, windowsHide: true });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  const encoded = /data-map-test="([^"]+)"/.exec(result.stdout)?.[1];
  assert.ok(encoded, 'Page did not complete browser checks');
  const report = JSON.parse(encoded.replaceAll('&quot;', '"').replaceAll('&amp;', '&'));
  assert.equal(report.passed, true, report.error);
  console.log(JSON.stringify(report));
} finally {
  rmSync(folder, { recursive: true, force: true, maxRetries: 4, retryDelay: 200 });
}
