// tock web. Plain DOM, no dependencies. Every string from the server is
// rendered with text nodes only: nothing here assigns HTML.
'use strict';

(() => {
  const $ = (id) => document.getElementById(id);
  const main = $('main');
  const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)');
  const narrow = matchMedia('(max-width: 640px)');

  // ---------- token ----------
  // The link tock web prints carries the token in the fragment, which the
  // browser never sends anywhere. Keep it for this tab and clean the URL.
  let token = '';
  try {
    const m = location.hash.match(/^#(?:t=)?([0-9a-f]{64})$/);
    if (m) sessionStorage.setItem('tock-token', m[1]);
    token = sessionStorage.getItem('tock-token') || '';
  } catch (_) { /* storage blocked: the token lives in memory only */ }
  if (!token) {
    const m = location.hash.match(/^#(?:t=)?([0-9a-f]{64})$/);
    if (m) token = m[1];
  }
  if (location.hash) history.replaceState(history.state, '', location.pathname + location.search);

  // ---------- DOM helpers ----------
  function h(tag, props, ...kids) {
    const el = document.createElement(tag);
    if (props) setProps(el, props);
    append(el, kids);
    return el;
  }
  const SVGNS = 'http://www.w3.org/2000/svg';
  function s(tag, attrs, ...kids) {
    const el = document.createElementNS(SVGNS, tag);
    for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, String(v));
    append(el, kids);
    return el;
  }
  function setProps(el, props) {
    for (const [k, v] of Object.entries(props)) {
      if (v === undefined || v === null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'style') for (const [p, val] of Object.entries(v)) el.style.setProperty(p, val);
      else if (k === 'data') for (const [p, val] of Object.entries(v)) el.dataset[p] = val;
      else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
      else if (k === 'text') el.textContent = v;
      else el.setAttribute(k, v === true ? '' : String(v));
    }
  }
  function append(el, kids) {
    for (const k of kids.flat(Infinity)) {
      if (k === null || k === undefined || k === false) continue;
      el.append(k instanceof Node ? k : document.createTextNode(String(k)));
    }
  }

  // ---------- formatting ----------
  // dur matches the CLI: "2h14", "41m", "45s".
  function dur(ms) {
    const sec = Math.floor(Math.max(0, ms) / 1000);
    if (sec >= 3600) return `${Math.floor(sec / 3600)}h${String(Math.floor((sec % 3600) / 60)).padStart(2, '0')}`;
    if (sec >= 60) return `${Math.floor(sec / 60)}m`;
    return `${sec}s`;
  }
  // hm is a table duration: "2:24", "0:41".
  function hm(ms) {
    const min = Math.round(Math.max(0, ms) / 60000);
    return `${Math.floor(min / 60)}:${String(min % 60).padStart(2, '0')}`;
  }
  const clock = (ms) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
  const pct = (part, whole) => (whole > 0 ? `${Math.round((100 * part) / whole)}%` : '');
  function parseDate(str) {
    const [y, m, d] = str.split('-').map(Number);
    return new Date(y, m - 1, d);
  }
  function fmtDate(d) {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }
  const addDays = (str, n) => { const d = parseDate(str); d.setDate(d.getDate() + n); return fmtDate(d); };
  const todayStr = () => fmtDate(new Date());
  const longDate = (str) => parseDate(str).toLocaleDateString([], { weekday: 'long', day: 'numeric', month: 'long' });
  const shortDate = (str) => parseDate(str).toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' });
  const mondayOf = (str) => { const d = parseDate(str); d.setDate(d.getDate() - ((d.getDay() + 6) % 7)); return fmtDate(d); };
  const KINDS = ['deep', 'shallow', 'neutral', 'browse', 'distract'];
  const KIND_NAMES = { deep: 'Deep', shallow: 'Shallow', neutral: 'Neutral', browse: 'Browsing', distract: 'Distracting', '': 'Unlabelled' };
  const kindClass = (k) => `k-${k || 'none'}`;
  // The reports name unlabelled time "Unclassified" or "(unlabelled)".
  const nice = (name) => (!name || name === 'Unclassified' || name === '(unlabelled)' ? 'Unlabelled' : name);
  const sourceNames = { rule: 'rule', learned: 'learned', manual: 'manual', call: 'call', calendar: 'calendar' };

  // ---------- springs ----------
  // A spring with Apple's two parameters: damping ratio and response (the
  // period in seconds). It animates from the current value and velocity,
  // so a new target mid-flight bends the motion instead of restarting it.
  class Spring {
    constructor(value, { damping = 1, response = 0.35, precision = 0.5, onUpdate = () => {}, onRest = null } = {}) {
      Object.assign(this, { value, target: value, velocity: 0, damping, response, precision, onUpdate, onRest, raf: 0, last: 0 });
      this.step = this.step.bind(this);
    }
    set(target, velocity) {
      this.target = target;
      if (velocity !== undefined) this.velocity = velocity;
      if (reduceMotion.matches) return this.jump(target);
      if (!this.raf) {
        this.last = performance.now();
        this.raf = requestAnimationFrame(this.step);
      }
    }
    jump(value) {
      cancelAnimationFrame(this.raf);
      this.raf = 0;
      this.value = this.target = value;
      this.velocity = 0;
      this.onUpdate(value);
      if (this.onRest) this.onRest(value);
    }
    step(now) {
      const dt = Math.min(0.064, (now - this.last) / 1000);
      this.last = now;
      const k = (2 * Math.PI / this.response) ** 2;
      const c = (4 * Math.PI * this.damping) / this.response;
      const n = Math.max(1, Math.ceil(dt / 0.004));
      const step = dt / n;
      for (let i = 0; i < n; i++) {
        const a = -k * (this.value - this.target) - c * this.velocity;
        this.velocity += a * step;
        this.value += this.velocity * step;
      }
      if (Math.abs(this.value - this.target) < this.precision && Math.abs(this.velocity) < this.precision * 10) {
        this.raf = 0;
        this.value = this.target;
        this.velocity = 0;
        this.onUpdate(this.value);
        if (this.onRest) this.onRest(this.value);
        return;
      }
      this.onUpdate(this.value);
      this.raf = requestAnimationFrame(this.step);
    }
  }

  // ---------- state ----------
  const params = new URLSearchParams(location.search);
  const state = {
    view: ['today', 'week', 'top', 'inbox'].includes(params.get('v')) ? params.get('v') : 'today',
    date: /^\d{4}-\d{2}-\d{2}$/.test(params.get('d') || '') ? params.get('d') : todayStr(),
    topRange: localGet('tock-top') || 'days:7',
    inboxDays: Number(localGet('tock-inbox-days')) || 7,
    unlabelledOnly: localGet('tock-unlabelled') === '1',
    data: null, // the current view's response
    sel: -1, // selected inbox item or activity row
    labels: null,
    loading: 0,
  };
  function localGet(k) { try { return localStorage.getItem(k); } catch (_) { return null; } }
  function localSet(k, v) { try { localStorage.setItem(k, v); } catch (_) { /* per-viewer nicety only */ } }

  // ---------- API ----------
  class APIError extends Error {
    constructor(status, msg) { super(msg); this.status = status; }
  }
  async function api(path, body) {
    let resp;
    try {
      resp = await fetch(path, {
        method: body ? 'POST' : 'GET',
        headers: Object.assign({ 'X-Tock-Token': token }, body ? { 'Content-Type': 'application/json' } : {}),
        body: body ? JSON.stringify(body) : undefined,
        cache: 'no-store',
        credentials: 'omit',
      });
    } catch (_) {
      setLive(false);
      throw new APIError(0, 'tock web is not running. Start it again with: tock web');
    }
    let data = null;
    try { data = await resp.json(); } catch (_) { /* not JSON */ }
    if (!resp.ok) {
      if (resp.status === 401) setLive(false);
      throw new APIError(resp.status, (data && data.error) || `request failed (${resp.status})`);
    }
    setLive(true);
    return data;
  }

  function setLive(on) {
    const dot = $('live');
    dot.classList.toggle('on', on);
    dot.classList.toggle('off', !on);
    dot.title = on ? 'connected' : 'not connected';
  }

  function banner(text, err) {
    const b = $('banner');
    b.hidden = !text;
    b.textContent = text || '';
    b.classList.toggle('err', !!err);
  }

  // ---------- nav ----------
  const tabs = $('tabs');
  tabs.addEventListener('pointerdown', (e) => {
    // Respond on press, not release.
    const btn = e.target.closest('button[data-view]');
    if (btn && e.button === 0) go(btn.dataset.view);
  });
  tabs.addEventListener('click', (e) => {
    const btn = e.target.closest('button[data-view]');
    if (btn && e.detail === 0) go(btn.dataset.view); // keyboard activation
  });

  // ---------- navigation ----------
  function go(view, date, push = true) {
    const changed = view !== state.view || (date && date !== state.date);
    state.view = view;
    if (date) state.date = date;
    state.sel = -1;
    if (push && changed) history.pushState(null, '', url());
    syncChrome();
    load({ enter: changed });
  }
  function url() {
    const q = new URLSearchParams();
    if (state.view !== 'today') q.set('v', state.view);
    if ((state.view === 'today' || state.view === 'week') && state.date !== todayStr()) q.set('d', state.date);
    const qs = q.toString();
    return location.pathname + (qs ? `?${qs}` : '');
  }
  addEventListener('popstate', () => {
    const p = new URLSearchParams(location.search);
    state.view = p.get('v') || 'today';
    state.date = p.get('d') || todayStr();
    syncChrome();
    load({ enter: true });
  });

  function syncChrome() {
    for (const b of tabs.querySelectorAll('button[data-view]')) {
      const on = b.dataset.view === state.view;
      b.setAttribute('aria-selected', on ? 'true' : 'false');
      b.tabIndex = on ? 0 : -1;
    }
    renderCal();
    const nav = $('datenav');
    const dated = state.view === 'today' || state.view === 'week';
    nav.classList.toggle('hidden', !dated);
    const t = todayStr();
    const label = $('date-label');
    if (state.view === 'today') {
      label.textContent = state.date === t ? 'Today' : state.date === addDays(t, -1) ? 'Yesterday' : shortDate(state.date);
      $('next').disabled = state.date >= t;
    } else if (state.view === 'week') {
      const mon = mondayOf(state.date);
      label.textContent = mon === mondayOf(t) ? 'This week' : mon === addDays(mondayOf(t), -7) ? 'Last week' : `Week of ${parseDate(mon).toLocaleDateString([], { day: 'numeric', month: 'short' })}`;
      $('next').disabled = mon >= mondayOf(t);
    }
    document.title = { today: 'Today', week: 'Week', top: 'Top', inbox: 'Inbox' }[state.view] + ' · tock';
  }
  function step(n) {
    if (state.view === 'today') {
      const d = addDays(state.date, n);
      if (d <= todayStr()) go('today', d);
    } else if (state.view === 'week') {
      const d = addDays(mondayOf(state.date), 7 * n);
      if (d <= mondayOf(todayStr())) go('week', d);
    }
  }
  $('prev').addEventListener('click', () => step(-1));
  $('next').addEventListener('click', () => step(1));
  $('date-label').addEventListener('click', () => go(state.view, todayStr()));

  // ---------- loading ----------
  function viewPath() {
    switch (state.view) {
      case 'today': return `/api/day?date=${state.date}`;
      case 'week': return `/api/week?date=${state.date}`;
      case 'top': {
        const [kind, val] = state.topRange.split(':');
        return kind === 'range' ? `/api/top?range=${val}` : `/api/top?days=${val}`;
      }
      default: return `/api/inbox?days=${state.inboxDays}`;
    }
  }

  let seq = 0;
  async function load({ enter = false, quiet = false } = {}) {
    if (!token) return renderNoToken();
    const my = ++seq;
    const path = viewPath();
    let skel = 0;
    if (!quiet) skel = setTimeout(() => { if (my === seq) render([h('div', { class: 'skel' }), h('div', { class: 'skel', style: { 'margin-top': '1rem' } })], false); }, 160);
    try {
      const data = await api(path);
      if (my !== seq) return;
      clearTimeout(skel);
      state.data = data;
      banner(null);
      const scroll = scrollY;
      render(VIEWS[state.view](data), enter && !quiet, quiet);
      if (quiet) scrollTo(0, scroll);
      if (state.view === 'inbox') setBadge(data.items.length);
    } catch (e) {
      clearTimeout(skel);
      if (my !== seq) return;
      if (e.status === 401) return renderNoToken();
      banner(e.message, true);
      if (!state.data) render([emptyState('Nothing to show', e.message)], enter);
    }
  }

  function render(nodes, enter, quiet) {
    main.classList.remove('enter');
    main.classList.toggle('quiet', !!quiet);
    main.replaceChildren(...[nodes].flat().filter(Boolean));
    if (enter && !reduceMotion.matches) {
      void main.offsetWidth;
      main.classList.add('enter');
    }
  }

  function renderNoToken() {
    setLive(false);
    $('datenav').classList.add('hidden');
    render([emptyState('Open tock from the terminal', 'This page needs the private link that tock web prints. Run tock web and use the link it opens.')]);
  }

  // firstRun is Today before anything has been recorded: what happens next,
  // in the order it happens. The page refreshes itself every minute.
  function firstRun() {
    const step = (title, text) => h('li', null, h('div', { class: 'big' }, title), h('div', null, text));
    return h('div', { class: 'empty first' },
      h('div', { class: 'big' }, 'Nothing recorded yet'),
      h('div', null, 'trackd records the app in front as you work. This page updates every minute.'),
      h('ol', { class: 'steps' },
        step('Work as usual', 'The first entries show up here within a minute or two.'),
        step('Label what no rule covers', 'It goes to the Inbox. Labelling an entry there also writes a rule, so you label each thing once.'),
        step('Write broader rules', 'They live in ~/.config/tock/rules.toml and apply to past days too.')));
  }

  function emptyState(title, text) {
    return h('div', { class: 'empty' }, h('div', { class: 'big' }, title), h('div', null, text));
  }

  async function refreshBadge() {
    try {
      const d = await api('/api/inbox?days=7');
      setBadge(d.items.length);
    } catch (_) { /* the banner covers errors */ }
  }
  function setBadge(n) {
    const b = $('inbox-count');
    b.hidden = !n;
    b.textContent = n > 99 ? '99+' : String(n);
  }

  async function loadLabels() {
    try {
      state.labels = await api('/api/labels');
      if (state.labels.rulesError) banner(`rules.toml has a problem, so tock uses the last rules that loaded: ${state.labels.rulesError}`);
    } catch (_) { /* the banner covers errors */ }
    return state.labels;
  }

  // ---------- calendar ----------
  let calMonth = null; // 'YYYY-MM' shown in the sidebar
  function renderCal() {
    const el = $('cal');
    const t = todayStr();
    const sel = state.view === 'today' ? state.date : '';
    if (!calMonth || (sel && sel.slice(0, 7) !== calMonth && !el.dataset.browsing)) calMonth = (sel || t).slice(0, 7);
    const [y, m] = calMonth.split('-').map(Number);
    const first = new Date(y, m - 1, 1);
    const days = new Date(y, m, 0).getDate();
    const lead = (first.getDay() + 6) % 7;
    const move = (n) => { const d = new Date(y, m - 1 + n, 1); calMonth = fmtDate(d).slice(0, 7); el.dataset.browsing = '1'; renderCal(); };
    const grid = h('div', { class: 'days' }, ['M', 'T', 'W', 'T', 'F', 'S', 'S'].map((d) => h('span', { class: 'h' }, d)));
    for (let i = 0; i < lead; i++) grid.append(h('span'));
    for (let d = 1; d <= days; d++) {
      const ds = `${calMonth}-${String(d).padStart(2, '0')}`;
      grid.append(h('button', {
        class: `${ds === sel ? 'on' : ''} ${ds === t ? 'today' : ''}`, disabled: ds > t, 'aria-label': longDate(ds),
        'aria-current': ds === sel ? 'date' : undefined,
        onclick: () => { delete el.dataset.browsing; go('today', ds); },
      }, String(d)));
    }
    el.replaceChildren(
      h('div', { class: 'cal-head' },
        h('button', { 'aria-label': 'Previous month', onclick: () => move(-1) }, '‹'),
        h('b', null, first.toLocaleDateString([], { month: 'long', year: 'numeric' })),
        h('button', { 'aria-label': 'Next month', disabled: calMonth >= t.slice(0, 7), onclick: () => move(1) }, '›')),
      grid);
  }

  // ---------- status ----------
  state.status = null;
  async function loadStatus() {
    try { state.status = await api('/api/status'); } catch (_) { state.status = null; }
    renderStatus();
    renderSession();
  }
  function renderStatus() {
    const st = state.status;
    const el = $('status');
    if (!st) { el.replaceChildren(); return; }
    const line = (cls, text, title) => h('div', { title: title || text }, h('i', { class: cls }), h('span', null, text));
    const gone = Date.now() - st.lastSeenMS;
    const ago = gone >= 48 * 3600e3 ? `${Math.floor(gone / 86400e3)} days` : dur(gone);
    el.replaceChildren(...[
      st.recording ? line('good', 'trackd recording') : line('bad', st.lastSeenMS ? `trackd stopped ${ago} ago` : 'trackd has not run yet', 'Run tock doctor to see why'),
      st.rulesError ? line('bad', 'rules.toml has an error', st.rulesError) : line('good', 'rules.toml OK'),
      st.configError ? line('bad', 'config.toml has an error', st.configError) : null].filter(Boolean));
  }
  function renderSession() {
    const el = $('sess');
    const ses = state.status && state.status.session;
    if (!ses) { el.hidden = true; return; }
    const now = Date.now();
    const total = Math.max(1, ses.plannedMS - ses.startMS);
    const done = Math.max(0, Math.min(total, now - ses.startMS));
    el.hidden = false;
    el.replaceChildren(
      h('span', { class: 'muted' }, 'Session'),
      h('b', { title: ses.goal || 'Focus session' }, ses.goal || 'Focus session'),
      h('span', { class: 'meter' }, h('i', { style: { width: `${(100 * done) / total}%` } })),
      h('span', { class: 'num' }, now > ses.plannedMS ? `${dur(now - ses.plannedMS)} over` : `${dur(ses.plannedMS - now)} left`));
  }

  // ---------- shared pieces ----------
  function card(title, body, opts = {}) {
    return h('section', { class: `card ${opts.cls || ''}` },
      title ? h('div', { class: 'card-head' }, h('h2', null, title), opts.aside || null) : null,
      body);
  }

  function kindStack(byKind, total) {
    const keys = [...KINDS, ''];
    return h('div', null,
      h('div', { class: 'stack', role: 'img', 'aria-label': keys.filter((k) => byKind[k]).map((k) => `${KIND_NAMES[k]} ${dur(byKind[k])}`).join(', ') },
        keys.filter((k) => byKind[k] > 0).map((k) => h('i', { class: kindClass(k), style: { flex: String(byKind[k]) } }))),
      legend(byKind, total));
  }
  function legend(byKind, total) {
    const keys = [...KINDS, ''];
    return h('div', { class: 'legend' },
      keys.filter((k) => byKind[k] > 0).map((k) => h('span', null, h('i', { class: `dot ${kindClass(k)}` }), KIND_NAMES[k], h('b', null, dur(byKind[k])), h('span', { class: 'cap' }, pct(byKind[k], total)))));
  }

  // ledger is a table: name, time and share, sub rows under a row, and a
  // totals row.
  function ledger(rows, { total, kindOf, sub, empty = 'Nothing yet', head = 'Name', limit = 0, totals = true }) {
    if (!rows.length) return h('div', { class: 'ledger-empty' }, empty);
    const shown = limit && rows.length > limit + 1 ? rows.slice(0, limit) : rows;
    const rest = rows.slice(shown.length);
    const body = h('tbody');
    for (const r of shown) {
      const kind = kindOf ? kindOf(r) : r.kind;
      body.append(h('tr', null,
        h('td', { class: 'name', title: nice(r.name) }, kind !== undefined ? h('i', { class: `dot ${kindClass(kind)}` }) : null, nice(r.name), r.mostly ? h('span', { class: 'mostly' }, `mostly ${nice(r.mostly)}`) : null),
        h('td', { class: 'r' }, hm(r.ms)),
        total ? h('td', { class: 'r p' }, pct(r.ms, total)) : null));
      for (const x of (sub ? sub(r) : [])) {
        body.append(h('tr', { class: 'sub' }, h('td', { class: 'name', title: x.name }, x.name), h('td', { class: 'r' }, hm(x.ms)), total ? h('td', { class: 'r p' }) : null));
      }
    }
    if (rest.length) {
      const ms = rest.reduce((n, r) => n + r.ms, 0);
      body.append(h('tr', null, h('td', { class: 'name muted' }, `${rest.length} more`), h('td', { class: 'r' }, hm(ms)), total ? h('td', { class: 'r p' }, pct(ms, total)) : null));
    }
    if (totals && total) {
      const sum = rows.reduce((n, r) => n + r.ms, 0);
      body.append(h('tr', { class: 'tot' }, h('td', null, 'Total'), h('td', { class: 'r' }, hm(sum)), h('td', { class: 'r p' }, pct(sum, total))));
    }
    return h('table', { class: 'ledger' },
      h('thead', null, h('tr', null, h('th', null, head), h('th', { class: 'r' }, 'Time'), total ? h('th', { class: 'r' }, 'Share') : null)),
      body);
  }

  // workText is the day's work hours for the subtitle.
  function workText(w) {
    if (!w) return '';
    if (w.off) return ' · day off, not scored';
    return ` · scored ${clock(w.from)}–${clock(w.to)}${w.outsideMS >= 60000 ? ` · ${dur(w.outsideMS)} outside` : ''}`;
  }

  function goalsCard(goals, cls = 'c6') {
    if (!goals || !goals.length) return null;
    const body = h('tbody');
    for (const g of goals) {
      const stateText = g.days ? `${g.held}/${g.days} days` : { met: 'Met', short: 'Not yet', over: 'Over', ok: 'Under' }[g.state] || g.state;
      body.append(h('tr', null,
        h('td', { class: 'name' }, g.name, h('span', { class: 'mostly' }, `${g.atMost ? 'at most' : 'at least'} ${dur(g.ms)} a ${g.week ? 'week' : 'day'}${g.streak >= 2 ? ` · ${g.streak}${g.week ? 'w' : 'd'} streak` : ''}`)),
        h('td', { class: 'r' }, g.days ? '' : hm(g.doneMS)),
        h('td', { class: `r goal-state ${g.state}`, style: { width: '84px' } }, stateText)));
    }
    return card('Goals', h('table', { class: 'ledger' },
      h('thead', null, h('tr', null, h('th', null, 'Goal'), h('th', { class: 'r' }, 'Done'), h('th', { class: 'r' }, 'State'))), body), { cls });
  }

  // ---------- Today ----------
  // A change against the usual day: "+48m" in good or bad. better says
  // which way is good; 0 means neither.
  function delta(now, usual, fmt, better) {
    if (usual === undefined || usual === null) return null;
    const diff = now - usual;
    const text = fmt(Math.abs(diff));
    if (text === fmt(0)) return h('small', null, '±0');
    const cls = !better ? '' : (diff > 0) === (better > 0) ? 'good' : 'bad';
    return h('small', { class: cls }, `${diff > 0 ? '+' : '−'}${text}`);
  }

  function kpi(label, value, d, title) {
    return h('div', { class: 'kpi', title }, h('div', { class: 'l' }, label), h('div', { class: 'v' }, value, d));
  }

  function kpis(d) {
    const sc = d.score;
    const u = d.usual;
    const n0 = (x) => String(Math.round(x));
    const f1 = (x) => x.toFixed(1);
    const parts = `How the score is made: deep time ${Math.round(sc.depth * 100)}% of the ${dur(sc.targetMS)} target, ` +
      `time in long blocks ${Math.round(sc.blocks * 100)}%, few switches ${Math.round(sc.calm * 100)}%, ` +
      `longest stretch ${Math.round(sc.stretch * 100)}%${sc.penalty ? `, −${sc.penalty} for distraction` : ''}.`;
    const row = h('div', { class: 'kpis' },
      kpi('Focus score', String(sc.value), u && delta(sc.value, u.score, n0, 1), parts),
      kpi('Deep focus', dur(d.stats.focusMS), u && delta(d.stats.focusMS, u.focusMS, dur, 1)),
      kpi('Longest block', dur(d.stats.longestMS), u && delta(d.stats.longestMS, u.longestMS, dur, 1)),
      kpi('Distracting', dur(d.stats.distractMS), u && delta(d.stats.distractMS, u.distractMS, dur, -1)),
      kpi('Switches / hour', f1(sc.switchesPerHour), u && delta(sc.switchesPerHour, u.switchesPerHour, f1, -1)),
      kpi('Tracked', dur(d.trackedMS), u && delta(d.trackedMS, u.trackedMS, dur, 0)));
    const day = parseDate(d.date).toLocaleDateString([], { weekday: 'long' });
    const note = u ? `Changes compare with your usual ${day}: the middle of your last ${u.days} ${day}s.` : `After two ${day}s with time tracked, these numbers show the change against your usual ${day}.`;
    return h('div', null, row, h('div', { class: 'foot-note' }, note));
  }

  function renderToday(d) {
    const nodes = [];
    nodes.push(h('div', { class: 'toolrow' },
      h('h1', null, longDate(d.date)),
      h('span', { class: 'sub' }, (d.trackedMS ? `${dur(d.trackedMS)} tracked${d.idleMS ? `, ${dur(d.idleMS)} away` : ''}` : 'Nothing tracked') + workText(d.work))));
    if (d.firstRun) {
      nodes.push(firstRun());
      return nodes;
    }
    if (!d.trackedMS && !d.timeline.length) {
      nodes.push(emptyState(d.today ? 'Nothing yet today' : 'Nothing tracked this day', d.today ? 'trackd records as you work. Check back in a few minutes.' : 'Pick another day in the calendar.'));
      return nodes;
    }
    const grid = h('div', { class: 'grid' });
    grid.append(card(null, kpis(d), { cls: 'c12' }));
    grid.append(card('Timeline', h('div', null, gantt(d), legend(d.byKind, d.trackedMS)), { cls: 'c12', aside: h('span', { class: 'cap' }, 'Click a bar to label it') }));
    grid.append(card(null, ledger(d.categories.map((c) => ({ name: c.name, ms: c.ms, kind: c.kind, projects: c.projects })), {
      head: 'Category', total: d.trackedMS,
      sub: (r) => (r.projects || []).filter((p) => p.name).slice(0, 4),
    }), { cls: 'c6' }));
    grid.append(card(null, ledger(appRows(d), { head: 'App', total: d.trackedMS, limit: 8, kindOf: (r) => r.kind }), { cls: 'c6' }));
    const work = blocksCard(d);
    const goals = goalsCard(d.goals, work ? 'c4' : 'c12');
    if (work) grid.append(work);
    if (goals) grid.append(goals);
    grid.append(activityCard(d));
    nodes.push(grid);
    return nodes;
  }

  // appRows adds the day's rows up by app, each in the kind most of its
  // time had.
  function appRows(d) {
    const by = new Map();
    for (const r of d.timeline) {
      if (r.idle) continue;
      const a = by.get(r.app) || { name: r.app, ms: 0, kinds: {} };
      const ms = r.end - r.start;
      a.ms += ms;
      a.kinds[r.kind || ''] = (a.kinds[r.kind || ''] || 0) + ms;
      by.set(r.app, a);
    }
    return [...by.values()].sort((x, y) => y.ms - x.ms)
      .map((a) => ({ name: a.name, ms: a.ms, kind: Object.entries(a.kinds).sort((x, y) => y[1] - x[1])[0][0] }));
  }

  // Work blocks: stretches of work on one thing, with Claude's lines when
  // tock summary has run.
  function blocksCard(d) {
    const blocks = d.blocks || [];
    if (!blocks.length && !d.summary) return null;
    const body = h('tbody');
    for (const b of blocks) {
      const facts = [];
      if (b.deepMS) facts.push(`deep ${pct(b.deepMS, b.activeMS)}`);
      if (b.distractMS >= 60e3) facts.push(`distracted ${dur(b.distractMS)}`);
      facts.push(...b.top.map((t) => t.name));
      body.append(h('tr', null,
        h('td', { class: 'r', style: { 'text-align': 'left', width: '96px', color: 'var(--muted)' } }, `${clock(b.start)}–${clock(b.end)}`),
        h('td', { class: 'name', title: b.summary || facts.join(' · ') },
          h('i', { class: `dot ${kindClass(b.kind)}` }), nice(b.name),
          h('span', { class: 'mostly' }, facts.join(' · ')),
          b.summary ? h('div', { class: 'blurb' }, b.summary) : null),
        h('td', { class: 'r' }, hm(b.activeMS))));
    }
    const aside = d.summary ? null : h('span', { class: 'cap' }, 'tock summary writes what you worked on');
    return card('Work blocks', h('div', null,
      d.summary ? h('p', { class: 'summary' }, d.summary) : null,
      blocks.length ? h('table', { class: 'ledger' }, body) : null), { cls: d.goals && d.goals.length ? 'c8' : 'c12', aside });
  }

  function dayBounds(d) {
    const start = parseDate(d.date).getTime();
    return { from: start + d.hourFirst * 3600e3, to: start + (d.hourLast + 1) * 3600e3 };
  }

  // gantt is the day on one hour axis, one row per category (then
  // unlabelled time, then time away).
  function gantt(d) {
    const { from, to } = dayBounds(d);
    const span = to - from;
    const hours = d.hourLast - d.hourFirst + 1;
    const order = d.categories.map((c) => nice(c.name));
    const rows = new Map();
    const rowOf = (r) => (r.idle ? 'Away' : r.category ? r.category : 'Unlabelled');
    d.timeline.forEach((r, i) => {
      const name = rowOf(r);
      if (!rows.has(name)) rows.set(name, { name, ms: 0, items: [] });
      const row = rows.get(name);
      row.ms += r.end - r.start;
      row.items.push(i);
    });
    const rank = (n) => (n === 'Away' ? 1e6 : n === 'Unlabelled' ? 1e5 : order.indexOf(n) < 0 ? 1e4 : order.indexOf(n));
    const sorted = [...rows.values()].sort((a, b) => rank(a.name) - rank(b.name));
    const el = h('div', { class: 'gantt', style: { '--hour': `${100 / hours}%` } });
    const axis = h('div', { class: 'lane' });
    const every = hours > 14 ? 2 : 1;
    for (let hr = d.hourFirst; hr <= d.hourLast; hr += every) {
      axis.append(h('span', { style: { left: `${(100 * (hr - d.hourFirst)) / hours}%` } }, String(hr % 24).padStart(2, '0')));
    }
    el.append(h('div', { class: 'grow-row axis', 'aria-hidden': 'true' }, h('div', { class: 'n' }), axis));
    let laneLeft = null;
    for (const row of sorted) {
      const lane = h('div', { class: 'lane', role: 'list', 'aria-label': row.name });
      for (const i of row.items) {
        const r = d.timeline[i];
        const a = Math.max(r.start, from);
        const b = Math.min(r.end, to);
        if (b <= a) continue;
        lane.append(h('i', {
          class: kindClass(r.idle ? 'idle' : r.kind || ''), role: 'listitem', data: { i: String(i) },
          'aria-label': `${clock(r.start)} to ${clock(r.end)}, ${r.idle ? 'away' : r.app}`,
          style: { left: `${(100 * (a - from)) / span}%`, width: `${(100 * (b - a)) / span}%` },
        }));
      }
      el.append(h('div', { class: `grow-row ${row.name === 'Away' ? 'away' : ''}` },
        h('div', { class: 'n' }, h('span', { title: row.name }, row.name), h('span', { class: 'num' }, dur(row.ms))), lane));
      laneLeft = laneLeft || lane;
    }
    if (d.today) {
      const now = Date.now();
      if (now > from && now < to) {
        const nowLine = h('div', { class: 'now' });
        el.append(nowLine);
        requestAnimationFrame(() => {
          if (!laneLeft) return;
          const x = laneLeft.offsetLeft + (laneLeft.offsetWidth * (now - from)) / span;
          nowLine.style.setProperty('left', `${x}px`);
        });
      }
    }
    el.addEventListener('pointermove', (e) => {
      const seg = e.target.closest('i[data-i]');
      if (!seg) return hideTip();
      showTip(rowTip(d.timeline[Number(seg.dataset.i)]), e.clientX, e.clientY);
    });
    el.addEventListener('pointerleave', hideTip);
    el.addEventListener('click', (e) => {
      const seg = e.target.closest('i[data-i]');
      if (!seg) return;
      const r = d.timeline[Number(seg.dataset.i)];
      if (!r.idle) { hideTip(); openRowSheet(r, seg); }
    });
    return el;
  }

  function rowTip(r) {
    const label = r.idle ? 'Away' : r.category ? (r.project ? `${r.category} / ${r.project}` : r.category) : r.project ? `project ${r.project}` : 'Unlabelled';
    return [
      h('div', { class: 'h' }, r.idle ? 'Away' : r.app),
      r.text ? h('div', { class: 's' }, r.text) : null,
      h('div', { class: 's' }, `${clock(r.start)}–${r.open ? 'now' : clock(r.end)} · ${dur(r.end - r.start)} · ${label}`),
    ];
  }

  function activityCard(d) {
    const rows = d.timeline.map((r, i) => ({ r, i })).filter(({ r }) => !state.unlabelledOnly || (!r.idle && !r.category && !r.project));
    const list = h('ul', { class: 'acts' });
    for (const { r, i } of [...rows].reverse()) {
      const kind = r.idle ? 'idle' : r.kind || '';
      const label = r.category ? (r.project ? `${r.category} / ${r.project}` : r.category) : r.project ? `project ${r.project}` : 'Unlabelled';
      const li = h('li', {
        class: `act ${r.idle ? 'idle' : ''}`, data: { i: String(i) }, tabindex: r.idle ? undefined : '0',
        onclick: r.idle ? undefined : (e) => openRowSheet(r, e.currentTarget),
        onkeydown: r.idle ? undefined : (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openRowSheet(r, e.currentTarget); } },
      },
        h('span', { class: 't' }, `${clock(r.start)}–${r.open ? 'now' : clock(r.end)}`),
        h('span', { class: 'd' }, dur(r.end - r.start)),
        h('span', { class: 'w', title: r.text || '' }, h('b', null, r.idle ? 'Away' : r.app), r.text ? h('span', null, r.text) : null),
        r.idle ? h('span') : h('span', { class: `chip ${kindClass(kind)}` }, label, r.source && sourceNames[r.source] && r.category ? h('span', { class: 'src' }, `· ${sourceNames[r.source]}`) : null));
      list.append(li);
    }
    const toggle = h('button', {
      class: 'toggle', onclick: () => {
        state.unlabelledOnly = !state.unlabelledOnly;
        localSet('tock-unlabelled', state.unlabelledOnly ? '1' : '0');
        render(renderToday(state.data), false, true);
      },
    }, state.unlabelledOnly ? 'Show all' : 'Unlabelled only');
    return h('section', { class: 'card flat c12' },
      h('div', { class: 'card-head' }, h('h2', null, 'Activity'), toggle),
      rows.length ? list : h('div', { class: 'empty', style: { padding: '2rem' } }, 'Everything here has a label.'));
  }

  // ---------- Week ----------
  function renderWeek(w) {
    const t = todayStr();
    const scored = w.days.filter((d) => !d.future && d.trackedMS > 0);
    const avg = scored.length ? Math.round(scored.reduce((n, d) => n + d.score, 0) / scored.length) : 0;
    const nodes = [h('div', { class: 'toolrow' },
      h('div', null,
        h('h1', null, mondayOf(t) === w.monday ? 'This week' : `Week of ${parseDate(w.monday).toLocaleDateString([], { day: 'numeric', month: 'long' })}`),
        h('p', { class: 'sub', style: { margin: 0 } }, w.trackedMS ? `${dur(w.trackedMS)} tracked${scored.length ? ` · average score ${avg}` : ''}` : 'Nothing tracked')))];
    const grid = h('div', { class: 'grid' });
    const max = Math.max(1, ...w.days.map((d) => d.trackedMS));
    const bars = h('div', { class: 'weekbars' });
    w.days.forEach((d, i) => {
      const col = h('div', { class: 'bars', style: { height: `${Math.max(2, (100 * d.trackedMS) / max)}%` } });
      for (const k of [...KINDS, '']) {
        const ms = d.byKind[k] || 0;
        if (ms > 0) col.append(h('i', { class: kindClass(k), style: { flex: String(ms), 'animation-delay': `${i * 30}ms` } }));
      }
      if (!d.trackedMS) col.style.setProperty('background', 'var(--card-2)');
      const open = () => { if (!d.future) go('today', d.date); };
      bars.append(h('div', {
        class: `wday ${d.future ? 'future' : ''} ${d.date === t ? 'today' : ''}`, role: 'button', tabindex: d.future ? undefined : '0',
        'aria-label': `${longDate(d.date)}: ${dur(d.trackedMS)}, score ${d.score}`, title: `${dur(d.trackedMS)} tracked · ${dur(d.deepMS)} deep`,
        onclick: open, onkeydown: (e) => { if (e.key === 'Enter') open(); },
      },
        h('span', { class: 'sc' }, d.future || !d.trackedMS ? '' : String(d.score)),
        h('div', { style: { flex: '1', width: '100%', display: 'flex', 'align-items': 'flex-end', 'justify-content': 'center' } }, col),
        h('span', { class: 'dn' }, parseDate(d.date).toLocaleDateString([], { weekday: 'short' }))));
    });
    grid.append(card(null, h('div', null, bars, h('div', { style: { 'margin-top': '1.25rem' } }, kindStack(w.byKind, w.trackedMS))), { cls: 'c12' }));
    grid.append(card(null, ledger(w.categories.map((c) => ({ name: c.name, ms: c.ms, kind: c.kind })), { head: 'Category', total: w.trackedMS }), { cls: 'c6' }));
    grid.append(card(null, ledger(w.projects, { head: 'Project', total: w.trackedMS, empty: 'No project time this week' }), { cls: 'c6' }));
    grid.append(card(null, ledger(w.apps, { head: 'App or site', total: w.trackedMS, limit: 10 }), { cls: 'c6' }));
    grid.append(card(null, ledger(w.distractions.map((p) => ({ ...p, kind: 'distract' })), { head: 'Distraction', total: w.trackedMS, empty: 'No distractions this week' }), { cls: 'c6' }));
    const goals = goalsCard(w.goals, w.plan ? 'c6' : 'c12');
    if (goals) grid.append(goals);
    if (w.plan) grid.append(card('Plan for the week', h('div', { class: 'plan' }, w.plan), { cls: 'c6' }));
    nodes.push(grid);
    return nodes;
  }

  // ---------- Top ----------
  const TOP_RANGES = [['range:today', 'Today'], ['days:7', '7 days'], ['range:week', 'This week'], ['range:month', 'This month'], ['days:30', '30 days']];
  function smallSeg(options, current, onPick, label) {
    return h('div', { class: 'seg small', role: 'tablist', 'aria-label': label },
      options.map(([value, text]) => h('button', {
        role: 'tab', 'aria-selected': String(value === current),
        onpointerdown: (e) => { if (e.button === 0 && value !== current) onPick(value); },
        onclick: (e) => { if (e.detail === 0 && value !== current) onPick(value); },
      }, text)));
  }

  function renderTop(t) {
    const range = `${shortDate(t.from)}${t.from === t.to ? '' : ` – ${shortDate(t.to)}`}`;
    const nodes = [h('div', { class: 'toolrow' },
      h('div', null, h('h1', null, 'Where the time went'), h('p', { class: 'sub', style: { margin: 0 } }, `${range} · ${dur(t.totalMS)} tracked`)),
      smallSeg(TOP_RANGES, state.topRange, (v) => { state.topRange = v; localSet('tock-top', v); load({}); }, 'Range'))];
    if (!t.totalMS) {
      nodes.push(emptyState('Nothing tracked in this range', 'Pick a longer range.'));
      return nodes;
    }
    const kindOfCat = {};
    for (const c of t.sections.category) kindOfCat[c.name] = c.kind;
    const byMostly = (r) => r.kind || kindOfCat[r.mostly] || '';
    const grid = h('div', { class: 'grid' });
    const sec = (head, rows, opts) => grid.append(card(null, ledger(rows, { head, total: t.totalMS, limit: 12, ...opts }), { cls: 'c6' }));
    sec('Category', t.sections.category, {});
    sec('Project', t.sections.project, { empty: 'No project time in this range' });
    sec('App', t.sections.app, { kindOf: byMostly });
    sec('Site', t.sections.site, { kindOf: byMostly, empty: 'No browser time with a URL' });
    nodes.push(grid);
    return nodes;
  }

  // ---------- Inbox ----------
  function renderInbox(d) {
    const nodes = [h('div', { class: 'toolrow' },
      h('div', null, h('h1', null, 'Inbox'),
        h('p', { class: 'sub', style: { margin: 0 } }, (d.items.length ? `${d.items.length} unlabelled · ${dur(d.totalMS)}` : `Last ${d.days} days`) + (d.smallKeys ? ` · ${d.smallKeys} small (${dur(d.smallMS)}) hidden` : ''))),
      smallSeg([[7, '7 days'], [30, '30 days'], [90, '90 days']], state.inboxDays, (v) => { state.inboxDays = v; localSet('tock-inbox-days', String(v)); state.sel = -1; load({}); }, 'Days'))];
    if (!d.items.length) {
      nodes.push(d.firstRun
        ? emptyState('Nothing recorded yet', 'Time no rule covers lands here, to label once.')
        : emptyState('All labelled', `Everything in the last ${d.days} days has a label.`));
      return nodes;
    }
    const list = h('div', { class: 'inbox', role: 'list' });
    d.items.forEach((it, i) => {
      const sg = it.suggestion;
      list.append(h('div', { class: `item ${i === state.sel ? 'sel' : ''}`, role: 'listitem', data: { i: String(i) } },
        h('div', { class: 'main' },
          h('div', { class: 'key', title: it.key }, it.key),
          it.example && !it.private ? h('div', { class: 'ex', title: it.example }, it.example) : null,
          h('div', { class: 'meta' },
            h('span', { class: 'cap' }, `${it.spans} ${it.spans === 1 ? 'span' : 'spans'}`),
            sg ? h('span', { class: `chip ${kindClass(kindOfLabel(sg.label))}` }, `→ ${sg.label.replace('/', ' / ')}`, sg.reason ? h('span', { class: 'src' }, `· ${sg.reason} ${Math.round(sg.share * 100)}%`) : h('span', { class: 'src' }, '· Claude')) : null)),
        h('div', { class: 'acts-r' },
          h('span', { class: 'dur' }, dur(it.ms)),
          sg ? h('button', { class: 'btn primary', onclick: () => accept(it, i) }, 'Accept') : null,
          h('button', { class: 'btn', onclick: (e) => openItemSheet(it, e.currentTarget) }, 'Label…'),
          h('button', { class: 'btn ghost', onclick: () => ignore(it, i), 'aria-label': `Ignore ${it.key}` }, 'Ignore'))));
    });
    nodes.push(list);
    nodes.push(h('div', { class: 'keys' }, h('kbd', null, 'j'), ' ', h('kbd', null, 'k'), ' move · ', h('kbd', null, 'a'), ' accept · ', h('kbd', null, 'l'), ' label · ', h('kbd', null, 'i'), ' ignore'));
    return nodes;
  }

  function kindOfLabel(label) {
    const cat = label.split('/')[0];
    const c = state.labels && state.labels.categories.find((x) => x.name === cat);
    return c ? c.kind : 'neutral';
  }

  function fadeItem(i) {
    const el = main.querySelector(`.item[data-i="${i}"]`);
    if (el) el.classList.add('gone');
    return new Promise((r) => setTimeout(r, reduceMotion.matches ? 0 : 200));
  }

  // accept learns the suggestion's first safe rule, as tock tag <n> does;
  // with no safe rule it labels these spans only.
  async function accept(it, i) {
    try {
      const p = await api(`/api/propose?key=${encodeURIComponent(it.key)}&label=${encodeURIComponent(it.suggestion.label)}`);
      let res;
      if (p.candidates.length) {
        res = await api('/api/learn', { key: it.key, label: it.suggestion.label, rule: p.candidates[0].rule });
        toast(`Learned: ${res.label} now covers ${dur(res.ms)}`);
      } else {
        res = await api('/api/tag', { key: it.key, label: it.suggestion.label });
        toast(`Labelled ${res.spans} ${res.spans === 1 ? 'span' : 'spans'} ${res.label} (no safe rule)`);
      }
      await fadeItem(i);
      afterChange();
    } catch (e) { toast(e.message); }
  }

  async function ignore(it, i) {
    try {
      await api('/api/ignore', { key: it.key });
      await fadeItem(i);
      afterChange();
      toast('Hidden from the inbox', 'Undo', async () => {
        await api('/api/ignore', { key: it.key, undo: true });
        afterChange();
      });
    } catch (e) { toast(e.message); }
  }

  async function undo() {
    const res = await api('/api/undo', {});
    toast(res.what ? `Undid: ${res.what}` : 'Nothing to undo');
    afterChange();
  }

  function afterChange() {
    load({ quiet: true });
    if (state.view !== 'inbox') refreshBadge();
  }

  // ---------- label sheet ----------
  const sheet = $('sheet');
  const scrim = $('scrim');
  let sheetOpen = false;
  let sheetReturn = null;
  const sheetSpring = new Spring(0, {
    damping: 1, response: 0.34, precision: 0.002,
    onUpdate: (p) => {
      scrim.style.setProperty('opacity', String(Math.max(0, Math.min(1, p))));
      sheet.style.setProperty('opacity', String(Math.max(0, Math.min(1, p))));
      const t = narrow.matches ? `translateY(${(1 - p) * 100}%)` : `translate(-50%, calc(-50% + ${(1 - p) * 18}px)) scale(${0.965 + 0.035 * p})`;
      sheet.style.setProperty('transform', t);
    },
    onRest: (p) => {
      if (p === 0 && !sheetOpen) { sheet.hidden = true; scrim.hidden = true; }
    },
  });

  function showSheet(content, from) {
    sheet.replaceChildren(...content.filter(Boolean));
    if (!sheetOpen) {
      sheetReturn = document.activeElement;
      if (sheetSpring.value === 0) sheetSpring.onUpdate(0); // no first-frame flash
    }
    sheetOpen = true;
    sheet.hidden = false;
    scrim.hidden = false;
    sheetSpring.set(1);
    const f = sheet.querySelector('[data-autofocus]') || sheet.querySelector('button');
    if (f) f.focus({ preventScroll: true });
  }
  function closeSheet() {
    if (!sheetOpen) return;
    sheetOpen = false;
    sheetSpring.set(0);
    if (sheetReturn && document.contains(sheetReturn)) sheetReturn.focus({ preventScroll: true });
  }
  scrim.addEventListener('pointerdown', closeSheet);
  sheet.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') { e.preventDefault(); closeSheet(); }
    if (e.key === 'Tab') {
      const f = [...sheet.querySelectorAll('button:not(:disabled)')];
      if (!f.length) return;
      if (e.shiftKey && document.activeElement === f[0]) { e.preventDefault(); f[f.length - 1].focus(); }
      else if (!e.shiftKey && document.activeElement === f[f.length - 1]) { e.preventDefault(); f[0].focus(); }
    }
  });

  // A target is what the sheet labels: an inbox key, or a timeline row
  // (which can also label every span with its key, or learn a rule).
  function openItemSheet(it, from) {
    labelSheet({ key: it.key, title: it.key, ctx: `${it.spans} ${it.spans === 1 ? 'span' : 'spans'} · ${dur(it.ms)} in the last ${state.inboxDays} days`, preset: it.suggestion && it.suggestion.label }, from);
  }
  function openRowSheet(r, from) {
    const now = r.category ? (r.project ? `${r.category} / ${r.project}` : r.category) : 'unlabelled';
    labelSheet({
      key: r.key, ids: r.ids, title: r.text || r.app, range: { from: r.start, to: r.end },
      ctx: `${r.app} · ${clock(r.start)}–${r.open ? 'now' : clock(r.end)} · ${dur(r.end - r.start)} · now ${now}${r.source === 'learned' ? ' by a learned rule' : ''}`,
      preset: r.category ? (r.project ? `${r.category}/${r.project}` : r.category) : '',
    }, from);
  }

  async function labelSheet(target, from) {
    const labels = await loadLabels();
    if (!labels) return;
    let cat = '';
    let proj = '';
    if (target.preset) [cat, proj = ''] = target.preset.split('/');
    const label = () => (cat ? (proj ? `${cat}/${proj}` : cat) : proj ? `/${proj}` : '');
    const err = h('div', { class: 'err-text', role: 'alert' });
    const buttons = [];
    const sync = () => {
      for (const b of sheet.querySelectorAll('.pick[data-cat]')) b.setAttribute('aria-pressed', String(b.dataset.cat === cat));
      for (const b of sheet.querySelectorAll('.pick[data-proj]')) b.setAttribute('aria-pressed', String(b.dataset.proj === proj));
      for (const b of buttons) b.disabled = !label();
      err.textContent = '';
    };
    const catPicks = h('div', { class: 'picks', role: 'group', 'aria-label': 'Category' },
      labels.categories.map((c, i) => h('button', {
        class: `pick ${kindClass(c.kind)}`, data: { cat: c.name }, 'aria-pressed': 'false', 'data-autofocus': i === 0 ? '' : undefined,
        onclick: () => { cat = cat === c.name ? '' : c.name; sync(); },
      }, h('i', { class: `dot ${kindClass(c.kind)}` }), c.name)));
    const projPicks = labels.projects.length ? h('div', { class: 'picks', role: 'group', 'aria-label': 'Project' },
      labels.projects.map((p) => h('button', {
        class: 'pick', data: { proj: p }, 'aria-pressed': 'false',
        onclick: () => { proj = proj === p ? '' : p; sync(); },
      }, p))) : null;

    const run = async (fn) => {
      for (const b of buttons) b.disabled = true;
      try { await fn(); } catch (e) { err.textContent = e.message; sync(); }
    };
    const once = (body, what) => run(async () => {
      const res = await api('/api/tag', body);
      closeSheet();
      toast(`Labelled ${what || `${res.spans} ${res.spans === 1 ? 'span' : 'spans'}`} → ${res.label}`);
      afterChange();
    });
    const mk = (text, cls, fn) => { const b = h('button', { class: `btn lg ${cls}`, onclick: fn }, text); buttons.push(b); return b; };
    const cut = () => run(async () => {
      await api('/api/edit', { kind: 'cut', from: target.range.from, to: target.range.to });
      closeSheet();
      toast(`Deleted ${clock(target.range.from)}–${clock(target.range.to)}`, 'Undo', undo);
      afterChange();
    });
    const foot = h('div', { class: 'foot' },
      h('button', { class: 'btn lg ghost', onclick: closeSheet }, 'Cancel'),
      target.range ? h('button', { class: 'btn lg ghost', onclick: cut, title: 'Remove this time from the day (u undoes it)' }, 'Delete time') : null,
      h('span', { class: 'spacer' }),
      target.ids ? mk('Just this block', '', () => once({ ids: target.ids, label: label() }, 'this block')) : null,
      target.key ? mk(target.ids ? 'All like it' : 'Just these', '', () => once({ key: target.key, label: label() })) : null,
      target.key ? mk('Make rule…', 'primary', () => run(() => ruleStep(target, label(), from))) : null);
    showSheet([
      h('h3', { id: 'sheet-title' }, target.title),
      h('div', { class: 'ctx' }, target.ctx),
      h('div', { class: 'group' }, h('span', { class: 'cap' }, 'Category'), catPicks),
      projPicks ? h('div', { class: 'group' }, h('span', { class: 'cap' }, 'Project (optional)'), projPicks) : null,
      target.ids ? h('div', { class: 'note' }, '“All like it” labels every span with this key so far; a rule also labels future ones.') : null,
      err, foot,
    ].filter(Boolean), from);
    sync();
  }

  async function ruleStep(target, label, from) {
    const p = await api(`/api/propose?key=${encodeURIComponent(target.key)}&label=${encodeURIComponent(label)}`);
    let pick = 0;
    const err = h('div', { class: 'err-text', role: 'alert' });
    const back = () => labelSheet({ ...target, preset: label }, from);
    if (!p.candidates.length) {
      showSheet([
        h('h3', { id: 'sheet-title' }, 'No safe rule'),
        h('div', { class: 'ctx' }, `Every rule that matches ${target.key} would also relabel time other rules already label. You can still label these spans.`),
        h('div', { class: 'foot' },
          h('button', { class: 'btn lg ghost', onclick: back }, 'Back'),
          h('span', { class: 'spacer' }),
          h('button', {
            class: 'btn lg primary', 'data-autofocus': '', onclick: async () => {
              try {
                const res = await api('/api/tag', { key: target.key, label });
                closeSheet();
                toast(`Labelled ${res.spans} spans → ${res.label}`);
                afterChange();
              } catch (e) { err.textContent = e.message; }
            },
          }, `Label ${p.spans} spans`)),
        err,
      ], from);
      return;
    }
    const cands = h('div', { class: 'cands', role: 'radiogroup', 'aria-label': 'Rule' });
    const sync = () => { for (const b of cands.children) b.setAttribute('aria-pressed', String(Number(b.dataset.i) === pick)); };
    p.candidates.forEach((c, i) => {
      const extra = [];
      if (c.new) extra.push(`${c.new} more unlabelled (${dur(c.newMS)})`);
      if (c.fixed) extra.push(`${c.fixed} fixed${c.replaces ? `, replaces ${c.replaces} learned ${c.replaces === 1 ? 'rule' : 'rules'}` : ''}`);
      cands.append(h('button', { class: 'cand', data: { i: String(i) }, role: 'radio', onclick: () => { pick = i; sync(); }, ondblclick: () => save() },
        h('code', null, c.rule),
        h('span', { class: 'cap' }, `${i === 0 ? 'Broadest · ' : i === p.candidates.length - 1 && i > 0 ? 'Most specific · ' : ''}matches ${c.matches} spans (${dur(c.matchMS)})${extra.length ? ` · ${extra.join(' · ')}` : ''}`)));
    });
    const save = async () => {
      try {
        const res = await api('/api/learn', { key: target.key, label, rule: p.candidates[pick].rule });
        closeSheet();
        toast(`Learned: ${res.label} now covers ${dur(res.ms)}`);
        afterChange();
      } catch (e) { err.textContent = e.message; }
    };
    showSheet([
      h('h3', { id: 'sheet-title' }, `Rule for ${p.label}`),
      h('div', { class: 'ctx' }, `Pick how broad the rule is. It labels these ${p.spans} spans (${dur(p.ms)}) and anything like them from now on. Saved to learned.toml.`),
      cands, err,
      h('div', { class: 'foot' },
        h('button', { class: 'btn lg ghost', onclick: back }, 'Back'),
        h('span', { class: 'spacer' }),
        h('button', { class: 'btn lg primary', 'data-autofocus': '', onclick: save }, 'Save rule')),
    ], from);
    sync();
  }

  // ---------- tooltip & toast ----------
  const tip = $('tip');
  function showTip(content, x, y) {
    tip.replaceChildren(...content.filter(Boolean));
    tip.hidden = false;
    const w = tip.offsetWidth;
    const hgt = tip.offsetHeight;
    tip.style.setProperty('left', `${Math.min(innerWidth - w - 8, Math.max(8, x - w / 2))}px`);
    tip.style.setProperty('top', `${y - hgt - 14 < 8 ? y + 18 : y - hgt - 14}px`);
  }
  function hideTip() { tip.hidden = true; }

  const toastEl = $('toast');
  let toastTimer = 0;
  function toast(text, action, onAction) {
    clearTimeout(toastTimer);
    toastEl.replaceChildren(h('span', null, text));
    if (action) toastEl.append(h('button', { class: 'btn', onclick: () => { hideToast(); onAction().catch((e) => toast(e.message)); } }, action));
    toastEl.hidden = false;
    toastEl.animate && !reduceMotion.matches && toastEl.animate([{ opacity: 0, transform: 'translate(-50%, 12px)' }, { opacity: 1, transform: 'translate(-50%, 0)' }], { duration: 260, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' });
    toastTimer = setTimeout(hideToast, action ? 6000 : 3500);
  }
  function hideToast() { toastEl.hidden = true; }

  // ---------- keyboard ----------
  addEventListener('keydown', (e) => {
    if (sheetOpen || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.target.closest && e.target.closest('input, textarea')) return;
    const views = ['today', 'week', 'top', 'inbox'];
    if (e.key >= '1' && e.key <= '4') { go(views[Number(e.key) - 1]); return; }
    if (e.key === 'ArrowLeft') { step(-1); return; }
    if (e.key === 'ArrowRight') { step(1); return; }
    if (e.key === 'r') { load({ quiet: true }); return; }
    if (e.key === 'u') { undo().catch((err) => toast(err.message)); return; }
    if (state.view !== 'inbox' || !state.data || !state.data.items) return;
    const items = state.data.items;
    if (!items.length) return;
    const move = (n) => {
      state.sel = Math.max(0, Math.min(items.length - 1, (state.sel < 0 ? (n > 0 ? -1 : items.length) : state.sel) + n));
      for (const el of main.querySelectorAll('.item')) el.classList.toggle('sel', Number(el.dataset.i) === state.sel);
      const el = main.querySelector(`.item[data-i="${state.sel}"]`);
      if (el) el.scrollIntoView({ block: 'nearest', behavior: reduceMotion.matches ? 'auto' : 'smooth' });
    };
    const it = items[state.sel];
    switch (e.key) {
      case 'j': case 'ArrowDown': e.preventDefault(); move(1); break;
      case 'k': case 'ArrowUp': e.preventDefault(); move(-1); break;
      case 'a': if (it && it.suggestion) accept(it, state.sel); break;
      case 'l': case 'Enter': if (it) openItemSheet(it, main.querySelector(`.item[data-i="${state.sel}"]`)); break;
      case 'i': if (it) ignore(it, state.sel); break;
      default:
    }
  });

  // ---------- chrome & refresh ----------
  const bar = $('bar');
  addEventListener('scroll', () => bar.classList.toggle('scrolled', scrollY > 4), { passive: true });

  // Today follows the clock: refresh every minute while visible, and when
  // the tab comes back.
  setInterval(() => {
    if (document.visibilityState !== 'visible') return;
    loadStatus();
    if (!sheetOpen && state.view === 'today' && state.date === todayStr()) load({ quiet: true });
  }, 60_000);
  setInterval(() => { if (document.visibilityState === 'visible') renderSession(); }, 15_000);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible' && !sheetOpen && token) { load({ quiet: true }); refreshBadge(); }
  });

  const VIEWS = { today: renderToday, week: renderWeek, top: renderTop, inbox: renderInbox };

  syncChrome();
  if (token) {
    loadLabels().then(() => { load({ enter: true }); refreshBadge(); loadStatus(); });
  } else {
    renderNoToken();
  }
})();
