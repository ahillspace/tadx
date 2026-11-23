'use strict';
// All demo data is local and fictional. The page never runs shell commands.
const demos = {
  "find": {
    "prompt": "Find the sales workbooks on my dev site.",
    "plan": "Searching dev for matching workbook names, then checking the Sales Overview workbook.",
    "steps": [
      {
        "title": "Search workbooks",
        "command": "tadx search sales --type workbook --env dev",
        "output": "source: live\npage:\n  returned: 3\n  limit: 20\n  more_available: false\nitems[3]{luid,type,name,project_path}:\n  29f76909-0c93-4ec0-b86b-bc373fed391a,workbook,Sales Overview,Executive\n  31a5e370-f827-4104-a024-82a67641f332,workbook,Regional Sales,Sales\n  98f18596-d1f4-419b-87a7-a3056487f322,workbook,Sales Pipeline,Revenue Ops"
      },
      {
        "title": "Inspect Sales Overview",
        "command": "tadx content workbook inspect --env dev --id \"29f76909-0c93-4ec0-b86b-bc373fed391a\"",
        "output": "status: found\nenvironment: dev\nworkbook:\n  luid: 29f76909-0c93-4ec0-b86b-bc373fed391a\n  name: Sales Overview\n  project_path: Executive"
      }
    ],
    "response": "Found three sales workbooks on dev. Sales Overview is in the Executive project.",
    "kind": "workbooks",
    "context": "dev / Tableau Cloud",
    "status": "Read only"
  },
  "metric": {
    "prompt": "Create a Monthly Revenue metric from Orders on dev. Sum Sales by Order Date, with Region available as a filter.",
    "plan": "Finding Orders and checking its fields before creating the Pulse metric.",
    "steps": [
      {
        "title": "Find the Orders datasource",
        "command": "tadx search Orders --type datasource --env dev",
        "output": "source: live\npage:\n  returned: 1\n  more_available: false\nitems[1]{luid,type,name,project_path}:\n  ad1f679c-9a51-4dcf-b70c-1b1ec0bc4a27,datasource,Orders,Sales"
      },
      {
        "title": "Inspect the datasource fields",
        "command": "tadx content datasource schema --env dev --id \"ad1f679c-9a51-4dcf-b70c-1b1ec0bc4a27\"",
        "output": "status: listed\ndatasource_luid: ad1f679c-9a51-4dcf-b70c-1b1ec0bc4a27\ndatasource_name: Orders\npage:\n  returned: 3\n  more_available: false\nfields[3]:\n  - id: sales\n    caption: Sales\n    role: measure\n    data_type: number\n    default_aggregation: sum\n  - id: order_date\n    caption: Order Date\n    role: date\n    data_type: date\n  - id: region\n    caption: Region\n    role: dimension\n    data_type: string"
      },
      {
        "title": "Create Monthly Revenue",
        "before": "Orders has the required fields. Creating the definition with monthly granularity and a default Pulse metric.",
        "command": "tadx pulse definition create --env dev --name \"Monthly Revenue\" --datasource-id \"ad1f679c-9a51-4dcf-b70c-1b1ec0bc4a27\" --measure-field sales --aggregation SUM --date-field order_date --minimum-granularity MONTH --dimension region",
        "output": "plan:\n  mode: execute\n  name: Monthly Revenue\n  measure:\n    field: sales\n    aggregation: SUM\n  time_dimension: order_date\n  allowed_dimensions[1]: region\n  minimum_granularity: MONTH\nresult:\n  status: succeeded\n  definition_luid: ad43a7fe-ea36-4f20-8440-f0a62efb73e1\n  default_metric_luid: 817603ad-a55e-4f1d-8f15-d88d10279866\n  default_metric_status: ready"
      }
    ],
    "response": "Created Monthly Revenue in Tableau Pulse. It sums Sales by Order Date, with Region available as a filter.",
    "kind": "metric",
    "context": "dev / Tableau Cloud",
    "status": "Created"
  },
  "publish": {
    "prompt": "Publish my updated Sales Overview workbook from analytics to Executive on prod, replacing the existing version.",
    "plan": "Checking the local workbook changes and the publish destination before replacing the existing workbook.",
    "steps": [
      {
        "title": "Check the updated local workbook",
        "command": "tadx workspace status --workspace analytics",
        "output": "status: attention\nworkspace:\n  name: analytics\nartifacts:\n  dirty: 1\n  artifacts[1]{kind,luid,name,path,state}:\n    workbook,29f76909-0c93-4ec0-b86b-bc373fed391a,Sales Overview,artifacts/workbook/Sales Overview,dirty"
      },
      {
        "title": "Check the publish plan",
        "command": "tadx content workbook publish --workspace analytics --id \"29f76909-0c93-4ec0-b86b-bc373fed391a\" --env prod --project \"Executive\" --overwrite --preview",
        "output": "plan:\n  mode: preview\n  operation: workbook.publish\n  source_luid: 29f76909-0c93-4ec0-b86b-bc373fed391a\n  workbook_name: Sales Overview\n  target:\n    environment: prod\n    project_path: Executive\n    existing_workbook_luid: f9f3527e-c1a1-4fbb-b057-0751754fa9cb\n  overwrite: true"
      },
      {
        "title": "Publish the workbook",
        "before": "The plan targets the existing Sales Overview in Executive on prod. Publishing the local update with overwrite enabled.",
        "command": "tadx content workbook publish --workspace analytics --id \"29f76909-0c93-4ec0-b86b-bc373fed391a\" --env prod --project \"Executive\" --overwrite",
        "output": "status: succeeded\noperation: workbook.publish\nenvironment: prod\nproject_path: Executive\nresult:\n  status: succeeded\n  workbook_luid: f9f3527e-c1a1-4fbb-b057-0751754fa9cb\n  workbook_name: Sales Overview"
      }
    ],
    "response": "Published the updated Sales Overview to Executive on prod.",
    "kind": "publish",
    "context": "prod / Tableau Cloud",
    "status": "Published"
  },
  "blocked": {
    "prompt": "Delete Sales Overview from prod. It has been retired.",
    "plan": "Looking up Sales Overview on prod, then using its exact ID for the requested deletion.",
    "steps": [
      {
        "title": "Find the target workbook",
        "command": "tadx search \"Sales Overview\" --type workbook --env prod",
        "output": "source: live\npage:\n  returned: 1\n  limit: 20\n  more_available: false\nitems[1]{luid,type,name,project_path}:\n  29f76909-0c93-4ec0-b86b-bc373fed391a,workbook,Sales Overview,Executive"
      },
      {
        "title": "Delete workbook",
        "blocked": true,
        "command": "tadx content workbook delete --env prod --id \"29f76909-0c93-4ec0-b86b-bc373fed391a\"",
        "output": "error:\n  id: policy.denied\n  operation: workbook.delete\n  summary: The administrator-managed policy blocks this operation.\n  phase: setup\n  outcome: not_attempted\n  retryable: false"
      },
      {
        "title": "Inspect managed policy",
        "command": "tadx policy status",
        "output": "state: active\nprotected: true\npath_protected: true\ncandidate_valid: true\nremote_mutations: false"
      }
    ],
    "response": "Your managed policy blocks workbook deletion. No deletion was attempted. An administrator must update the policy before this operation can proceed.",
    "kind": "blocked",
    "context": "prod / Tableau Cloud",
    "status": "Blocked by policy"
  }
};
    const lockIcon = '<svg class="icon" aria-hidden="true" viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/><path d="M12 14v3"/></svg>';
    const icon = name => `<svg class="icon" aria-hidden="true"><use href="#${name}"/></svg>`;
    const body = document.getElementById('session-body');
const tabs = Array.from(document.querySelectorAll('[data-demo]'));
const panel = document.getElementById('demo-panel');
const announcer = document.getElementById('announcer');
const replay = document.getElementById('replay-button');
const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
const table = body.querySelector('.workbooks').cloneNode(true);
let currentDemo = 'find';
let replayController = null;

function buildSession(name) {
  const d = demos[name];
  const prompt = document.createElement('div');
  prompt.className = 'prompt';
  prompt.innerHTML = '<span class="prompt-mark" aria-hidden="true">›</span><p class="prompt-text"></p>';
  prompt.querySelector('p').textContent = d.prompt;
  const userLabel = document.createElement('span');
  userLabel.className = 'session-role';
  userLabel.textContent = 'You';
  prompt.querySelector('p').prepend(userLabel);
  const tool = document.createElement('div');
  tool.className = 'agent-turn';
  tool.innerHTML = '<span class="session-role">Agent</span><p class="agent-plan"></p><p class="tool-hint">Expand a tool call to see the command and output.</p>';
  tool.querySelector('.agent-plan').textContent = d.plan;
  for (const step of d.steps) {
    if (step.before) {
      const note = document.createElement('p');
      note.className = 'agent-plan';
      note.style.marginTop = '18px';
      note.textContent = step.before;
      tool.append(note);
    }
    const call = document.createElement('details');
    call.className = 'tool-call' + (step.blocked ? ' is-blocked' : '');
    call.innerHTML = '<summary><span class="tool-tick" aria-hidden="true"></span><span class="tool-title"></span><span class="tool-state"></span><svg class="icon chevron" aria-hidden="true"><use href="#chevron"/></svg></summary><div class="tool-detail"><p class="tool-label">Shell command</p><pre class="command"></pre><p class="tool-label">TADX output excerpt</p><pre class="tool-output"></pre></div>';
    call.querySelector('.tool-tick').innerHTML = step.blocked ? lockIcon : icon('check');
    call.querySelector('.tool-title').textContent = step.title;
    call.querySelector('.tool-state').textContent = step.blocked ? 'Blocked' : 'Complete';
    call.querySelector('.command').textContent = step.command;
    call.querySelector('.tool-output').textContent = step.output;
    tool.append(call);
  }
  const response = document.createElement('div');
  response.className = 'response';
  const copy = document.createElement('p');
  copy.className = 'response-copy';
  copy.textContent = d.response;
  response.append(copy);
  if (d.kind === 'workbooks') {
    response.append(table.cloneNode(true));
  } else if (d.kind === 'metric') {
    const saved = document.createElement('p');
    saved.className = 'change-title';
    saved.textContent = 'Saved in Tableau Pulse';
    response.append(saved);
    const card = document.createElement('div');
    card.className = 'metric-card';
    for (const [label, value] of [['Metric', 'Monthly Revenue'], ['Measure', 'Sum of Sales'], ['Minimum granularity', 'Month'], ['Filter by', 'Region']]) {
      const field = document.createElement('div');
      field.className = 'metric-field';
      const caption = document.createElement('span');
      caption.textContent = label;
      const content = document.createElement('strong');
      content.textContent = value;
      field.append(caption, content);
      card.append(field);
    }
    response.append(card);
  } else if (d.kind === 'blocked') {
    const notice = document.createElement('p');
    notice.className = 'blocked-result';
    notice.innerHTML = lockIcon;
    notice.append(document.createTextNode('Deletion stopped before reaching Tableau'));
    response.append(notice);
  } else {
    const change = document.createElement('div');
    change.className = 'change';
    change.innerHTML = `<p class="change-title">Sales Overview published</p><div class="change-path"><span>analytics <strong>/ local</strong></span>${icon('arrow')}<span>prod <strong>/ Executive</strong></span></div>`;
    response.append(change);
  }
  body.replaceChildren(prompt, tool, response);
  document.getElementById('session-context').textContent = d.context;
  document.getElementById('session-status').textContent = d.status;
  return { prompt, tool, response };
}

function cancelReplay() {
  if (replayController) replayController.abort();
  replayController = null;
  replay.disabled = false;
}
function selectDemo(name, animate = true) {
  cancelReplay();
  currentDemo = name;
  for (const tab of tabs) {
    const selected = tab.dataset.demo === name;
    tab.setAttribute('aria-selected', String(selected));
    tab.tabIndex = selected ? 0 : -1;
  }
  panel.setAttribute('aria-labelledby', 'tab-' + name);
  const session = buildSession(name);
  if (animate && !reduceMotion.matches) session.response.classList.add('reveal-step');
  announcer.textContent = demos[name].response;
}
function arrowTabs(buttons, select) {
  buttons.forEach((button, index) => button.addEventListener('keydown', event => {
    let next;
    if (event.key === 'ArrowRight') next = (index + 1) % buttons.length;
    else if (event.key === 'ArrowLeft') next = (index - 1 + buttons.length) % buttons.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = buttons.length - 1;
    else return;
    event.preventDefault();
    buttons[next].focus();
    select(buttons[next]);
  }));
}
tabs.forEach(tab => tab.addEventListener('click', () => selectDemo(tab.dataset.demo)));
arrowTabs(tabs, tab => selectDemo(tab.dataset.demo));
const wait = (ms, signal) => new Promise((resolve, reject) => {
  if (signal.aborted) { reject(new DOMException('Aborted', 'AbortError')); return; }
  const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve(); }, ms);
  function abort() { clearTimeout(timer); reject(new DOMException('Aborted', 'AbortError')); }
  signal.addEventListener('abort', abort, { once: true });
});
replay.addEventListener('click', async () => {
  cancelReplay();
  const session = buildSession(currentDemo);
  if (reduceMotion.matches) return;
  const controller = new AbortController();
  replayController = controller;
  replay.disabled = true;
  session.tool.style.visibility = 'hidden';
  session.response.style.visibility = 'hidden';
  session.prompt.classList.add('reveal-step');
  try {
    await wait(450, controller.signal);
    session.tool.style.visibility = 'visible';
    session.tool.classList.add('reveal-step');
    await wait(700, controller.signal);
    session.response.style.visibility = 'visible';
    session.response.classList.add('reveal-step');
    announcer.textContent = demos[currentDemo].response;
  } catch (error) {
    if (error.name !== 'AbortError') console.error(error);
  } finally {
    if (replayController === controller) {
      session.tool.style.visibility = 'visible';
      session.response.style.visibility = 'visible';
      replayController = null;
      replay.disabled = false;
    }
  }
});

selectDemo('find', false);

const dialog = document.getElementById('install-dialog');
const osTabs = Array.from(document.querySelectorAll('[data-os]'));
let copyGeneration = 0;
const installers = {
  unix: {
    download: 'curl -fsSL https://tadx.net/install.sh | sh',
    run: 'tadx update'
  },
  windows: {
    download: 'irm https://tadx.net/install.ps1 | iex',
    run: 'tadx update'
  }
};
function resetCopyLabels() {
  document.querySelectorAll('[data-copy] > span').forEach(label => { label.textContent = 'Copy'; });
}
function selectOS(os) {
  copyGeneration++;
  for (const tab of osTabs) {
    const selected = tab.dataset.os === os;
    tab.setAttribute('aria-selected', String(selected));
    tab.tabIndex = selected ? 0 : -1;
  }
  document.getElementById('install-panel').setAttribute('aria-labelledby', 'os-' + os);
  document.getElementById('download-code').textContent = installers[os].download;
  document.getElementById('run-code').textContent = installers[os].run;
  resetCopyLabels();
}
osTabs.forEach(tab => tab.addEventListener('click', () => selectOS(tab.dataset.os)));
arrowTabs(osTabs, tab => selectOS(tab.dataset.os));
if (/win/i.test(navigator.userAgentData?.platform || navigator.platform || '')) selectOS('windows');
document.querySelectorAll('[data-install]').forEach(link => {
  if (typeof dialog.showModal !== 'function') return;
  link.setAttribute('aria-haspopup', 'dialog');
  link.addEventListener('click', event => {
    if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    dialog.showModal();
    document.body.classList.add('modal-open');
  });
});
dialog.querySelector('.close-dialog').addEventListener('click', () => dialog.close());
dialog.addEventListener('close', () => {
  copyGeneration++;
  resetCopyLabels();
  document.body.classList.remove('modal-open');
});
dialog.addEventListener('click', event => {
  const r = dialog.getBoundingClientRect();
  if (event.target === dialog && (event.clientX < r.left || event.clientX > r.right || event.clientY < r.top || event.clientY > r.bottom)) dialog.close();
});
document.querySelectorAll('[data-copy]').forEach(button => {
  let reset;
  button.addEventListener('click', async () => {
    clearTimeout(reset);
    const generation = ++copyGeneration;
    const code = document.getElementById(button.dataset.copy);
    const text = code.textContent;
    resetCopyLabels();
    button.querySelector('span').textContent = 'Copying';
    let copied = false;
    let clipboardTimeout;
    try {
      if (navigator.clipboard?.writeText && window.isSecureContext) {
        const nativeCopy = navigator.clipboard.writeText(text).then(() => true, () => false);
        const deadline = new Promise(resolve => { clipboardTimeout = setTimeout(() => resolve(false), 500); });
        copied = await Promise.race([nativeCopy, deadline]);
      }
    } catch (_) { /* Local files may need the selection-based clipboard fallback. */ }
    clearTimeout(clipboardTimeout);
    if (generation !== copyGeneration || !dialog.open) return;
    if (!copied) {
      const input = document.createElement('textarea');
      input.value = text;
      input.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none;';
      dialog.append(input);
      input.select();
      try { copied = document.execCommand('copy'); } catch (_) { copied = false; }
      input.remove();
      button.focus();
    }
    button.querySelector('span').textContent = copied ? 'Copied' : 'Select to copy';
    if (!copied) {
      const range = document.createRange();
      range.selectNodeContents(code);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
    }
    announcer.textContent = copied ? 'Command copied.' : 'Command selected. Press Control+C or Command+C to copy.';
    reset = setTimeout(() => { button.querySelector('span').textContent = 'Copy'; }, 2000);
  });
});
