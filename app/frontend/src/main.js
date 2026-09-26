/* ==========================================================================
   frp 客户端 · 界面逻辑
   纯原生 JS，无构建步骤。与 Go 后端通过 Wails 绑定通信：
     window.go.main.App.方法名(参数…)  →  Promise
   后端方法签名见 app.go / export.go。
   ========================================================================== */
'use strict';

/* ============================== 小工具 ============================== */

const $ = (id) => document.getElementById(id);

/** HTML 转义。所有来自后端的文本都要经过它，避免代理名里有 < > 时把结构冲掉。 */
function esc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

/** 内联 SVG 图标 */
function svg(path, size = 15, w = 2) {
  return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" ` +
    `stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round">${path}</svg>`;
}

/**
 * 实心图标：传进来的是 <path> 的 d 数据，这里负责套上 <path> 元素。
 *
 * ⚠ 这里犯过一次错：早期直接把 d 数据当成 svg 的子节点插进去，
 *   结果它变成了纯文本，图形完全不显示（GitHub 图标"凭空消失"就是这么来的）。
 *   `I` 里的图标常量自带 <path> 标签，所以那套走 svg()；品牌图标只有 d 数据，走这里。
 */
function svgFill(d, size = 15) {
  return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="currentColor">` +
    `<path d="${d}"/></svg>`;
}

const I = {
  ok: '<path d="M20 6 9 17l-5-5"/>',
  err: '<circle cx="12" cy="12" r="9"/><path d="M12 8v4M12 16h.01"/>',
  info: '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/>',
  warn: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
  edit: '<path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  lock: '<rect x="4" y="10" width="16" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  chevron: '<path d="m9 6 6 6-6 6"/>',
  play: '<path d="M5 3l14 9-14 9V3z"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="1.5"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
  mail: '<rect x="2" y="4" width="20" height="16" rx="2.5"/><path d="m2.5 7.5 9.5 6 9.5-6"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a15 15 0 0 1 0 18 15 15 0 0 1 0-18z"/>',
  repo: '<path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H20v14.5H6.5A2.5 2.5 0 0 0 4 20z"/><path d="M4 20a2.5 2.5 0 0 1 2.5-2.5H20"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"/>',
  coffee: '<path d="M4 8h13v5a5 5 0 0 1-5 5H9a5 5 0 0 1-5-5z"/><path d="M17 9h1.8a2.7 2.7 0 0 1 0 5.4H17"/><path d="M6 3.5c0 .7.7.9.7 1.6M9.5 3c0 .8.8 1 .8 1.8M13 3.5c0 .7.7.9.7 1.6"/>',
  external: '<path d="M14 4h6v6"/><path d="M20 4 10 14"/><path d="M18 14v5a1.5 1.5 0 0 1-1.5 1.5H5A1.5 1.5 0 0 1 3.5 19V7.5A1.5 1.5 0 0 1 5 6h5"/>',
};

// 品牌图标用 Simple Icons 的官方路径（viewBox 都是 0 0 24 24，实心填充）。
// ⚠ 之前那版 GitHub 路径是凭印象写的，结果整个图形不显示 —— 换成官方路径。
//    来源：https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/<name>.svg
const BRAND_PATHS = {
  github: 'M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12',
  gitee: 'M11.984 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.016 0zm6.09 5.333c.328 0 .593.266.592.593v1.482a.594.594 0 0 1-.593.592H9.777c-.982 0-1.778.796-1.778 1.778v5.63c0 .327.266.592.593.592h5.63c.982 0 1.778-.796 1.778-1.778v-.296a.593.593 0 0 0-.592-.593h-4.15a.592.592 0 0 1-.592-.592v-1.482a.593.593 0 0 1 .593-.592h6.815c.327 0 .593.265.593.592v3.408a4 4 0 0 1-4 4H5.926a.593.593 0 0 1-.593-.593V9.778a4.444 4.444 0 0 1 4.445-4.444h8.296Z',
};

/* ============================== 状态 ============================== */

const S = {
  ready: false,
  page: 'proxies',
  state: null,          // AppState
  config: null,         // ConnConfig（前端持有的完整副本，保存时整体回传）
  proxies: [],
  visitors: [],
  logs: [],             // 内存日志（上限 2000）
  lastSeq: 0,
  paused: false,
  autoscroll: true,
  proxySig: '',
  editName: '',         // 正在编辑的代理名，'' 表示新建
  editVisName: '',
  pluginValues: {},     // 插件参数暂存
  settings: { closeToTray: true, autoRestart: true },
  portCheck: {},        // 端口占用检查结果缓存
};

/* ============================== 后端调用 ============================== */

function app() {
  const a = window.go && window.go.main && window.go.main.App;
  if (!a) throw new Error('后端未就绪，请通过程序启动界面');
  return a;
}

async function call(method, ...args) {
  const fn = app()[method];
  if (typeof fn !== 'function') throw new Error('后端不支持的操作：' + method);
  try {
    return await fn(...args);
  } catch (e) {
    throw new Error(cleanErr(e));
  }
}

function cleanErr(e) {
  let m = (e && e.message) ? e.message : String(e);
  m = m.replace(/^Error:\s*/, '').replace(/^"|"$/g, '');
  return m || '未知错误';
}

/* ============================== 轻提示 ============================== */

function toast(title, desc, type = 'ok') {
  const box = document.createElement('div');
  box.className = 'toast ' + type;
  const ic = svg(I[type] || I.info, 15, 2.2);
  box.innerHTML = ic + '<div class="toast-body"><div class="toast-title"></div>' +
    (desc ? '<div class="toast-desc"></div>' : '') + '</div>';
  box.querySelector('.toast-title').textContent = title;
  if (desc) box.querySelector('.toast-desc').textContent = desc;
  $('toasts').appendChild(box);
  setTimeout(() => {
    box.classList.add('out');
    setTimeout(() => box.remove(), 220);
  }, 4600);
}

const ok = (t, d) => toast(t, d, 'ok');
const err = (t, d) => toast(t, d, 'err');

/* ============================== 弹窗 ============================== */

let confirmResolve = null;

function openModal(id) { $(id).classList.remove('hidden'); }

function closeModal(id) {
  const n = $(id);
  if (!n) return;
  n.classList.add('hidden');
  if (id === 'confirm-modal' && confirmResolve) {
    confirmResolve(false);
    confirmResolve = null;
  }
}

function confirmDialog(opt) {
  $('cf-title').textContent = opt.title || '确认';
  $('cf-text').textContent = opt.text || '';
  const h = $('cf-hint');
  if (opt.hint) { h.textContent = opt.hint; h.classList.remove('hidden'); }
  else { h.textContent = ''; h.classList.add('hidden'); }
  const b = $('cf-ok');
  b.textContent = opt.okText || '确定';
  b.className = 'btn ' + (opt.danger ? 'btn-danger' : 'btn-primary');
  openModal('confirm-modal');
  return new Promise((res) => { confirmResolve = res; });
}

/* ============================== 页面切换 ============================== */

function showPage(name, push) {
  S.page = name;
  document.querySelectorAll('.nav-item').forEach((b) => {
    b.classList.toggle('active', b.dataset.page === name);
  });
  document.querySelectorAll('.page').forEach((p) => {
    p.classList.toggle('hidden', p.id !== 'page-' + name);
  });
  if (push !== false) history.replaceState(null, '', '#' + name);
  if (name === 'logs') scrollLogsToEnd();
}

/* ============================== 顶栏渲染 ============================== */

const STATE_DOT = { stopped: 'off', starting: 'warn', connected: 'on', failed: 'err' };

function renderTopbar() {
  const st = S.state;
  if (!st) return;

  const dot = $('conn-dot');

  // 外部进程的场景要单独说清楚：
  // frp 确实在跑、代理也确实在线，但连接状态我们无从得知
  // （日志在别人手里），所以只陈述事实，不假装知道。
  let text, dotCls, pulse = false;
  if (st.externalRunning) {
    text = '外部进程运行中';
    dotCls = 'warn';
  } else {
    text = st.stateText || '未运行';
    dotCls = STATE_DOT[st.state] || 'off';
    pulse = st.state === 'starting' || st.state === 'connected';
    // 卡在"正在连接"时给个秒数，好过无限转圈让人以为程序死了
    if (st.state === 'starting' && st.startingSeconds >= 5) {
      text = `正在连接…（已 ${st.startingSeconds} 秒）`;
    }
    if (st.state === 'starting' && st.startingSeconds >= 20) {
      dotCls = 'warn';
    }
  }
  dot.className = 'dot ' + dotCls + (pulse ? ' pulse' : '');

  $('conn-text').textContent = text;
  $('brand-sub').textContent = text;

  $('meta-server').textContent = st.serverAddr
    ? `${st.serverAddr}:${st.serverPort || 7000}` : '—';
  $('meta-user').textContent = st.user || '—';

  $('sidebar-foot').innerHTML = st.user
    ? '登录身份：<b>' + esc(st.user) + '</b>' : '未配置身份';

  // 运行控制按钮
  //
  // ⚠ externalRunning：本机有一个 frpc 在跑，但不是本程序拉起的。
  //    对它仍然可以"停止"（走管理接口），但不能"重启"。
  const running = st.processRunning || st.externalRunning;
  $('toggle-run-text').textContent = running ? '停止' : '启动';
  $('toggle-run-icon').innerHTML = running ? I.stop : I.play;
  $('btn-toggle-run').className = 'btn btn-sm ' + (running ? 'btn-ghost' : 'btn-primary');
  $('btn-restart').disabled = !st.processRunning;

  $('cfg-path-hint').textContent = st.configPath ? '配置文件：' + st.configPath : '';
}

/** 界面能否修改代理：自己的进程在跑，或者能连上外部 frpc 的管理接口。 */
function canEdit() {
  const st = S.state || {};
  return !!(st.processRunning || st.externalRunning);
}

/* ============================== 提示条（代理页） ============================== */

function renderNotices() {
  const st = S.state;
  const box = $('proxy-notices');
  if (!st) { box.innerHTML = ''; return; }
  let h = '';

  if (st.missingExe) {
    h += `<div class="notice err">${svg(I.err, 16)}
      <div class="notice-body"><b>找不到 frpc.exe</b>
      <div style="margin-top:2px;opacity:.9">请把官方下载的 frpc.exe 放到本程序所在的文件夹，然后重启本程序。</div>
      <div class="hint" style="color:inherit;opacity:.75;margin-top:4px">期望位置：<span class="mono">${esc(st.exePath || '')}</span></div>
      <div class="notice-actions"><button class="btn btn-sm btn-ghost" style="background:transparent" id="notice-open-folder">打开程序文件夹</button></div></div></div>`;
  } else if (!st.configured) {
    h += `<div class="notice warn">${svg(I.warn, 16)}
      <div class="notice-body"><b>还没有完成连接配置</b>
      <div style="margin-top:2px;opacity:.9">填写服务器地址后，frp 才能连接服务端。服务端若未开启认证，令牌留空即可。</div>
      <div class="notice-actions"><button class="btn btn-sm btn-primary" id="notice-open-wizard">打开配置向导</button></div></div></div>`;
  } else if (st.state === 'failed') {
    h += `<div class="notice err">${svg(I.err, 16)}
      <div class="notice-body"><b>连接服务端失败</b>
      <div style="margin-top:2px;opacity:.9">${esc(st.stateError || '请检查服务器地址、端口和认证令牌是否正确。')}</div>
      <div class="notice-actions">
        <button class="btn btn-sm btn-ghost" style="background:transparent" id="notice-diagnose">连接诊断</button>
        <button class="btn btn-sm btn-ghost" style="background:transparent" id="notice-open-settings">检查设置</button>
      </div></div></div>`;
  }

  if (st.externalRunning) {
    h += `<div class="notice info">${svg(I.info, 16)}
      <div class="notice-body">
        <b>检测到本机已有另一个 frp 进程在运行</b>
        <div style="margin-top:2px;opacity:.9">
          管理端口 <span class="mono">${esc((S.config && S.config.webAddr) || '127.0.0.1')}:${esc(String((S.config && S.config.webPort) || 7400))}</span>
          已经有一个 frpc 在响应，但它不是本程序启动的（可能是上次异常退出留下的，或你另外运行的）。
          为避免冲突，本程序不会再去启动第二个。
        </div>
        <div style="margin-top:3px;opacity:.9">
          代理仍然可以在界面里查看和修改；如果要重新由本程序接管，请先把它停掉。
        </div>
        <div class="notice-actions">
          <button class="btn btn-sm btn-ghost" style="background:transparent" id="notice-stop-external">停止那个 frp 进程</button>
        </div>
      </div></div>`;
  }

  // 装错架构的包：ARM 电脑上跑了 x64 版会进模拟层，WebView2 在这个组合下有已知崩溃问题
  if (st.emulated && st.archHint) {
    h += `<div class="notice warn">${svg(I.warn, 16)}
      <div class="notice-body">
        <b>你装的可能是错误的架构版本</b>
        <div style="margin-top:2px;opacity:.9">${esc(st.archHint)}</div>
        <div class="hint" style="color:inherit;opacity:.75;margin-top:4px">
          当前：程序 ${esc(st.processArch)} ／ 机器 ${esc(st.nativeArch)}
        </div>
      </div></div>`;
  }

  if (st.configFixed) {
    h += `<div class="notice info">${svg(I.info, 16)}
      <div class="notice-body"><b>启动时自动修正了配置文件</b>
      <div style="margin-top:2px;opacity:.9">${esc(st.configFixed)}</div>
      </div></div>`;
  }

  // 进程在跑却读不到日志 —— 这是"永远停在正在连接"的根因，必须点破
  if (st.noLogs) {
    h += `<div class="notice err">${svg(I.err, 16)}
      <div class="notice-body">
        <b>读不到 frp 的日志，所以无法判断连接状态</b>
        <div style="margin-top:2px;opacity:.9">
          frp 正在运行，但它没有把日志输出到本程序能读到的位置。
          原因通常是 <span class="mono">frpc.toml</span> 里的
          <span class="mono">log.to</span> 指向了文件而不是 <span class="mono">"console"</span>。
          （日志写成文件时，程序看不到它，就会一直显示"正在连接…"，即使其实早就连上了。）
        </div>
        <div class="notice-actions">
          <button class="btn btn-sm btn-primary" id="notice-fix-log">修正并重启 frp</button>
          <button class="btn btn-sm btn-ghost" style="background:transparent" id="notice-show-config">查看配置文件</button>
        </div>
      </div></div>`;
  }

  if (st.fileProxies && st.fileProxies.length) {
    h += `<div class="notice warn">${svg(I.warn, 16)}
      <div class="notice-body">
        <b>在配置文件里检测到 ${st.fileProxies.length} 个代理，它们无法在界面中启停或编辑。</b>
        <div style="margin-top:2px;opacity:.85">这些代理写在 frpc.toml 的 <span class="mono">[[proxies]]</span> 里，界面只能查看它们的状态。
        导入后即可在界面里统一管理（原配置会自动备份）。</div>
        <div class="hint mono" style="color:inherit;opacity:.7;margin-top:4px">${esc(st.fileProxies.join('、'))}</div>
        <div class="notice-actions">
          <button class="btn btn-sm btn-primary" id="notice-migrate">导入到界面管理</button>
        </div>
      </div></div>`;
  }

  box.innerHTML = h;

  const f = $('notice-open-folder'); if (f) f.onclick = () => call('OpenConfigFolder').catch(() => {});
  const w = $('notice-open-wizard'); if (w) w.onclick = () => openWizard();
  const s = $('notice-open-settings'); if (s) s.onclick = () => showPage('settings');
  const d = $('notice-diagnose'); if (d) d.onclick = () => { showPage('diagnose'); setTimeout(runDiagnose, 60); };
  const m = $('notice-migrate'); if (m) m.onclick = doMigrate;
  const se = $('notice-stop-external'); if (se) se.onclick = stopExternalFrp;
  const fl = $('notice-fix-log'); if (fl) fl.onclick = fixLogTarget;
  const sc = $('notice-show-config'); if (sc) sc.onclick = showConfigText;
}

/** 修正 log.to 并重启 frp（用户手工改坏了配置时用）。 */
async function fixLogTarget() {
  try {
    await call('FixLogTargetNow');
    ok('已修正并重启', '现在应该能看到日志和连接状态了');
    await refreshAll();
  } catch (e) {
    err('修正失败', cleanErr(e));
  }
}

/** 停止"不是本程序拉起"的那个 frpc（走管理接口，用户显式触发）。 */
async function stopExternalFrp() {
  const yes = await confirmDialog({
    title: '停止外部 frp 进程',
    text: '确定要停止本机上那个不是本程序启动的 frp 进程吗？',
    hint: '停止后它的所有代理都会下线。如果它是你另外配置的开机自启或系统服务，' +
      '下次开机还会自己起来。之后你就可以用本程序重新启动了。',
    okText: '停止它',
    danger: true,
  });
  if (!yes) return;
  try {
    await call('StopExternalFrp');
    ok('已停止', '现在可以用本程序启动了');
    setTimeout(() => refreshAll(), 900);
  } catch (e) {
    err('停止失败', cleanErr(e));
  }
}

/* ============================== 代理列表 ============================== */

const STATUS_PILL = {
  running: 'ok', new: 'info', 'wait start': 'warn', waiting: 'info',
  'start error': 'err', 'check failed': 'err', closed: 'idle', disabled: 'idle',
  enabled: 'ok',
};
const STATUS_DOT = {
  running: 'on', new: 'warn', 'wait start': 'warn', waiting: 'warn',
  'start error': 'err', 'check failed': 'err', closed: 'off', disabled: 'off',
  enabled: 'on',
};
const STATUS_FILTER = {
  running: ['running'],
  starting: ['new', 'wait start', 'waiting'],
  failed: ['start error', 'check failed'],
  disabled: ['disabled'],
};

function statusPill(v) {
  const cls = STATUS_PILL[v.status] || 'idle';
  const txt = v.statusText || (v.status || '未知');
  return `<span class="pill ${cls}"><i></i>${esc(txt)}</span>`;
}

/**
 * 对外地址的展示文本。
 *
 * ⚠ 这里做过一次修正。frp 对有域名的代理（http/https）报回来的 remote_addr 是
 *   「域名:VhostHTTPPort」（源码 server/proxy/http.go:105 用 CanonicalAddr 拼的），
 *   那个端口是 **frps 的内部端口**：
 *     - 服务端一般用 nginx 之类在 80/443 上反代到它，公网直接访问域名即可
 *     - 那个端口通常还被防火墙挡着，直连根本连不上
 *   官方 Web 界面是原样显示的，结果就是"照着界面上的地址加端口，访问不了"。
 *   所以这里只显示域名，端口和解释放进悬停提示。
 *   tcpmux 不一样：本来就是靠那个端口做 CONNECT 复用，端口必须留着。
 */
function remoteText(v) {
  if (!v.remoteAddr) return { text: '—', dim: true, title: '' };

  if (v.type === 'http' || v.type === 'https') {
    const m = String(v.remoteAddr).match(/^(.*):(\d+)$/);
    const host = m ? m[1] : String(v.remoteAddr);
    const port = m ? m[2] : '';
    const scheme = v.type === 'https' ? 'https' : 'http';

    // 代理还没上线时，服务端只给了子域名，补个占位说明
    if (host.indexOf('.') < 0) {
      return {
        text: host + '.<服务端域名>',
        dim: true,
        title: '代理上线后，服务端会把它拼成 子域名.服务端域名',
      };
    }
    return {
      text: host,
      dim: false,
      title: `浏览器里访问 ${scheme}://${host}/` +
        (port && port !== '80' && port !== '443'
          ? `\n（服务端的 vhost 端口是 ${port}，那是内部端口，一般由反向代理转发，公网访问不需要带端口）`
          : ''),
    };
  }

  // tcp / udp：remote_addr 形如 ":20001"，前面补上服务端地址
  if (/^\d+$/.test(String(v.remoteAddr))) {
    return {
      text: '<服务器地址>:' + v.remoteAddr,
      dim: true,
      title: '实际地址是 服务端域名:' + v.remoteAddr,
    };
  }

  return { text: String(v.remoteAddr), dim: false, title: String(v.remoteAddr) };
}

function filteredProxies() {
  const q = $('proxy-search').value.trim().toLowerCase();
  const tf = $('proxy-type-filter').value;
  const sf = $('proxy-status-filter').value;
  return S.proxies.filter((v) => {
    if (tf && v.type !== tf) return false;
    if (sf && !(STATUS_FILTER[sf] || []).includes(v.status)) return false;
    if (q) {
      const hay = [v.name, v.type, v.localAddr, v.remoteAddr, v.statusText, v.errText]
        .join(' ').toLowerCase();
      if (hay.indexOf(q) < 0) return false;
    }
    return true;
  });
}

function renderProxies(force) {
  const sig = JSON.stringify(S.proxies) + '|' + S.ready;
  if (!force && sig === S.proxySig) return;
  S.proxySig = sig;

  const list = filteredProxies();
  const st = S.state || {};
  const editable = canEdit();

  $('nav-proxy-count').textContent = String(S.proxies.length);
  $('proxy-count-text').textContent =
    list.length === S.proxies.length ? `共 ${list.length} 条` : `显示 ${list.length} / ${S.proxies.length} 条`;

  const rows = $('proxy-rows');
  rows.innerHTML = list.map((v) => {
    const rc = remoteText(v);
    const disabled = !v.enabled;
    const nameLock = v.fromFile
      ? `<span class="lock" title="写在 frpc.toml 里，界面无法管理">${svg(I.lock, 10, 2.5)}配置文件</span>` : '';
    const errLine = v.err
      ? `<div class="err-line" title="${esc(v.errText || v.err)}">${esc(v.errText || v.err)}</div>` : '';
    const actions = v.fromFile
      ? `<span class="muted" style="font-size:12px">仅查看</span>`
      : `<div class="switch ${v.enabled ? 'on' : ''} ${editable ? '' : 'disabled'}"
             data-act="toggle" data-name="${esc(v.name)}" data-on="${v.enabled ? '1' : '0'}"
             title="${editable ? '启用 / 停用' : 'frp 未运行，无法操作'}"></div>
         <button class="icon-btn" data-act="edit" data-name="${esc(v.name)}" title="编辑"
             ${editable ? '' : 'disabled'}>${svg(I.edit)}</button>
         <button class="icon-btn danger" data-act="del" data-name="${esc(v.name)}" title="删除"
             ${editable ? '' : 'disabled'}>${svg(I.trash)}</button>`;

    return `<div class="row ${disabled ? 'off' : ''}">
      <div class="row-name">
        <span class="dot ${STATUS_DOT[v.status] || 'off'}" style="width:7px;height:7px;box-shadow:none"></span>
        <span title="${esc(v.name)}">${esc(v.name)}</span>${nameLock}
      </div>
      <div><span class="tag">${esc(v.type)}</span></div>
      <div class="addr ${v.localAddr ? '' : 'dim'}" title="${esc(v.localAddr)}">${esc(v.localAddr || '—')}</div>
      <div class="addr ${rc.dim ? 'dim' : ''}" title="${esc(rc.title || rc.text)}">${esc(rc.text)}</div>
      <div>${statusPill(v)}${errLine}</div>
      <div class="row-actions">${actions}</div>
    </div>`;
  }).join('');

  const empty = $('proxy-empty');
  if (S.proxies.length === 0) {
    empty.classList.remove('hidden');
    $('proxy-empty-title').textContent = editable ? '还没有任何代理' : 'frp 未运行';
    $('proxy-empty-desc').textContent = editable
      ? '代理用来把本机的服务发布到公网。比如把本机 3306 的 MySQL 发布出去，别人就能通过服务器地址访问它。'
      : '启动 frp 之后，这里会显示所有代理。现在只能看到写在 frpc.toml 里的代理。';
    $('btn-empty-new').textContent = editable ? '新建第一条代理' : '启动 frp';
  } else {
    empty.classList.add('hidden');
  }
}

/* ============================== 访问者列表 ============================== */

function renderVisitors() {
  $('nav-visitor-count').textContent = String(S.visitors.length);
  const st = S.state || {};
  const editable = canEdit();

  $('visitor-rows').innerHTML = S.visitors.map((v) => {
    const rc = { text: v.localAddr || '—', dim: !v.localAddr };
    const target = [v.serverUser, v.serverName].filter(Boolean).join(' / ') || '—';
    return `<div class="row ${v.enabled ? '' : 'off'}">
      <div class="row-name">
        <span class="dot ${STATUS_DOT[v.status] || 'off'}" style="width:7px;height:7px;box-shadow:none"></span>
        <span title="${esc(v.name)}">${esc(v.name)}</span>
      </div>
      <div><span class="tag">${esc(v.type)}</span></div>
      <div class="addr" title="${esc(target)}">${esc(target)}</div>
      <div class="addr ${rc.dim ? 'dim' : ''}">${esc(rc.text)}</div>
      <div>${statusPill(v)}${v.err ? `<div class="err-line">${esc(v.err)}</div>` : ''}</div>
      <div class="row-actions">
        <div class="switch ${v.enabled ? 'on' : ''} ${editable ? '' : 'disabled'}"
             data-vact="toggle" data-name="${esc(v.name)}" data-on="${v.enabled ? '1' : '0'}"></div>
        <button class="icon-btn" data-vact="edit" data-name="${esc(v.name)}" title="编辑"
            ${editable ? '' : 'disabled'}>${svg(I.edit)}</button>
        <button class="icon-btn danger" data-vact="del" data-name="${esc(v.name)}" title="删除"
            ${editable ? '' : 'disabled'}>${svg(I.trash)}</button>
      </div>
    </div>`;
  }).join('');

  $('visitor-empty').classList.toggle('hidden', S.visitors.length > 0);
}

/* ============================== 日志 ============================== */

const LEVEL_META = {
  error: { pill: 'err', text: '错误' },
  warn: { pill: 'warn', text: '警告' },
  info: { pill: 'ok', text: '信息' },
  debug: { pill: 'idle', text: '调试' },
  trace: { pill: 'idle', text: '追踪' },
};

function logNode(e) {
  const lv = LEVEL_META[e.level] || LEVEL_META.info;
  const n = document.createElement('div');
  n.className = 'log-item';
  n.dataset.level = e.level || 'info';
  n.dataset.search = [e.summary, e.detail, e.scope, e.raw, lv.text]
    .join(' ').toLowerCase();

  n.innerHTML = `<div class="log-top">
      <span class="log-time">${esc(e.time || '')}</span>
      <span class="pill ${lv.pill}" style="height:20px;font-size:11.5px"><i></i>${lv.text}</span>
      ${e.scope ? `<span class="log-scope">[${esc(e.scope)}]</span>` : ''}
    </div>
    <div class="log-msg">${esc(e.summary || e.raw || '')}</div>
    ${e.detail ? `<div class="log-detail">${esc(e.detail)}</div>` : ''}
    ${e.raw ? `<button class="log-raw-btn">${svg(I.chevron, 11, 2.5)}查看原始日志</button>
      <div class="log-raw">${esc(e.raw)}</div>` : ''}`;

  const btn = n.querySelector('.log-raw-btn');
  if (btn) {
    btn.onclick = () => {
      const open = n.classList.toggle('open');
      btn.lastChild.textContent = open ? '收起原始日志' : '查看原始日志';
    };
  }
  return n;
}

function applyLogFilter(n) {
  const q = $('log-search').value.trim().toLowerCase();
  const lf = $('log-level-filter').value;
  let hide = false;
  if (lf) {
    if (lf === 'warn') hide = !(n.dataset.level === 'warn' || n.dataset.level === 'error');
    else if (lf === 'error') hide = n.dataset.level !== 'error';
    else hide = n.dataset.level !== lf;
  }
  if (!hide && q) hide = n.dataset.search.indexOf(q) < 0;
  n.classList.toggle('hidden', hide);
}

function refilterLogs() {
  $('log-list').childNodes.forEach(applyLogFilter);
}

function scrollLogsToEnd() {
  if (!S.autoscroll || S.paused) return;
  if (S.page !== 'logs') return;
  const c = $('content');
  c.scrollTop = c.scrollHeight;
}

function appendLogs(entries) {
  const list = $('log-list');
  for (const e of entries) {
    S.logs.push(e);
    const n = logNode(e);
    applyLogFilter(n);
    list.appendChild(n);
  }
  // 控制内存与 DOM 规模
  while (S.logs.length > 2000) {
    S.logs.shift();
    if (list.firstElementChild) list.firstElementChild.remove();
  }
  $('log-empty').classList.toggle('hidden', list.childElementCount > 0);
  scrollLogsToEnd();
}

/* ============================== 设置页 ============================== */

function fillConfigForm() {
  const c = S.config;
  if (!c) return;
  $('cfg-serverAddr').value = c.serverAddr || '';
  $('cfg-serverPort').value = c.serverPort || '';
  $('cfg-authToken').value = c.authToken || '';
  $('cfg-user').value = c.user || '';
  $('cfg-clientID').value = c.clientID || '';
}

async function loadSettings() {
  try {
    const s = await call('GetSettings');
    if (s) S.settings = s;
  } catch (e) { /* 用默认值 */ }
  $('sw-close-to-tray').classList.toggle('on', !!S.settings.closeToTray);
  $('sw-auto-restart').classList.toggle('on', !!S.settings.autoRestart);
  try {
    const a = await call('GetAutoStart');
    $('sw-autostart').classList.toggle('on', !!a);
  } catch (e) { /* 读不到就保持关闭 */ }
}

async function persistSettings() {
  try {
    await call('SaveSettings', {
      closeToTray: $('sw-close-to-tray').classList.contains('on'),
      autoRestart: $('sw-auto-restart').classList.contains('on'),
    });
  } catch (e) {
    err('设置没能保存', cleanErr(e));
  }
}

function renderEnv() {
  const st = S.state || {};
  const c = S.config || {};
  const items = [
    ['frp 主程序', st.exePath || '—', st.exeExists ? 'ok' : 'err'],
    ['配置文件', st.configPath || '—', 'ok'],
    ['本机管理端口', (st.processRunning || st.externalRunning)
      ? `http://${c.webAddr || '127.0.0.1'}:${c.webPort || 7400}`
      : '未运行（端口无响应）', st.externalRunning ? 'warn' : 'ok'],
    ['代理存储', c.storePath || '—', st.storeEnabled ? 'ok' : 'warn'],
    ['frp 进程', st.processRunning ? '运行中（由本程序管理）'
      : (st.externalRunning ? '运行中（由外部程序启动，本程序无法停止它）' : '未运行'),
      st.processRunning ? 'ok' : (st.externalRunning ? 'warn' : 'warn')],
  ];
  $('env-info').innerHTML = items.map(([k, v, s]) => `
    <div class="setting-row">
      <div class="diag-icon ${s}">${s === 'ok' ? svg(I.ok, 14, 3) : svg(I.warn, 14, 2.6)}</div>
      <div class="setting-text">
        <div class="setting-name">${esc(k)}</div>
        <div class="setting-desc mono" style="font-size:11.5px;word-break:break-all">${esc(v)}</div>
      </div>
    </div>`).join('');
}

/* ============================== 诊断 ============================== */

async function runDiagnose() {
  const btn = $('btn-diagnose');
  btn.disabled = true;
  const old = btn.innerHTML;
  btn.innerHTML = svg(I.refresh, 15, 2.2) + '诊断中…';
  try {
    const items = await call('Diagnose');
    const box = $('diag-results');
    if (!items || !items.length) {
      box.innerHTML = `<div class="empty"><div class="empty-title">没有发现问题</div>
        <div class="empty-desc">各项检查都通过了。</div></div>`;
    } else {
      box.innerHTML = items.map((it) => {
        const cls = it.ok ? 'ok' : (it.warn ? 'warn' : 'err');
        const ic = it.ok ? svg(I.ok, 14, 3) : (it.warn ? svg(I.warn, 14, 2.6) : svg(I.err, 14, 2.6));
        return `<div class="setting-row">
          <div class="diag-icon ${cls}">${ic}</div>
          <div class="setting-text">
            <div class="setting-name">${esc(it.name)}</div>
            <div class="setting-desc" style="${it.ok ? '' : 'color:var(--' + (it.warn ? 'warn' : 'danger') + '-fg)'}">${esc(it.message)}</div>
          </div>
        </div>`;
      }).join('');
    }
    ok('诊断完成', items ? items.length + ' 项检查已更新' : '');
  } catch (e) {
    err('诊断失败', cleanErr(e));
  } finally {
    btn.disabled = false;
    btn.innerHTML = old;
  }
}

/* ============================== 代理弹窗 ============================== */

let curType = 'tcp';
let curBackend = 'direct';

const TYPE_HINT = {
  tcp: '把本机的一个端口原样发布到服务器的一个端口上。数据库、远程桌面、游戏服都用它。',
  udp: '和 TCP 类似，但走 UDP。适用于部分游戏、DNS、语音服务。',
  http: '发布网站。服务器根据域名把请求转发给你，不需要额外占用端口。',
  https: '发布 HTTPS 网站。由服务器按域名转发，加密由 frp 隧道承担。',
  tcpmux: '通过一个端口复用多种服务，需要服务端开启 tcpmuxHTTPConnectPort。',
  stcp: '私密 TCP：只有拿着相同密钥的「访问者」才能连，不对外暴露端口。',
  sudp: '私密 UDP：同 STCP，走 UDP。',
  xtcp: '点对点直连，速度最快。但需要双方网络都支持打洞，失败时会自动降级。',
};

const PLUGIN_FIELDS = {
  http2https: [{ k: 'localAddr', label: '本机 HTTPS 地址', ph: '127.0.0.1:443', req: true }],
  https2http: [
    { k: 'localAddr', label: '本机 HTTP 地址', ph: '127.0.0.1:80', req: true },
    { k: 'hostHeaderRewrite', label: '改写 Host 头', ph: '127.0.0.1' },
    { k: 'crtPath', label: '证书文件（crt）', ph: 'C:\\cert\\a.crt' },
    { k: 'keyPath', label: '私钥文件（key）', ph: 'C:\\cert\\a.key' },
  ],
  https2https: [
    { k: 'localAddr', label: '本机 HTTPS 地址', ph: '127.0.0.1:443', req: true },
    { k: 'crtPath', label: '证书文件（crt）', ph: 'C:\\cert\\a.crt' },
    { k: 'keyPath', label: '私钥文件（key）', ph: 'C:\\cert\\a.key' },
  ],
  http2http: [
    { k: 'localAddr', label: '本机 HTTP 地址', ph: '127.0.0.1:80', req: true },
    { k: 'hostHeaderRewrite', label: '改写 Host 头', ph: '127.0.0.1' },
  ],
  http_proxy: [
    { k: 'httpUser', label: '代理用户名（可选）' },
    { k: 'httpPassword', label: '代理密码（可选）' },
  ],
  socks5: [
    { k: 'username', label: '用户名（可选）' },
    { k: 'password', label: '密码（可选）' },
  ],
  static_file: [
    { k: 'localPath', label: '本机目录', ph: 'D:\\www', req: true },
    { k: 'stripPrefix', label: 'URL 前缀剥离', ph: '/static' },
    { k: 'httpUser', label: '访问用户名（可选）' },
    { k: 'httpPassword', label: '访问密码（可选）' },
  ],
  unix_domain_socket: [{ k: 'unixPath', label: 'unix socket 路径', ph: '/var/run/a.sock', req: true }],
  tls2raw: [
    { k: 'localAddr', label: '本机地址', ph: '127.0.0.1:8080', req: true },
    { k: 'crtPath', label: '证书文件（crt）', ph: 'C:\\cert\\a.crt' },
    { k: 'keyPath', label: '私钥文件（key）', ph: 'C:\\cert\\a.key' },
  ],
};

function renderPluginParams(preset) {
  const type = $('pf-pluginType').value;
  const fields = PLUGIN_FIELDS[type] || [];
  const vals = preset && typeof preset === 'object' ? preset : S.pluginValues[type] || {};
  $('pf-plugin-params').innerHTML = fields.map((f) => `
    <div class="field" style="margin-bottom:var(--s4)">
      <label class="label">${esc(f.label)}${f.req ? '<span class="req">*</span>' : ''}</label>
      <input class="input" data-pk="${esc(f.k)}" placeholder="${esc(f.ph || '')}"
             value="${esc(vals[f.k] || '')}">
    </div>`).join('');
  $('pf-plugin-params').querySelectorAll('[data-pk]').forEach((inp) => {
    inp.addEventListener('input', () => {
      S.pluginValues[type] = S.pluginValues[type] || {};
      S.pluginValues[type][inp.dataset.pk] = inp.value;
    });
  });
}

function collectPluginParams() {
  const out = {};
  $('pf-plugin-params').querySelectorAll('[data-pk]').forEach((inp) => {
    const v = inp.value.trim();
    if (v) out[inp.dataset.pk] = v;
  });
  return out;
}

function applyType(type) {
  curType = type;
  document.querySelectorAll('#proxy-modal [data-show-for]').forEach((n) => {
    n.classList.toggle('hidden', n.dataset.showFor.split(',').indexOf(type) < 0);
  });
  document.querySelectorAll('#pm-types .type-card').forEach((c) => {
    c.classList.toggle('on', c.dataset.type === type);
  });
  $('pm-type-hint').textContent = TYPE_HINT[type] || '';
  $('healthcheck-path-field').classList.toggle('hidden', type !== 'http' && type !== 'https');
  updateAddrPreview();
}

function applyBackend(mode) {
  curBackend = mode;
  document.querySelectorAll('#proxy-modal [data-show-backend]').forEach((n) => {
    n.classList.toggle('hidden', n.dataset.showBackend !== mode);
  });
  document.querySelectorAll('#proxy-modal .seg[data-backend]').forEach((b) => {
    b.classList.toggle('on', b.dataset.backend === mode);
  });
}

function updateAddrPreview() {
  const st = S.state || {};
  const host = st.serverAddr || '服务器地址';
  const port = $('pf-remotePort').value.trim();
  $('pf-remotePort-eg').textContent = host + ':' + (port || '端口');
  const sub = $('pf-subdomain').value.trim();
  const c = S.config || {};
  $('pf-subdomain-hint').innerHTML = sub
    ? `只写最左边一段，实际访问地址由服务端拼成 <span class="mono">${esc(sub)}.<服务端域名></span>`
    : '只写最左边一段，实际访问地址由服务端拼成 <span class="mono">子域名.服务端域名</span>';
}

function setSwitch(id, on) { $(id).classList.toggle('on', !!on); }
function getSwitch(id) { return $(id).classList.contains('on'); }

/* ============================== 可折叠区块 ============================== */

function setFoldOpen(fold, on) {
  fold.classList.toggle('open', !!on);
  const head = fold.querySelector('.fold-head');
  if (head) head.setAttribute('aria-expanded', on ? 'true' : 'false');
}

/**
 * 绑定所有 .fold 的展开 / 收起。
 *
 * ⚠ 样式和结构早就有了（`.fold.open .fold-body{display:block}`），但**一直没人写
 *   切换 .open 的那行 JS** —— 所以「高级选项（可选）」那个按钮从做出来就是死的：
 *   点上去毫无反应。CSS 定义了状态、HTML 搭好了骨架，缺的就是这一句。
 */
function bindFolds() {
  document.querySelectorAll('.fold').forEach((fold) => {
    const head = fold.querySelector('.fold-head');
    if (!head || head.dataset.foldBound) return;
    head.dataset.foldBound = '1';
    setFoldOpen(fold, fold.classList.contains('open')); // 初始化 aria-expanded
    head.onclick = () => setFoldOpen(fold, !fold.classList.contains('open'));
  });
}

/**
 * 按折叠区里当前的内容决定要不要自动展开。
 *
 * 为什么需要：编辑一条"开着健康检查 / 加密 / 限速"的代理时，折叠区如果默认收起，
 * 用户**根本看不到这些设置是开着的**，很容易以为没开。所以只要里面有任何一项
 * 不是默认值，就自动展开一次。
 */
function syncFoldOpen(rootEl) {
  const fold = rootEl.querySelector('.fold');
  if (!fold) return;
  const body = fold.querySelector('.fold-body');
  const hasText = Array.from(body.querySelectorAll('input'))
    .some((i) => (i.value || '').trim() !== '');
  const hasSwitch = Array.from(body.querySelectorAll('.switch'))
    .some((s) => s.classList.contains('on'));
  const hasChip = Array.from(body.querySelectorAll('.chips'))
    .some((c) => c.querySelector('.chip'));
  setFoldOpen(fold, hasText || hasSwitch || hasChip);
}

function openProxyModal(v) {
  S.editName = v ? v.name : '';
  $('pm-title').textContent = v ? '编辑代理' : '新建代理';
  $('pm-error').textContent = '';
  $('btn-save-proxy').disabled = false;

  // 重置
  $('pf-name').value = v ? v.name : '';
  $('pf-name').disabled = false;
  ['pf-remotePort', 'pf-subdomain', 'pf-localIP', 'pf-localPort', 'pf-bandwidthLimit',
    'pf-loadBalancerGroup', 'pf-httpUser', 'pf-httpPassword', 'pf-routeByHTTPUser',
    'pf-hostHeaderRewrite', 'pf-secretKey', 'pf-hc-interval', 'pf-hc-timeout', 'pf-hc-path']
    .forEach((id) => { $(id).value = ''; });
  ['sw-encryption', 'sw-compression', 'sw-healthcheck', 'sw-natTraversal']
    .forEach((id) => setSwitch(id, false));
  $('healthcheck-params').classList.add('hidden');
  setChips('customDomains', []);
  setChips('allowUsers', []);
  setChips('locations', []);
  $('pf-pluginType').value = 'http2https';
  S.pluginValues = {};

  let type = 'tcp';
  let backendMode = 'direct';

  if (v) {
    const d = v.def || {};
    type = (d.type || v.type || 'tcp');
    const b = d[type] || {};

    $('pf-name').value = d.name || v.name || '';
    $('pf-remotePort').value = b.remotePort || '';
    $('pf-subdomain').value = b.subdomain || '';
    setChips('customDomains', b.customDomains || []);
    setChips('allowUsers', b.allowUsers || []);
    setChips('locations', b.locations || []);
    $('pf-multiplexer').value = b.multiplexer || 'httpconnect';
    $('pf-secretKey').value = b.secretKey || '';
    $('pf-httpUser').value = b.httpUser || '';
    $('pf-httpPassword').value = b.httpPassword || '';
    $('pf-routeByHTTPUser').value = b.routeByHTTPUser || '';
    $('pf-hostHeaderRewrite').value = b.hostHeaderRewrite || '';

    const tr = b.transport || {};
    $('pf-bandwidthLimit').value = tr.bandwidthLimit || '';
    setSwitch('sw-encryption', tr.useEncryption);
    setSwitch('sw-compression', tr.useCompression);
    $('pf-loadBalancerGroup').value = (b.loadBalancer && b.loadBalancer.group) || '';

    const hc = b.healthCheck || {};
    if (hc.type) {
      setSwitch('sw-healthcheck', true);
      $('healthcheck-params').classList.remove('hidden');
      if (hc.intervalSeconds) $('pf-hc-interval').value = hc.intervalSeconds + 's';
      if (hc.timeoutSeconds) $('pf-hc-timeout').value = hc.timeoutSeconds + 's';
      if (hc.path) $('pf-hc-path').value = hc.path;
    }
    if (b.natTraversal) setSwitch('sw-natTraversal', !!(b.natTraversal).disableAssistedAddrs);

    if (b.plugin && b.plugin.type) {
      backendMode = 'plugin';
      $('pf-pluginType').value = b.plugin.type;
      S.pluginValues[b.plugin.type] = Object.assign({}, b.plugin);
      delete S.pluginValues[b.plugin.type].type;
      renderPluginParams(S.pluginValues[b.plugin.type]);
    } else {
      $('pf-localIP').value = b.localIP || '127.0.0.1';
      $('pf-localPort').value = b.localPort || '';
    }
  }

  if (backendMode === 'direct' && !v) $('pf-localIP').value = '127.0.0.1';

  applyType(type);
  applyBackend(backendMode);
  updateLocalPortHint();
  // 高级选项里只要有内容就展开，别让用户看不到自己开过什么
  syncFoldOpen($('proxy-modal'));
  openModal('proxy-modal');
  setTimeout(() => $('pf-name').focus(), 50);
}

function updateLocalPortHint() {
  const port = parseInt($('pf-localPort').value, 10);
  const h = $('pf-localPort-hint');
  h.className = 'hint';
  if (!port) { h.textContent = '本机上真正提供服务的端口'; return; }
  h.textContent = '正在检查本机端口…';
  call('CheckLocalPort', port).then((open) => {
    if (parseInt($('pf-localPort').value, 10) !== port) return;
    if (open) {
      h.className = 'hint ok';
      h.innerHTML = svg(I.ok, 12, 2.6) + `<span>本机 ${port} 端口有服务在监听</span>`;
    } else {
      h.className = 'hint warn';
      h.innerHTML = svg(I.warn, 12, 2.4) +
        `<span>本机 ${port} 端口暂时没有服务在监听。如果服务还没启动，可以先保存，之后启动服务即可。</span>`;
    }
  }).catch(() => { h.textContent = '本机上真正提供服务的端口'; });
}

function readNum(s) { const n = parseInt(s, 10); return isNaN(n) ? 0 : n; }

/** 把表单内容组装成 frp 的 ProxyDefinition 结构。 */
function collectProxyDef() {
  const name = $('pf-name').value.trim();
  if (!name) throw new Error('请填写代理名称');
  if (!/^[^\/\s]+$/.test(name)) throw new Error('代理名不能包含空格或斜杠');

  const b = {};
  b.name = name;
  b.type = curType;

  const direct = curBackend === 'direct';

  if (curType === 'tcp' || curType === 'udp') {
    const p = readNum($('pf-remotePort').value);
    if (!direct && curBackend === 'plugin') { /* 插件模式也允许 remotePort */ }
    if (!p && p !== 0) throw new Error('请填写对外端口');
    b.remotePort = p;
  }
  if (curType === 'http' || curType === 'https') {
    const sub = $('pf-subdomain').value.trim();
    const doms = chipValues('customDomains');
    if (!sub && !doms.length) throw new Error('HTTP / HTTPS 代理必须填「自定义域名」或「子域名」其中之一');
    if (sub && doms.length) throw new Error('「自定义域名」和「子域名」只能填一个');
    if (sub) {
      if (sub.indexOf('.') >= 0) throw new Error('子域名只能是一段，不能包含点号（不要写 *. 或 a.b）');
      b.subdomain = sub;
    }
    if (doms.length) b.customDomains = doms;
  }
  if (curType === 'tcpmux') {
    const doms = chipValues('customDomains');
    if (!doms.length) throw new Error('TCPMUX 代理必须填至少一个自定义域名');
    b.customDomains = doms;
    b.multiplexer = $('pf-multiplexer').value || 'httpconnect';
  }
  if (curType === 'stcp' || curType === 'sudp' || curType === 'xtcp') {
    const sk = $('pf-secretKey').value.trim();
    if (!sk) throw new Error('请填写密钥 Secret Key，访问者需要用它才能连接');
    b.secretKey = sk;
    const au = chipValues('allowUsers');
    if (au.length) b.allowUsers = au;
  }

  // 后端
  if (direct) {
    const lp = readNum($('pf-localPort').value);
    if (!lp) throw new Error('请填写本机端口');
    b.localIP = $('pf-localIP').value.trim() || '127.0.0.1';
    b.localPort = lp;
  } else {
    const params = collectPluginParams();
    const fields = PLUGIN_FIELDS[$('pf-pluginType').value] || [];
    for (const f of fields) {
      if (f.req && !params[f.k]) throw new Error('请填写「' + f.label + '」');
    }
    b.plugin = Object.assign({ type: $('pf-pluginType').value }, params);
  }

  // HTTP 专属
  if (curType === 'http' || curType === 'tcpmux') {
    const u = $('pf-httpUser').value.trim();
    const p = $('pf-httpPassword').value;
    if (u) b.httpUser = u;
    if (p) b.httpPassword = p;
    const r = $('pf-routeByHTTPUser').value.trim();
    if (r) b.routeByHTTPUser = r;
  }
  if (curType === 'http') {
    const loc = chipValues('locations');
    if (loc.length) b.locations = loc;
    const hh = $('pf-hostHeaderRewrite').value.trim();
    if (hh) b.hostHeaderRewrite = hh;
  }

  // transport
  const tr = {};
  if (getSwitch('sw-encryption')) tr.useEncryption = true;
  if (getSwitch('sw-compression')) tr.useCompression = true;
  const bl = $('pf-bandwidthLimit').value.trim();
  if (bl) tr.bandwidthLimit = bl;
  if (Object.keys(tr).length) b.transport = tr;

  // loadBalancer
  const lg = $('pf-loadBalancerGroup').value.trim();
  if (lg) b.loadBalancer = { group: lg };

  // healthCheck
  if (getSwitch('sw-healthcheck')) {
    const hc = { type: (curType === 'http' || curType === 'https') ? 'http' : 'tcp' };
    const iv = $('pf-hc-interval').value.trim();
    const to = $('pf-hc-timeout').value.trim();
    hc.intervalSeconds = parseDur(iv, 10);
    hc.timeoutSeconds = parseDur(to, 3);
    const p = $('pf-hc-path').value.trim();
    if (p) hc.path = p;
    b.healthCheck = hc;
  }

  // natTraversal（仅 xtcp）
  if (curType === 'xtcp' && getSwitch('sw-natTraversal')) {
    b.natTraversal = { disableAssistedAddrs: true };
  }

  const def = { name: name, type: curType };
  def[curType] = b;
  return def;
}

function parseDur(s, def) {
  if (!s) return def;
  const m = String(s).match(/^(\d+)/);
  return m ? parseInt(m[1], 10) : def;
}

async function saveProxy() {
  let def;
  try {
    def = collectProxyDef();
  } catch (e) {
    $('pm-error').textContent = cleanErr(e);
    return;
  }
  const btn = $('btn-save-proxy');
  btn.disabled = true;
  $('pm-error').textContent = '';
  const old = btn.textContent;
  btn.textContent = '保存中…';
  try {
    await call('SaveProxy', def, !!S.editName);
    closeModal('proxy-modal');
    ok(S.editName ? '代理已更新' : '代理已创建',
      '「' + def.name + '」' + (S.editName ? '的修改已生效' : '已添加到列表'));
    await refreshProxies(true);
  } catch (e) {
    $('pm-error').textContent = cleanErr(e);
  } finally {
    btn.disabled = false;
    btn.textContent = old;
  }
}

/* ============================== 访问者弹窗 ============================== */

function openVisitorModal(v) {
  S.editVisName = v ? v.name : '';
  $('vm-title').textContent = v ? '编辑访问者' : '新建访问者';
  $('vm-error').textContent = '';
  $('vf-type').value = v ? v.type : 'stcp';
  $('vf-name').value = v ? v.name : '';
  $('vf-name').disabled = false;
  const b = v && v.def ? (v.def[v.type] || {}) : {};
  $('vf-serverUser').value = b.serverUser || '';
  $('vf-serverName').value = b.serverName || '';
  $('vf-secretKey').value = b.secretKey || '';
  $('vf-bindAddr').value = b.bindAddr || '127.0.0.1';
  $('vf-bindPort').value = b.bindPort || '';
  $('vf-type').disabled = !!v; // 类型不可改（改了等于换一条）
  openModal('visitor-modal');
  setTimeout(() => $('vf-name').focus(), 50);
}

async function saveVisitor() {
  const name = $('vf-name').value.trim();
  const type = $('vf-type').value;
  const e = $('vm-error');
  if (!name) { e.textContent = '请填写名称'; return; }
  const serverName = $('vf-serverName').value.trim();
  if (!serverName) { e.textContent = '请填写对方的服务名称 serverName'; return; }
  const secretKey = $('vf-secretKey').value.trim();
  if (!secretKey) { e.textContent = '请填写密钥 Secret Key'; return; }
  const bindPort = readNum($('vf-bindPort').value);
  if (!bindPort) { e.textContent = '请填写映射到本机的端口'; return; }

  const b = {
    name: name, type: type,
    serverName: serverName,
    secretKey: secretKey,
    bindAddr: $('vf-bindAddr').value.trim() || '127.0.0.1',
    bindPort: bindPort,
  };
  const su = $('vf-serverUser').value.trim();
  if (su) b.serverUser = su;

  const def = { name: name, type: type };
  def[type] = b;

  const btn = $('btn-save-visitor');
  btn.disabled = true;
  const old = btn.textContent;
  btn.textContent = '保存中…';
  try {
    await call('SaveVisitor', def, !!S.editVisName);
    closeModal('visitor-modal');
    ok(S.editVisName ? '访问者已更新' : '访问者已创建', '「' + name + '」已生效');
    await refreshVisitors(true);
  } catch (ex) {
    e.textContent = cleanErr(ex);
  } finally {
    btn.disabled = false;
    btn.textContent = old;
  }
}

/* ============================== 标签输入控件 ============================== */

const chipStore = {};

function chipValues(key) { return (chipStore[key] || []).slice(); }

function setChips(key, arr) {
  chipStore[key] = (arr || []).slice();
  const box = document.querySelector(`[data-chips="${key}"]`);
  if (box) paintChips(box);
}

function paintChips(box) {
  const key = box.dataset.chips;
  const input = box.querySelector('input');
  box.querySelectorAll('.chip').forEach((c) => c.remove());
  (chipStore[key] || []).forEach((val, i) => {
    const c = document.createElement('span');
    c.className = 'chip';
    c.innerHTML = '<span></span><button type="button" title="移除">' +
      svg('<path d="M18 6 6 18M6 6l12 12"/>', 10, 2.6) + '</button>';
    c.firstChild.textContent = val;
    c.querySelector('button').onclick = () => {
      chipStore[key].splice(i, 1);
      paintChips(box);
    };
    box.insertBefore(c, input);
  });
}

function initChips() {
  document.querySelectorAll('[data-chips]').forEach((box) => {
    const key = box.dataset.chips;
    if (!chipStore[key]) chipStore[key] = [];
    const input = box.querySelector('input');
    input.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' || ev.key === ',' || ev.key === '，') {
        ev.preventDefault();
        commitChip(box, input);
      } else if (ev.key === 'Backspace' && input.value === '' && chipStore[key].length) {
        chipStore[key].pop();
        paintChips(box);
      }
    });
    input.addEventListener('blur', () => commitChip(box, input));
    box.addEventListener('click', () => input.focus());
    paintChips(box);
  });
}

function commitChip(box, input) {
  const key = box.dataset.chips;
  const v = input.value.trim().replace(/[,，]$/, '');
  input.value = '';
  if (!v) return;
  if (chipStore[key].indexOf(v) >= 0) return;
  chipStore[key].push(v);
  paintChips(box);
}

/* ============================== 首次配置向导 ============================== */

let wzStep = 1;

/**
 * 渲染向导第 3 步的三条检查结果。
 *
 * ⚠ 这个函数会被**反复调用**（钩子在 refreshState 末尾）。原因：
 *   保存配置的那一刻 frpc 通常还没打印出 "login to server success"，
 *   而连接状态只能从日志推断 —— 只渲染一次的话，就会永远停在
 *   "正在连接…"，其实早就连上了。
 */
function paintWizardDone(st) {
  st = st || {};
  const connected = st.state === 'connected';
  const rows = [
    ['配置文件已生成', (S.config && S.config.storePath) ? '代理会保存在 ' + S.config.storePath : '已写入 frpc.toml', 'ok'],
    ['frp 进程', st.processRunning ? '已启动' : '未能启动，请到「连接诊断」查看原因',
      st.processRunning ? 'ok' : 'warn'],
    ['服务端连接', connected ? '已连接，认证通过' : (st.stateText || '正在连接…'),
      connected ? 'ok' : 'warn'],
  ];
  $('wz-done-list').innerHTML = rows.map(([k, v, s]) => `
    <div class="setting-row">
      <div class="diag-icon ${s}">${s === 'ok' ? svg(I.ok, 14, 3) : svg(I.warn, 14, 2.6)}</div>
      <div class="setting-text">
        <div class="setting-name">${esc(k)}</div>
        <div class="setting-desc">${esc(v)}</div>
      </div>
    </div>`).join('');
  $('wz-done-desc').textContent = !st.processRunning
    ? '配置已保存，但 frp 没能启动，请查看日志或运行连接诊断。'
    : (connected ? '配置已保存，已连接到服务端。' : '配置已保存，正在连接服务端…');
}

function openWizard() {
  wzStep = 1;
  $('wizard').classList.remove('hidden');
  const st = S.state || {};
  const ic = $('wz-exe-icon');
  if (st.exeExists) {
    ic.className = 'diag-icon ok';
    ic.innerHTML = svg(I.ok, 14, 3);
    $('wz-exe-desc').textContent = '已就位：' + (st.exePath || 'frpc.exe');
  } else {
    ic.className = 'diag-icon err';
    ic.innerHTML = svg(I.err, 14, 2.6);
    $('wz-exe-desc').textContent = '没找到 frpc.exe，请把它放到 ' + (st.exePath || '本程序目录');
  }
  const c = S.config || {};
  $('wz-serverAddr').value = c.serverAddr || '';
  $('wz-serverPort').value = c.serverPort || 7000;
  $('wz-authToken').value = c.authToken || '';
  $('wz-user').value = c.user || '';
  $('wz-clientID').value = c.clientID || '';
  $('wz-error').classList.add('hidden');
  paintWizard();
}

function paintWizard() {
  [1, 2, 3].forEach((n) => $('wz-' + n).classList.toggle('hidden', n !== wzStep));
  document.querySelectorAll('.wizard-step').forEach((d) => {
    d.classList.toggle('on', parseInt(d.dataset.step, 10) <= wzStep);
  });
  $('wz-foot-left').textContent = `第 ${wzStep} 步 / 共 3 步`;
  $('wz-prev').classList.toggle('hidden', wzStep === 1);
  $('wz-next').textContent = wzStep === 3 ? '开始使用' : '下一步';
}

async function wizardNext() {
  if (wzStep === 1) { wzStep = 2; paintWizard(); return; }

  if (wzStep === 2) {
    const addr = $('wz-serverAddr').value.trim();
    const token = $('wz-authToken').value.trim();
    const box = $('wz-error');
    if (!addr) { box.textContent = '请填写服务器地址'; box.classList.remove('hidden'); return; }
    // ⚠ 令牌不校验非空：服务端可能没有开启认证，这时客户端必须留空。
    box.classList.add('hidden');

    const btn = $('wz-next');
    btn.disabled = true;
    btn.textContent = '正在保存…';
    try {
      const cfg = Object.assign({}, S.config || {}, {
        serverAddr: addr,
        serverPort: readNum($('wz-serverPort').value) || 7000,
        authToken: token,
        user: $('wz-user').value.trim(),
        clientID: $('wz-clientID').value.trim(),
      });
      await call('SaveConfig', cfg);
      S.config = await call('GetConfig');
      fillConfigForm();
      await refreshState(true);

      // 渲染交给 paintWizardDone，之后 refreshState 会持续更新它。
      wzStep = 3;
      paintWizard();
      paintWizardDone(S.state);
    } catch (e) {
      box.textContent = cleanErr(e);
      box.classList.remove('hidden');
    } finally {
      btn.disabled = false;
      // ⚠ 不能无脑写回 "下一步"：第 3 步的按钮该叫「开始使用」。
      //   这里曾经覆盖掉 paintWizard() 设好的文案，导致向导最后一步
      //   一直显示"下一步"（按钮功能是对的，只是字错了）。
      btn.textContent = wzStep === 3 ? '开始使用' : '下一步';
    }
    return;
  }

  // 第 3 步：关闭向导
  $('wizard').classList.add('hidden');
  await refreshAll();
}

/* ============================== 关于页 ============================== */

let aboutInfo = null;

function stateClassOf(text) {
  if (!text) return '';
  if (text.indexOf('已连接') >= 0) return 'ok';
  if (text.indexOf('失败') >= 0) return 'err';
  return 'warn';
}

/** 统计块里放不下完整的状态描述，给个短版本。 */
function compactState(text) {
  if (!text) return '未知';
  if (text.indexOf('已连接') >= 0) return '已连接';
  if (text.indexOf('正在连接') >= 0) return '连接中';
  if (text.indexOf('外部进程') >= 0) return '外部运行';
  if (text.indexOf('失败') >= 0) return '连接失败';
  if (text.indexOf('未运行') >= 0) return '未运行';
  return text;
}

async function renderAbout(force) {
  if (aboutInfo && !force) return;
  try {
    aboutInfo = await call('GetAboutInfo');
  } catch (e) {
    return;
  }
  const a = aboutInfo;

  $('ab-name').textContent = a.author || '—';
  $('ab-tagline').textContent = a.tagline || '';

  $('ab-chips').innerHTML = [
    `<span class="pill ok"><i></i>CikPier v${esc(a.version)}</span>`,
    `<span class="pill info"><i></i>frp 内核 ${esc(a.frpVersion)}</span>`,
  ].join('');

  // 外链按钮。邮箱点一下是复制，其它是打开浏览器。
  const links = [
    { key: 'github', icon: svgFill(BRAND_PATHS.github, 15), label: 'GitHub', url: a.github },
    { key: 'gitee', icon: svgFill(BRAND_PATHS.gitee, 15), label: 'Gitee', url: a.gitee },
    { key: 'site', icon: svg(I.globe, 15, 1.9), label: '个人网站', url: a.website },
    { key: 'mail', icon: svg(I.mail, 15, 1.9), label: a.email, copy: a.email, tip: '点击复制邮箱地址' },
  ];
  $('ab-links').innerHTML = links.map((l) => `
    <button class="ab-link" data-ab="${esc(l.key)}" title="${esc(l.tip || l.url || '')}">
      ${l.icon}<span>${esc(l.label)}</span>
    </button>`).join('');

  $('ab-links').querySelectorAll('[data-ab]').forEach((btn) => {
    const l = links.find((x) => x.key === btn.dataset.ab);
    btn.onclick = () => {
      if (l.copy) copyText(l.copy, '邮箱已复制');
      else openExternal(l.url);
    };
  });

  // 统计
  const stats = [
    { n: String(a.proxyCount), l: '代理' },
    { n: String(a.visitorCount), l: '访问者' },
    { n: compactState(a.frpState), l: 'frp 状态', cls: stateClassOf(a.frpState) },
    { n: a.uptime || '—', l: '已运行' },
  ];
  $('ab-stats').innerHTML = stats.map((s) => `
    <div class="about-stat">
      <div class="n ${s.cls || ''}" style="${s.l === 'frp 状态' ? 'font-size:15px' : ''}">${esc(s.n)}</div>
      <div class="l">${esc(s.l)}</div>
    </div>`).join('');

  // 版本与构建
  const rows = [
    ['程序版本', `v${a.version}`],
    ['frp 内核', a.frpVersion],
    ['运行时', a.goVersion],
    ['界面框架', 'Wails ' + a.wailsVersion],
    ['目标平台', a.platform],
    ['运行架构', a.emulated
      ? `${a.processArch} → ${a.nativeArch}（跑在模拟层里，建议换 arm64 版）`
      : `${a.processArch}（原生，机器 ${a.nativeArch}）`],
    ['构建时间', a.buildTime || '—'],
    ['构建来源', a.vcsRevision || '—'],
  ];
  $('ab-build').innerHTML = rows.map(([k, v]) =>
    `<dt>${esc(k)}</dt><dd>${esc(v)}</dd>`).join('');

  $('ab-paths').innerHTML =
    '程序目录：<span class="mono">' + esc(a.baseDir) + '</span><br>' +
    '配置文件：<span class="mono">' + esc(a.configPath) + '</span>';

  $('ab-license').innerHTML =
    'frp 由 <b>fatedier</b> 开发，<b>Apache License 2.0</b>；' +
    '界面基于 <b>Wails v2</b>（MIT）。<br>' +
    '本程序分发的 <code>frpc.exe</code> 为官方原版二进制，未做任何修改。';
}

async function openExternal(url) {
  if (!url) return;
  try {
    await call('OpenExternal', url);
  } catch (e) {
    err('打不开链接', cleanErr(e));
  }
}

function copyText(text, okMsg) {
  const done = () => ok(okMsg || '已复制', text);
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(() => fallbackCopy(text, done));
  } else {
    fallbackCopy(text, done);
  }
}

function fallbackCopy(text, done) {
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
    done();
  } catch (e) {
    err('复制失败', '请手动选中复制：' + text);
  }
}

/* ============================== 官方管理页面 ============================== */

let panelInfo = null;

/**
 * 打开「官方管理页面」凭据弹窗。
 *
 * 这个用户名和密码是 SaveConfig 自动生成的，用户从来没见过；
 * 直接打开浏览器只会看到一个不知道怎么填的登录框。所以先亮出来。
 */
async function openPanelModal() {
  try {
    panelInfo = await call('GetPanelInfo');
  } catch (e) {
    err('读不到管理页面信息', cleanErr(e));
    return;
  }
  const p = panelInfo;
  $('pi-url').value = p.url;
  $('pi-user').value = p.user || '（未设置）';
  $('pi-pass').value = p.password || '（未设置）';

  const hints = [];
  if (!p.password) {
    hints.push('当前没有设置密码。这意味着本机上任何程序都能访问这个管理接口 —— ' +
      '因为它只监听 127.0.0.1，外网访问不到，所以风险有限。');
  }
  if (!p.localOnly) {
    hints.push('⚠ 管理接口监听在 ' + p.addr + '，不是仅本机 —— 同网段的机器也能访问它，建议改成 127.0.0.1。');
  }
  $('pi-hint').textContent = hints.join(' ');

  $('pi-state').textContent = p.running ? '管理接口正常' : 'frp 未运行，现在打不开';
  $('btn-open-panel').disabled = !p.running;
  openModal('panel-modal');
}

function bindCopy(btnId, inputId, what) {
  const b = $(btnId);
  if (!b) return;
  b.onclick = () => {
    const v = $(inputId).value;
    if (!v || v.indexOf('（') === 0) { err('没有可复制的内容'); return; }
    copyText(v, what + '已复制');
  };
}

/* ============================== 数据刷新 ============================== */

let refreshing = false;

async function refreshState(force) {
  try {
    S.state = await call('GetState');
    if (!S.config) {
      S.config = await call('GetConfig');
      fillConfigForm();
    }
    if (force) renderEnv();
  } catch (e) {
    /* frpc 未就绪时静默 */
  }
  renderTopbar();
  renderNotices();
  // 向导停在第 3 步时，让它跟着连接状态实时更新。
  // 后端状态一变就会发 frp:state（见 boot 里的事件注册），所以连上的**瞬间**
  // 这里就会被调用一次 —— 不需要额外开轮询定时器。
  if (wzStep === 3 && !$('wizard').classList.contains('hidden')) paintWizardDone(S.state);
}

async function refreshProxies(force) {
  try {
    const list = await call('ListProxies');
    S.proxies = list || [];
  } catch (e) {
    S.proxies = [];
  }
  renderProxies(force);
}

async function refreshVisitors(force) {
  try {
    const list = await call('ListVisitors');
    S.visitors = list || [];
  } catch (e) {
    S.visitors = [];
  }
  renderVisitors();
}

async function refreshAll() {
  if (refreshing) return;
  refreshing = true;
  try {
    await refreshState(true);
    await refreshProxies(true);
    await refreshVisitors(true);
    renderEnv();
  } finally {
    refreshing = false;
  }
}

async function pollLogs() {
  if (S.paused) return;
  try {
    const entries = await call('GetLogs', S.lastSeq);
    if (entries && entries.length) {
      S.lastSeq = entries[entries.length - 1].seq;
      appendLogs(entries);
    }
  } catch (e) { /* 忽略 */ }
}

/* ============================== 动作 ============================== */

async function toggleRun() {
  const st = S.state || {};
  const ours = st.processRunning;
  const external = st.externalRunning;
  const btn = $('btn-toggle-run');
  btn.disabled = true;
  try {
    if (external) {
      btn.disabled = false;
      await stopExternalFrp();
      return;
    }
    if (ours) {
      await call('StopFrp');
      ok('frp 已停止', '代理已全部下线');
    } else {
      await call('StartFrp');
      ok('frp 已启动', '正在连接服务端…');
    }
  } catch (e) {
    err('操作失败', cleanErr(e));
  } finally {
    btn.disabled = false;
    setTimeout(() => refreshAll(), 400);
  }
}

async function restartFrp() {
  const btn = $('btn-restart');
  btn.disabled = true;
  try {
    await call('RestartFrp');
    ok('frp 已重启', '正在重新连接服务端…');
  } catch (e) {
    err('重启失败', cleanErr(e));
  } finally {
    btn.disabled = false;
    setTimeout(() => refreshAll(), 800);
  }
}

async function toggleProxy(name, on, node) {
  node.classList.add('busy');
  try {
    await call('ToggleProxy', name, on);
    ok(on ? '已启用「' + name + '」' : '已停用「' + name + '」',
      on ? '正在向服务端注册…' : '服务端已下线该代理');
  } catch (e) {
    node.classList.toggle('on', !on); // 回滚
    err('操作失败', cleanErr(e));
  } finally {
    node.classList.remove('busy');
    setTimeout(() => refreshProxies(true), 500);
  }
}

async function deleteProxy(v) {
  const yes = await confirmDialog({
    title: '删除代理',
    text: `确定要删除代理「${v.name}」吗？`,
    hint: '删除后本地服务和公网访问都会断开。这个操作会立即生效，无法撤销。',
    okText: '删除',
    danger: true,
  });
  if (!yes) return;
  try {
    await call('DeleteProxy', v.name);
    ok('已删除', '「' + v.name + '」已从列表移除');
    await refreshProxies(true);
  } catch (e) {
    err('删除失败', cleanErr(e));
  }
}

async function toggleVisitor(name, on, node) {
  node.classList.add('busy');
  try {
    await call('ToggleVisitor', name, on);
    ok(on ? '已启用「' + name + '」' : '已停用「' + name + '」');
  } catch (e) {
    node.classList.toggle('on', !on);
    err('操作失败', cleanErr(e));
  } finally {
    node.classList.remove('busy');
    setTimeout(() => refreshVisitors(true), 500);
  }
}

async function deleteVisitor(v) {
  const yes = await confirmDialog({
    title: '删除访问者',
    text: `确定要删除访问者「${v.name}」吗？`,
    hint: '删除后本机到对方服务的映射会断开。',
    okText: '删除', danger: true,
  });
  if (!yes) return;
  try {
    await call('DeleteVisitor', v.name);
    ok('已删除');
    await refreshVisitors(true);
  } catch (e) {
    err('删除失败', cleanErr(e));
  }
}

async function doMigrate() {
  const st = S.state || {};
  const n = (st.fileProxies || []).length;
  if (!n) return;
  if (!canEdit()) {
    err('frp 未运行', '请先启动 frp，再执行导入');
    return;
  }
  const yes = await confirmDialog({
    title: '导入配置文件里的代理',
    text: `把 frpc.toml 里的 ${n} 个代理导入到界面管理？`,
    hint: '导入后这些代理会移到 frpc-store.json，就能在界面里单独启停和编辑了。' +
      '原来的 frpc.toml 会自动备份为 frpc.toml.bak，随时可以还原。',
    okText: '导入',
  });
  if (!yes) return;
  try {
    const res = await call('MigrateFileProxies');
    if (res && res.failed && res.failed.length) {
      err('部分导入失败', '成功 ' + res.moved + ' 个，失败：' + res.failed.join('；'));
    } else {
      ok('导入完成', '已把 ' + (res ? res.moved : n) + ' 个代理移到界面管理');
    }
    await refreshAll();
  } catch (e) {
    err('导入失败', cleanErr(e));
  }
}

async function saveConfigForm() {
  const btn = $('btn-save-config');
  const addr = $('cfg-serverAddr').value.trim();
  const token = $('cfg-authToken').value.trim();
  if (!addr) { err('保存失败', '服务器地址不能为空'); return; }
  // 令牌允许为空 —— 服务端未开启认证时就应该留空。

  const yes = await confirmDialog({
    title: '保存并重启 frp',
    text: '改动会写入 frpc.toml，并重启 frp 使其生效。',
    hint: '重启期间所有代理会短暂下线，通常几秒内恢复。',
    okText: '保存并重启',
  });
  if (!yes) return;

  btn.disabled = true;
  try {
    const cfg = Object.assign({}, S.config || {}, {
      serverAddr: addr,
      serverPort: readNum($('cfg-serverPort').value) || 0,
      authToken: token,
      user: $('cfg-user').value.trim(),
      clientID: $('cfg-clientID').value.trim(),
    });
    await call('SaveConfig', cfg);
    S.config = await call('GetConfig');
    fillConfigForm();
    ok('已保存', 'frp 正在使用新的配置重启');
    setTimeout(() => refreshAll(), 900);
  } catch (e) {
    err('保存失败', cleanErr(e));
  } finally {
    btn.disabled = false;
  }
}

async function setAutoStart(on) {
  const sw = $('sw-autostart');
  sw.classList.add('busy');
  try {
    await call('SetAutoStart', on);
    sw.classList.toggle('on', on);
    ok(on ? '已开启开机自启' : '已关闭开机自启',
      on ? '下次登录 Windows 时会自动启动本程序并拉起 frp' : '');
  } catch (e) {
    err('设置失败', cleanErr(e));
  } finally {
    sw.classList.remove('busy');
  }
}

async function showConfigText() {
  try {
    const t = await call('GetConfigText');
    $('cm-path').textContent = (S.state && S.state.configPath) || '';
    $('cm-content').value = t || '(配置文件还不存在)';
    openModal('config-modal');
  } catch (e) {
    err('读取失败', cleanErr(e));
  }
}

async function exportLogs() {
  try {
    const p = await call('ExportLogs');
    if (p) ok('日志已导出', p);
  } catch (e) {
    if (cleanErr(e) !== '已取消') err('导出失败', cleanErr(e));
  }
}

/* ============================== 事件绑定 ============================== */

function bindEvents() {
  // 导航
  document.querySelectorAll('.nav-item').forEach((b) => {
    b.onclick = () => {
      showPage(b.dataset.page);
      if (b.dataset.page === 'settings') loadSettings();
      if (b.dataset.page === 'diagnose') renderEnv();
      if (b.dataset.page === 'about') renderAbout(true);
    };
  });

  // 关于页
  $('ab-coffee').onclick = () => openModal('donate-modal');

  // 弹窗关闭
  document.querySelectorAll('[data-close]').forEach((b) => {
    b.onclick = () => closeModal(b.dataset.close);
  });
  document.querySelectorAll('.scrim').forEach((s) => {
    s.addEventListener('mousedown', (e) => { if (e.target === s) closeModal(s.id); });
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      const open = Array.prototype.find.call(
        document.querySelectorAll('.scrim'), (s) => !s.classList.contains('hidden'));
      if (open) closeModal(open.id);
    }
  });

  $('cf-ok').onclick = () => {
    const r = confirmResolve;
    confirmResolve = null;
    closeModal('confirm-modal');
    if (r) r(true);
  };

  // 顶栏
  $('btn-toggle-run').onclick = toggleRun;
  $('btn-restart').onclick = restartFrp;
  $('btn-refresh').onclick = () => { refreshAll(); pollLogs(); };

  // 代理页
  $('btn-new-proxy').onclick = () => openProxyModal(null);
  $('btn-empty-new').onclick = () => {
    if (canEdit()) openProxyModal(null);
    else toggleRun();
  };
  $('btn-migrate').onclick = doMigrate;
  $('proxy-search').oninput = () => renderProxies(true);
  $('proxy-type-filter').onchange = () => renderProxies(true);
  $('proxy-status-filter').onchange = () => renderProxies(true);

  $('proxy-rows').addEventListener('click', async (e) => {
    const t = e.target.closest('[data-act]');
    if (!t) return;
    const name = t.dataset.name;
    const v = S.proxies.find((x) => x.name === name);
    if (!v) return;
    const act = t.dataset.act;
    if (act === 'toggle') await toggleProxy(name, t.dataset.on !== '1', t);
    else if (act === 'edit') openProxyModal(v);
    else if (act === 'del') await deleteProxy(v);
  });

  // 访问者页
  $('btn-new-visitor').onclick = () => openVisitorModal(null);
  $('visitor-rows').addEventListener('click', async (e) => {
    const t = e.target.closest('[data-vact]');
    if (!t) return;
    const name = t.dataset.name;
    const v = S.visitors.find((x) => x.name === name);
    if (!v) return;
    const act = t.dataset.vact;
    if (act === 'toggle') await toggleVisitor(name, t.dataset.on !== '1', t);
    else if (act === 'edit') openVisitorModal(v);
    else if (act === 'del') await deleteVisitor(v);
  });

  // 代理弹窗内部
  $('pm-types').addEventListener('click', (e) => {
    const c = e.target.closest('.type-card');
    if (c) applyType(c.dataset.type);
  });
  document.querySelectorAll('#proxy-modal .seg[data-backend]').forEach((b) => {
    b.onclick = () => applyBackend(b.dataset.backend);
  });
  $('pf-remotePort').oninput = updateAddrPreview;
  $('pf-subdomain').oninput = updateAddrPreview;
  $('pf-localPort').oninput = updateLocalPortHint;
  $('pf-pluginType').onchange = () => renderPluginParams();
  $('btn-save-proxy').onclick = saveProxy;
  bindFolds(); // 「高级选项」等可折叠区块
  $('sw-healthcheck').onclick = () => {
    const on = !getSwitch('sw-healthcheck');
    setSwitch('sw-healthcheck', on);
    $('healthcheck-params').classList.toggle('hidden', !on);
  };
  ['sw-encryption', 'sw-compression', 'sw-natTraversal'].forEach((id) => {
    $(id).onclick = () => setSwitch(id, !getSwitch(id));
  });
  $('btn-show-secret').onclick = () => {
    const i = $('pf-secretKey');
    const show = i.type === 'password';
    i.type = show ? 'text' : 'password';
    $('btn-show-secret').textContent = show ? '隐藏' : '显示';
  };
  $('btn-gen-secret').onclick = () => {
    const chars = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    let s = '';
    const buf = new Uint32Array(24);
    (window.crypto || {}).getRandomValues
      ? window.crypto.getRandomValues(buf)
      : buf.forEach((_, i) => { buf[i] = Math.floor(Math.random() * 4294967296); });
    for (let i = 0; i < 24; i++) s += chars[buf[i] % chars.length];
    $('pf-secretKey').value = s;
    $('pf-secretKey').type = 'text';
    $('btn-show-secret').textContent = '隐藏';
  };

  // 访问者弹窗
  $('btn-save-visitor').onclick = saveVisitor;
  $('btn-show-vsecret').onclick = () => {
    const i = $('vf-secretKey');
    const show = i.type === 'password';
    i.type = show ? 'text' : 'password';
    $('btn-show-vsecret').textContent = show ? '隐藏' : '显示';
  };

  // 日志页
  $('log-search').oninput = refilterLogs;
  $('log-level-filter').onchange = refilterLogs;
  $('log-autoscroll').onchange = (e) => { S.autoscroll = e.target.checked; scrollLogsToEnd(); };
  $('btn-pause-logs').onclick = () => {
    S.paused = !S.paused;
    $('pause-logs-text').textContent = S.paused ? '继续刷新' : '暂停刷新';
    $('btn-pause-logs').className = 'btn btn-sm ' + (S.paused ? 'btn-primary' : 'btn-ghost');
    if (!S.paused) pollLogs();
  };
  $('btn-clear-logs').onclick = async () => {
    if (S.logs.length === 0) return;
    const yes = await confirmDialog({
      title: '清空日志', text: '清空界面上的日志显示？',
      hint: '只影响界面显示，磁盘上的 frpc.log 文件不受影响。',
      okText: '清空',
    });
    if (!yes) return;
    S.logs = [];
    S.lastSeq = 0;
    $('log-list').innerHTML = '';
    try { await call('ClearLogs'); } catch (e) { /* 忽略 */ }
    $('log-empty').classList.remove('hidden');
    ok('已清空');
  };
  $('btn-export-logs').onclick = exportLogs;

  // 设置页
  $('btn-show-token').onclick = () => {
    const i = $('cfg-authToken');
    const show = i.type === 'password';
    i.type = show ? 'text' : 'password';
    $('btn-show-token').textContent = show ? '隐藏' : '显示';
  };
  $('btn-save-config').onclick = saveConfigForm;
  $('btn-reload-config').onclick = async () => {
    S.config = await call('GetConfig').catch(() => S.config);
    fillConfigForm();
    ok('已还原为当前生效的配置');
  };
  $('sw-autostart').onclick = () => setAutoStart(!$('sw-autostart').classList.contains('on'));
  $('sw-close-to-tray').onclick = () => {
    $('sw-close-to-tray').classList.toggle('on');
    persistSettings();
  };
  $('sw-auto-restart').onclick = () => {
    $('sw-auto-restart').classList.toggle('on');
    persistSettings();
  };
  $('btn-open-official').onclick = openPanelModal;
  bindCopy('btn-copy-url', 'pi-url', '地址');
  bindCopy('btn-copy-user', 'pi-user', '用户名');
  bindCopy('btn-copy-pass', 'pi-pass', '密码');
  $('btn-open-panel').onclick = async () => {
    // 把密码先塞进剪贴板：浏览器弹登录框时直接 Ctrl+V 就行，
    // 省得用户在两个窗口之间来回找。
    if (panelInfo && panelInfo.password) {
      copyText(panelInfo.password, '密码已复制，粘贴到浏览器的登录框即可');
    } else {
      ok('已打开管理页面', '该页面没有设置密码');
    }
    try {
      await call('OpenOfficialUI');
    } catch (e) {
      err('打不开管理页面', cleanErr(e));
    }
  };
  $('btn-open-folder').onclick = () => call('OpenConfigFolder').catch(() => {});
  $('btn-show-config').onclick = showConfigText;

  // 诊断页
  $('btn-diagnose').onclick = runDiagnose;

  // 向导
  $('wz-next').onclick = wizardNext;
  $('wz-prev').onclick = () => { wzStep = Math.max(1, wzStep - 1); paintWizard(); };
  $('wz-show-token').onclick = () => {
    const i = $('wz-authToken');
    const show = i.type === 'password';
    i.type = show ? 'text' : 'password';
    $('wz-show-token').textContent = show ? '隐藏' : '显示';
  };
}

/* ============================== 启动 ============================== */

async function boot() {
  bindEvents();
  initChips();
  renderEnv();

  // ⚠ 只在预览工装里认 hash（预览会设 window.__PREVIEW__）。
  //   真实程序里 WebView2 会记住上次的 URL（包括 hash），
  //   结果就是"上次关在关于页，这次打开还是关于页" —— 对工具类程序不合适，
  //   每次启动都应该从「代理」开始。
  const hash = (location.hash || '').replace('#', '');
  const pages = ['proxies', 'logs', 'settings', 'visitors', 'diagnose', 'about'];
  if (window.__PREVIEW__ && pages.indexOf(hash) >= 0) {
    showPage(hash, false);
  } else {
    showPage('proxies', false);
  }

  if (!window.go) {
    $('conn-text').textContent = '后端未就绪';
    $('conn-dot').className = 'dot err';
    err('后端未就绪', '请通过程序运行界面，而不是直接用浏览器打开这个文件。');
    return;
  }

  S.ready = true;
  await refreshAll();
  await loadSettings();
  await pollLogs();
  if (S.page === 'about') await renderAbout(true);

  // 首次使用：自动进入配置向导
  const st = S.state || {};
  if (st.needSetup) setTimeout(openWizard, 500);

  // 轮询
  setInterval(() => { refreshState(); refreshProxies(); }, 2000);
  setInterval(() => { refreshVisitors(); }, 4000);
  setInterval(pollLogs, 1200);

  // 后端状态变化事件：立刻刷新，不用等轮询
  try {
    window.runtime.EventsOn('frp:state', () => { refreshState(true); });
    window.runtime.EventsOn('frp:toast', (d) => {
      if (d && d.title) toast(d.title, d.desc, d.type || 'info');
      // 托盘里改了配置，界面上的表单也要跟着更新
      call('GetConfig').then((c) => { S.config = c; fillConfigForm(); }).catch(() => {});
      call('GetAutoStart').then((a) => $('sw-autostart').classList.toggle('on', !!a)).catch(() => {});
    });
  } catch (e) { /* 没有事件系统也不影响 */ }
}

window.addEventListener('DOMContentLoaded', boot);


