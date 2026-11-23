const browser = document.querySelector(".command-browser");
const controls = document.querySelector(".command-controls");
const search = document.querySelector("#command-search");
const list = document.querySelector("#command-list");
const detail = document.querySelector("#command-detail");
const detailPane = detail.closest(".command-detail");
const loadStatus = document.querySelector("#load-status");
const toggleGroups = document.querySelector("#toggle-groups");
const mobile = matchMedia("(max-width: 800px)");

const inventoryLists = new Set([
  "admin.group.list", "admin.user.list", "datasource.list",
  "flow.list", "project.list", "workbook.list",
]);
const executionFilters = {
  read: cap => !cap.remote_mutation && !cap.local_write,
  remote: cap => cap.remote_mutation,
  "not-remote": cap => !cap.remote_mutation,
  local: cap => cap.local_write,
  write: cap => cap.remote_mutation || cap.local_write,
  admin: cap => cap.administrative,
};
const behaviorFilters = {
  preview: cap => cap.supports_preview,
  cache: cap => cap.cache,
  search: cap => cap.id === "search.run",
  list: cap => inventoryLists.has(cap.id),
  pull: cap => cap.id.endsWith(".pull"),
  remote: cap => cap.remote_mutation,
  local: cap => cap.local_write,
};
const effectLabel = cap => cap.remote_mutation ? "Remote write" : cap.local_write ? "Local write" : "Read only";
const yesNo = value => value ? "Yes" : "No";
const normalize = value => value.toLowerCase().trim().replace(/\s+/g, " ");
const partOf = (cap, index) => cap.command_path?.[index] ?? cap.id.split(".")[index];
const preferredDomains = ["content", "search", "catalog", "pulse", "admin"];
const domainsInOrder = (a, b) => {
  const rank = value => preferredDomains.includes(value) ? preferredDomains.indexOf(value) : preferredDomains.length;
  return rank(a) - rank(b) || a.localeCompare(b);
};
const text = (tag, value, className) => {
  const node = document.createElement(tag);
  if (className) node.className = className;
  node.textContent = value;
  return node;
};

function defaultBehavior(cap) {
  if (cap.remote_mutation) return "Remote writes require saved consent for the selected server and exact site, managed policy permission, and Tableau authorization. Supported previews are read-only and still require permission for their capability IDs.";
  if (cap.id.endsWith(".pull")) return cap.supports_preview
    ? "Retrieves from Tableau and writes a managed artifact by default. Use --preview to inspect the planned write."
    : "Retrieves from Tableau and writes a managed artifact by default.";
  if (cap.id === "search.run") return "Uses Tableau native search for nonempty content terms. --cache reads local state only.";
  if (inventoryLists.has(cap.id)) return "Performs a bounded live read. Use --all to collect matching inventory or --cache to read local state only.";
  return cap.local_write ? "Performs a local write after validation." : "Performs a read-only operation.";
}

function facts(items) {
  const dl = document.createElement("dl");
  dl.className = "fact-list";
  for (const [label, value] of items) {
    const row = document.createElement("div");
    row.append(text("dt", label), text("dd", value || "None"));
    dl.append(row);
  }
  return dl;
}

function showCommand(cap) {
  if (!cap) {
    detail.replaceChildren(text("p", "Select a command to see its details.", "selection-empty"));
    return;
  }

  const title = text("h3", cap.surface, "detail-title");
  title.tabIndex = -1;
  const tags = document.createElement("div");
  tags.className = "command-tags";
  tags.append(text("span", effectLabel(cap), cap.remote_mutation ? "effect write" : "effect"));
  if (cap.administrative) tags.append(text("span", "Administrative", "effect"));
  if (cap.supports_preview) tags.append(text("span", "Preview supported", "effect"));
  if (cap.verification === "blocked") tags.append(text("span", "Verification blocked", "effect blocked"));
  if (cap.implementation !== "implemented") tags.append(text("span", cap.implementation, "effect"));

  const command = document.createElement("div");
  command.className = "copy-command";
  const syntax = text("code", cap.surface);
  const copy = text("button", "Copy");
  copy.type = "button";
  copy.setAttribute("aria-label", "Copy command");
  const copyStatus = text("span", "", "sr-only");
  copyStatus.setAttribute("role", "status");
  copy.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(cap.surface);
      copy.textContent = "Copied";
      copyStatus.textContent = "Command copied.";
    } catch {
      const range = document.createRange();
      range.selectNodeContents(syntax);
      const selection = getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      copy.textContent = "Copy manually";
      copyStatus.textContent = "Clipboard unavailable. The command is selected for manual copying.";
    }
  });
  command.append(syntax, copy, copyStatus);

  const metadata = document.createElement("details");
  metadata.className = "command-metadata";
  metadata.append(text("summary", "Registry and verification metadata"), facts([
    ["Capability ID", cap.id],
    ["Operation type", cap.type],
    ["Disposition", cap.disposition],
    ["Implementation", cap.implementation],
    ["Verification", cap.verification],
    ["Supports batches", yesNo(cap.supports_batch)],
    ["Raw capable", yesNo(cap.raw_capable)],
    ["Local write", yesNo(cap.local_write)],
    ["Remote write", yesNo(cap.remote_mutation)],
    ["Cache opt-in", yesNo(cap.cache)],
    ["Upstream operation", cap.upstream_operation],
    ["Evidence level", cap.evidence_level],
    ["Evidence", cap.evidence],
    ["Validation", cap.validation],
  ]));
  detail.replaceChildren(
    title, text("p", cap.outcome, "outcome"), tags, command,
    facts([
      ["Default behavior", defaultBehavior(cap)],
      ["Selectors", cap.selectors],
      ["Availability", cap.availability],
      ["Safety guard", cap.safety_guard],
      ["Artifact effect", cap.artifact_effect],
      ...(cap.verification === "blocked" ? [["Blocker", cap.validation]] : []),
    ]),
    metadata,
  );
}

async function init() {
  loadStatus.hidden = false;
  let inventory;
  try {
    const response = await fetch(new URL("./capabilities.json", import.meta.url));
    if (!response.ok) throw new Error("The command inventory is unavailable.");
    inventory = (await response.json()).filter(cap => cap.owner === "cli");
    if (!inventory.length) throw new Error("The command inventory is empty.");
  } catch {
    loadStatus.firstChild.textContent = "Could not load the command inventory. Reload this page to try again. ";
    return;
  }

  const capabilities = inventory.map(cap => ({
    ...cap,
    domain: partOf(cap, 0),
    resource: cap.command_path?.length > 2 ? partOf(cap, 1) : "",
    cache: /(?:^|\s)--cache(?:\s|[;,]|$)/i.test([cap.selectors, cap.safety_guard, cap.outcome].join(" ")),
    searchText: normalize(Object.values(cap).join(" ") + " " + effectLabel(cap)),
  }));
  const domains = [...new Set(capabilities.map(cap => cap.domain))].sort(domainsInOrder);
  const domainSelect = controls.elements.namedItem("domain");
  for (const domain of domains) domainSelect.add(new Option(domain, domain));

  const current = new URL(location.href);
  const fields = ["domain", "effect", "behavior", "state", "verification"]
    .map(name => controls.elements.namedItem(name));
  const [family, effect, behavior, state, verification] = fields;
  for (const field of fields) {
    const value = current.searchParams.get(field.name);
    if ([...field.options].some(option => option.value === value)) field.value = value;
  }
  search.value = current.searchParams.get("q") || "";
  const requestedCommand = new URLSearchParams(current.hash.slice(1)).get("command");
  let selected = requestedCommand || "workbook.inspect";
  let selectedButton;
  const filtersCount = document.querySelector("#filter-count");
  const resultCount = document.querySelector("#result-count");
  const resetFilters = controls.querySelector('[type="reset"]');

  function updateURL() {
    const url = new URL(location.href);
    if (search.value.trim()) url.searchParams.set("q", search.value.trim());
    else url.searchParams.delete("q");
    for (const field of fields) {
      if (field.value) url.searchParams.set(field.name, field.value);
      else url.searchParams.delete(field.name);
    }
    if (url.hash.startsWith("#command=")) url.hash = "";
    history.replaceState(null, "", url);
  }

  function render() {
    const query = normalize(search.value);
    const terms = query ? query.split(" ") : [];
    const matching = capabilities.filter(cap =>
      terms.every(word => cap.searchText.includes(word)) &&
      (!family.value || cap.domain === family.value) &&
      (!state.value || cap.implementation === state.value) &&
      (!verification.value || (cap.verification === "blocked") === (verification.value === "blocked")) &&
      (!effect.value || executionFilters[effect.value](cap)) &&
      (!behavior.value || behaviorFilters[behavior.value](cap))
    );
    const selection = matching.find(cap => cap.id === selected) || matching[0];
    selected = selection?.id || "";
    selectedButton = null;

    const activeFilters = fields.slice(1).filter(field => field.value).length;
    filtersCount.textContent = String(activeFilters);
    filtersCount.hidden = !activeFilters;
    resetFilters.hidden = !query && !fields.some(field => field.value);
    resultCount.textContent = matching.length + " of " + capabilities.length + " commands";
    document.querySelector("#no-results").hidden = matching.length > 0;
    toggleGroups.hidden = matching.length === 0;
    document.querySelector("#back-to-list").hidden = !selection;

    const groups = new Map();
    for (const cap of matching) {
      if (!groups.has(cap.domain)) groups.set(cap.domain, []);
      groups.get(cap.domain).push(cap);
    }
    const showMatches = query || fields.some(field => field.value);

    const panels = [];
    for (const domain of domains) {
      const commands = groups.get(domain);
      if (!commands) continue;
      const group = makeGroup(domain, commands.length, showMatches || domain === selection?.domain);
      const resources = new Map();
      for (const cap of commands) {
        if (!resources.has(cap.resource)) resources.set(cap.resource, []);
        resources.get(cap.resource).push(cap);
      }
      const resourceNames = [...resources.keys()].sort((a, b) => a.localeCompare(b));
      // Put the workbook entry point first within the content family.
      const workbook = resourceNames.indexOf("workbook");
      if (domain === "content" && workbook > 0) resourceNames.unshift(...resourceNames.splice(workbook, 1));

      for (const resource of resourceNames) {
        const commands = resources.get(resource);
        const entries = document.createElement("ul");
        for (const cap of commands) entries.append(commandEntry(cap));
        if (resource) {
          const selectedResource = domain === selection?.domain && resource === selection.resource;
          const resourceGroup = makeGroup(resource, commands.length, showMatches || selectedResource);
          resourceGroup.classList.add("resource-group");
          resourceGroup.append(entries);
          group.append(resourceGroup);
        } else {
          group.append(entries);
        }
      }
      panels.push(group);
    }
    list.replaceChildren(...panels);
    updateGroupButton();
    showCommand(selection);
  }

  function makeGroup(label, count, open) {
    const group = document.createElement("details");
    group.className = "command-group";
    group.open = Boolean(open);
    const summary = text("summary", label);
    summary.append(text("span", count, "group-count"));
    group.append(summary);
    group.addEventListener("toggle", updateGroupButton);
    return group;
  }

  function commandEntry(cap) {
    const item = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.append(text("code", cap.surface.replace(/^tadx /, "")), text("span", effectLabel(cap), "row-effect"));
    if (cap.id === selected) {
      button.setAttribute("aria-current", "true");
      selectedButton = button;
    }
    button.addEventListener("click", () => {
      selectedButton?.removeAttribute("aria-current");
      selectedButton = button;
      selected = cap.id;
      button.setAttribute("aria-current", "true");
      const url = new URL(location.href);
      url.hash = new URLSearchParams({command: cap.id});
      history.replaceState(null, "", url);
      showCommand(cap);
      if (mobile.matches) {
        detail.querySelector("h3").focus({preventScroll: true});
        detailPane.scrollIntoView();
      }
    });
    item.append(button);
    return item;
  }

  function updateGroupButton() {
    const groups = [...list.querySelectorAll("details")];
    toggleGroups.textContent = groups.length && groups.every(group => group.open) ? "Collapse all" : "Expand all";
  }

  toggleGroups.addEventListener("click", () => {
    const groups = [...list.querySelectorAll("details")];
    const open = !groups.every(group => group.open);
    for (const group of groups) group.open = open;
  });

  function applyFilters() {
    updateURL();
    render();
  }

  controls.addEventListener("submit", event => event.preventDefault());
  search.addEventListener("input", applyFilters);
  controls.addEventListener("change", event => {
    if (event.target !== search) applyFilters();
  });
  controls.addEventListener("reset", event => {
    event.preventDefault();
    search.value = "";
    for (const field of fields) field.value = "";
    applyFilters();
    search.focus();
  });
  document.querySelector("#back-to-list").addEventListener("click", () => {
    if (selectedButton) {
      for (const group of list.querySelectorAll("details")) {
        if (group.contains(selectedButton)) group.open = true;
      }
      selectedButton.focus();
    } else {
      document.querySelector("#command-list-title").focus();
    }
  });
  document.addEventListener("keydown", event => {
    if (event.key === "/" && !event.ctrlKey && !event.metaKey && !event.altKey && !event.target.closest("input, textarea, select, [contenteditable]")) {
      event.preventDefault();
      search.focus();
    }
    if (event.key === "Escape" && event.target === search && search.value) {
      search.value = "";
      applyFilters();
    }
  });
  loadStatus.hidden = true;
  browser.hidden = false;
  render();
  if (requestedCommand === selected && mobile.matches) {
    detailPane.scrollIntoView();
  } else if (location.hash) {
    // Restore a page anchor after the command browser changes the page height.
    document.getElementById(location.hash.slice(1))?.scrollIntoView();
  }
}

init();
