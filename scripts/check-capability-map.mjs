// Optional local browser regression: node scripts/check-capability-map.mjs <chrome-path> [page-path]
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
const page = readFileSync(process.argv[3] || join(root, 'docs/reference/capability-map.html'), 'utf8');
const folder = mkdtempSync(join(tmpdir(), 'tadx-capability-map-'));

function checkPage() {
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  const control = id => document.getElementById(id);
  const card = value => [...document.querySelectorAll('[data-card-filter]')].find(button => button.dataset.cardValue === value);
  const press = value => card(value).click();
  const clear = () => control('clearFilters').click();
  const definitions = JSON.parse(control('capability-data').textContent);
  check(document.documentElement.scrollWidth <= innerWidth, 'Command browser overflows horizontally');
  check(document.querySelector('.site-brand').href === 'https://tadx.net/', 'Home navigation missing');
  check(getComputedStyle(document.body).backgroundColor === 'rgb(243, 243, 235)', 'Homepage theme missing');
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
  clear();
  control('mutationFilter').value = 'administrative';
  control('mutationFilter').dispatchEvent(new Event('change', { bubbles: true }));
  const administrative = definitions.filter(item => item.administrative);
  check(administrative.length > 0, 'Registry lacks administrative classification');
  check(Number(control('matchedCount').textContent) === administrative.length, 'Administrative filter does not match registry classification');
  const adminRow = [...document.querySelectorAll('.tree-row')].find(row => row.getAttribute('aria-label')?.startsWith(`${administrative[0].id},`));
  check(adminRow, 'Administrative capability missing from filtered tree');
  adminRow.click();
  check(control('detailContent').textContent.includes('Administrative'), 'Administrative detail field missing');
  check(control('detailContent').textContent.includes('Not evaluated by this static page'), 'Static page implies machine authority');
  clear();
  control('searchInput').value = 'workbook.publish';
  control('searchInput').dispatchEvent(new Event('input', { bubbles: true }));
  const publish = [...document.querySelectorAll('.tree-row')].find(row => row.getAttribute('aria-label')?.startsWith('workbook.publish,'));
  check(publish, 'Publish capability not found');
  publish.click();
  check(control('detailContent').textContent.includes('canonical server and exact site'), 'Remote mutation lacks site consent explanation');
  check(control('detailContent').textContent.includes('managed policy permission'), 'Remote mutation lacks managed ceiling explanation');
  check(!document.body.innerText.includes('TADX_ENABLE_MUTATIONS'), 'Removed global override still advertised');
  check(document.documentElement.scrollWidth <= innerWidth, 'Administrative or mutation details overflow horizontally');
  return { passed: true, capabilities: definitions.length, scenarios: 6 };
}

try {
  const probe = `<script>try { document.body.dataset.mapTest = JSON.stringify((${checkPage.toString()})()); } catch (error) { document.body.dataset.mapTest = JSON.stringify({passed:false,error:String(error)}); }</script>`;
  const path = join(folder, 'map.html');
  writeFileSync(path, page.replace('</body>', `${probe}</body>`));
  for (const width of [1440, 900, 500]) {
  const result = spawnSync(browser, ['--headless=new', '--no-first-run', '--no-default-browser-check', `--window-size=${width},1000`, `--user-data-dir=${join(folder, `profile-${width}`)}`, '--dump-dom', pathToFileURL(path).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024, windowsHide: true });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  const encoded = /data-map-test="([^"]+)"/.exec(result.stdout)?.[1];
  assert.ok(encoded, 'Page did not complete browser checks');
  const report = JSON.parse(encoded.replaceAll('&quot;', '"').replaceAll('&amp;', '&'));
  assert.equal(report.passed, true, report.error);
  console.log(JSON.stringify({ ...report, width }));
  }
} finally {
  rmSync(folder, { recursive: true, force: true, maxRetries: 4, retryDelay: 200 });
}
