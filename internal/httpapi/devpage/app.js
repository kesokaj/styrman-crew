// styrman dev console — minimal Phase 1 test page. Deleted when the real UI
// lands (Phase 2). Implements the terminal protocol: binary frames = PTY
// bytes, JSON text = resize, close 4000 = exited, 4001 = replaced (never
// auto-reconnect on 4001).
'use strict';

const $ = (sel) => document.querySelector(sel);
let term = null, fit = null, ws = null;
let current = null; // {project, conv}

async function api(path, opts = {}) {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts });
  if (res.status === 204) return null;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error?.message || res.statusText);
  return body;
}

async function refresh() {
  const { projects } = await api('/api/projects');
  const el = $('#projects');
  el.innerHTML = '';
  for (const p of projects) {
    const div = document.createElement('div');
    div.className = 'proj' + (current?.project === p.name ? ' active' : '');
    const label = document.createElement('span');
    label.textContent = `${p.name} (${p.agent})`;
    const state = document.createElement('span');
    state.className = `state ${p.state}`;
    state.textContent = p.state;
    div.append(label, state);
    div.onclick = () => openProject(p.name);
    el.appendChild(div);
    if (current?.project === p.name) {
      const convs = document.createElement('div');
      convs.className = 'convs';
      convs.id = 'convs';
      el.appendChild(convs);
      loadConvs(p.name, convs);
    }
  }
}

async function loadConvs(project, el) {
  const { conversations } = await api(`/api/projects/${project}/conversations`);
  el.innerHTML = '';
  for (const c of conversations) {
    const d = document.createElement('div');
    d.className = 'conv' + (current?.conv === c.id ? ' active' : '');
    d.style.display = 'flex';
    d.style.justifyContent = 'space-between';
    const label = document.createElement('span');
    label.textContent = `conversation ${c.id}` + (c.attached ? ' •' : '');
    const kill = document.createElement('span');
    kill.textContent = '✕';
    kill.title = 'kill this conversation (ends whatever runs in it)';
    kill.style.color = '#c66';
    kill.onclick = async (e) => {
      e.stopPropagation();
      if (!confirm(`Kill conversation ${c.id}? This ends the session inside it.`)) return;
      await api(`/api/projects/${project}/conversations/${c.id}`, { method: 'DELETE' });
      refresh();
    };
    d.append(label, kill);
    d.onclick = (e) => { e.stopPropagation(); connect(project, c.id); };
    el.appendChild(d);
  }
  const add = document.createElement('div');
  add.className = 'conv';
  add.style.color = '#69c';
  add.textContent = '+ new conversation';
  add.onclick = async (e) => {
    e.stopPropagation();
    const { id } = await api(`/api/projects/${project}/conversations`, { method: 'POST' });
    connect(project, id);
  };
  el.appendChild(add);
}

async function openProject(name) {
  const { conversations } = await api(`/api/projects/${name}/conversations`);
  const id = conversations[0]?.id || (await api(`/api/projects/${name}/conversations`, { method: 'POST' })).id;
  connect(name, id);
}

function ensureTerm() {
  if (term) return;
  term = new Terminal({ fontSize: 14, theme: { background: '#000' }, cursorBlink: true, scrollback: 5000 });
  fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open($('#term'));
  window.addEventListener('resize', () => {
    if (!fit) return;
    fit.fit();
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
  });
  term.onData((d) => { if (ws?.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(d)); });
}

function connect(project, conv) {
  ensureTerm();
  if (ws) { ws.onclose = null; ws.close(); }
  current = { project, conv };
  fit.fit();
  $('#status').textContent = `${project} / conversation ${conv} — connecting…`;
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws/term/${project}/${conv}?cols=${term.cols}&rows=${term.rows}`);
  ws.binaryType = 'arraybuffer';
  ws.onopen = () => { $('#status').textContent = `${project} / conversation ${conv} — connected`; term.focus(); refresh(); };
  ws.onmessage = (ev) => term.write(new Uint8Array(ev.data));
  ws.onclose = (ev) => {
    if (ev.code === 4001) { $('#status').textContent = `${project} / ${conv} — replaced by another tab`; return; }
    if (ev.code === 4000) { $('#status').textContent = `${project} / ${conv} — session exited`; refresh(); return; }
    $('#status').textContent = `${project} / ${conv} — disconnected (${ev.code}), reconnecting…`;
    setTimeout(() => { if (current?.project === project && current?.conv === conv) connect(project, conv); }, 1500);
  };
}

$('#newProject').onsubmit = async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  const btn = e.target.querySelector('button');
  btn.disabled = true; btn.textContent = 'creating (boots a microVM)…';
  try {
    const p = await api('/api/projects', {
      method: 'POST',
      body: JSON.stringify({ name: f.get('name'), agent: f.get('agent'), gitInit: f.get('gitInit') === 'on' }),
    });
    e.target.reset();
    await refresh();
    openProject(p.name);
  } catch (err) {
    alert(err.message);
  } finally {
    btn.disabled = false; btn.textContent = 'New Project';
  }
};

// ── First-time setup: hidden system microVM running styrman-setup ──────────
async function refreshSetup() {
  try {
    const st = await api('/api/setup/status');
    const banner = $('#setupBanner');
    if (st.done) { banner.style.display = 'none'; return; }
    banner.style.display = 'block';
    const mark = (ok, label) => `${ok ? '✔' : '✗'} ${label}`;
    $('#setupItems').textContent = [
      mark(st.gitIdentity, 'git'), mark(st.sshKey, 'ssh'), mark(st.github, 'gh'),
      mark(st.claude, 'claude'), mark(st.keyring, 'keyring'),
    ].join('  ');
  } catch { /* server restarting; retry on next tick */ }
}

function connectSetup() {
  ensureTerm();
  if (ws) { ws.onclose = null; ws.close(); }
  current = { setup: true };
  fit.fit();
  $('#status').textContent = 'setup — booting hidden VM…';
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws/setup?cols=${term.cols}&rows=${term.rows}`);
  ws.binaryType = 'arraybuffer';
  ws.onopen = () => { $('#status').textContent = 'setup — follow the steps in the terminal'; term.focus(); };
  ws.onmessage = (ev) => term.write(new Uint8Array(ev.data));
  ws.onclose = (ev) => {
    if (ev.code === 4001) { $('#status').textContent = 'setup — opened in another tab'; return; }
    $('#status').textContent = 'setup — session ended';
    refreshSetup();
  };
}

$('#setupOpen').onclick = async () => {
  $('#setupOpen').disabled = true;
  $('#setupOpen').textContent = 'booting setup VM…';
  try { await api('/api/setup/start', { method: 'POST' }); connectSetup(); }
  catch (e) { alert(e.message); }
  finally { $('#setupOpen').disabled = false; $('#setupOpen').textContent = 'Open setup'; }
};

$('#setupDone').onclick = async () => {
  if (!confirm('Mark setup as done? The hidden setup VM is removed; your sign-ins stay in the shared home.')) return;
  await api('/api/setup/done', { method: 'POST' });
  if (current?.setup && ws) { ws.onclose = null; ws.close(); $('#status').textContent = 'setup complete'; }
  refreshSetup();
};

refresh();
refreshSetup();
setInterval(refresh, 5000);
setInterval(refreshSetup, 10000);
