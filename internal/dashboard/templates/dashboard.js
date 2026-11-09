let apiKey = '';
let currentDimension = 'feature';
let cursors = [null]; // cursors[i] = cursor used to load page i
let page = 0;
let nextCursor = null;

const $ = (id) => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`Element not found: ${id}`);
  return el;
};
const setText = (id, text) => { $(id).textContent = text; };
const show = (id, visible) => { $(id).style.display = visible ? 'flex' : 'none'; };
const fmt = (n) => Number(n || 0).toLocaleString();
const usd = (v) => `$${v ?? '0'}`;
const pct = (part, total, fallback = 0) =>
  total > 0 ? Math.round((part / total) * 100) : fallback;

function cell(value, className = '') {
  const td = document.createElement('td');
  td.className = className;
  td.textContent = value ?? '';
  return td;
}

function badgeCell(text, type = 'neutral') {
  const td = document.createElement('td');
  const span = document.createElement('span');
  span.className = `badge ${type}`;
  span.textContent = text;
  td.appendChild(span);
  return td;
}

// Fills a table body with rows, or an empty-state row if there are no items.
function renderRows(tbodyId, items, colspan, emptyMsg, buildCells) {
  const tbody = $(tbodyId);
  tbody.replaceChildren();

  if (!items?.length) {
    const row = tbody.insertRow();
    const td = cell(emptyMsg, 'empty-state');
    td.colSpan = colspan;
    row.appendChild(td);
    return false;
  }

  items.forEach((item) => {
    const row = document.createElement('tr');
    row.append(...buildCells(item));
    tbody.appendChild(row);
  });
  return true;
}

const formatDate = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

const showError = (msg) => { setText('error-message', msg); show('error-banner', true); };
const hideError = () => show('error-banner', false);

// ---------- Auth ----------
window.addEventListener('DOMContentLoaded', () => {
  const params = new URLSearchParams(window.location.search);
  const keyParam = params.get('key')?.trim();

  if (keyParam) {
    apiKey = keyParam;
    sessionStorage.setItem('tracker_api_key', apiKey);

    // Remove API key from browser URL
    params.delete('key');
    const query = params.toString();
    const { pathname, hash } = window.location;
    window.history.replaceState({}, '', `${pathname}${query ? `?${query}` : ''}${hash}`);
  } else {
    apiKey = sessionStorage.getItem('tracker_api_key') || '';
  }

  if (apiKey) $('api-key-input').value = apiKey;
  updateAuthUI();
  setDatePreset('30d');
});

function applyKey() {
  const input = $('api-key-input').value.trim();
  if (!input) return showError('Please enter your Tracker API Key.');

  apiKey = input;
  sessionStorage.setItem('tracker_api_key', apiKey);
  updateAuthUI();
  loadAllMetrics();
}

function clearKey() {
  apiKey = '';
  sessionStorage.removeItem('tracker_api_key');
  $('api-key-input').value = '';
  updateAuthUI();
}

function updateAuthUI() {
  const hasKey = Boolean(apiKey);
  show('auth-group', !hasKey);
  show('tenant-display', hasKey);
  if (!hasKey) return;

  const [a, b, c] = apiKey.split('_');
  setText('tenant-id-val', c ? `${a}_${b}_${c.slice(0, 8)}...` : `${apiKey.slice(0, 16)}...`);
}

function setDatePreset(preset) {
  const days = { today: 0, '7d': 7, '30d': 30 };
  Object.keys(days).forEach((key) =>
    $(`btn-${key}`).classList.toggle('active', key === preset)
  );

  const to = new Date();
  const from = new Date(to);
  from.setDate(to.getDate() - (days[preset] ?? 0));

  $('date-from').value = formatDate(from);
  $('date-to').value = formatDate(to);

  if (apiKey) loadAllMetrics();
}

const getDateParams = () =>
  new URLSearchParams({ from: $('date-from').value, to: $('date-to').value });

async function apiFetch(path, params = getDateParams()) {
  const response = await fetch(`/api/v1/${path}?${params}`, {
    headers: { Authorization: `Bearer ${apiKey}`, Accept: 'application/json' }
  });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const error = await response.json();
      if (error.message) message += `: ${error.message}`;
    } catch {
      console.log('response does not contain valid JSON');
    }
    throw new Error(message);
  }
  return response.json();
}

async function loadAllMetrics() {
  if (!apiKey) return showError('Please enter your Tracker API Key to load metrics.');
  hideError();
  show('loading-banner', true);

  try {
    await Promise.all([
      loadSummary(),
      loadDailyCosts(),
      loadModels(),
      loadTopSpenders(),
      resetRequestsPage()
    ]);
  } catch (error) {
    showError(error.message || 'Failed to load dashboard metrics.');
  } finally {
    show('loading-banner', false);
  }
}

async function loadSummary() {
  const d = await apiFetch('summary');

  // Cost
  setText('kpi-cost', usd(d.estimated_cost_usd || '0.000000'));
  setText('kpi-microdollars', fmt(d.estimated_cost_microdollars));

  // Pricing status
  const incomplete = Number(d.incomplete_usage_count || 0);
  const badge = $('badge-pricing');
  let label = 'Complete';
  let type = 'success';
  let notice = '';

  if (d.pricing_status === 'partial') {
    label = 'Partial Pricing';
    type = 'warning';
    notice = `Unpriced models detected in this period. ${incomplete} requests had unpriced or missing metrics.`;
  } else if (incomplete > 0) {
    label = 'Complete Rates';
    notice = `${incomplete} requests had incomplete usage (e.g. cancelled stream or unrecorded usage).`;
  }

  badge.textContent = label;
  badge.className = `badge ${type}`;
  show('uncertainty-banner', Boolean(notice));
  if (notice) setText('uncertainty-text', notice);

  // Requests
  const total = Number(d.total_requests || 0);
  const ok = Number(d.successful_requests || 0);
  setText('kpi-requests', fmt(total));
  setText('kpi-req-success', `${fmt(ok)} 2xx`);
  setText('kpi-req-fail', `${fmt(d.failed_requests)} 4xx/5xx`);
  setText('kpi-success-rate', `${pct(ok, total, 100)}% OK`);

  // Tokens
  const tokens = Number(d.total_tokens || 0);
  const cached = Number(d.total_cached_tokens || 0);
  setText('kpi-tokens', fmt(tokens));
  setText('kpi-tokens-in', fmt(d.total_input_tokens));
  setText('kpi-tokens-out', fmt(d.total_output_tokens));
  setText('kpi-tokens-cache', fmt(cached));
  setText('kpi-cached-ratio', `${pct(cached, tokens)}% Cached`);

  // Latency
  setText('kpi-latency-avg', `${d.avg_latency_ms || 0} ms`);
  setText('kpi-latency-p95', d.p95_latency_ms || 0);
  setText('kpi-latency-min', d.min_latency_ms || 0);
  setText('kpi-latency-max', d.max_latency_ms || 0);
}

// Daily chart
async function loadDailyCosts() {
  const data = await apiFetch('daily-costs');
  const chart = $('chart-container');
  chart.replaceChildren();

  if (!data.items?.length) {
    const empty = document.createElement('div');
    empty.className = 'empty-state';
    empty.textContent = 'No daily usage recorded for this period.';
    chart.appendChild(empty);
    return;
  }

  const maxCost = Math.max(...data.items.map((i) => Number(i.estimated_cost_microdollars || 0)), 1);
  data.items.forEach((item) => {
    const cost = Number(item.estimated_cost_microdollars || 0);

    const make = (className, text) => {
      const el = document.createElement('div');
      el.className = className;
      if (text !== undefined) el.textContent = text;
      return el;
    };

    const bar = make('chart-bar');
    bar.style.height = `${Math.max(Math.round((cost / maxCost) * 140), 6)}px`;
    bar.title = `${item.date}: $${item.estimated_cost_usd} (${item.requests} requests, ${fmt(item.total_tokens)} tokens)`;

    const col = make('chart-bar-col');
    col.append(
      make('chart-bar-cost', usd(item.estimated_cost_usd)),
      bar,
      make('chart-bar-label', String(item.date || '').slice(5))
    );
    chart.appendChild(col);
  });
}

// Models 
async function loadModels() {
  const data = await apiFetch('models');
  renderRows('models-table-body', data.items, 5, 'No model usage recorded in this period.', (i) => [
    cell(i.model, 'mono'),
    cell(fmt(i.requests)),
    cell(fmt(i.input_tokens)),
    cell(fmt(i.output_tokens)),
    cell(usd(i.estimated_cost_usd), 'mono')
  ]);
}

//Top spenders
function setSpenderDimension(dimension) {
  if (!['feature', 'user'].includes(dimension)) return;
  currentDimension = dimension;

  ['feature', 'user'].forEach((d) =>
    $(`btn-spender-${d}`).classList.toggle('active', d === dimension)
  );
  setText('spender-col-title', dimension === 'feature' ? 'Feature' : 'User ID');

  loadTopSpenders().catch((e) => showError(e.message || 'Failed to load top spenders.'));
}

async function loadTopSpenders() {
  const params = getDateParams();
  params.set('by', currentDimension);
  params.set('limit', '10');

  const data = await apiFetch('top-spenders', params);
  renderRows('spenders-table-body', data.items, 4, `No attributed ${currentDimension}s recorded.`, (i) => [
    cell(i.key, 'mono'),
    cell(fmt(i.requests)),
    cell(fmt(i.total_tokens)),
    cell(usd(i.estimated_cost_usd), 'mono')
  ]);
}

// Requests table and pagination 
const USAGE_BADGES = {
  complete: ['Complete', 'success'],
  unpriced_model: ['Unpriced', 'warning'],
  missing_usage: ['Missing Usage', 'error']
};

const tokenOrDash = (v) => (v != null ? fmt(v) : '—');

function updatePager() {
  $('btn-prev-page').disabled = page === 0;
  $('btn-next-page').disabled = !nextCursor;
  setText('pagination-status', `Page ${page + 1}`);
}

async function resetRequestsPage() {
  cursors = [null];
  await fetchRequestsPage(0);
}

const loadNextRequestsPage = () => nextCursor && goToPage(page + 1, 'next');
const loadPreviousRequestsPage = () => page > 0 && goToPage(page - 1, 'previous');

async function goToPage(index, label) {
  try {
    await fetchRequestsPage(index);
  } catch (error) {
    showError(error.message || `Failed to load ${label} page.`);
  }
}

async function fetchRequestsPage(index) {
  const params = getDateParams();
  params.set('limit', '15');
  if (cursors[index]) params.set('cursor', cursors[index]);

  const data = await apiFetch('requests', params);

  renderRows('requests-table-body', data.items, 10, 'No requests recorded for this period.', (r) => {
    const idCell = cell(r.id ? `${String(r.id).slice(0, 8)}...` : '—', 'mono');
    if (r.id) idCell.title = String(r.id);

    const status = Number(r.status_code);
    const [usageLabel, usageType] =
      USAGE_BADGES[r.usage_status] || [r.usage_status || 'Unknown', 'neutral'];

    return [
      cell(r.created_at ? String(r.created_at).replace('T', ' ').slice(0, 19) : '—', 'mono'),
      idCell,
      cell([r.feature_name, r.user_id].filter(Boolean).join(' / ') || '—'),
      cell(r.reported_model || r.requested_model || '—', 'mono'),
      badgeCell(r.status_code ?? 'Unknown', status >= 200 && status < 300 ? 'success' : 'error'),
      badgeCell(r.is_streaming ? 'SSE' : 'Sync'),
      cell(`${r.latency_ms ?? 0} ms`, 'mono'),
      cell(
        `${tokenOrDash(r.input_tokens)} / ${tokenOrDash(r.output_tokens)} / ${fmt(r.cached_input_tokens)}`,
        'mono'
      ),
      badgeCell(usageLabel, usageType),
      cell(r.estimated_cost_usd != null ? `$${r.estimated_cost_usd}` : 'Unavailable', 'mono')
    ];
  });

  page = index;
  nextCursor = data.items?.length ? data.next_cursor || null : null;
  cursors[page + 1] = nextCursor;
  updatePager();
}