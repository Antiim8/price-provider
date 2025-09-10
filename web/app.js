const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

const symbolsEl = $('#symbols');
const sideEl = $('#side');
const marketsEl = $('#markets');
const refreshEl = $('#refresh');
const statusEl = $('#status');
const tbody = $('#deals tbody');
const loadAllBtn = $('#loadAllBtn');

// Simple defaults
symbolsEl.value = localStorage.getItem('symbols') || 'AK-47 | Redline (Field-Tested)';
sideEl.value = localStorage.getItem('side') || 'all';
marketsEl.value = localStorage.getItem('markets') || '';
refreshEl.value = localStorage.getItem('refresh') || '15';

function saveState() {
  localStorage.setItem('symbols', symbolsEl.value);
  localStorage.setItem('side', sideEl.value);
  localStorage.setItem('markets', marketsEl.value);
  localStorage.setItem('refresh', refreshEl.value);
}

async function fetchLatest() {
  saveState();
  const symbolsCSV = symbolsEl.value.trim();
  if (!symbolsCSV) return;
  const side = sideEl.value;
  const markets = marketsEl.value.trim();
  const allSymbols = splitCSV(symbolsCSV);
  const chunkSize = 250; // keep requests light; server default limit is 1000
  const chunks = [];
  for (let i = 0; i < allSymbols.length; i += chunkSize) {
    chunks.push(allSymbols.slice(i, i + chunkSize));
  }
  const t0 = performance.now();
  statusEl.textContent = `Loading ${allSymbols.length} symbols…`;
  try {
    const results = [];
    let errors = 0;
    // fetch chunks sequentially but continue on errors to avoid stalling
    for (let i = 0; i < chunks.length; i++) {
      const params = new URLSearchParams({ symbols: chunks[i].join(','), side });
      if (markets) params.set('markets', markets);
      const url = `/api/latest?${params.toString()}`;
      statusEl.textContent = `Loading ${allSymbols.length} symbols… (${i+1}/${chunks.length})` + (errors ? `, errors: ${errors}` : '');
      try {
        const res = await fetch(url, { headers: { 'Accept': 'application/json' } });
        const text = await res.text();
        if (!res.ok) throw new Error(`HTTP ${res.status}: ${text.slice(0, 200)}`);
        const data = JSON.parse(text);
        if (data && Array.isArray(data.latest)) results.push(...data.latest);
      } catch (e) {
        // record and move on
        errors++;
      }
    }
    const merged = mergeLatest(results);
    renderTable(merged);
    const ms = Math.round(performance.now() - t0);
    statusEl.textContent = `Loaded ${merged.length} in ${ms} ms`;
  } catch (e) {
    statusEl.textContent = `Error: ${e.message}`;
    renderTable([]);
  }
}

function splitCSV(s) {
  return s.split(',').map(t => t.trim()).filter(Boolean);
}

function mergeLatest(rows) {
  const byKey = new Map();
  for (const r of rows) {
    const symbol = pick(r,'symbol','Symbol');
    const market = pick(r,'market','Market');
    const side = pick(r,'side','Side');
    const currency = pick(r,'currency','Currency');
    const key = `${symbol}|||${market}|||${side}|||${currency}`;
    const cur = byKey.get(key);
    if (!cur) { byKey.set(key, r); continue; }
    const a = Date.parse(pick(r,'received_at','ReceivedAt') || 0) || 0;
    const b = Date.parse(pick(cur,'received_at','ReceivedAt') || 0) || 0;
    if (a >= b) byKey.set(key, r);
  }
  return Array.from(byKey.values());
}

function pick(obj, a, b) { return (obj && (obj[a] ?? obj[b])) ?? ''; }

function renderTable(rows) {
  rows.sort((a, b) => pick(a,'symbol','Symbol').localeCompare(pick(b,'symbol','Symbol'))
    || pick(a,'market','Market').localeCompare(pick(b,'market','Market'))
    || pick(a,'side','Side').localeCompare(pick(b,'side','Side')));
  const now = Date.now();
  tbody.innerHTML = rows.map(r => {
    const symbol = pick(r,'symbol','Symbol');
    const market = pick(r,'market','Market');
    const side = pick(r,'side','Side');
    const currency = pick(r,'currency','Currency');
    const price = pick(r,'price','Price');
    const provider = pick(r,'provider','Provider');
    const receivedAt = pick(r,'received_at','ReceivedAt');
    const ts = receivedAt ? Date.parse(receivedAt) : now;
    const ageSec = Math.max(0, Math.round((now - ts) / 1000));
    const ageCls = ageSec > 60 ? 'age-old' : '';
    return `<tr>
      <td>${escapeHtml(symbol)}</td>
      <td>${escapeHtml(market)}</td>
      <td>${escapeHtml(side)}</td>
      <td>${escapeHtml(currency)}</td>
      <td class="price">${escapeHtml(price)}</td>
      <td>${escapeHtml(provider)}</td>
      <td class="${ageCls}" title="${receivedAt}">${ageSec}s ago</td>
    </tr>`;
  }).join('');
}

function escapeHtml(s) { return (s ?? '').toString().replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;','\'':'&#39;'}[c])); }

$('#fetchBtn').addEventListener('click', () => fetchLatest());

let timer = null;
function setupAutoRefresh() {
  if (timer) clearInterval(timer);
  const sec = Math.max(0, parseInt(refreshEl.value || '0', 10));
  if (sec > 0) timer = setInterval(fetchLatest, sec * 1000);
}
refreshEl.addEventListener('change', setupAutoRefresh);

setupAutoRefresh();
fetchLatest();

// Load all item names from backend then fetch
async function loadAllItems() {
  try {
    statusEl.textContent = 'Loading all item names…';
    const res = await fetch('/api/items', { headers: { 'Accept': 'application/json' } });
    const text = await res.text();
    if (!res.ok) throw new Error(`HTTP ${res.status}: ${text.slice(0,200)}`);
    const data = JSON.parse(text);
    const items = Array.isArray(data.items) ? data.items : [];
    if (items.length === 0) { statusEl.textContent = 'No items returned'; return; }
    symbolsEl.value = items.join(', ');
    await fetchLatest();
  } catch (e) {
    statusEl.textContent = `Error loading items: ${e.message}`;
  }
}

if (loadAllBtn) loadAllBtn.addEventListener('click', loadAllItems);
