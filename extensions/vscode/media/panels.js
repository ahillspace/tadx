/* All text and attributes are assigned through DOM APIs. No inventory data is HTML. */
(function () {
  'use strict';
  const vscode = acquireVsCodeApi();
  const app = document.getElementById('app');
  const announcement = document.getElementById('announcement');
  const view = document.body.dataset.view;
  const saved = vscode.getState() || {};
  const state = {
    sourceKind: saved.sourceKind === 'workspaces' ? 'workspaces' : 'sites',
    sourceId: typeof saved.sourceId === 'string' ? saved.sourceId : '',
    query: typeof saved.query === 'string' ? saved.query : '',
    kind: typeof saved.kind === 'string' ? saved.kind : '',
    selected: new Set(Array.isArray(saved.selected) ? saved.selected.filter(key => typeof key === 'string') : []),
    sort: ['name', 'kind', 'location'].includes(saved.sort) ? saved.sort : 'name',
    descending: saved.descending === true,
    page: Number.isInteger(saved.page) && saved.page >= 0 ? saved.page : 0,
    artifactKey: '',
  };
  let snapshot = { sources: { sites: [], workspaces: [] }, artifacts: [] };
  let operation;
  let error = '';
  let initialized = false;

  function element(tag, text, className) {
    const node = document.createElement(tag);
    if (text !== undefined) node.textContent = text;
    if (className) node.className = className;
    return node;
  }
  function add(parent, ...children) { children.forEach(child => parent.appendChild(child)); return parent; }
  function button(text, action, className, disabled, title) {
    const node = element('button', text, className);
    node.type = 'button';
    node.disabled = Boolean(disabled);
    if (title) node.title = title;
    node.addEventListener('click', action);
    return node;
  }
  function request(action, values) { vscode.postMessage({ type: 'action', action, ...values }); }
  function persist() { vscode.setState({ ...state, selected: [...state.selected] }); }
  function announce(message) { announcement.textContent = message; }
  function notice(text, severity = 'info') { return element('p', text, `notice ${severity}`); }
  function source() { return snapshot.sources[state.sourceKind].find(item => item.id === state.sourceId); }
  function sourceRows() { return snapshot.artifacts.filter(row => row.sourceKind === state.sourceKind && row.sourceId === state.sourceId); }
  function canTransfer(row, action) { return ['workbook', 'datasource', 'flow'].includes(row.kind) && row.actions.includes(action); }
  function typeLabel(kind) { return kind === 'pulse-definition' ? 'Pulse definition' : kind ? kind.charAt(0).toUpperCase() + kind.slice(1) : 'Artifact'; }
  function status(text) { return element('span', text, 'badge'); }
  function refreshButton(sourceKind, sourceId) {
    return button(sourceKind === 'sites' ? 'Refresh site cache' : 'Rescan workspace', () => request('refresh', { sourceKind, sourceId }), 'secondary', snapshot.busy || !sourceId);
  }
  function heading(title, subtitle, actions = []) {
    return add(element('header', undefined, 'page-header'),
      add(element('div'), element('p', 'TADX', 'eyebrow'), element('h1', title), element('p', subtitle, 'subtitle')),
      add(element('div', undefined, 'header-actions'), ...actions));
  }
  function globalNotices(parent) {
    if (snapshot.busy) add(parent, notice('An operation is in progress. Artifact changes are temporarily unavailable.'));
    (snapshot.errors || []).forEach(message => add(parent, notice(message, 'error')));
    if (error) {
      const box = add(element('div', undefined, 'notice error'), element('p', error), button('Dismiss', () => { error = ''; render(); }, 'text-button'));
      box.setAttribute('role', 'alert');
      add(parent, box);
    }
  }
  function sourceNotices(parent, selected) {
    if (!selected) return;
    const line = add(element('div', undefined, 'source-status'), status(selected.status || 'Inventory status unavailable'));
    if (selected.description) add(line, element('span', selected.description, 'muted'));
    add(parent, line);
    if (selected.complete === false) add(parent, notice('Coverage is incomplete. These rows do not represent the entire source.', 'warning'));
    (selected.warnings || []).forEach(warning => add(parent, notice(warning, 'warning')));
  }
  function actionFor(row, action) {
    const labels = { download: 'Download…', upload: 'Upload…', openRemote: 'Open in Tableau', openPayload: 'Open payload', openMetadata: 'Open metadata', reveal: 'Reveal folder', move: 'Move local…', delete: 'Delete local…' };
    return button(labels[action], () => request(action, action === 'download' || action === 'upload' ? { keys: [row.key] } : { key: row.key }),
      action === 'download' && row.sourceKind === 'sites' || action === 'upload' && row.sourceKind === 'workspaces' ? 'primary' : 'secondary',
      !row.actions.includes(action) || snapshot.busy && ['download', 'upload', 'move', 'delete'].includes(action),
      action === 'upload' && row.sourceKind !== 'workspaces' ? 'Select an exact local copy to upload.' : action === 'download' && row.sourceKind !== 'sites' ? 'Select a cached site artifact to download.' : undefined);
  }
  function renderArtifact() {
    const row = snapshot.artifacts.find(item => item.key === state.artifactKey);
    if (!row) {
      add(app, heading('Artifact unavailable', 'The selected artifact is no longer in the current inventory.'));
      globalNotices(app);
      add(app, notice('Select an artifact in Sites or Local workspaces. Refresh its source if the inventory has changed.'));
      return;
    }
    const selected = snapshot.sources[row.sourceKind].find(item => item.id === row.sourceId);
    add(app, heading(row.name, `${typeLabel(row.kind)} · ${row.sourceKind === 'sites' ? 'Cached site artifact' : 'Selected local copy'}`, [refreshButton(row.sourceKind, row.sourceId)]));
    globalNotices(app);
    sourceNotices(app, selected);
    const actions = add(element('div', undefined, 'action-bar'), actionFor(row, 'download'), actionFor(row, 'upload'));
    ['openRemote', 'openPayload'].filter(action => row.actions.includes(action)).forEach(action => add(actions, actionFor(row, action)));
    add(app, actions);
    if (row.description) add(app, element('p', row.description, 'description'));
    const location = add(element('section', undefined, 'location-card'), element('h2', row.sourceKind === 'sites' ? 'Site location' : 'Exact local copy'), element('strong', selected ? selected.label : row.sourceId));
    if (row.location !== row.path) add(location, element('p', row.location));
    if (row.path) add(location, element('code', row.path, 'path'));
    if (row.state) add(location, status(row.state));
    const related = (row.relatedKeys || []).map(key => snapshot.artifacts.find(item => item.key === key)).filter(Boolean);
    const copies = add(element('section', undefined, 'location-card'), element('h2', row.sourceKind === 'sites' ? 'Local copies' : 'Source identity'));
    if (related.length) {
      related.forEach(copy => {
        const sourceLabel = snapshot.sources[copy.sourceKind].find(item => item.id === copy.sourceId)?.label || copy.sourceId;
        add(copies, add(element('div', undefined, 'copy-row'), button(`${sourceLabel} · ${copy.name}`, () => request('showArtifact', { key: copy.key }), 'text-button'), element('code', copy.path || copy.location), copy.state ? status(copy.state) : element('span')));
      });
    } else {
      if (row.sourceKind === 'sites') {
        add(copies, element('p', 'No verified association is available in this inventory.', 'muted'));
        add(copies, element('p', 'Browse Local workspaces to select an exact copy for upload.', 'muted'));
      } else {
        const sourceFields = (row.metadata || []).filter(field => field.label.startsWith('Source '));
        sourceFields.forEach(field => add(copies, element('p', `${field.label}: ${field.value}`)));
        add(copies, element('p', sourceFields.length ? 'Recorded at acquisition. No comparison with the current remote artifact has been made.' : 'Source information is unavailable in this CLI result.', 'muted'));
      }
    }
    add(app, add(element('div', undefined, 'locations'), location, copies));
    const metadata = [{ label: 'Type', value: typeLabel(row.kind) }, { label: row.sourceKind === 'sites' ? 'Remote state' : 'Local state', value: row.sourceKind === 'sites' ? 'Not checked live' : row.state || 'Unknown' }, ...(row.updatedAt ? [{ label: 'Remote modified', value: row.updatedAt }] : []), ...(row.artifactId ? [{ label: 'Artifact ID', value: row.artifactId }] : []), ...(row.metadata || [])];
    const fields = element('dl', undefined, 'metadata');
    metadata.forEach(field => add(fields, add(element('div'), element('dt', field.label), element('dd', field.value))));
    add(app, fields, notice(row.sourceKind === 'sites' ? 'Download verifies the selected remote identity and workspace through TADX. Cached inventory does not establish permission or current remote state.' : row.actions.includes('upload') ? 'Upload publishes this exact local copy and preserves its local files. Local change status does not establish equality with Tableau.' : 'Local file actions operate on this exact artifact. They do not change Tableau content.'));
    const secondaryActions = ['openMetadata', 'reveal', 'move', 'delete'].filter(action => row.actions.includes(action));
    if (secondaryActions.length) add(app, add(element('details', undefined, 'secondary-actions'), element('summary', 'Local file actions'), add(element('div', undefined, 'action-bar'), ...secondaryActions.map(action => actionFor(row, action)))));
  }
  function changeSource(sourceKind, sourceId) {
    const nextSourceId = sourceId || (sourceKind === state.sourceKind ? state.sourceId : '') || snapshot.sources[sourceKind][0]?.id || '';
    if (sourceKind === state.sourceKind && nextSourceId === state.sourceId) return;
    state.sourceKind = sourceKind;
    state.sourceId = nextSourceId;
    state.selected.clear();
    state.kind = '';
    state.page = 0;
    error = '';
    persist();
    render();
    if (state.sourceId) request('selectSource', { sourceKind, sourceId: state.sourceId });
  }
  function labeledSelect(labelText, id, entries, value, changed) {
    const field = element('div', undefined, 'field');
    const label = element('label', labelText);
    label.htmlFor = id;
    const select = element('select');
    select.id = id;
    entries.forEach(entry => {
      const option = element('option', entry.label);
      option.value = entry.id;
      add(select, option);
    });
    select.value = value;
    select.disabled = !entries.length;
    select.addEventListener('change', () => changed(select.value));
    add(field, label, select);
    return field;
  }
  function renderInventory() {
    const remote = state.sourceKind === 'sites';
    add(app, heading('Bulk Actions', remote ? 'Download workbooks, datasources, and flows from one site into one workspace.' : 'Upload workbooks, datasources, and flows from one workspace into one site project.', [refreshButton(state.sourceKind, state.sourceId)]));
    globalNotices(app);
    const modes = element('div', undefined, 'source-modes');
    modes.setAttribute('role', 'group');
    modes.setAttribute('aria-label', 'Bulk action');
    [['sites', 'Download from site'], ['workspaces', 'Upload from workspace']].forEach(([kind, label]) => {
      const item = button(label, () => changeSource(kind), state.sourceKind === kind ? 'mode selected' : 'mode');
      item.setAttribute('aria-pressed', String(state.sourceKind === kind));
      add(modes, item);
    });
    add(app, modes);
    const controls = element('div', undefined, 'inventory-controls');
    add(controls, labeledSelect(remote ? 'Site' : 'Workspace', 'source', snapshot.sources[state.sourceKind], state.sourceId, id => changeSource(state.sourceKind, id)));
    const kinds = [...new Set(sourceRows().map(row => row.kind))].sort();
    add(controls, labeledSelect('Type', 'kind', [{ id: '', label: 'All types' }, ...kinds.map(kind => ({ id: kind, label: typeLabel(kind) }))], state.kind, kind => { state.kind = kind; state.page = 0; persist(); render(); }));
    const search = element('input');
    search.id = 'search';
    search.type = 'search';
    search.value = state.query;
    search.placeholder = 'Name, project, or path';
    search.addEventListener('input', () => { state.query = search.value; state.page = 0; persist(); renderTable(); });
    add(controls, add(element('label', undefined, 'field search-field'), element('span', 'Filter inventory'), search));
    add(app, controls);
    sourceNotices(app, source());
    add(app, element('div', undefined, 'inventory-content'));
    renderTable();
  }
  function visibleRows() {
    const query = state.query.trim().toLocaleLowerCase();
    return sourceRows().filter(row => (!state.kind || row.kind === state.kind) && (!query || [row.name, row.location, row.path || ''].join(' ').toLocaleLowerCase().includes(query))).sort((a, b) => {
      const order = String(a[state.sort] || '').localeCompare(String(b[state.sort] || '')) || a.key.localeCompare(b.key);
      return state.descending ? -order : order;
    });
  }
  function renderTable() {
    const container = app.querySelector('.inventory-content');
    if (!container) return;
    const focusKey = document.activeElement?.dataset?.focusKey;
    container.replaceChildren();
    const matches = visibleRows();
    const pageSize = 100;
    state.page = Math.min(state.page, Math.max(0, Math.ceil(matches.length / pageSize) - 1));
    const rows = matches.slice(state.page * pageSize, (state.page + 1) * pageSize);
    const all = sourceRows();
    const action = state.sourceKind === 'sites' ? 'download' : 'upload';
    const selectedRows = all.filter(row => state.selected.has(row.key));
    const hiddenCount = selectedRows.filter(row => !rows.some(visible => visible.key === row.key)).length;
    const eligible = rows.filter(row => canTransfer(row, action));
    const allChecked = eligible.length > 0 && eligible.every(row => state.selected.has(row.key));
    const selectedLabel = `${selectedRows.length} selected${hiddenCount ? ` (${hiddenCount} outside this page)` : ''}`;
    const selectionBar = add(element('div', undefined, 'selection-bar'), element('p', `${matches.length} matching · ${selectedLabel}`, 'muted'), button(state.sourceKind === 'sites' ? 'Download selected…' : 'Upload selected…', () => request(action, { keys: selectedRows.map(row => row.key) }), 'primary', !selectedRows.length || snapshot.busy), button('Clear selection', () => { state.selected.clear(); persist(); renderTable(); announce('Selection cleared.'); }, 'secondary', !selectedRows.length));
    add(container, selectionBar);
    if (selectedRows.length >= 100) add(container, notice('The batch limit is 100 artifacts. Clear selections before adding another artifact.', 'warning'));
    else add(container, element('p', 'Select up to 100 artifacts across types. Filters preserve selection; changing the source clears it.', 'selection-help'));
    add(container, element('p', state.sourceKind === 'sites' ? 'Choose one destination workspace after selecting Download.' : 'Choose one destination site and project after selecting Upload.', 'selection-help'));
    if (!rows.length) {
      const empty = add(element('section', undefined, 'empty-state'), element('h2', all.length ? 'No matching artifacts' : 'No inventory rows'), element('p', all.length ? 'Change the filter to see more artifacts.' : !source() ? `No ${state.sourceKind === 'sites' ? 'sites' : 'workspaces'} are available. Configure TADX, then reload the inventory.` : 'This source has no loaded rows. Check coverage and warnings above before treating it as empty.'));
      add(container, empty);
      return;
    }
    const table = element('table', undefined, 'inventory-table');
    add(table, element('caption', 'Select up to 100 workbooks, datasources, and flows from one source.', 'visually-hidden'));
    const header = element('tr');
    const allBox = element('input');
    allBox.type = 'checkbox';
    allBox.checked = allChecked;
    allBox.indeterminate = !allChecked && eligible.some(row => state.selected.has(row.key));
    allBox.disabled = snapshot.busy || !eligible.length || state.selected.size >= 100 && !allChecked;
    allBox.setAttribute('aria-label', 'Select visible eligible artifacts, up to 100');
    allBox.dataset.focusKey = 'select-all';
    allBox.addEventListener('change', () => {
      if (allChecked) eligible.forEach(row => state.selected.delete(row.key));
      else eligible.forEach(row => { if (state.selected.size < 100) state.selected.add(row.key); });
      persist(); renderTable(); announce(`${state.selected.size} artifacts selected.`);
    });
    add(header, add(element('th', undefined, 'checkbox-cell'), allBox));
    [['name', 'Name'], ['kind', 'Type'], ['location', state.sourceKind === 'sites' ? 'Project' : 'Local path']].forEach(([key, label]) => {
      const th = element('th');
      th.scope = 'col';
      th.setAttribute('aria-sort', state.sort === key ? state.descending ? 'descending' : 'ascending' : 'none');
      const sortButton = button(`${label}${state.sort === key ? state.descending ? ' ↓' : ' ↑' : ''}`, () => { state.descending = state.sort === key && !state.descending; state.sort = key; state.page = 0; persist(); renderTable(); }, 'sort-button');
      sortButton.dataset.focusKey = `sort-${key}`;
      add(header, add(th, sortButton));
    });
    const stateHeader = element('th', 'State');
    stateHeader.scope = 'col';
    add(header, stateHeader);
    add(table, add(element('thead'), header));
    const body = element('tbody');
    rows.forEach(row => {
      const checked = state.selected.has(row.key);
      const tr = element('tr', undefined, checked ? 'selected' : undefined);
      const checkbox = element('input');
      checkbox.type = 'checkbox';
      checkbox.checked = checked;
      checkbox.disabled = snapshot.busy || !canTransfer(row, action) || !checked && state.selected.size >= 100;
      checkbox.setAttribute('aria-label', `Select ${row.name}`);
      checkbox.dataset.focusKey = row.key;
      checkbox.addEventListener('change', () => { if (checkbox.checked) state.selected.add(row.key); else state.selected.delete(row.key); persist(); renderTable(); announce(`${state.selected.size} artifacts selected.`); });
      add(tr, add(element('td', undefined, 'checkbox-cell'), checkbox), add(element('td'), button(row.name, () => request('showArtifact', { key: row.key }), 'artifact-link')), element('td', typeLabel(row.kind)), element('td', row.path || row.location, 'path-cell'), element('td', row.state || 'Unknown'));
      add(body, tr);
    });
    add(table, body);
    const scroll = add(element('div', undefined, 'table-scroll'), table);
    scroll.tabIndex = 0;
    scroll.setAttribute('role', 'region');
    scroll.setAttribute('aria-label', 'Scrollable artifact inventory');
    add(container, scroll);
    if (matches.length > pageSize) {
      const previous = button('Previous', () => { state.page--; persist(); renderTable(); announce(`Page ${state.page + 1}.`); }, 'secondary', state.page === 0);
      previous.dataset.focusKey = 'page-previous';
      const next = button('Next', () => { state.page++; persist(); renderTable(); announce(`Page ${state.page + 1}.`); }, 'secondary', (state.page + 1) * pageSize >= matches.length);
      next.dataset.focusKey = 'page-next';
      add(container, add(element('nav', undefined, 'pagination'), previous, element('span', `${state.page * pageSize + 1}-${state.page * pageSize + rows.length} of ${matches.length}`, 'muted'), next));
    }
    if (focusKey) {
      const controls = [...container.querySelectorAll('[data-focus-key]')];
      const retained = controls.find(node => node.dataset.focusKey === focusKey && !node.disabled);
      const adjacentPage = focusKey.startsWith('page-') ? controls.find(node => node.dataset.focusKey.startsWith('page-') && !node.disabled) : undefined;
      (retained || adjacentPage || scroll).focus();
    }
  }
  function renderOperation() {
    if (!operation) { add(app, heading('Operation', 'Select an activity to inspect its result.')); return; }
    add(app, heading(operation.title, 'TADX operation details'), status(typeLabel(operation.state)), element('p', operation.summary, 'operation-summary'));
    if (['unknown', 'pending', 'partial'].includes(operation.state)) add(app, notice('Keep the operation identity when checking progress. An uncertain outcome does not authorize an automatic retry.', 'warning'));
    if (operation.items?.length) {
      const list = element('ol', undefined, 'operation-items');
      operation.items.forEach(item => add(list, add(element('li'), element('strong', item.name), status(item.state), ...(item.message ? [element('p', item.message)] : []))));
      add(app, list);
    }
    if (operation.details) add(app, add(element('section', undefined, 'operation-details'), element('h2', 'Details'), element('pre', operation.details)));
  }
  function render() {
    const activeId = document.activeElement?.id;
    app.replaceChildren();
    if (view === 'artifact') renderArtifact();
    else if (view === 'operation') renderOperation();
    else renderInventory();
    if (activeId) document.getElementById(activeId)?.focus();
  }
  window.addEventListener('message', event => {
    const message = event.data;
    if (!message || typeof message !== 'object') return;
    if (message.type === 'error') { error = String(message.message || 'The action failed.'); render(); return; }
    if (message.type !== 'state') return;
    snapshot = message.snapshot;
    operation = message.operation;
    state.artifactKey = message.artifactKey || '';
    const previousSource = `${state.sourceKind}:${state.sourceId}`;
    if ((!initialized || message.reveal) && message.inventorySource) {
      state.sourceKind = message.inventorySource.sourceKind;
      state.sourceId = message.inventorySource.sourceId || '';
    }
    if (!snapshot.sources[state.sourceKind].some(item => item.id === state.sourceId)) state.sourceId = snapshot.sources[state.sourceKind][0]?.id || '';
    if (previousSource !== `${state.sourceKind}:${state.sourceId}`) {
      state.selected.clear();
      state.kind = '';
      state.page = 0;
    }
    const validRows = sourceRows();
    const action = state.sourceKind === 'sites' ? 'download' : 'upload';
    state.selected = new Set([...state.selected].filter(key => validRows.some(row => row.key === key && canTransfer(row, action))).slice(0, 100));
    if (state.kind && !validRows.some(row => row.kind === state.kind)) state.kind = '';
    initialized = true;
    persist();
    render();
  });
  vscode.postMessage({ type: 'ready' });
}());
