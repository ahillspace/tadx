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
  const registry = JSON.parse(control('capability-data').textContent);
  const definitions = registry.filter(item => item.owner === 'cli');
  check(document.documentElement.scrollWidth <= document.documentElement.clientWidth, 'Command browser overflows horizontally');
  check(control('filterDisclosure').open === (innerWidth > 600), 'Filter disclosure does not match the viewport');
  check(document.querySelector('.site-brand').href === 'https://tadx.net/', 'Home navigation missing');
  check(getComputedStyle(document.body).backgroundColor === 'rgb(243, 243, 235)', 'Homepage theme missing');
  check(definitions.length > 0 && definitions.length < registry.length, 'Registry must include CLI commands and external operations to exercise exclusion');
  for (const id of ['sourceTotal', 'totalCount', 'footerTotal']) {
    check(Number(control(id).textContent) === definitions.length, `CLI total failed to render in ${id}`);
  }
  for (const [id, predicate] of [
    ['implementedCount', item => item.implementation === 'implemented'],
    ['plannedCount', item => item.implementation === 'planned'],
    ['blockedCount', item => item.verification === 'blocked']
  ]) {
    check(Number(control(id).textContent) === definitions.filter(predicate).length, `CLI status count failed to render in ${id}`);
  }
  check(!control('ownerFilter') && !control('delegatedCount'), 'Non-CLI inventory filter or status card is present');
  check(![...control('stateFilter').options].some(option => option.value.includes('delegated')), 'Non-CLI state remains filterable');
  check(!document.querySelector('.stat-card[data-card-value="external/delegated"]'), 'Non-CLI status card is present');
  check(document.querySelector('.mcp-companions'), 'MCP companion guidance is missing');
  check(document.querySelector('.mcp-companions').innerText.includes('Tableau Desktop Authoring MCP'), 'Desktop authoring companion is missing');
  check(document.querySelector('.mcp-companions').innerText.includes('TADX does not call, configure, or grant access'), 'MCP boundary is unclear');
  for (const id of ['catalog.database.update', 'catalog.table.update', 'catalog.column.update', 'catalog.audit', 'content.label.update', 'admin.label.category.update']) {
    check(definitions.some(item => item.id === id), `Missing capability ${id}`);
  }
  for (const state of ['implemented', 'planned']) {
    clear();
    press('blocked');
    check(control('blockerFilter').value === 'blocked', 'Blocked card did not apply its filter');
    press(state);
    check(control('blockerFilter').value === '', `Blocked filter survived clicking ${state}`);
    check(control('stateFilter').value === state, `State filter not set for ${state}`);
    check(Number(control('matchedCount').textContent) === definitions.filter(item => item.implementation === state).length, `State filter count included non-CLI records for ${state}`);
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
  const adminRow = [...document.querySelectorAll('.tree-row')].find(row => row.querySelector('.node-id')?.textContent === administrative[0].id);
  check(adminRow, 'Administrative capability missing from filtered tree');
  adminRow.click();
  check(control('detailContent').textContent.includes('Administrative'), 'Administrative detail field missing');
  check(control('detailContent').textContent.includes('Not evaluated by this static page'), 'Static page implies machine authority');
  clear();
  control('searchInput').value = 'workbook.publish';
  control('searchInput').dispatchEvent(new Event('input', { bubbles: true }));
  const publish = [...document.querySelectorAll('.tree-row')].find(row => row.querySelector('.node-id')?.textContent === 'workbook.publish');
  check(publish, 'Publish capability not found');
  publish.click();
  check(control('detailContent').textContent.includes('canonical server and exact site'), 'Remote mutation lacks site consent explanation');
  check(control('detailContent').textContent.includes('managed policy permission'), 'Remote mutation lacks managed ceiling explanation');
  check(!document.body.innerText.includes('TADX_ENABLE_MUTATIONS'), 'Removed global override still advertised');
  check(document.documentElement.scrollWidth <= document.documentElement.clientWidth, 'Administrative or mutation details overflow horizontally');
  clear();
  control('expandAll').click();
  const visibleLeaves = () => [...document.querySelectorAll('.tree-row.leaf')].filter(row => !row.closest('ul[hidden]'));
  check(visibleLeaves().length === definitions.length, 'Expand all hid capabilities');
  for (const pill of document.querySelectorAll('.tree-row .count-pill')) {
    const row = pill.closest('.tree-row');
    const subtree = row.parentElement.querySelectorAll('.tree-row.leaf');
    check(Number(pill.textContent) === subtree.length, `Branch count does not match its CLI descendants: ${row.textContent}`);
    check([...subtree].every(leaf => definitions.some(item => item.id === leaf.querySelector('.node-id')?.textContent)), `Branch contains a non-CLI entry: ${row.textContent}`);
  }
  control('collapseAll').click();
  check(visibleLeaves().length === 0, 'Collapse left hidden branches expanded');
  const first = control('capability-tree').querySelector('.tree-row');
  first.focus();
  first.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
  check(document.activeElement !== first && document.activeElement.matches('.tree-row'), 'Arrow key did not move tree focus');
  document.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true }));
  check(document.activeElement === control('searchInput'), 'Search shortcut did not focus search');
  control('searchInput').value = 'no-such-capability-regression';
  control('searchInput').dispatchEvent(new Event('input', { bubbles: true }));
  check(Number(control('matchedCount').textContent) === 0 && !control('emptyState').hidden, 'Empty search lacks recovery');
  clear();
  check(Number(control('matchedCount').textContent) === definitions.length, 'Reset did not restore CLI commands');
  const displayedIDs = [...document.querySelectorAll('.tree-row .node-id')].map(node => node.textContent);
  check(displayedIDs.every(id => definitions.some(item => item.id === id)), 'Non-CLI registry entry appeared in the command tree');
  check(!control('capability-tree').innerText.includes('Delegated operations'), 'Delegated branch appeared in the command tree');
  return { passed: true, width: innerWidth, registry: registry.length, commands: definitions.length, scenarios: 9 };
}

try {
  const probe = `<script>addEventListener('load', () => { try { parent.postMessage({mapTest:(${checkPage.toString()})()}, '*'); } catch (error) { parent.postMessage({mapTest:{passed:false,error:String(error)}}, '*'); } });</script>`;
  const checkedPage = page.replace('</body>', () => `${probe}</body>`);
  const escaped = checkedPage.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
  for (const width of [1440, 768, 390, 320]) {
    // An iframe preserves the requested CSS viewport below Chromium's minimum window width.
    const path = join(folder, `map-${width}.html`);
    writeFileSync(path, `<!doctype html><html><head><meta charset="utf-8"><style>body{margin:0}iframe{border:0;width:${width}px;height:1000px}</style></head><body><script>addEventListener('message', event => { if(event.source === document.querySelector('iframe').contentWindow && event.data.mapTest) document.body.dataset.mapTest = JSON.stringify(event.data.mapTest); });</script><iframe title="Command browser viewport" srcdoc="${escaped}"></iframe></body></html>`);
    const result = spawnSync(browser, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--force-device-scale-factor=1', '--virtual-time-budget=3500', `--window-size=${Math.max(width + 24, 560)},1024`, `--user-data-dir=${join(folder, `profile-${width}`)}`, '--dump-dom', pathToFileURL(path).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024, windowsHide: true });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    const encoded = /data-map-test="([^"]+)"/.exec(result.stdout)?.[1];
    assert.ok(encoded, 'Page did not complete browser checks');
    const report = JSON.parse(encoded.replaceAll('&quot;', '"').replaceAll('&amp;', '&'));
    assert.equal(report.passed, true, report.error);
    assert.equal(report.width, width, 'Browser did not honor requested CSS viewport width');
    console.log(JSON.stringify(report));
  }
} finally {
  rmSync(folder, { recursive: true, force: true, maxRetries: 4, retryDelay: 200 });
}
