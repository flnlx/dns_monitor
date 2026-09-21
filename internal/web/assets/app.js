'use strict';

(() => {
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
  const filterStorageKey = 'dns-monitor.filters.v1';
  const storedFilters = (() => { try { const raw = typeof localStorage !== 'undefined' ? localStorage.getItem(filterStorageKey) : null; return raw ? JSON.parse(raw) : {}; } catch { return {}; } })();
  const sessionKey = 'dns-monitor.session.v1';
  const storedSession = (() => { try { if (typeof sessionStorage === 'undefined') return null; const raw = sessionStorage.getItem(sessionKey); return raw ? JSON.parse(raw) : null; } catch { return null; } })();
  const state = { token: storedSession?.token || '', data: null, range: '24h', detailRange: '24h', page: ['overview', 'config', 'service', 'guide'].includes(storedSession?.page) ? storedSession.page : 'overview', sort: 'grade', descending: false, detailID: null, history: [], statusHistory: [], historyMetrics: null, results: [], result: null, refreshTimer: null, refreshing: false, configDirty: false, loadingResults: null, resultEnd: false, detailRequest: 0, resultsRequest: 0, serviceBusy: false, manualRefresh: null, manualTimer: null, detailProbe: null, detailProbeTimer: null, importing: null, filters: { protocol: Array.isArray(storedFilters.protocol) ? storedFilters.protocol.filter(value => typeof value === 'string') : [], pollution: Array.isArray(storedFilters.pollution) ? storedFilters.pollution.filter(value => ['matched', 'clean', 'suspicious', 'polluted', 'unknown'].includes(value)) : [], grade: Array.isArray(storedFilters.grade) ? storedFilters.grade.filter(value => typeof value === 'string') : [] } };
  const persistFilters = () => { try { if (typeof localStorage !== 'undefined') localStorage.setItem(filterStorageKey, JSON.stringify({ protocol: state.filters.protocol, pollution: state.filters.pollution, grade: state.filters.grade })); } catch { /* storage unavailable */ } };
  const persistSession = () => { try { if (typeof sessionStorage !== 'undefined') sessionStorage.setItem(sessionKey, JSON.stringify({ token: state.token, page: state.page })); } catch { /* storage unavailable */ } };
  const clearSession = () => { try { if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem(sessionKey); } catch { /* storage unavailable */ } };
  const ranges = { '24h': 24 * 3600000, '7d': 7 * 86400000, '30d': 30 * 86400000 };
  const titles = { overview: ['监测总览', 'NETWORK OBSERVABILITY'], config: ['探测配置', 'MONITORING PREFERENCES'], service: ['Windows 服务', 'ALWAYS-ON MONITORING'], guide: ['使用指南', 'YOUR OBSERVATION HANDBOOK'] };
  const escape = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
  const number = (value, digits = 1) => Number.isFinite(Number(value)) ? Number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits }) : '—';
  const integer = value => number(value, 0);
  const timestamp = (value, full = false) => value ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', ...(full ? { second: '2-digit' } : {}), hour12: false }) : '尚未探测';
  const hasSamples = metrics => (metrics?.samples || 0) > 0;
  const hasCoverage = metrics => (metrics?.coverage || 0) > 0;
  const hasLatency = metrics => hasSamples(metrics) && (metrics.success_rate > 0 || metrics.average_ms > 0 || metrics.p95_ms > 0);
  const percent = (value, metrics, availability = false) => (availability ? hasCoverage(metrics) : hasSamples(metrics)) ? `${number(value)}%` : '—';
  const currentServer = () => state.data?.servers.find(server => server.id === state.detailID);
  const trustedResult = result => !!result.trusted || !!state.data?.servers.find(server => server.id === result.server_id)?.trusted;
  const currentEvaluation = server => server?.current || {};
  const serverPollution = server => server.trusted ? 'clean' : currentEvaluation(server).pollution;
  const displayedGrade = (metrics, trusted = false) => trusted && ['E', 'F'].includes(metrics?.grade) ? 'pending' : metrics?.grade;
  const trustedBadge = '<span class="pollution-badge trusted" title="用户指定的可信 DNS，豁免污染判定；性能评级仍需足够采样。">✓ 用户可信 / 无污染</span>';
  const pollutionKind = value => ['matched', 'clean', 'suspicious', 'polluted'].includes(value) ? value : 'unknown';
  const pollutionLabel = value => ({ matched: '参考一致', clean: '正常', suspicious: '可疑', polluted: '疑似污染', unknown: '待判定' })[pollutionKind(value)];
  const pollutionHints = {
    matched: '可比较的返回 IP 均命中 TTL 内的新鲜可信参考。',
    clean: '可比较的返回 IP 命中有效可信历史，或已由用户手动标记正常。',
    suspicious: '有 IP 未命中有效可信参考，但未命中的 IP 均与参考地址的前两段匹配。',
    polluted: '有 IP 既未命中有效可信参考，前两段也不匹配；此为疑似判定。',
    unknown: '查询失败、没有可比较的 IPv4 答案，或没有有效可信参考，暂无法判定。'
  };
  const badge = (value, confirmed = false) => `<span class="pollution-badge ${pollutionKind(value)}" title="${escape(confirmed && value === 'polluted' ? '用户已手动确认污染，可在域名菜单中修改判定。' : pollutionHints[pollutionKind(value)])}"><span aria-hidden="true">${['polluted', 'suspicious'].includes(pollutionKind(value)) ? '!' : ['matched', 'clean'].includes(pollutionKind(value)) ? '✓' : '·'}</span>${confirmed && value === 'polluted' ? '已确认污染' : pollutionLabel(value)}</span>`;
  const gradeHints = {
    A: '优秀：综合评分 ≥ 95，依据可用率、查询成功率和 P95 时延。',
    B: '良好：综合评分 ≥ 85 且 < 95，依据可用率、查询成功率和 P95 时延。',
    C: '一般：综合评分 ≥ 70 且 < 85，依据可用率、查询成功率和 P95 时延。',
    D: '较差：综合评分 < 70，依据可用率、查询成功率和 P95 时延。',
    E: '解析结果可疑，优先评 E；未命中的 IP 与可信参考的前两段匹配。',
    F: '存在疑似或人工确认污染，优先评 F。',
    unavailable: '不可用：最新一轮正式探测全部查询失败（连接失败、超时或解析错误）；可信标记不豁免。恢复成功后重新评级。',
    pending: '等待足够的正式采样与有效覆盖；评级门槛可在探测配置中调整。'
  };
  const gradeHint = (value, evaluation = {}) => {
    const description = gradeHints[value] || gradeHints.pending;
    if (!evaluation.window_minutes) return description;
    const prefix = `最近 ${integer(evaluation.window_minutes)} 分钟。`;
    if (value !== 'pending' && gradeHints[value]) return prefix + description;
    const reason = ({
      no_samples: '评级窗口内没有正式采样，等待下一轮探测。',
      quality_unknown: '最新正式探测的解析质量待判定，可查看域名记录中的原因。',
      insufficient_samples: '正式采样次数尚未达到评级门槛。',
      insufficient_coverage: '有效监测覆盖尚未达到评级门槛。'
    })[evaluation.pending_reason] || description;
    const covered = Math.floor((evaluation.covered_minutes || 0) * 10) / 10;
    return `${prefix}${reason}采样 ${integer(evaluation.samples || 0)}/${integer(evaluation.min_samples)} 次，覆盖 ${number(covered)}/${integer(evaluation.min_coverage_minutes)} 分钟。`;
  };
  const gradeHTML = (value, evaluation) => value === 'unavailable' ? `<span class="grade grade-unavailable" title="${escape(gradeHint(value, evaluation))}">不可用</span>` : ['A', 'B', 'C', 'D', 'E', 'F'].includes(value) ? `<span class="grade grade-${value.toLowerCase()}" title="${escape(gradeHint(value, evaluation))}">${value}</span>` : `<span class="grade grade-pending" title="${escape(gradeHint('pending', evaluation))}">待评估</span>`;

  const statusLabel = (value, channel) => value === 'no_data' ? '无数据' : channel === 'quality' ? pollutionLabel(value) : ({ pending: '待评估', unavailable: '不可用' })[value] || value;
  const statusValue = (point, channel) => {
    const value = channel === 'quality' ? point.pollution : point.grade;
    const allowed = channel === 'quality' ? ['matched', 'clean', 'unknown', 'suspicious', 'polluted'] : ['A', 'B', 'C', 'D', 'E', 'F', 'pending', 'unavailable'];
    return point.snapshots > 0 && allowed.includes(value) ? value : 'no_data';
  };
  function statusDescription(point, channel) {
    const name = channel === 'quality' ? '解析质量' : '评级';
    const heading = `${timestamp(point.timestamp)} – ${timestamp(point.end)} · ${name}`;
    if (!point.snapshots) return heading + '：无数据（此时段没有正式探测快照）';
    const counts = channel === 'quality' ? point.quality_counts : point.grade_counts;
    const distribution = Object.entries(counts || {}).map(([key, count]) => `${statusLabel(key, channel)} ${integer(count)} 次`).join('、');
    let text = `${heading}：最差记录 ${statusLabel(statusValue(point, channel), channel)}。${integer(point.snapshots)} 次探测快照；${distribution}。`;
    if (point.latest) {
      const latest = point.latest.current;
      text += `该时段末次快照（${timestamp(point.latest.timestamp, true)}）：解析质量 ${pollutionLabel(latest.pollution)}，评级 ${statusLabel(latest.grade, 'grade')}${point.latest.trusted ? '，当时为用户可信 DNS' : ''}。最近 ${integer(latest.window_minutes)} 分钟采样 ${integer(latest.samples)} 次，成功率 ${percent(latest.success_rate, latest)}，平均时延 ${hasLatency(latest) ? number(latest.average_ms) + ' ms' : '—'}。`;
    }
    return text;
  }
  function statusStrip(points, channel, detail = false) {
    return `<span class="status-strip${detail ? ' expanded' : ''}">${points.map((point, index) => {
      const value = statusValue(point, channel);
      const description = escape(statusDescription(point, channel));
      return detail ? `<button type="button" class="status-block status-${value.toLowerCase()}" data-status-index="${index}" data-status-channel="${channel}" aria-label="${description}" title="${description}"></button>` : `<span class="status-block status-${value.toLowerCase()}" title="${description}"></span>`;
    }).join('')}</span>`;
  }
  function compactHistory(server, channel) {
    const points = server.status_history || [];
    const name = channel === 'quality' ? '解析质量' : '评级';
    if (!points.length) return '<span class="sub-status">历史暂无数据</span>';
    return `<button type="button" class="history-open" data-action="detail" data-id="${server.id}" aria-label="查看 ${escape(server.name)} 的${name}历史">${statusStrip(points, channel)}<span class="history-caption">${({ '24h': '24 小时', '7d': '7 天', '30d': '30 天' })[state.loadedRange || state.range]}历史 ↗</span></button>`;
  }
  function renderStatusHistory() {
    const target = $('#status-history');
    const points = state.statusHistory;
    if (!points.length) { target.innerHTML = '<p class="field-help">暂无状态历史。</p>'; return; }
    const legend = (values, channel) => values.map(value => `<span><i class="status-${value.toLowerCase()}"></i>${escape(statusLabel(value, channel))}</span>`).join('');
    target.innerHTML = `<div class="status-history-heading"><h4>解析质量与评级历史</h4><span>过去 → 现在</span></div><p class="field-help">每格 ${({ '24h': '30 分钟', '7d': '3 小时', '30d': '12 小时' })[state.detailRange]}，显示该时段探测快照中的最差状态；悬停、聚焦或点击查看详情。</p><div class="status-track"><h5>解析质量</h5>${statusStrip(points, 'quality', true)}<div class="status-legend">${legend(['matched', 'clean', 'unknown', 'suspicious', 'polluted', 'no_data'], 'quality')}</div></div><div class="status-track"><h5>探测时评级</h5>${statusStrip(points, 'grade', true)}<div class="status-legend">${legend(['A', 'B', 'C', 'D', 'pending', 'E', 'F', 'unavailable', 'no_data'], 'grade')}</div></div><div class="status-axis"><span>${timestamp(points[0].timestamp)}</span><span>${timestamp(points.at(-1).end)}</span></div><p id="status-history-inspection" class="status-inspection" role="status">请选择色块查看状态分布及当时的评级指标。</p><p class="field-help">仅记录正式探测完成时的状态，不代表整段时间持续如此。评级使用当时的观察窗口、配置与可信设置；后续修改不会重写历史。升级前未记录或无探测的时段显示为无数据。</p>`;
  }

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
    clearSession();
    clearTimeout(state.refreshTimer);
    clearTimeout(state.manualTimer);
    clearTimeout(state.detailProbeTimer);
    state.manualRefresh = null;
    state.detailProbe = null;
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
      const error = new Error(body?.error || `请求未完成（HTTP ${response.status}）`);
      error.status = response.status;
      throw error;
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
      persistSession();
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
    if (!state.token || state.refreshing) return null;
    state.refreshing = true;
    renderRefreshButton();
    try {
      const requestedRange = state.range;
      const response = await api(`/state?range=${encodeURIComponent(requestedRange)}`);
      state.loadedRange = requestedRange;
      const previous = currentServer()?.last_probe || 0;
      state.data = response;
      state.data.servers ||= [];
      renderOverview();
      renderRuntime();
      renderService();
      if (initial || (state.page === 'config' && !state.configDirty)) populateConfig();
      $('#restart-notice').hidden = !response.listen_restart_required;
      $('#last-refresh').textContent = `更新于 ${new Date().toLocaleTimeString('zh-CN', { hour12: false })}`;
      setError('#global-error', response.runtime?.last_error ? `最近运行提示：${response.runtime.last_error}` : '');
      if ($('#detail-dialog').open) {
        renderDetailHeader();
        if ((currentServer()?.last_probe || 0) > previous) await Promise.allSettled([loadHistory(), loadResults(true)]);
        else renderDetailCurrent();
      }
      return response;
    } catch (error) {
      if (state.token) setError('#global-error', `无法更新数据：${error.message === 'Failed to fetch' ? '连接中断，请检查程序或 Windows 服务是否运行。' : error.message}`);
      return null;
    } finally {
      state.refreshing = false;
      renderRefreshButton();
    }
  }

  function renderRefreshButton() {
    const button = $('#refresh-button');
    const batch = state.manualRefresh;
    $('#export-button').disabled = !state.data || state.loadedRange !== state.range || state.refreshing;
    const paused = !!state.data?.runtime?.paused || state.data?.config?.concurrency === 0;
    button.disabled = !!batch || paused;
    button.innerHTML = batch ? '<span class="spinner" aria-hidden="true"></span> 探测 ' + integer(batch.completed || 0) + '/' + integer(batch.total || 0) : '<span aria-hidden="true">↻</span> 刷新';
    button.title = paused ? '并发数为 0，探测已暂停；调整配置后可手动刷新' : batch ? '等待本轮实际探测完成' : '立即探测所有已启用 DNS，并读取新结果';
  }

  function showProbeProgress(batch, done = false) {
    const element = $('#probe-progress');
    element.hidden = false;
    element.className = 'notice ' + (batch.error ? 'warning' : 'info') + ' probe-progress';
    element.textContent = batch.error ? '本轮刷新：' + batch.error : done ? '刷新完成：' + integer(batch.completed || 0) + ' 台 DNS 已完成新的探测。' : '正在重新探测 DNS：' + integer(batch.completed || 0) + ' / ' + integer(batch.total || 0) + ' 已完成，' + integer(Math.max(0, Number(batch.total || 0) - Number(batch.completed || 0))) + ' 台等待完成。';
  }

  async function manualRefresh() {
    if (state.manualRefresh) return;
    if (state.data?.runtime?.paused || state.data?.config?.concurrency === 0) return toast('探测已暂停，请将并发数设为大于 0 后重试', true);
    state.manualRefresh = { id: null, total: 0, completed: 0, pending: 0, starting: true, failures: 0 };
    renderRefreshButton();
    try {
      const batch = await api('/probes', { method: 'POST', body: {} });
      state.manualRefresh = { ...batch, failures: 0 };
      showProbeProgress(batch);
      await pollManualRefresh();
    } catch (error) {
      state.manualRefresh = null;
      $('#probe-progress').hidden = true;
      toast(error.message, true);
      renderRefreshButton();
    }
  }

  async function pollManualRefresh() {
    clearTimeout(state.manualTimer);
    const tracking = state.manualRefresh;
    if (!tracking || !state.token) return;
    const response = await refreshState();
    if (!state.token || state.manualRefresh !== tracking) return;
    const reported = response?.runtime?.refresh;
    if (reported && String(reported.id) === String(tracking.id)) {
      Object.assign(tracking, reported, { failures: 0 });
    } else if (response && reported?.id && String(reported.id) !== String(tracking.id)) {
      state.manualRefresh = null;
      toast('服务器上的刷新批次已变化，当前列表已更新；可再次点击刷新。', true);
      renderRefreshButton();
      return;
    } else if (!response && !state.refreshing) {
      tracking.failures = (tracking.failures || 0) + 1;
      if (tracking.failures >= 5) {
        state.manualRefresh = null;
        showProbeProgress({ error: '暂时无法读取探测进度，后台任务可能仍在运行。恢复连接后再刷新状态。' });
        renderRefreshButton();
        return;
      }
    }
    const done = tracking.pending === false || Number(tracking.completed || 0) >= Number(tracking.total || 0);
    showProbeProgress(tracking, done);
    renderRefreshButton();
    if (done) {
      state.manualRefresh = null;
      if ($('#detail-dialog').open) await Promise.allSettled([loadHistory(), loadResults(true)]);
      renderRefreshButton();
      toast(tracking.error || (tracking.total ? '所有已启用 DNS 的新一轮探测已完成' : '没有需要探测的已启用 DNS'), !!tracking.error);
      return;
    }
    state.manualTimer = setTimeout(pollManualRefresh, 1000);
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

  const multiSelectSpecs = {
    protocol: { label: '全部协议', list: servers => [...new Set(servers.map(server => server.protocol).filter(Boolean))].sort().map(value => ({ value, label: value.toUpperCase() })), valueOf: server => server.protocol },
    pollution: { label: '全部解析状态', list: () => [['matched', '参考一致'], ['clean', '正常'], ['suspicious', '可疑'], ['polluted', '疑似 / 确认污染'], ['unknown', '待判定']].map(([value, label]) => ({ value, label })), valueOf: server => pollutionKind(serverPollution(server)) },
    grade: { label: '全部评级', list: () => [['A', 'A · 优秀'], ['B', 'B · 良好'], ['C', 'C · 一般'], ['D', 'D · 较差'], ['E', 'E · 可疑'], ['F', 'F · 疑似 / 确认污染'], ['pending', '待评估'], ['unavailable', '不可用']].map(([value, label]) => ({ value, label })), valueOf: server => { const value = displayedGrade(currentEvaluation(server), server.trusted); return ['A', 'B', 'C', 'D', 'E', 'F', 'unavailable'].includes(value) ? value : 'pending'; } }
  };
  let filterSignatures = {};

  function filterOptionList(key) {
    const servers = state.data?.servers || [];
    const list = multiSelectSpecs[key].list(servers);
    const counts = new Map(list.map(option => [option.value, 0]));
    servers.forEach(server => { const value = multiSelectSpecs[key].valueOf(server); const count = counts.get(value); if (count !== undefined) counts.set(value, count + 1); });
    return { list, counts };
  }

  function updateFilterSummary(key) {
    const summary = $(`[data-summary="${key}"]`);
    if (summary) summary.textContent = state.filters[key].length ? `已选 ${state.filters[key].length} 项` : multiSelectSpecs[key].label;
  }

  function renderFilterOptions(key) {
    const root = $(`#${key}-filter-root`);
    if (!root) return;
    const { list, counts } = filterOptionList(key);
    const available = list.map(option => option.value);
    state.filters[key] = state.filters[key].filter(value => available.includes(value));
    const signature = list.map(option => `${option.value}:${option.label}:${counts.get(option.value) || 0}`).join('|');
    const optionsElement = $(`[data-options="${key}"]`, root);
    if (!optionsElement) return;
    if (filterSignatures[key] !== signature) {
      optionsElement.innerHTML = list.map(option => `<label class="multi-option"><input type="checkbox" value="${escape(option.value)}"><span class="multi-option-text">${escape(option.label)}</span><span class="multi-option-count">${integer(counts.get(option.value) || 0)}</span></label>`).join('');
      filterSignatures[key] = signature;
    }
    $$('input[type=checkbox]', optionsElement).forEach(input => { input.checked = state.filters[key].includes(input.value); });
    const all = $(`[data-all="${key}"]`, root);
    if (all) all.checked = state.filters[key].length === 0;
    updateFilterSummary(key);
  }

  function renderAllFilters() { Object.keys(multiSelectSpecs).forEach(renderFilterOptions); }

  function closeFilterPopovers() {
    $$('.multi-select[data-filter]').forEach(root => {
      const key = root.dataset.filter;
      const popover = $(`[data-popover="${key}"]`, root);
      if (popover) popover.hidden = true;
      const trigger = $(`[data-trigger="${key}"]`, root);
      if (trigger) trigger.setAttribute('aria-expanded', 'false');
    });
  }

  function toggleFilterPopover(key) {
    const root = $(`#${key}-filter-root`);
    if (!root) return;
    const popover = $(`[data-popover="${key}"]`, root);
    if (!popover) return;
    const opening = popover.hidden;
    closeFilterPopovers();
    popover.hidden = !opening;
    const trigger = $(`[data-trigger="${key}"]`, root);
    if (trigger) trigger.setAttribute('aria-expanded', String(opening));
  }

  function renderOverview() {
    const servers = state.data.servers;
    const enabled = servers.filter(server => server.enabled);
    const online = enabled.filter(server => server.last_probe && server.last_success);
    const polluted = servers.filter(server => pollutionKind(serverPollution(server)) === 'polluted');
    const suspicious = servers.filter(server => pollutionKind(serverPollution(server)) === 'suspicious');
    const sampled = servers.filter(server => hasSamples(server.metrics));
    const responding = sampled.filter(server => hasLatency(server.metrics));
    const average = responding.length ? responding.reduce((sum, server) => sum + server.metrics.average_ms, 0) / responding.length : null;
    const samples = sampled.reduce((sum, server) => sum + server.metrics.samples, 0);
    const cards = [
      { title: '监测服务器', value: integer(servers.length), unit: '台', caption: `${enabled.length} 台已启用 · ${servers.filter(server => server.trusted).length} 台可信 DNS`, icon: '◫', kind: '' },
      { title: '最近查询正常', value: integer(online.length), unit: '台', caption: `${enabled.filter(server => server.last_probe && !server.last_success).length} 台异常 · ${enabled.filter(server => !server.last_probe).length} 台待首次探测`, icon: '↗', kind: 'good' },
      { title: '解析异常', value: integer(polluted.length + suspicious.length), unit: '台', caption: `${polluted.length} 台 F · ${suspicious.length} 台 E · ${servers.filter(server => pollutionKind(serverPollution(server)) === 'unknown').length} 台待判定`, icon: '!', kind: polluted.length ? 'bad' : suspicious.length ? 'warning' : '' },
      { title: '服务器平均时延', value: average === null ? '—' : number(average, 0), unit: 'ms', caption: `${responding.length} 台响应服务器均值 · ${integer(samples)} 次采样`, icon: '⌁', kind: '' }
    ];
    $('#summary-cards').innerHTML = cards.map(card => `<article class="summary-card ${card.kind}"><div class="summary-top"><span>${escape(card.title)}</span><span class="summary-icon" aria-hidden="true">${escape(card.icon)}</span></div><div class="summary-value">${card.value}<small>${card.unit}</small></div><p class="summary-caption">${escape(card.caption)}</p></article>`).join('');
    $('#server-total').textContent = servers.length;
    $('#rating-explanation').textContent = `时延、可用率和成功率按所选历史区间统计；当前解析质量取最新正式探测，当前评级使用最近 ${state.data.config?.rating_window_minutes ?? 60} 分钟。待判定时不延续旧的正常评级；点击域名可查看原因与人工复核。`;
    renderAllFilters();
    renderServerRows();
  }

  function humanDuration(seconds) {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}天${hours}小时${minutes}分钟`;
    if (hours > 0) return `${hours}小时${minutes}分钟`;
    return `${Math.max(1, minutes)} 分钟`;
  }

  function relativeProbe(server) {
    if (!server.enabled) return '已停止调度';
    if (!server.last_probe) return '等待首次采样';
    if (state.data.runtime?.paused) return '调度已暂停';
    if (server.next_due > Date.now()) {
      const seconds = Math.ceil((server.next_due - Date.now()) / 1000);
      const text = seconds >= 60 ? `${humanDuration(seconds)}后` : `${seconds} 秒后`;
      return `${server.failures >= 3 && state.data.config?.smart_backoff && state.data.config?.max_backoff_hours > 0 ? '退避 · ' : ''}${text}`;
    }
    return '等待下一次探测';
  }

  function rateCell(value, metrics, availability = false) {
    if (!(availability ? hasCoverage(metrics) : hasSamples(metrics))) return '<span class="muted">—</span>';
    const rate = Math.max(0, Math.min(100, Number(value) || 0));
    return `<div class="rate-cell"><span>${number(rate)}%</span><span class="mini-track" aria-hidden="true"><span class="${rate < 90 ? 'bad' : rate < 99 ? 'warning' : ''}" style="width:${rate}%"></span></span></div>`;
  }

  function visibleServers() {
    if (!state.data) return [];
    const search = $('#search-filter').value.trim().toLowerCase();
    const protocol = state.filters.protocol;
    const pollution = state.filters.pollution;
    const grade = state.filters.grade;
    const servers = state.data.servers.filter(server => {
      const metrics = server.metrics || {};
      const value = displayedGrade(currentEvaluation(server), server.trusted);
      const gradeValue = ['A', 'B', 'C', 'D', 'E', 'F', 'unavailable'].includes(value) ? value : 'pending';
      return (!search || [server.name, server.provider, server.address].some(value => String(value || '').toLowerCase().includes(search))) && (!protocol.length || protocol.includes(server.protocol)) && (!pollution.length || pollution.includes(pollutionKind(serverPollution(server)))) && (!grade.length || grade.includes(gradeValue));
    });
    servers.sort((left, right) => {
      // Failed servers stay last for every sort direction and metric.
      const leftUnavailable = currentEvaluation(left).grade === 'unavailable';
      const rightUnavailable = currentEvaluation(right).grade === 'unavailable';
      if (leftUnavailable !== rightUnavailable) return leftUnavailable ? 1 : -1;
      const lm = left.metrics || {};
      const rm = right.metrics || {};
      if (state.sort === 'grade') {
        const grades = ['A', 'B', 'C', 'D', 'E', 'F'];
        const leftRank = grades.indexOf(displayedGrade(currentEvaluation(left), left.trusted));
        const rightRank = grades.indexOf(displayedGrade(currentEvaluation(right), right.trusted));
        if (leftRank < 0 || rightRank < 0) return leftRank < 0 && rightRank < 0 ? left.id - right.id : leftRank < 0 ? 1 : -1;
        return (state.descending ? rightRank - leftRank : leftRank - rightRank) || left.id - right.id;
      }
      const comparable = state.sort === 'average_ms' ? hasLatency : state.sort === 'availability' ? hasCoverage : hasSamples;
      if (!comparable(lm) || !comparable(rm)) return comparable(lm) ? -1 : comparable(rm) ? 1 : left.id - right.id;
      const difference = (lm[state.sort] || 0) - (rm[state.sort] || 0);
      return (state.descending ? -difference : difference) || left.id - right.id;
    });
    return servers;
  }

  function renderServerRows() {
    if (!state.data) return;
    const servers = visibleServers();
    $('#server-rows').innerHTML = servers.map(server => {
      const metrics = server.metrics || {};
      const evaluation = currentEvaluation(server);
      const known = !!server.last_probe;
      const statusClass = !server.enabled || !known ? 'unknown' : server.last_success ? '' : 'offline';
      const status = !server.enabled ? '已停用' : !known ? '待探测' : server.last_success ? '查询正常' : '查询异常';
      return `<tr data-server-id="${server.id}"><td><button class="server-name-button" data-action="detail" data-id="${server.id}" title="查看 ${escape(server.name)} 的历史记录"><span class="server-avatar" aria-hidden="true">${escape((server.provider || server.name || 'D').slice(0, 1).toUpperCase())}</span><span><span class="server-name"><span class="name-text">${escape(server.name)}</span>${server.trusted ? '<span class="trusted-mark">可信</span>' : ''}</span><span class="server-secondary" title="${escape(server.address)}">${escape(server.provider || server.address)}</span></span></button></td><td title="最后探测：${escape(timestamp(server.last_probe, true))}"><span class="status-label ${statusClass}"><span class="status-dot"></span>${status}</span><div class="sub-status">${escape(relativeProbe(server))}</div></td><td><span class="protocol-badge">${escape((server.protocol || '—').toUpperCase())}</span></td><td><span class="latency-value">${hasLatency(metrics) ? number(metrics.average_ms, 0) : '—'}<small>ms</small></span></td><td>${rateCell(metrics.availability, metrics, true)}</td><td>${rateCell(metrics.success_rate, metrics)}</td><td>${server.trusted ? trustedBadge : badge(evaluation.pollution)}${compactHistory(server, 'quality')}</td><td>${gradeHTML(displayedGrade(evaluation, server.trusted), evaluation)}${compactHistory(server, 'grade')}</td><td class="actions-cell"><div class="row-actions"><button class="row-action" data-action="edit" data-id="${server.id}" aria-label="编辑 ${escape(server.name)}">编辑</button><button class="row-action delete" data-action="delete" data-id="${server.id}" aria-label="删除 ${escape(server.name)}">删除</button></div></td></tr>`;
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

  async function navigate(page, boot = false) {
    if (!titles[page] || (state.page === page && !boot)) return;
    if (state.page === 'config' && state.configDirty) {
      if (!(await confirmAction('离开探测配置', '当前更改尚未保存，离开将放弃这些更改。', '放弃更改'))) return;
      state.configDirty = false;
    }
    state.page = page;
    if (!boot) persistSession();
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
      if (state.detailID === id && $('#detail-dialog').open) { renderDetailHeader(); renderResults(); await loadHistory(); }
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
    row.innerHTML = `<input type="text" class="domain-name" aria-label="探测域名" placeholder="www.youtube.com" spellcheck="false" maxlength="253" required value="${escape(domain.name)}"><select class="domain-type-select" aria-label="记录类型">${types.map(type => `<option value="${type}"${type === domain.type ? ' selected' : ''}>${type}</option>`).join('')}</select><button class="icon-button remove-domain" type="button" aria-label="删除这个探测域名">×</button>`;
    return row;
  }

  function validateConfigNumber(input, showEmpty = false) {
    input.setCustomValidity('');
    const range = `${input.min}–${input.max} ${input.dataset.unit}`;
    const validity = input.validity;
    let message = '';
    if (validity.badInput) message = `请输入 ${range} 范围内的数字。`;
    else if (input.value.trim() === '' || validity.rangeUnderflow || validity.rangeOverflow) message = `请输入 ${range} 范围内的数值。`;
    else if (validity.stepMismatch) message = input.step === '1' ? `请输入 ${range} 范围内的整数。` : `请输入 ${range} 范围内的数值，最多保留 1 位小数。`;
    if (!message && input.name === 'rating_min_coverage_minutes') {
      const window = $('#config-form').elements.rating_window_minutes;
      if (window.value !== '' && window.validity.valid && input.valueAsNumber > window.valueAsNumber) message = `最小覆盖不能超过评级观察窗口（${window.value} 分钟）。`;
    }
    const visible = !!message && (showEmpty || input.value !== '' || validity.badInput);
    input.setCustomValidity(message);
    input.setAttribute('aria-invalid', String(visible));
    input.closest('.input-unit').classList.toggle('invalid', visible);
    const hint = $(`#${input.id}-error`);
    hint.textContent = visible ? message : '';
    hint.hidden = !visible;
    if (input.name === 'rating_window_minutes') validateConfigNumber($('#config-form').elements.rating_min_coverage_minutes);
    return !message;
  }

  function populateConfig() {
    if (!state.data?.config) return;
    const form = $('#config-form');
    const config = state.data.config;
    ['listen', 'interval_seconds', 'timeout_seconds', 'concurrency', 'max_backoff_hours', 'reference_ttl_seconds', 'reference_history_hours', 'rating_window_minutes', 'rating_min_samples', 'rating_min_coverage_minutes'].forEach(field => { form.elements[field].value = config[field] ?? ''; });
    $$('#config-form input[type=number]').forEach(input => validateConfigNumber(input));
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
    $$('#config-form input[type=number]').forEach(input => validateConfigNumber(input, true));
    if (!form.reportValidity()) return;
    const body = { listen: form.elements.listen.value.trim(), smart_backoff: form.elements.smart_backoff.checked, domains: $$('.domain-row').map(row => ({ name: $('.domain-name', row).value.trim(), type: $('.domain-type-select', row).value })) };
    ['interval_seconds', 'timeout_seconds', 'concurrency', 'max_backoff_hours', 'reference_ttl_seconds', 'reference_history_hours', 'rating_window_minutes', 'rating_min_samples', 'rating_min_coverage_minutes'].forEach(field => { body[field] = Number(form.elements[field].value); });
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
    state.historyMetrics = null;
    state.detailRange = state.range;
    state.results = [];
    state.history = [];
    state.statusHistory = [];
    state.resultEnd = false;
    renderDetailHeader();
    $('#detail-metrics').innerHTML = '';
    $('#status-history').innerHTML = '<p class="field-help">正在读取状态历史…</p>';
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
    $('#detail-probe').disabled = !server.enabled || !!state.data.runtime?.paused || state.data.config?.concurrency === 0 || state.detailProbe?.id === server.id;
    $('#detail-probe').textContent = state.detailProbe?.id === server.id ? '等待探测结果…' : '立即探测';
    $$('[data-detail-range]').forEach(button => {
      const active = button.dataset.detailRange === state.detailRange;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
  }

  function renderDetailCurrent(evaluation = currentEvaluation(currentServer())) {
    const target = $('#detail-current-evaluation');
    if (!target || !state.historyMetrics) return;
    target.innerHTML = `<span>当前评级</span>${gradeHTML(displayedGrade(evaluation, currentServer()?.trusted), evaluation)}`;
    $('#chart-note').textContent = `图表和性能指标按所选历史区间统计，空白表示未采样；区间内 ${integer(state.historyMetrics.samples || 0)} 次采样。当前解析质量：${currentServer()?.trusted ? '用户可信 / 无污染' : pollutionLabel(evaluation.pollution)}；当前评级窗口：最近 ${integer(evaluation.window_minutes || state.data?.config?.rating_window_minutes || 60)} 分钟。`;
  }

  async function loadHistory() {
    const request = ++state.detailRequest;
    const id = state.detailID;
    const range = state.detailRange;
    renderDetailHeader();
    setError('#detail-error', '');
    state.statusHistory = [];
    $('#status-history').innerHTML = '<p class="field-help">正在读取状态历史…</p>';
    try {
      const response = await api(`/servers/${id}/history?range=${range}`);
      if (request !== state.detailRequest || id !== state.detailID || range !== state.detailRange) return;
      state.history = (response.points || []).slice().sort((left, right) => left.timestamp - right.timestamp);
      state.statusHistory = response.status_history || [];
      renderStatusHistory();
      const metrics = response.metrics || {};
      state.historyMetrics = metrics;
      const evaluation = response.current || currentEvaluation(currentServer());
      $('#detail-metrics').innerHTML = `<div class="detail-metric"><span>可用率</span><strong>${percent(metrics.availability, metrics, true)}</strong></div><div class="detail-metric"><span>查询成功率</span><strong>${percent(metrics.success_rate, metrics)}</strong></div><div class="detail-metric"><span>P95 时延</span><strong>${hasLatency(metrics) ? number(metrics.p95_ms, 0) : '—'}<small>ms</small></strong></div><div class="detail-metric"><span>原始采样</span><strong>${integer(metrics.samples || 0)}<small>次</small></strong></div><div class="detail-metric"><span>时间覆盖率</span><strong>${hasSamples(metrics) ? number(metrics.coverage) + '%' : '—'}</strong></div><div class="detail-metric" id="detail-current-evaluation"></div>`;
      renderDetailCurrent(evaluation);
      renderCharts();
    } catch (error) { if (request === state.detailRequest) { $('#status-history').innerHTML = '<p class="field-help">状态历史读取失败，请重试。</p>'; setError('#detail-error', `历史读取失败：${error.message}`); } }
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
    if (trustedResult(result)) return 'clean';
    if (result.override === 'clean' || result.override === 'polluted') return result.override;
    return result.effective_pollution ?? result.pollution;
  }

  function renderResults() {
    $('#result-rows').innerHTML = state.results.length ? state.results.map((result, index) => `<tr><td>${escape(timestamp(result.timestamp, true))}</td><td><button class="domain-button ${pollutionKind(effectivePollution(result))}" data-result-index="${index}" title="查看解析证据与人工判定">${['polluted', 'suspicious'].includes(effectivePollution(result)) ? '<span aria-label="解析异常标记">⚑</span>' : ''}${escape(result.domain)}<span aria-hidden="true">⌄</span></button><span class="domain-type">${escape(result.type)}</span></td><td title="${escape(result.error || '')}"><span class="result-code${result.success ? '' : ' failed'}">${escape(result.rcode || (result.received ? '响应异常' : '无响应'))}</span></td><td>${result.received ? number(result.latency_ms, 0) + ' ms' : '—'}</td><td>${trustedResult(result) ? trustedBadge : badge(effectivePollution(result), result.override === 'polluted')}${!trustedResult(result) && result.override && result.override !== 'auto' ? '<small class="muted" style="margin-left:5px;font-size:13px">人工</small>' : ''}</td></tr>`).join('') : '<tr><td colspan="5" class="muted" style="text-align:center;padding:35px">尚无探测记录。启用服务器并等待下一轮采样，或点击“立即探测”。</td></tr>';
    $('#results-count').textContent = `已显示 ${integer(state.results.length)} 条记录 · 从新到旧`;
    $('#results-more').hidden = state.resultEnd || !state.results.length;
  }

  async function probeNow() {
    const server = currentServer();
    if (!server || state.detailProbe?.id === server.id) return;
    state.detailProbe = { id: server.id, baseline: server.last_probe || 0, failures: 0 };
    renderDetailHeader();
    try {
      await api(`/servers/${server.id}/probe`, { method: 'POST', body: {} });
      toast('已加入探测队列，正在等待新的真实结果');
      pollDetailProbe();
    } catch (error) {
      state.detailProbe = null;
      toast(error.message, true);
      renderDetailHeader();
    }
  }

  async function pollDetailProbe() {
    clearTimeout(state.detailProbeTimer);
    const tracking = state.detailProbe;
    if (!tracking || !state.token) return;
    if (!$('#detail-dialog').open || tracking.id !== state.detailID) {
      state.detailProbe = null;
      return;
    }
    const response = await refreshState();
    if (state.detailProbe !== tracking) return;
    const server = state.data?.servers.find(item => item.id === tracking.id);
    if (server && server.last_probe > tracking.baseline) {
      state.detailProbe = null;
      await Promise.allSettled([loadHistory(), loadResults(true)]);
      renderDetailHeader();
      toast('新的探测结果已写入并显示');
      return;
    }
    if (!server || !server.enabled || response?.runtime?.paused || response?.config?.concurrency === 0) {
      state.detailProbe = null;
      renderDetailHeader();
      setError('#detail-error', '探测已暂停或服务器已停用，请检查配置后重试。');
      return;
    }
    if (!response && !state.refreshing) tracking.failures++; else if (response) tracking.failures = 0;
    if (tracking.failures >= 5) {
      state.detailProbe = null;
      renderDetailHeader();
      setError('#detail-error', '无法读取新探测进度。后台任务可能仍在运行，请恢复连接后更新记录。');
      return;
    }
    state.detailProbeTimer = setTimeout(pollDetailProbe, 1000);
  }

  function answerEvidence(answer, queryTime, reference = false) {
    if (!answer.records?.length) return answer.answers?.join('\n') || answer.error || '没有返回答案';
    return answer.records.map(record => {
      const ttl = Number(record.ttl_seconds);
      const ttlText = Number.isFinite(ttl) && ttl >= 0 ? ` · TTL ${integer(ttl)} 秒` : '';
      const freshness = reference && record.expires_at && queryTime ? record.expires_at > queryTime ? ' · 当时新鲜' : ' · 近期历史' : '';
      return `${record.value}${ttlText}${freshness}`;
    }).join('\n');
  }

  function showVerdict(index) {
    const result = state.results[index];
    if (!result) return;
    state.result = result;
    $('[data-verdict="polluted"]').hidden = trustedResult(result);
    $('[data-verdict="polluted"]').disabled = trustedResult(result);
    $('#verdict-domain').textContent = result.domain;
    $('#verdict-subtitle').textContent = `${result.type} · ${timestamp(result.timestamp, true)} · ${currentServer()?.name || ''}`;
    const references = result.references || [];
    const legacy = !trustedResult(result) && (result.policy_version || 0) < 2 && result.pollution === 'polluted' && effectivePollution(result) === 'unknown';
    const reason = trustedResult(result) ? '此服务器由用户指定为可信 DNS，其解析质量固定标为无污染。' : legacy ? '旧版自动判定，仅保留原始证据，不参与当前 E/F 评级。' : result.reason || '当前没有可用于判定的充分证据。';
    $('#verdict-evidence').innerHTML = `<div class="evidence-summary">${trustedResult(result) ? trustedBadge : `${badge(effectivePollution(result), result.override === 'polluted')}<span class="tag">${result.override && result.override !== 'auto' ? '人工判定' : '自动判定'}</span><span class="tag">原始：${pollutionLabel(result.pollution)}${legacy ? '（旧规则）' : ''}</span>`}<span class="tag">${escape(result.rcode || '无响应')}</span>${result.received ? `<span class="tag">${number(result.latency_ms)} ms</span>` : ''}</div><p class="evidence-reason">${escape(reason)}${result.error ? `<br>查询错误：${escape(result.error)}` : ''}</p><section class="evidence-section"><h4>被测服务器的答案</h4><pre class="evidence-answers">${escape(answerEvidence(result, result.timestamp))}</pre></section><section class="evidence-section"><h4>可信参考 <span class="muted">${references.length} 条</span></h4>${references.length ? references.map(reference => `<div class="reference-item"><div class="reference-heading"><span>${escape(reference.address)} · ${escape(reference.rcode || (reference.success ? '成功' : '失败'))}</span><span>${escape(timestamp(reference.timestamp, true))}</span></div><pre class="evidence-answers">${escape(answerEvidence(reference, result.compared_at || result.timestamp, true))}</pre>${reference.raw ? `<details class="raw-details"><summary>参考原始输出</summary><pre class="evidence-answers">${escape(reference.raw)}</pre></details>` : ''}</div>`).join('') : '<p class="field-help">本条记录没有可信参考。可在服务器管理中启用用户可信 DNS。</p>'}</section>${result.raw ? `<details class="raw-details"><summary>查看 doggo 原始输出</summary><pre class="evidence-answers">${escape(result.raw)}</pre></details>` : ''}`;
    $('#verdict-note').value = '';
    setError('#verdict-error', '');
    $('#verdict-dialog').showModal();
  }

  async function saveVerdict(verdict) {
    const result = state.result;
    if (!result) return;
    if (verdict === 'polluted' && trustedResult(result)) return toast('用户可信 DNS 不能标记为污染', true);
    const buttons = $$('[data-verdict]');
    buttons.forEach(button => { button.disabled = true; });
    setError('#verdict-error', '');
    try {
      await api('/overrides', { method: 'PUT', body: { server_id: result.server_id, domain: result.domain, type: result.type, verdict, note: $('#verdict-note').value.trim() } });
      $('#verdict-dialog').close();
      toast(verdict === 'auto' ? '已恢复自动判定' : verdict === 'clean' ? '已将该域名判定为正常' : '已将该域名标记为污染');
      await Promise.allSettled([refreshState(), loadHistory(), loadResults(true)]);
    } catch (error) { setError('#verdict-error', error.message); }
    finally { buttons.forEach(button => { button.disabled = button.dataset.verdict === 'polluted' && trustedResult(result); }); }
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
    const padding = { left: 49, right: 22, top: 18, bottom: 35 };
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
    context.font = '12px "Segoe UI", "Microsoft YaHei", sans-serif';
    context.textBaseline = 'middle';
    context.strokeStyle = '#edf1f3';
    context.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = plot.y + plot.height * i / 4;
      context.beginPath(); context.moveTo(plot.x, y); context.lineTo(plot.x + plot.width, y); context.stroke();
      context.fillStyle = '#5e6c75'; context.textAlign = 'right';
      const value = maximum * (4 - i) / 4;
      context.fillText(`${value >= 1000 ? number(value / 1000, 1) + 'k' : number(value, value < 10 ? 1 : 0)}${percentage ? '%' : ''}`, plot.x - 8, y);
    }
    context.textAlign = 'center';
    for (let i = 0; i <= 4; i++) {
      const time = start + (end - start) * i / 4;
      const label = state.detailRange === '24h' ? new Date(time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) : new Date(time).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' });
      context.fillText(label, plot.x + plot.width * i / 4, plot.y + plot.height + 19);
    }
    const valid = percentage ? point => hasCoverage(point) || hasSamples(point) : hasLatency;
    const usable = points.filter(valid);
    if (!usable.length) {
      context.fillStyle = '#637781';
      context.font = '13px "Segoe UI", "Microsoft YaHei", sans-serif';
      context.fillText('尚无采样，数据到来后将在这里呈现', plot.x + plot.width / 2, plot.y + plot.height / 2);
    } else {
      const interval = points.length > 1 ? Math.max(1, points[1].timestamp - points[0].timestamp) : ranges[state.detailRange] / 96;
      for (const item of series) {
        const seriesValid = item.key === 'availability' ? hasCoverage : percentage ? hasSamples : hasLatency;
        context.strokeStyle = item.color;
        context.lineWidth = 1.8;
        context.lineJoin = 'round';
        let started = false;
        let previousTime = 0;
        context.beginPath();
        for (const point of points) {
          if (!seriesValid(point) || point.timestamp < start || !Number.isFinite(Number(point[item.key]))) { started = false; continue; }
          const x = plot.x + (point.timestamp - start) / (end - start) * plot.width;
          const y = plot.y + plot.height - Math.max(0, Math.min(maximum, Number(point[item.key]))) / maximum * plot.height;
          if (started && point.timestamp - previousTime <= interval * 1.6) context.lineTo(x, y); else context.moveTo(x, y);
          started = true;
          previousTime = point.timestamp;
        }
        context.stroke();
        context.fillStyle = item.color;
        if (usable.length < 70) for (const point of usable) {
          if (!seriesValid(point) || point.timestamp < start) continue;
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
    tooltip.innerHTML = `<strong>${escape(timestamp(closest.timestamp))}</strong>${data.series.map(item => `<span>${item.label}：${(item.key === 'availability' ? hasCoverage(closest) : data.percentage ? hasSamples(closest) : hasLatency(closest)) ? number(closest[item.key]) + (data.percentage ? '%' : ' ms') : '—'}</span>`).join('')}<span>${integer(closest.samples)} 次采样</span>`;
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

  function summaryCSVCell(value) {
    let text = String(value ?? '');
    if (/^[=+\-@\t\r\n]/.test(text) || /^[\s\uFEFF]*[=+\-@]/.test(text)) text = "'" + text;
    return '"' + text.replace(/"/g, '""') + '"';
  }

  function summaryCSV() {
    const servers = visibleServers();
    const range = state.loadedRange || state.range;
    const snapshot = new Date(state.data?.now || Date.now()).toISOString();
    const headers = ['名称', '供应商', '服务器地址', '协议', '当前状态', '平均时延(ms)', 'P95时延(ms)', '可用率(%)', '查询成功率(%)', '当前解析质量', '当前评级', '历史区间样本数', '覆盖率(%)', '最近探测时间(UTC)', '下次探测时间(UTC)', '统计区间', '数据快照时间(UTC)', '用户可信', '已启用', '评级窗口(分钟)', '评级正式采样数', '评级有效覆盖(分钟)', '近期性能评分', '评级说明'];
    const rows = servers.map(server => {
      const metrics = server.metrics || {};
      const status = !server.enabled ? '已停用' : !server.last_probe ? '待探测' : server.last_success ? '查询正常' : '查询异常';
      const evaluation = currentEvaluation(server);
      const grade = displayedGrade(evaluation, server.trusted);
      return [server.name, server.provider, server.address, server.protocol, status, hasLatency(metrics) ? metrics.average_ms : '', hasLatency(metrics) ? metrics.p95_ms : '', hasCoverage(metrics) ? metrics.availability : '', hasSamples(metrics) ? metrics.success_rate : '', server.trusted ? '用户可信 / 无污染' : pollutionLabel(evaluation.pollution), grade === 'unavailable' ? '不可用' : ['A', 'B', 'C', 'D', 'E', 'F'].includes(grade) ? grade : '待评估', metrics.samples || 0, metrics.coverage || 0, server.last_probe ? new Date(server.last_probe).toISOString() : '', server.next_due ? new Date(server.next_due).toISOString() : '', range, snapshot, server.trusted ? '是' : '否', server.enabled ? '是' : '否', evaluation.window_minutes ?? '', evaluation.samples ?? '', evaluation.covered_minutes ?? '', evaluation.score ?? '', gradeHint(grade, evaluation)];
    });
    return { text: '\uFEFF' + [headers, ...rows].map(row => row.map(summaryCSVCell).join(',')).join('\r\n') + '\r\n', count: servers.length, range };
  }

  function exportSummary() {
    if (!state.data || state.loadedRange !== state.range || state.refreshing) return toast('正在更新所选区间，请等待数据加载后导出', true);
    const result = summaryCSV();
    const blob = new Blob([result.text], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'dns-monitor-current-' + result.range + '-' + new Date().toISOString().slice(0, 10) + '.csv';
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
    toast('已导出当前结果：' + result.count + ' 台服务器，保留当前筛选与排序');
  }

  async function exportCSV(serverID = null) {
    const range = serverID ? state.detailRange : state.range;
    return downloadCSV('/export?range=' + range + (serverID ? '&server_id=' + serverID : ''), 'dns-monitor-results-' + (serverID || 'all') + '-' + range + '-' + new Date().toISOString().slice(0, 10) + '.csv', serverID ? $('#detail-export') : $('#export-all-results-button'), '原始记录 CSV 已导出');
  }

  async function downloadCSV(path, filename, button, successMessage = 'CSV 已导出') {
    if (button) button.disabled = true;
    try {
      const file = typeof window.showSaveFilePicker === 'function' ? await window.showSaveFilePicker({ suggestedName: filename, types: [{ description: 'CSV 文件', accept: { 'text/csv': ['.csv'] } }] }) : null;
      const response = await fetch('/api' + path, { headers: { 'X-DNSMonitor-Token': state.token }, cache: 'no-store' });
      if (!response.ok) {
        if (response.status === 401) resetSession('会话已失效，请重新登录。');
        let body;
        try { body = await response.json(); } catch { body = null; }
        throw new Error(body?.error || '导出失败（HTTP ' + response.status + '）');
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
      toast(successMessage);
    } catch (error) { if (error.name !== 'AbortError') toast(error.message, true); }
    finally { if (button) button.disabled = false; }
  }

  function importConflict(row) {
    return !!row.duplicate || !!row.existing || Number(row.duplicate_of_row || 0) > 0;
  }

  function openImport() {
    if (state.importing?.busy) return toast('导入操作正在执行，请稍候', true);
    state.importing = { csv: '', filename: '', snapshot: '', rows: [], errors: 0, decisions: new Map(), busy: false };
    $('#import-file').value = '';
    $('#import-preview').hidden = true;
    $('#import-preview-button').disabled = true;
    $('#import-commit-button').disabled = true;
    $('#import-status').textContent = '选择文件后将自动预览';
    setError('#import-error', '');
    $('#import-dialog').showModal();
  }

  function importBusy(busy, message = '') {
    if (!state.importing) return;
    state.importing.busy = busy;
    $('#import-file').disabled = busy;
    $('#import-preview-button').disabled = busy || !state.importing.csv;
    $('#import-commit-button').disabled = busy || !state.importing.snapshot || state.importing.errors > 0 || !state.importing.rows.length;
    $$('#import-rows select').forEach(select => { select.disabled = busy; });
    $('#import-overwrite-all').disabled = busy;
    $('#import-skip-all').disabled = busy;
    syncImportBulk();
    if (message) $('#import-status').textContent = message;
  }

  async function readImportFile() {
    const file = $('#import-file').files?.[0];
    const session = state.importing;
    if (!file || !session || session.busy) return;
    session.csv = '';
    session.snapshot = '';
    session.rows = [];
    session.filename = file.name;
    $('#import-preview').hidden = true;
    setError('#import-error', '');
    importBusy(true, '正在读取 CSV 文件…');
    try {
      if (file.size > 900 * 1024) throw new Error('CSV 文件超过 900 KiB，请拆分后分别导入。');
      const bytes = await file.arrayBuffer();
      if (state.importing !== session) return;
      let csv;
      try { csv = new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/^\uFEFF/, ''); }
      catch { throw new Error('文件不是有效的 UTF-8 编码，请以 UTF-8 CSV 格式重新保存。'); }
      if (!csv.trim()) throw new Error('CSV 文件为空，请先填写服务器记录。');
      if (new TextEncoder().encode(JSON.stringify({ csv })).length >= 1024 * 1024) throw new Error('CSV 的请求内容超过 1 MiB，请拆分后再导入。');
      session.csv = csv;
    } catch (error) {
      setError('#import-error', error.message);
      $('#import-status').textContent = '文件尚未通过检查';
    } finally { if (state.importing === session) importBusy(false); }
    if (session.csv && state.importing === session) await previewImport();
  }

  async function previewImport() {
    const session = state.importing;
    if (!session?.csv || session.busy) return;
    session.snapshot = '';
    setError('#import-error', '');
    importBusy(true, '正在校验服务器与重复项…');
    try {
      const preview = await api('/servers/import/preview', { method: 'POST', body: { csv: session.csv } });
      if (state.importing !== session) return;
      session.snapshot = preview.snapshot || '';
      session.rows = preview.rows || [];
      session.errors = Math.max(Number(preview.errors || 0), session.rows.filter(row => row.error).length);
      session.decisions = new Map(session.rows.filter(importConflict).map(row => [Number(row.row), 'skip']));
      if (session.rows.length > 1000) throw new Error('CSV 超过 1000 条记录，请拆分后导入。');
      renderImport();
      $('#import-status').textContent = session.errors ? '修正 CSV 中的无效行后，请重新选择文件' : '预览完成，请确认重复项的处理方式';
    } catch (error) {
      session.snapshot = '';
      setError('#import-error', error.message);
      $('#import-status').textContent = '预览失败，可修正文件后重试';
    } finally { if (state.importing === session) importBusy(false); }
  }

  function renderImport() {
    const session = state.importing;
    if (!session) return;
    const conflicts = session.rows.filter(importConflict);
    const fresh = session.rows.filter(row => !row.error && !importConflict(row)).length;
    $('#import-preview').hidden = false;
    $('#import-summary').textContent = session.filename + ' · ' + session.rows.length + ' 行 · ' + fresh + ' 条新记录 · ' + conflicts.length + ' 条重复 · ' + session.errors + ' 条无效';
    $('#import-rows').innerHTML = session.rows.map(row => {
      const server = row.server || {};
      const conflict = importConflict(row);
      const existing = row.existing;
      const detail = row.duplicate_of_row ? '与 CSV 数据行 ' + row.duplicate_of_row + ' 重复' : existing ? '已存在：' + (existing.name || existing.address || '') : '与已有服务器重复';
      const status = row.error ? '<span class="import-row-error">' + escape(row.error) + '</span>' : conflict ? '<span class="import-duplicate">' + escape(detail) + '</span>' : '<span class="import-new">可新增</span>';
      const action = row.error ? '<span class="muted">请修正</span>' : conflict ? '<select data-import-row="' + Number(row.row) + '" aria-label="数据行 ' + Number(row.row) + ' 的重复项处理"><option value="skip"' + (session.decisions.get(Number(row.row)) === 'skip' ? ' selected' : '') + '>跳过这行</option><option value="overwrite"' + (session.decisions.get(Number(row.row)) === 'overwrite' ? ' selected' : '') + '>覆盖配置</option></select>' : '<span class="import-new">创建服务器</span>';
      return '<tr><td>' + Number(row.row) + '</td><td><strong>' + escape(server.name || '—') + '</strong><span class="server-secondary">' + escape(server.provider || '未填写供应商') + '</span><span class="import-flags"><span class="tag">' + (server.enabled ? '已启用' : '已停用') + '</span><span class="tag">' + (server.trusted ? '用户可信' : '普通 DNS') + '</span></span></td><td>' + escape(server.address || '—') + '</td><td>' + status + '</td><td>' + action + '</td></tr>';
    }).join('');
    syncImportBulk();
    if (session.errors) setError('#import-error', '存在 ' + session.errors + ' 条无效记录，当前不能导入。请修正原 CSV 文件并重新选择。');
  }

  function syncImportBulk() {
    const actions = [...(state.importing?.decisions.values() || [])];
    $('#import-overwrite-all').checked = actions.length > 0 && actions.every(action => action === 'overwrite');
    $('#import-skip-all').checked = actions.length > 0 && actions.every(action => action === 'skip');
    $('#import-overwrite-all').disabled = !actions.length || !!state.importing?.busy;
    $('#import-skip-all').disabled = !actions.length || !!state.importing?.busy;
  }

  function bulkImport(action) {
    const session = state.importing;
    if (!session || session.busy) return;
    session.decisions.forEach((_, row) => session.decisions.set(row, action));
    $$('#import-rows select').forEach(select => { select.value = action; });
    syncImportBulk();
  }

  async function commitImport() {
    const session = state.importing;
    if (!session?.snapshot || session.busy || session.errors || !session.rows.length) return;
    const body = { csv: session.csv, snapshot: session.snapshot, decisions: [...session.decisions].map(([row, action]) => ({ row, action })) };
    if (new TextEncoder().encode(JSON.stringify(body)).length >= 1024 * 1024) return setError('#import-error', '包含处理选项后的请求超过 1 MiB，请拆分 CSV 后导入。');
    setError('#import-error', '');
    importBusy(true, '正在写入服务器清单…');
    try {
      const result = await api('/servers/import', { method: 'POST', body });
      $('#import-dialog').close();
      toast('导入完成：新增 ' + integer(result.created || 0) + '，覆盖 ' + integer(result.updated || 0) + '，跳过 ' + integer(result.skipped || 0));
      await refreshState();
    } catch (error) {
      if (error.status === 409) {
        session.snapshot = '';
        setError('#import-error', '服务器清单在预览后发生变化，请点击“重新预览”，重新核对重复项后再导入。');
        $('#import-status').textContent = '预览已过期，需要重新预览';
      } else {
        setError('#import-error', error.message);
        $('#import-status').textContent = '导入失败，请检查提示后重试';
      }
    } finally { if (state.importing === session) importBusy(false); }
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
  $('#refresh-button').addEventListener('click', manualRefresh);
  $$('.nav-item').forEach(button => button.addEventListener('click', () => navigate(button.dataset.page)));
  $('.sidebar>.brand').addEventListener('click', event => { event.preventDefault(); navigate('overview'); });
  $$('[data-open-guide]').forEach(button => button.addEventListener('click', () => navigate('guide')));
  $$('[data-range]').forEach(button => button.addEventListener('click', () => changeRange(button.dataset.range)));
  $$('.sort-button').forEach(button => button.addEventListener('click', () => { state.descending = state.sort === button.dataset.sort ? !state.descending : !['average_ms', 'grade'].includes(button.dataset.sort); state.sort = button.dataset.sort; renderServerRows(); }));
  $('#search-filter').addEventListener('input', renderServerRows);
  $$('[data-trigger]').forEach(button => button.addEventListener('click', event => { event.stopPropagation(); toggleFilterPopover(button.dataset.trigger); }));
  $$('.multi-select [data-popover]').forEach(popover => popover.addEventListener('change', event => {
    const checkbox = event.target.closest('input[type=checkbox]');
    if (!checkbox) return;
    const key = popover.dataset.popover;
    if (checkbox.dataset.all !== undefined) state.filters[key] = [];
    else {
      const current = state.filters[key];
      const index = current.indexOf(checkbox.value);
      if (checkbox.checked && index < 0) current.push(checkbox.value);
      if (!checkbox.checked && index >= 0) current.splice(index, 1);
    }
    persistFilters();
    renderFilterOptions(key);
    renderServerRows();
  }));
  document.addEventListener('click', event => { if (!event.target.closest('.multi-select')) closeFilterPopovers(); });
  document.addEventListener('keydown', event => { if (event.key === 'Escape') closeFilterPopovers(); });
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
  $('#config-form').addEventListener('input', event => {
    markConfigDirty();
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target);
  });
  $('#config-form').addEventListener('change', event => {
    markConfigDirty();
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target, true);
  });
  $('#config-form').addEventListener('focusout', event => {
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target, true);
  });
  $('#config-form').addEventListener('invalid', event => {
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target, true);
  }, true);
  $('#config-form').addEventListener('submit', saveConfig);
  $('#add-domain-button').addEventListener('click', () => { const row = domainRow(); $('#domain-rows').append(row); $('.domain-name', row).focus(); markConfigDirty(); });
  $('#domain-rows').addEventListener('click', event => {
    const button = event.target.closest('.remove-domain');
    if (!button) return;
    if ($$('.domain-row').length <= 1) return toast('至少需要保留一个探测域名', true);
    button.closest('.domain-row').remove();
    markConfigDirty();
  });
  ['mouseover', 'focusin', 'click'].forEach(eventName => $('#status-history').addEventListener(eventName, event => {
    const block = event.target.closest('[data-status-index]');
    if (!block) return;
    const point = state.statusHistory[Number(block.dataset.statusIndex)];
    if (point) $('#status-history-inspection').textContent = statusDescription(point, block.dataset.statusChannel);
  }));
  $$('[data-detail-range]').forEach(button => button.addEventListener('click', () => { if (button.dataset.detailRange === state.detailRange) return; state.detailRange = button.dataset.detailRange; loadHistory(); }));
  $('#detail-probe').addEventListener('click', probeNow);
  $('#detail-edit').addEventListener('click', () => openServer(currentServer()));
  $('#results-refresh').addEventListener('click', () => { setError('#detail-error', ''); Promise.allSettled([loadHistory(), loadResults(true)]); });
  $('#results-more').addEventListener('click', () => loadResults());
  $('#result-rows').addEventListener('click', event => { const button = event.target.closest('[data-result-index]'); if (button) showVerdict(Number(button.dataset.resultIndex)); });
  $$('[data-verdict]').forEach(button => button.addEventListener('click', () => saveVerdict(button.dataset.verdict)));
  $('#service-buttons').addEventListener('click', event => { const button = event.target.closest('[data-service-action]'); if (button && !button.disabled) serviceAction(button.dataset.serviceAction); });
  $('#import-servers-button').addEventListener('click', openImport);
  $('#import-file').addEventListener('change', readImportFile);
  $('#import-preview-button').addEventListener('click', previewImport);
  $('#import-commit-button').addEventListener('click', commitImport);
  $('#import-overwrite-all').addEventListener('change', () => bulkImport('overwrite'));
  $('#import-skip-all').addEventListener('change', () => bulkImport('skip'));
  $('#import-rows').addEventListener('change', event => { const select = event.target.closest('[data-import-row]'); if (!select || !state.importing || state.importing.busy) return; state.importing.decisions.set(Number(select.dataset.importRow), select.value); syncImportBulk(); });
  $('#export-servers-button').addEventListener('click', () => downloadCSV('/servers/export', 'dns-monitor-servers.csv', $('#export-servers-button'), 'DNS 服务器清单已导出'));
  ['server-template-button', 'import-template-button'].forEach(id => $('#' + id).addEventListener('click', () => downloadCSV('/servers/template', 'dns-monitor-servers-template.csv', $('#' + id), '服务器 CSV 模板已下载，请替换其中的示例行')));
  $('#export-button').addEventListener('click', exportSummary);
  $('#export-all-results-button').addEventListener('click', () => exportCSV());
  $('#detail-export').addEventListener('click', () => exportCSV(state.detailID));
  ['rate-chart', 'latency-chart'].forEach(id => { const canvas = $(`#${id}`); canvas.addEventListener('pointermove', chartHover); canvas.addEventListener('pointerleave', () => { $('.chart-tooltip', canvas.parentElement).hidden = true; }); });
  let resizeTimer;
  window.addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(renderCharts, 100); });
  document.addEventListener('visibilitychange', () => { if (!document.hidden && state.token) refreshState(); });
  window.addEventListener('beforeunload', event => { if (state.configDirty) { event.preventDefault(); event.returnValue = ''; } });
  if (storedSession?.token) {
    state.token = storedSession.token;
    persistSession();
    $('#login-screen').hidden = true;
    $('#app').hidden = false;
    if (state.page !== 'overview') navigate(state.page, true);
    refreshState(true);
    scheduleRefresh();
  }
})();
