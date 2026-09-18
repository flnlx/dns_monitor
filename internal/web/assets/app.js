'use strict';

(() => {
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
  const state = { token: '', data: null, range: '24h', detailRange: '24h', page: 'overview', sort: 'average_ms', descending: false, detailID: null, history: [], results: [], result: null, refreshTimer: null, refreshing: false, configDirty: false, loadingResults: null, resultEnd: false, detailRequest: 0, resultsRequest: 0, serviceBusy: false };
  const ranges = { '24h': 24 * 3600000, '7d': 7 * 86400000, '30d': 30 * 86400000 };
  const titles = { overview: ['监测总览', 'NETWORK OBSERVABILITY'], config: ['探测配置', 'MONITORING PREFERENCES'], service: ['Windows 服务', 'ALWAYS-ON MONITORING'], guide: ['使用指南', 'YOUR OBSERVATION HANDBOOK'] };
  const escape = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
  const number = (value, digits = 1) => Number.isFinite(Number(value)) ? Number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits }) : '—';
  const integer = value => number(value, 0);
  const timestamp = (value, full = false) => value ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', ...(full ? { second: '2-digit' } : {}), hour12: false }) : '尚未探测';
  const hasSamples = metrics => (metrics?.samples || 0) > 0;
  const hasLatency = metrics => hasSamples(metrics) && (metrics.success_rate > 0 || metrics.average_ms > 0 || metrics.p95_ms > 0);
  const percent = (value, metrics) => hasSamples(metrics) ? `${number(value)}%` : '—';
  const currentServer = () => state.data?.servers.find(server => server.id === state.detailID);
  const pollutionKind = value => value === 'polluted' ? 'polluted' : value === 'clean' ? 'clean' : 'unknown';
  const pollutionLabel = value => ({ polluted: '存在污染', clean: '未发现污染', unknown: '尚未确定' })[pollutionKind(value)];
  const badge = value => `<span class="pollution-badge ${pollutionKind(value)}"><span aria-hidden="true">${pollutionKind(value) === 'polluted' ? '!' : pollutionKind(value) === 'clean' ? '✓' : '·'}</span>${pollutionLabel(value)}</span>`;
  const gradeHTML = value => ['A', 'B', 'C', 'D', 'F'].includes(value) ? `<span class="grade grade-${value.toLowerCase()}" title="${({ A: '优秀', B: '良好', C: '一般', D: '较差', F: '污染' })[value]}">${value}</span>` : '<span class="grade grade-pending">待评估</span>';

  function setError(selector, message) {
    const element = $(selector);
    element.textContent = message || '';
    element.hidden = !message;
  }

  function toast(message, error = false) {
    const element = document.createElement('div');
    element.className = `toast${error ? ' error' : ''}`;
    element.textContent = message;
    $('#toast-stack').append(element);
    setTimeout(() => element.remove(), error ? 6500 : 4000);
  }

  function closeDialogs() {
    $$('dialog[open]').forEach(dialog => dialog.close());
  }

  function resetSession(message = '') {
    state.token = '';
    clearTimeout(state.refreshTimer);
    closeDialogs();
    $('#app').hidden = true;
    $('#login-screen').hidden = false;
    setError('#login-error', message);
    $('#access-key').value = '';
    $('#access-key').focus();
  }

  async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});
    headers.set('X-DNSMonitor-Token', state.token);
    if (options.body !== undefined && typeof options.body !== 'string') {
      headers.set('Content-Type', 'application/json');
      options.body = JSON.stringify(options.body);
    }
    const response = await fetch(`/api${path}`, { ...options, headers, cache: 'no-store' });
    let body;
    try { body = await response.json(); } catch { body = null; }
    if (!response.ok) {
      if (response.status === 401) resetSession('会话已失效，请重新输入访问密钥。');
      throw new Error(body?.error || `请求未完成（HTTP ${response.status}）`);
    }
    return body;
  }

  async function login(event) {
    event.preventDefault();
    const button = $('.login-submit');
    const accessKey = $('#access-key').value.trim();
    if (!accessKey) return;
    button.disabled = true;
    setError('#login-error', '');
    try {
      const bytes = new TextEncoder().encode(`admin:${accessKey}`);
      const authorization = `Basic ${btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(''))}`;
      const response = await fetch('/api/session', { headers: { Authorization: authorization }, cache: 'no-store' });
      let body;
      try { body = await response.json(); } catch { body = null; }
      if (!response.ok || !body?.token) throw new Error(response.status === 401 ? '访问密钥不正确，请检查 data/access-key.txt。' : body?.error || '无法连接观测台，请确认程序正在运行。');
      state.token = body.token;
      $('#access-key').value = '';
      $('#login-screen').hidden = true;
      $('#app').hidden = false;
      await refreshState(true);
      scheduleRefresh();
    } catch (error) {
      setError('#login-error', error.message === 'Failed to fetch' ? '连接失败，请确认 DNS Monitor 正在运行。' : error.message);
    } finally {
      button.disabled = false;
    }
  }

  async function logout() {
    if (state.configDirty && !(await confirmAction('退出观测台', '探测配置有未保存的更改，退出将放弃这些更改。', '退出'))) return;
    try { await api('/session', { method: 'DELETE' }); } catch { /* The local session is cleared even if the server is unreachable. */ }
    state.configDirty = false;
    state.data = null;
    resetSession();
  }

  function scheduleRefresh() {
    clearTimeout(state.refreshTimer);
    if (!state.token) return;
    state.refreshTimer = setTimeout(async () => {
      if (!document.hidden) await refreshState();
      scheduleRefresh();
    }, 15000);
  }

  async function refreshState(initial = false) {
    if (!state.token || state.refreshing) return;
    state.refreshing = true;
    const button = $('#refresh-button');
    button.disabled = true;
    try {
      const response = await api(`/state?range=${encodeURIComponent(state.range)}`);
      state.data = response;
      state.data.servers ||= [];
      renderOverview();
      renderRuntime();
      renderService();
      if (initial || (state.page === 'config' && !state.configDirty)) populateConfig();
      $('#restart-notice').hidden = !response.listen_restart_required;
      $('#last-refresh').textContent = `更新于 ${new Date().toLocaleTimeString('zh-CN', { hour12: false })}`;
      setError('#global-error', response.runtime?.last_error ? `最近运行提示：${response.runtime.last_error}` : '');
    } catch (error) {
      if (state.token) setError('#global-error', `无法更新数据：${error.message === 'Failed to fetch' ? '连接中断，请检查程序或 Windows 服务是否运行。' : error.message}`);
    } finally {
      state.refreshing = false;
      button.disabled = false;
    }
  }

  function renderRuntime() {
    const runtime = state.data.runtime || {};
    const config = state.data.config || {};
    const paused = runtime.paused || config.concurrency === 0;
    $('#runtime-title').textContent = paused ? '探测已暂停' : '调度器运行中';
    $('#runtime-dot').className = `status-dot${paused ? ' paused' : runtime.last_error ? ' error' : ''}`;
    $('#runtime-description').textContent = paused ? '并发数设为 0，保存记录仍可查看。' : `${runtime.active || 0} / ${config.concurrency ?? '—'} 并发 · 基础周期 ${config.interval_seconds || '—'} 秒`;
    $('#version').textContent = state.data.version ? `v${String(state.data.version).replace(/^v/, '')}` : 'DNS Monitor';
    $('#nav-server-count').textContent = state.data.servers.length;
  }

  function renderOverview() {
    const servers = state.data.servers;
    const enabled = servers.filter(server => server.enabled);
    const online = enabled.filter(server => server.last_probe && server.last_success);
    const polluted = servers.filter(server => pollutionKind(server.metrics?.pollution) === 'polluted');
    const sampled = servers.filter(server => hasSamples(server.metrics));
    const responding = sampled.filter(server => hasLatency(server.metrics));
    const average = responding.length ? responding.reduce((sum, server) => sum + server.metrics.average_ms, 0) / responding.length : null;
    const samples = sampled.reduce((sum, server) => sum + server.metrics.samples, 0);
    const cards = [
      { title: '监测服务器', value: integer(servers.length), unit: '台', caption: `${enabled.length} 台已启用 · ${servers.filter(server => server.trusted).length} 台可信 DNS`, icon: '◫', kind: '' },
      { title: '最近查询正常', value: integer(online.length), unit: '台', caption: `${enabled.filter(server => server.last_probe && !server.last_success).length} 台异常 · ${enabled.filter(server => !server.last_probe).length} 台待首次探测`, icon: '↗', kind: 'good' },
      { title: '存在解析污染', value: integer(polluted.length), unit: '台', caption: `所选区间 · ${servers.filter(server => pollutionKind(server.metrics?.pollution) === 'unknown').length} 台尚未确定`, icon: '!', kind: polluted.length ? 'bad' : '' },
      { title: '服务器平均时延', value: average === null ? '—' : number(average, 0), unit: 'ms', caption: `${responding.length} 台响应服务器均值 · ${integer(samples)} 次采样`, icon: '⌁', kind: '' }
    ];
    $('#summary-cards').innerHTML = cards.map(card => `<article class="summary-card ${card.kind}"><div class="summary-top"><span>${escape(card.title)}</span><span class="summary-icon" aria-hidden="true">${escape(card.icon)}</span></div><div class="summary-value">${card.value}<small>${card.unit}</small></div><p class="summary-caption">${escape(card.caption)}</p></article>`).join('');
    $('#server-total').textContent = servers.length;
    const selectedProtocol = $('#protocol-filter').value;
    const protocols = [...new Set(servers.map(server => server.protocol).filter(Boolean))].sort();
    $('#protocol-filter').innerHTML = `<option value="">全部协议</option>${protocols.map(protocol => `<option value="${escape(protocol)}">${escape(protocol.toUpperCase())}</option>`).join('')}`;
    $('#protocol-filter').value = protocols.includes(selectedProtocol) ? selectedProtocol : '';
    renderServerRows();
  }

  function relativeProbe(server) {
    if (!server.enabled) return '已停止调度';
    if (!server.last_probe) return '等待首次采样';
    if (state.data.runtime?.paused) return '调度已暂停';
    if (server.next_due > Date.now()) {
      const seconds = Math.ceil((server.next_due - Date.now()) / 1000);
      const text = seconds >= 60 ? `${Math.ceil(seconds / 60)} 分钟后` : `${seconds} 秒后`;
      return `${server.failures >= 3 && state.data.config?.smart_backoff && state.data.config?.max_backoff_hours > 0 ? '退避 · ' : ''}${text}`;
    }
    return '等待下一次探测';
  }

  function rateCell(value, metrics) {
    if (!hasSamples(metrics)) return '<span class="muted">—</span>';
    const rate = Math.max(0, Math.min(100, Number(value) || 0));
    return `<div class="rate-cell"><span>${number(rate)}%</span><span class="mini-track" aria-hidden="true"><span class="${rate < 90 ? 'bad' : rate < 99 ? 'warning' : ''}" style="width:${rate}%"></span></span></div>`;
  }

  function renderServerRows() {
    if (!state.data) return;
    const search = $('#search-filter').value.trim().toLowerCase();
    const protocol = $('#protocol-filter').value;
    const pollution = $('#pollution-filter').value;
    const grade = $('#grade-filter').value;
    const servers = state.data.servers.filter(server => {
      const metrics = server.metrics || {};
      const gradeValue = ['A', 'B', 'C', 'D', 'F'].includes(metrics.grade) ? metrics.grade : 'pending';
      return (!search || [server.name, server.provider, server.address].some(value => String(value || '').toLowerCase().includes(search))) && (!protocol || server.protocol === protocol) && (!pollution || pollutionKind(metrics.pollution) === pollution) && (!grade || gradeValue === grade);
    });
    servers.sort((left, right) => {
      const lm = left.metrics || {};
      const rm = right.metrics || {};
      const comparable = state.sort === 'average_ms' ? hasLatency : hasSamples;
      if (!comparable(lm) || !comparable(rm)) return comparable(lm) ? -1 : comparable(rm) ? 1 : left.id - right.id;
      const difference = (lm[state.sort] || 0) - (rm[state.sort] || 0);
      return (state.descending ? -difference : difference) || left.id - right.id;
    });
    $('#server-rows').innerHTML = servers.map(server => {
      const metrics = server.metrics || {};
      const known = !!server.last_probe;
      const statusClass = !server.enabled || !known ? 'unknown' : server.last_success ? '' : 'offline';
      const status = !server.enabled ? '已停用' : !known ? '待探测' : server.last_success ? '查询正常' : '查询异常';
      return `<tr data-server-id="${server.id}"><td><button class="server-name-button" data-action="detail" data-id="${server.id}" title="查看 ${escape(server.name)} 的历史记录"><span class="server-avatar" aria-hidden="true">${escape((server.provider || server.name || 'D').slice(0, 1).toUpperCase())}</span><span><span class="server-name"><span class="name-text">${escape(server.name)}</span>${server.trusted ? '<span class="trusted-mark">可信</span>' : ''}</span><span class="server-secondary" title="${escape(server.address)}">${escape(server.provider || server.address)}</span></span></button></td><td title="最后探测：${escape(timestamp(server.last_probe, true))}"><span class="status-label ${statusClass}"><span class="status-dot"></span>${status}</span><div class="sub-status">${escape(relativeProbe(server))}</div></td><td><span class="protocol-badge">${escape((server.protocol || '—').toUpperCase())}</span></td><td><span class="latency-value">${hasLatency(metrics) ? number(metrics.average_ms, 0) : '—'}<small>ms</small></span></td><td>${rateCell(metrics.availability, metrics)}</td><td>${rateCell(metrics.success_rate, metrics)}</td><td>${badge(metrics.pollution)}</td><td>${gradeHTML(metrics.grade)}</td><td class="actions-cell"><div class="row-actions"><button class="row-action" data-action="edit" data-id="${server.id}" aria-label="编辑 ${escape(server.name)}">编辑</button><button class="row-action delete" data-action="delete" data-id="${server.id}" aria-label="删除 ${escape(server.name)}">删除</button></div></td></tr>`;
    }).join('');
    $('#servers-empty').hidden = servers.length > 0;
    if (!servers.length) {
      $('#servers-empty h3').textContent = state.data.servers.length ? '没有符合条件的服务器' : '从第一台 DNS 开始';
      $('#servers-empty p').textContent = state.data.servers.length ? '调整搜索内容或筛选条件，继续查看其他服务器。' : '添加服务器和可信 DNS，观测台会按配置自动开始采样。';
      $('#empty-add-button').hidden = state.data.servers.length > 0;
    }
    $('#table-count').textContent = `显示 ${servers.length} / ${state.data.servers.length} 台服务器`;
    $$('.sort-button').forEach(button => {
      const active = button.dataset.sort === state.sort;
      button.classList.toggle('active', active);
      $('span', button).textContent = active ? (state.descending ? '↓' : '↑') : '↕';
      button.closest('th').setAttribute('aria-sort', active ? state.descending ? 'descending' : 'ascending' : 'none');
    });
  }

  async function navigate(page) {
    if (!titles[page] || state.page === page) return;
    if (state.page === 'config' && state.configDirty) {
      if (!(await confirmAction('离开探测配置', '当前更改尚未保存，离开将放弃这些更改。', '放弃更改'))) return;
      state.configDirty = false;
    }
    state.page = page;
    $$('.page').forEach(section => { section.hidden = section.id !== `page-${page}`; });
    $$('.nav-item').forEach(button => {
      const active = button.dataset.page === page;
      button.classList.toggle('active', active);
      if (active) button.setAttribute('aria-current', 'page'); else button.removeAttribute('aria-current');
    });
    $('#page-title').textContent = titles[page][0];
    $('#page-eyebrow').textContent = titles[page][1];
    $('#add-server-button').hidden = page !== 'overview';
    if (page === 'config') populateConfig();
    if (page === 'service') refreshService();
    window.scrollTo({ top: 0, behavior: 'instant' });
  }

  async function changeRange(range) {
    if (range === state.range || !ranges[range]) return;
    state.range = range;
    $$('[data-range]').forEach(button => {
      button.classList.toggle('active', button.dataset.range === range);
      button.setAttribute('aria-pressed', String(button.dataset.range === range));
    });
    // An in-flight refresh may still use the previous range, so refresh again once it finishes.
    while (state.refreshing) await new Promise(resolve => setTimeout(resolve, 50));
    await refreshState();
  }

  function openServer(server = null) {
    const form = $('#server-form');
    form.reset();
    form.elements.id.value = server?.id || '';
    ['name', 'provider', 'address', 'notes'].forEach(field => { form.elements[field].value = server?.[field] || ''; });
    form.elements.enabled.checked = server ? server.enabled : true;
    form.elements.trusted.checked = server ? server.trusted : false;
    $('#server-dialog-title').textContent = server ? '编辑 DNS 服务器' : '添加 DNS 服务器';
    setError('#server-error', '');
    $('#server-dialog').showModal();
  }

  async function saveServer(event) {
    event.preventDefault();
    const form = event.currentTarget;
    const id = Number(form.elements.id.value);
    const body = {};
    ['name', 'provider', 'address', 'notes'].forEach(field => { body[field] = form.elements[field].value.trim(); });
    body.enabled = form.elements.enabled.checked;
    body.trusted = form.elements.trusted.checked;
    if (id) body.id = id;
    const button = $('#save-server-button');
    button.disabled = true;
    setError('#server-error', '');
    try {
      await api(`/servers${id ? `/${id}` : ''}`, { method: id ? 'PUT' : 'POST', body });
      $('#server-dialog').close();
      toast(id ? '服务器配置已更新' : 'DNS 服务器已添加');
      await refreshState();
      if (state.detailID === id && $('#detail-dialog').open) renderDetailHeader();
    } catch (error) { setError('#server-error', error.message); }
    finally { button.disabled = false; }
  }

  async function deleteServer(id) {
    const server = state.data.servers.find(item => item.id === id);
    if (!server || !(await confirmAction('删除 DNS 服务器', `确定删除“${server.name}”？\n该服务器及其关联历史数据将被删除，此操作无法撤销。`, '删除服务器'))) return;
    try {
      await api(`/servers/${id}`, { method: 'DELETE' });
      if (state.detailID === id) $('#detail-dialog').close();
      toast('服务器已删除');
      await refreshState();
    } catch (error) { toast(error.message, true); }
  }

  function domainRow(domain = { name: '', type: 'A' }) {
    const row = document.createElement('div');
    row.className = 'domain-row';
    const types = ['A', 'CNAME', 'TXT', 'NS', 'MX', 'SOA', 'SRV', 'CAA', 'HTTPS', 'SVCB', 'PTR'];
    row.innerHTML = `<input type="text" class="domain-name" aria-label="探测域名" placeholder="example.com" spellcheck="false" maxlength="253" required value="${escape(domain.name)}"><select class="domain-type-select" aria-label="记录类型">${types.map(type => `<option value="${type}"${type === domain.type ? ' selected' : ''}>${type}</option>`).join('')}</select><button class="icon-button remove-domain" type="button" aria-label="删除这个探测域名">×</button>`;
    return row;
  }

  function populateConfig() {
    if (!state.data?.config) return;
    const form = $('#config-form');
    const config = state.data.config;
    ['listen', 'interval_seconds', 'timeout_seconds', 'concurrency', 'max_backoff_hours', 'reference_ttl_seconds'].forEach(field => { form.elements[field].value = config[field] ?? ''; });
    form.elements.smart_backoff.checked = config.smart_backoff;
    $('#domain-rows').replaceChildren(...(config.domains || []).map(domainRow));
    if (!config.domains?.length) $('#domain-rows').append(domainRow());
    state.configDirty = false;
    $('#config-save-state').textContent = '当前配置已加载';
    $('#config-save-state').classList.remove('unsaved');
    setError('#config-error', '');
  }

  function markConfigDirty() {
    state.configDirty = true;
    $('#config-save-state').textContent = '有尚未保存的更改';
    $('#config-save-state').classList.add('unsaved');
  }

  async function saveConfig(event) {
    event.preventDefault();
    const form = event.currentTarget;
    const body = { listen: form.elements.listen.value.trim(), smart_backoff: form.elements.smart_backoff.checked, domains: $$('.domain-row').map(row => ({ name: $('.domain-name', row).value.trim(), type: $('.domain-type-select', row).value })) };
    ['interval_seconds', 'timeout_seconds', 'concurrency', 'max_backoff_hours', 'reference_ttl_seconds'].forEach(field => { body[field] = Number(form.elements[field].value); });
    const button = $('#save-config-button');
    button.disabled = true;
    setError('#config-error', '');
    try {
      const response = await api('/config', { method: 'PUT', body });
      state.data.config = response.config || body;
      state.configDirty = false;
      $('#config-save-state').textContent = '配置已保存';
      $('#config-save-state').classList.remove('unsaved');
      $('#restart-notice').hidden = !response.restart_required;
      toast(response.restart_required ? '配置已保存，监听地址将在重启后生效' : body.concurrency === 0 ? '配置已保存，全部探测已暂停' : '探测配置已保存');
      await refreshState();
    } catch (error) { setError('#config-error', error.message); }
    finally { button.disabled = false; }
  }

  async function openDetail(id) {
    state.detailID = id;
    state.detailRange = state.range;
    state.results = [];
    state.history = [];
    state.resultEnd = false;
    renderDetailHeader();
    $('#detail-metrics').innerHTML = '';
    $('#result-rows').innerHTML = '<tr><td colspan="5" class="muted" style="text-align:center;padding:28px">正在读取原始记录…</td></tr>';
    $('#results-count').textContent = '';
    $('#results-more').hidden = true;
    setError('#detail-error', '');
    $('#detail-dialog').showModal();
    renderCharts();
    await Promise.allSettled([loadHistory(), loadResults(true)]);
  }

  function renderDetailHeader() {
    const server = currentServer();
    if (!server) return;
    $('#detail-title').textContent = server.name;
    $('#detail-address').textContent = server.address;
    $('#detail-tags').innerHTML = `<span class="protocol-badge">${escape((server.protocol || '—').toUpperCase())}</span>${server.provider ? `<span class="tag">${escape(server.provider)}</span>` : ''}${server.trusted ? '<span class="tag">用户可信 DNS</span>' : ''}<span class="tag">${server.enabled ? '已启用' : '已停用'}</span><span class="tag">最近 ${escape(timestamp(server.last_probe))}</span>`;
    $('#detail-probe').disabled = !server.enabled || !!state.data.runtime?.paused || state.data.config?.concurrency === 0;
    $$('[data-detail-range]').forEach(button => {
      const active = button.dataset.detailRange === state.detailRange;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
  }

  async function loadHistory() {
    const request = ++state.detailRequest;
    const id = state.detailID;
    const range = state.detailRange;
    renderDetailHeader();
    try {
      const response = await api(`/servers/${id}/history?range=${range}`);
      if (request !== state.detailRequest || id !== state.detailID || range !== state.detailRange) return;
      state.history = (response.points || []).slice().sort((left, right) => left.timestamp - right.timestamp);
      const metrics = response.metrics || {};
      $('#detail-metrics').innerHTML = `<div class="detail-metric"><span>可用率</span><strong>${percent(metrics.availability, metrics)}</strong></div><div class="detail-metric"><span>查询成功率</span><strong>${percent(metrics.success_rate, metrics)}</strong></div><div class="detail-metric"><span>P95 时延</span><strong>${hasLatency(metrics) ? number(metrics.p95_ms, 0) : '—'}<small>ms</small></strong></div><div class="detail-metric"><span>原始采样</span><strong>${integer(metrics.samples || 0)}<small>次</small></strong></div><div class="detail-metric"><span>时间覆盖率</span><strong>${hasSamples(metrics) ? number(metrics.coverage) + '%' : '—'}</strong></div><div class="detail-metric"><span>质量评级</span>${gradeHTML(metrics.grade)}</div>`;
      $('#chart-note').textContent = `图表按时间聚合，空白表示未采样；当前区间 ${integer(metrics.samples || 0)} 次采样，${pollutionLabel(metrics.pollution)}。`;
      renderCharts();
    } catch (error) { if (request === state.detailRequest) setError('#detail-error', `历史读取失败：${error.message}`); }
  }

  async function loadResults(reset = false) {
    const id = state.detailID;
    if (state.loadingResults === id) return;
    const request = ++state.resultsRequest;
    state.loadingResults = id;
    $('#results-more').disabled = true;
    $('#results-refresh').disabled = true;
    try {
      const last = state.results.at(-1);
      const before = !reset && last ? `&before=${last.timestamp}&before_id=${last.id}` : '';
      const response = await api(`/servers/${id}/results?limit=100${before}`);
      if (id !== state.detailID || request !== state.resultsRequest) return;
      const results = response.results || [];
      state.results = reset ? results : [...state.results, ...results];
      state.resultEnd = results.length < 100;
      renderResults();
    } catch (error) { if (id === state.detailID) setError('#detail-error', `原始记录读取失败：${error.message}`); }
    finally {
      if (request === state.resultsRequest) {
        state.loadingResults = null;
        $('#results-more').disabled = false;
        $('#results-refresh').disabled = false;
      }
    }
  }

  function effectivePollution(result) {
    return result.override === 'clean' || result.override === 'polluted' ? result.override : result.pollution;
  }

  function renderResults() {
    $('#result-rows').innerHTML = state.results.length ? state.results.map((result, index) => `<tr><td>${escape(timestamp(result.timestamp, true))}</td><td><button class="domain-button ${pollutionKind(effectivePollution(result))}" data-result-index="${index}" title="查看解析证据与人工判定">${effectivePollution(result) === 'polluted' ? '<span aria-label="污染标记">⚑</span>' : ''}${escape(result.domain)}<span aria-hidden="true">⌄</span></button><span class="domain-type">${escape(result.type)}</span></td><td title="${escape(result.error || '')}"><span class="result-code${result.success ? '' : ' failed'}">${escape(result.rcode || (result.received ? '响应异常' : '无响应'))}</span></td><td>${result.received ? number(result.latency_ms, 0) + ' ms' : '—'}</td><td>${badge(effectivePollution(result))}${result.override && result.override !== 'auto' ? '<small class="muted" style="margin-left:5px;font-size:8px">人工</small>' : ''}</td></tr>`).join('') : '<tr><td colspan="5" class="muted" style="text-align:center;padding:35px">尚无探测记录。启用服务器并等待下一轮采样，或点击“立即探测”。</td></tr>';
    $('#results-count').textContent = `已显示 ${integer(state.results.length)} 条记录 · 从新到旧`;
    $('#results-more').hidden = state.resultEnd || !state.results.length;
  }

  async function probeNow() {
    const button = $('#detail-probe');
    button.disabled = true;
    try {
      await api(`/servers/${state.detailID}/probe`, { method: 'POST', body: {} });
      toast('已加入探测队列，结果会在采样完成后写入');
      setTimeout(async () => {
        if (!state.token || !$('#detail-dialog').open) return;
        await Promise.allSettled([refreshState(), loadHistory(), loadResults(true)]);
      }, Math.max(3000, Math.min(15000, (state.data.config?.timeout_seconds || 3) * 1500)));
    } catch (error) { toast(error.message, true); }
    finally { button.disabled = false; renderDetailHeader(); }
  }

  function showVerdict(index) {
    const result = state.results[index];
    if (!result) return;
    state.result = result;
    $('#verdict-domain').textContent = result.domain;
    $('#verdict-subtitle').textContent = `${result.type} · ${timestamp(result.timestamp, true)} · ${currentServer()?.name || ''}`;
    const references = result.references || [];
    $('#verdict-evidence').innerHTML = `<div class="evidence-summary">${badge(effectivePollution(result))}<span class="tag">${result.override && result.override !== 'auto' ? '人工判定' : '自动判定'}</span><span class="tag">原始：${pollutionLabel(result.pollution)}</span><span class="tag">${escape(result.rcode || '无响应')}</span>${result.received ? `<span class="tag">${number(result.latency_ms)} ms</span>` : ''}</div><p class="evidence-reason">${escape(result.reason || '当前没有可用于判定的充分证据。')}${result.error ? `<br>查询错误：${escape(result.error)}` : ''}</p><section class="evidence-section"><h4>被测服务器的答案</h4><pre class="evidence-answers">${escape(result.answers?.join('\n') || '没有返回答案')}</pre></section><section class="evidence-section"><h4>可信参考 <span class="muted">${references.length} 条</span></h4>${references.length ? references.map(reference => `<div class="reference-item"><div class="reference-heading"><span>${escape(reference.address)} · ${escape(reference.rcode || (reference.success ? '成功' : '失败'))}</span><span>${escape(timestamp(reference.timestamp, true))}</span></div><pre class="evidence-answers">${escape(reference.answers?.join('\n') || reference.error || '没有返回答案')}</pre>${reference.raw ? `<details class="raw-details"><summary>参考原始输出</summary><pre class="evidence-answers">${escape(reference.raw)}</pre></details>` : ''}</div>`).join('') : '<p class="field-help">本条记录没有可信参考。可在服务器管理中启用用户可信 DNS。</p>'}</section>${result.raw ? `<details class="raw-details"><summary>查看 doggo 原始输出</summary><pre class="evidence-answers">${escape(result.raw)}</pre></details>` : ''}`;
    $('#verdict-note').value = '';
    setError('#verdict-error', '');
    $('#verdict-dialog').showModal();
  }

  async function saveVerdict(verdict) {
    const result = state.result;
    if (!result) return;
    const buttons = $$('[data-verdict]');
    buttons.forEach(button => { button.disabled = true; });
    setError('#verdict-error', '');
    try {
      await api('/overrides', { method: 'PUT', body: { server_id: result.server_id, domain: result.domain, type: result.type, verdict, note: $('#verdict-note').value.trim() } });
      $('#verdict-dialog').close();
      toast(verdict === 'auto' ? '已恢复自动判定' : verdict === 'clean' ? '已取消该域名的污染判定' : '已将该域名标记为污染');
      await Promise.allSettled([refreshState(), loadHistory(), loadResults(true)]);
    } catch (error) { setError('#verdict-error', error.message); }
    finally { buttons.forEach(button => { button.disabled = false; }); }
  }

  const chartState = new Map();

  function renderCharts() {
    if (!$('#detail-dialog').open) return;
    drawChart($('#rate-chart'), [{ key: 'availability', label: '可用率', color: '#14877d' }, { key: 'success_rate', label: '成功率', color: '#589ad9' }], true);
    drawChart($('#latency-chart'), [{ key: 'p95_ms', label: 'P95', color: '#9a85c7' }, { key: 'average_ms', label: '平均', color: '#14877d' }], false);
  }

  function drawChart(canvas, series, percentage) {
    const rect = canvas.getBoundingClientRect();
    const width = Math.max(200, rect.width);
    const height = Math.max(140, rect.height);
    const scale = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(width * scale);
    canvas.height = Math.round(height * scale);
    const context = canvas.getContext('2d');
    context.scale(scale, scale);
    const padding = { left: 42, right: 12, top: 15, bottom: 30 };
    const plot = { x: padding.left, y: padding.top, width: width - padding.left - padding.right, height: height - padding.top - padding.bottom };
    const points = state.history;
    const now = state.data?.now || Date.now();
    const end = Math.max(now, points.at(-1)?.timestamp || 0);
    const start = end - ranges[state.detailRange];
    let maximum = percentage ? 100 : points.reduce((max, point) => hasSamples(point) ? Math.max(max, ...series.map(item => Number(point[item.key]) || 0)) : max, 0);
    if (!percentage) {
      maximum = Math.max(10, maximum * 1.15);
      const step = 10 ** Math.floor(Math.log10(maximum));
      maximum = Math.ceil(maximum / step) * step;
    }
    context.font = '9px "Segoe UI", "Microsoft YaHei", sans-serif';
    context.textBaseline = 'middle';
    context.strokeStyle = '#edf1f3';
    context.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = plot.y + plot.height * i / 4;
      context.beginPath(); context.moveTo(plot.x, y); context.lineTo(plot.x + plot.width, y); context.stroke();
      context.fillStyle = '#a5b2ba'; context.textAlign = 'right';
      const value = maximum * (4 - i) / 4;
      context.fillText(`${value >= 1000 ? number(value / 1000, 1) + 'k' : number(value, value < 10 ? 1 : 0)}${percentage ? '%' : ''}`, plot.x - 8, y);
    }
    context.textAlign = 'center';
    for (let i = 0; i <= 4; i++) {
      const time = start + (end - start) * i / 4;
      const label = state.detailRange === '24h' ? new Date(time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) : new Date(time).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' });
      context.fillText(label, plot.x + plot.width * i / 4, plot.y + plot.height + 19);
    }
    const valid = percentage ? hasSamples : hasLatency;
    const usable = points.filter(valid);
    if (!usable.length) {
      context.fillStyle = '#aab8bf';
      context.font = '11px "Segoe UI", "Microsoft YaHei", sans-serif';
      context.fillText('尚无采样，数据到来后将在这里呈现', plot.x + plot.width / 2, plot.y + plot.height / 2);
    } else {
      const interval = points.length > 1 ? Math.max(1, points[1].timestamp - points[0].timestamp) : ranges[state.detailRange] / 96;
      for (const item of series) {
        context.strokeStyle = item.color;
        context.lineWidth = 1.8;
        context.lineJoin = 'round';
        let started = false;
        let previousTime = 0;
        context.beginPath();
        for (const point of points) {
          if (!valid(point) || point.timestamp < start || !Number.isFinite(Number(point[item.key]))) { started = false; continue; }
          const x = plot.x + (point.timestamp - start) / (end - start) * plot.width;
          const y = plot.y + plot.height - Math.max(0, Math.min(maximum, Number(point[item.key]))) / maximum * plot.height;
          if (started && point.timestamp - previousTime <= interval * 1.6) context.lineTo(x, y); else context.moveTo(x, y);
          started = true;
          previousTime = point.timestamp;
        }
        context.stroke();
        context.fillStyle = item.color;
        if (usable.length < 70) for (const point of usable) {
          if (point.timestamp < start) continue;
          const x = plot.x + (point.timestamp - start) / (end - start) * plot.width;
          const y = plot.y + plot.height - Math.max(0, Math.min(maximum, Number(point[item.key]) || 0)) / maximum * plot.height;
          context.beginPath(); context.arc(x, y, 2.1, 0, Math.PI * 2); context.fill();
        }
      }
    }
    chartState.set(canvas, { plot, start, end, series, percentage, points: usable });
    canvas.setAttribute('aria-label', `${percentage ? '可用率与查询成功率' : '响应时延'}历史图，${usable.length} 个有样本的时间段。具体汇总指标见上方。`);
  }

  function chartHover(event) {
    const canvas = event.currentTarget;
    const data = chartState.get(canvas);
    const tooltip = $('.chart-tooltip', canvas.parentElement);
    if (!data?.points.length) { tooltip.hidden = true; return; }
    const rect = canvas.getBoundingClientRect();
    const x = event.clientX - rect.left;
    if (x < data.plot.x || x > data.plot.x + data.plot.width) { tooltip.hidden = true; return; }
    const time = data.start + (x - data.plot.x) / data.plot.width * (data.end - data.start);
    const closest = data.points.reduce((best, point) => Math.abs(point.timestamp - time) < Math.abs(best.timestamp - time) ? point : best);
    tooltip.innerHTML = `<strong>${escape(timestamp(closest.timestamp))}</strong>${data.series.map(item => `<span>${item.label}：${number(closest[item.key])}${data.percentage ? '%' : ' ms'}</span>`).join('')}<span>${integer(closest.samples)} 次采样</span>`;
    tooltip.hidden = false;
    tooltip.style.left = `${Math.max(0, Math.min(x + 10, rect.width - tooltip.offsetWidth - 5))}px`;
  }

  function renderService() {
    const service = state.data?.service || {};
    const serviceState = String(service.state || '').toLowerCase();
    const running = serviceState === 'running';
    const installed = !!service.installed;
    const canManage = service.can_manage !== false && !state.serviceBusy && !['starting', 'stopping', 'pending', 'unavailable'].includes(serviceState);
    const names = { running: '正在运行', stopped: '已停止', start_pending: '启动中', stop_pending: '停止中', paused: '已暂停', starting: '启动中', stopping: '停止中', pending: '状态转换中', unavailable: '暂时无法读取', unknown: '状态未知', not_installed: '尚未安装', absent: '尚未安装' };
    $('#service-state').textContent = names[serviceState] || (installed ? service.state || '状态未知' : '尚未安装');
    $('#service-description').textContent = service.message || (state.serviceBusy ? '正在执行服务管理操作，请稍候。' : !canManage ? '服务状态正在变化或暂时不可用，请刷新后重试。' : running ? 'Windows 正在后台运行 DNS Monitor。' : installed ? '服务已注册，可在此启动或移除。' : '当前以普通程序方式运行，可按需安装为服务。');
    const buttons = !installed ? [{ action: 'install', text: '安装 Windows 服务', primary: true }] : [{ action: running ? 'stop' : 'start', text: running ? '停止服务' : '启动服务', primary: !running }, ...(running ? [{ action: 'restart', text: '重启服务' }] : []), { action: 'uninstall', text: '卸载服务', danger: true }];
    $('#service-buttons').innerHTML = buttons.map(button => `<button class="button ${button.primary ? 'primary' : button.danger ? 'danger' : 'secondary'}" data-service-action="${button.action}"${canManage ? '' : ' disabled'}>${button.text}</button>`).join('') + '<button class="button subtle" data-service-action="refresh">↻ 刷新状态</button>';
  }

  async function refreshService() {
    if (!state.data) return;
    try { state.data.service = await api('/service'); renderService(); }
    catch (error) { $('#service-feedback').textContent = `无法读取服务状态：${error.message}`; }
  }

  async function serviceAction(action) {
    if (action === 'refresh') return refreshService();
    if (state.serviceBusy) return;
    const names = { install: '安装', uninstall: '卸载', start: '启动', stop: '停止', restart: '重启' };
    if (['stop', 'restart', 'uninstall'].includes(action)) {
      const message = action === 'uninstall' ? '将移除 Windows 服务注册。配置与监测数据保留；正在运行的服务可能停止，网页连接会中断。' : `即将${names[action]} Windows 服务。网页连接可能中断，${action === 'restart' ? '服务恢复后请重新登录。' : '再次启动服务或主程序后才能继续访问。'}`;
      if (!(await confirmAction(`${names[action]} Windows 服务`, message, `确认${names[action]}`))) return;
    }
    state.serviceBusy = true;
    $('[data-service-action]').forEach(button => { button.disabled = true; });
    $('#service-feedback').textContent = `正在${names[action]}服务；如果出现 Windows 管理员授权窗口，请在本机完成授权。`;
    try {
      const response = await api(`/service/${action}`, { method: 'POST', body: {} });
      $('#service-feedback').textContent = response.message || `已提交${names[action]}请求。`;
      toast(response.message || `服务${names[action]}请求已提交`);
      if (!['stop', 'restart', 'uninstall'].includes(action)) setTimeout(refreshService, 1500);
    } catch (error) {
      const message = ['stop', 'restart', 'uninstall'].includes(action) && error.message === 'Failed to fetch' ? '连接已中断，可能是服务正在停止。请在本机检查服务状态，恢复后重新登录。' : `服务操作失败：${error.message}`;
      $('#service-feedback').textContent = message;
      toast(message, true);
    } finally { state.serviceBusy = false; renderService(); }
  }

  async function exportCSV(serverID = null) {
    const button = serverID ? $('#detail-export') : $('#export-button');
    button.disabled = true;
    try {
      const range = serverID ? state.detailRange : state.range;
      const filename = `dns-monitor-${serverID || 'all'}-${range}-${new Date().toISOString().slice(0, 10)}.csv`;
      // Chromium can stream large exports straight to disk instead of buffering a month in memory.
      const file = typeof window.showSaveFilePicker === 'function' ? await window.showSaveFilePicker({ suggestedName: filename, types: [{ description: 'CSV 原始探测记录', accept: { 'text/csv': ['.csv'] } }] }) : null;
      const response = await fetch(`/api/export?range=${range}${serverID ? `&server_id=${serverID}` : ''}`, { headers: { 'X-DNSMonitor-Token': state.token }, cache: 'no-store' });
      if (!response.ok) {
        if (response.status === 401) resetSession('会话已失效，请重新登录。');
        let body;
        try { body = await response.json(); } catch { body = null; }
        throw new Error(body?.error || `导出失败（HTTP ${response.status}）`);
      }
      if (file && response.body) {
        const output = await file.createWritable();
        await response.body.pipeTo(output);
      } else {
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = filename;
        document.body.append(link);
        link.click();
        link.remove();
        setTimeout(() => URL.revokeObjectURL(url), 30000);
      }
      toast('CSV 已导出，包含所选区间的原始记录');
    } catch (error) { if (error.name !== 'AbortError') toast(error.message, true); }
    finally { button.disabled = false; }
  }

  function confirmAction(title, description, confirmText = '确认') {
    return new Promise(resolve => {
      const dialog = $('#confirm-dialog');
      $('#confirm-title').textContent = title;
      $('#confirm-description').textContent = description;
      $('#confirm-ok').textContent = confirmText;
      dialog.returnValue = '';
      const onClose = () => { dialog.removeEventListener('close', onClose); resolve(dialog.returnValue === 'confirm'); };
      dialog.addEventListener('close', onClose);
      dialog.showModal();
      $('#confirm-cancel').focus();
    });
  }

  $('#login-form').addEventListener('submit', login);
  $('#logout-button').addEventListener('click', logout);
  $('#refresh-button').addEventListener('click', async () => { await refreshState(); if (state.page === 'service') await refreshService(); });
  $$('.nav-item').forEach(button => button.addEventListener('click', () => navigate(button.dataset.page)));
  $('.sidebar>.brand').addEventListener('click', event => { event.preventDefault(); navigate('overview'); });
  $$('[data-open-guide]').forEach(button => button.addEventListener('click', () => navigate('guide')));
  $$('[data-range]').forEach(button => button.addEventListener('click', () => changeRange(button.dataset.range)));
  $$('.sort-button').forEach(button => button.addEventListener('click', () => { state.descending = state.sort === button.dataset.sort ? !state.descending : button.dataset.sort !== 'average_ms'; state.sort = button.dataset.sort; renderServerRows(); }));
  $('#search-filter').addEventListener('input', renderServerRows);
  ['protocol', 'pollution', 'grade'].forEach(filter => $(`#${filter}-filter`).addEventListener('change', renderServerRows));
  $('#add-server-button').addEventListener('click', () => openServer());
  $('#empty-add-button').addEventListener('click', () => openServer());
  $('#server-form').addEventListener('submit', saveServer);
  $('#server-rows').addEventListener('click', event => {
    const button = event.target.closest('[data-action]');
    if (!button) return;
    const id = Number(button.dataset.id);
    if (button.dataset.action === 'detail') openDetail(id);
    if (button.dataset.action === 'edit') openServer(state.data.servers.find(server => server.id === id));
    if (button.dataset.action === 'delete') deleteServer(id);
  });
  $$('[data-close]').forEach(button => button.addEventListener('click', () => $(`#${button.dataset.close}`).close()));
  $('#confirm-cancel').addEventListener('click', () => $('#confirm-dialog').close('cancel'));
  $('#confirm-ok').addEventListener('click', () => $('#confirm-dialog').close('confirm'));
  $('#config-form').addEventListener('input', markConfigDirty);
  $('#config-form').addEventListener('change', markConfigDirty);
  $('#config-form').addEventListener('submit', saveConfig);
  $('#add-domain-button').addEventListener('click', () => { const row = domainRow(); $('#domain-rows').append(row); $('.domain-name', row).focus(); markConfigDirty(); });
  $('#domain-rows').addEventListener('click', event => {
    const button = event.target.closest('.remove-domain');
    if (!button) return;
    if ($$('.domain-row').length <= 1) return toast('至少需要保留一个探测域名', true);
    button.closest('.domain-row').remove();
    markConfigDirty();
  });
  $$('[data-detail-range]').forEach(button => button.addEventListener('click', () => { if (button.dataset.detailRange === state.detailRange) return; state.detailRange = button.dataset.detailRange; loadHistory(); }));
  $('#detail-probe').addEventListener('click', probeNow);
  $('#detail-edit').addEventListener('click', () => openServer(currentServer()));
  $('#results-refresh').addEventListener('click', () => { setError('#detail-error', ''); Promise.allSettled([loadHistory(), loadResults(true)]); });
  $('#results-more').addEventListener('click', () => loadResults());
  $('#result-rows').addEventListener('click', event => { const button = event.target.closest('[data-result-index]'); if (button) showVerdict(Number(button.dataset.resultIndex)); });
  $$('[data-verdict]').forEach(button => button.addEventListener('click', () => saveVerdict(button.dataset.verdict)));
  $('#service-buttons').addEventListener('click', event => { const button = event.target.closest('[data-service-action]'); if (button && !button.disabled) serviceAction(button.dataset.serviceAction); });
  $('#export-button').addEventListener('click', () => exportCSV());
  $('#detail-export').addEventListener('click', () => exportCSV(state.detailID));
  ['rate-chart', 'latency-chart'].forEach(id => { const canvas = $(`#${id}`); canvas.addEventListener('pointermove', chartHover); canvas.addEventListener('pointerleave', () => { $('.chart-tooltip', canvas.parentElement).hidden = true; }); });
  let resizeTimer;
  window.addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(renderCharts, 100); });
  document.addEventListener('visibilitychange', () => { if (!document.hidden && state.token) refreshState(); });
  window.addEventListener('beforeunload', event => { if (state.configDirty) { event.preventDefault(); event.returnValue = ''; } });
})();
