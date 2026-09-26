/* ==========================================================================
   界面预览用的假后端。
   只在 design/preview/ 下使用，不会被打包进程序。
   它模拟 window.go.main.App.* 与 window.runtime.*，
   这样可以在浏览器里用真实界面代码 + 真实 CSS 出图做视觉验证。
   ========================================================================== */
(function () {
  'use strict';

  const enabled = true;

  function ok(v) { return Promise.resolve(v); }
  function fail(msg) { return Promise.reject(new Error(msg)); }

  const CONFIG = {
    serverAddr: 'frp.cikian.cn',
    serverPort: 7500,
    authMethod: 'token',
    authToken: 'k7Jf2mQ9xLp3vN8sT5wY1zA6bC4dE0fG',
    user: 'Cikian',
    clientID: 'stargis-025',
    webAddr: '127.0.0.1',
    webPort: 7400,
    webUser: 'frpcgui',
    webPass: 'generated-panel-password',
    storePath: 'frpc-store.json',
    logTo: 'console',
    logLevel: 'info',
    logMaxDays: 3,
    loginFailExit: false,
  };

  let STATE = {
    processRunning: true,
    state: 'connected',
    stateText: '已连接到服务端',
    stateError: '',
    configured: true,
    serverAddr: 'frp.cikian.cn',
    serverPort: 7500,
    user: 'Cikian',
    exeExists: true,
    exePath: 'D:\\frp-client\\frpc.exe',
    configPath: 'D:\\frp-client\\frpc.toml',
    storeEnabled: true,
    needSetup: false,
    missingExe: false,
    fileProxies: [],
    externalRunning: false,
    // 架构。地址栏加 #emulated 可以切成"跑在模拟层里"的样子，
    // 用来预览"装错架构"那条提示长得对不对。
    processArch: 'amd64',
    nativeArch: 'amd64',
    emulated: false,
    archHint: '',
  };

  if ((location.hash || '').indexOf('emulated') >= 0) {
    STATE.processArch = 'amd64';
    STATE.nativeArch = 'arm64';
    STATE.emulated = true;
    STATE.archHint = '当前运行的是 amd64 版本，但它正跑在 ARM 电脑的模拟层里。' +
      'Windows 虽然能模拟，但 WebView2 在「ARM 系统 + x64 程序」这个组合下' +
      '有已知的崩溃问题（消息一大会死锁），而且性能有损失。' +
      '建议换成文件名带 -arm64 的那个版本。';
  }

  function def(name, type, block) {
    const d = { name: name, type: type };
    d[type] = Object.assign({ name: name, type: type }, block);
    return d;
  }

  const PROXIES = [
    {
      name: 'web', type: 'http', enabled: true,
      status: 'running', statusText: '运行中', err: '', errText: '', errKnown: false,
      localAddr: '127.0.0.1:8080', remoteAddr: 'web.frp.cikian.cn:18181', fromFile: false,
      def: def('web', 'http', {
        localIP: '127.0.0.1', localPort: 8080, subdomain: 'web',
        transport: { useEncryption: true },
      }),
    },
    {
      name: 'mysql', type: 'tcp', enabled: true,
      status: 'running', statusText: '运行中', err: '', errText: '', errKnown: false,
      localAddr: '127.0.0.1:3306', remoteAddr: 'frp.cikian.cn:20001', fromFile: false,
      def: def('mysql', 'tcp', { localIP: '127.0.0.1', localPort: 3306, remotePort: 20001 }),
    },
    {
      name: 'nas', type: 'http', enabled: false,
      status: 'disabled', statusText: '已停用', err: '', errText: '', errKnown: false,
      localAddr: '127.0.0.1:5000', remoteAddr: 'nas', fromFile: false,
      def: def('nas', 'http', { localIP: '127.0.0.1', localPort: 5000, subdomain: 'nas', enabled: false }),
    },
    {
      name: 'tcp-demo', type: 'tcp', enabled: true,
      status: 'check failed', statusText: '健康检查失败',
      err: 'do one health check failed: dial tcp 127.0.0.1:22: connectex: No connection could be made because the target machine actively refused it.',
      errText: '本机 127.0.0.1:22 拒绝连接，健康检查未通过',
      errKnown: true,
      localAddr: '127.0.0.1:22', remoteAddr: 'frp.cikian.cn:20022', fromFile: false,
      def: def('tcp-demo', 'tcp', {
        localIP: '127.0.0.1', localPort: 22, remotePort: 20022,
        healthCheck: { type: 'tcp', intervalSeconds: 10, timeoutSeconds: 3 },
      }),
    },
    {
      name: 'tcpmux-demo', type: 'tcpmux', enabled: true,
      status: 'start error', statusText: '启动失败',
      err: 'tcpmux with multiplexer httpconnect not supported because this feature is not enabled in server',
      errText: '服务端未开启 TCPMUX 功能',
      errKnown: true,
      localAddr: '127.0.0.1:9000', remoteAddr: 'mux.frp.cikian.cn:18181', fromFile: false,
      def: def('tcpmux-demo', 'tcpmux', {
        localIP: '127.0.0.1', localPort: 9000, customDomains: ['mux.frp.cikian.cn'], multiplexer: 'httpconnect',
      }),
    },
    {
      name: 'legacy-rdp', type: 'tcp', enabled: true,
      status: 'running', statusText: '运行中', err: '', errText: '', errKnown: false,
      localAddr: '127.0.0.1:3389', remoteAddr: 'frp.cikian.cn:20089', fromFile: true,
      def: { name: 'legacy-rdp', type: 'tcp', tcp: { localIP: '127.0.0.1', localPort: 3389, remotePort: 20089 } },
    },
  ];

  const VISITORS = [
    {
      name: '张三的 MySQL', type: 'stcp', enabled: true,
      status: 'enabled', statusText: '已启用', err: '',
      serverUser: 'zhangsan', serverName: 'mysql', localAddr: '127.0.0.1:13306',
      def: def('张三的 MySQL', 'stcp', {
        serverUser: 'zhangsan', serverName: 'mysql', secretKey: 'Zx8kQ2mN5pR7tV1w',
        bindAddr: '127.0.0.1', bindPort: 13306,
      }),
    },
    {
      name: '李四的内网穿透', type: 'xtcp', enabled: false,
      status: 'disabled', statusText: '已停用', err: '',
      serverUser: 'lisi', serverName: 'intranet', localAddr: '127.0.0.1:18080',
      def: def('李四的内网穿透', 'xtcp', {
        serverUser: 'lisi', serverName: 'intranet', secretKey: 'Qq3Ww4Ee5Rr6Tt7Y',
        bindAddr: '127.0.0.1', bindPort: 18080, enabled: false,
      }),
    },
  ];

  const LOGS = [
    { seq: 1, time: '20:29:26', level: 'info', scope: '', summary: 'frp 客户端已启动，开始连接服务端', detail: '', raw: '2026-09-24 20:29:26.101 [I] [client/service.go:213] start service: frpc', kind: 'started' },
    { seq: 2, time: '20:29:28', level: 'info', scope: '', summary: '连接服务端 frp.cikian.cn:7500', detail: '正在建立控制连接', raw: '2026-09-24 20:29:28.204 [I] [client/control.go:96] [0ab2ace915055641] try to connect to server...', kind: '' },
    { seq: 3, time: '20:29:29', level: 'info', scope: '', summary: '登录成功，身份 Cikian', detail: '认证通过，服务端已接受本客户端', raw: '2026-09-24 20:29:29.412 [I] [client/control.go:174] [0ab2ace915055641] login to server success, get run id [0ab2ace915055641]', kind: 'connected' },
    { seq: 4, time: '20:29:29', level: 'info', scope: 'web', summary: '代理已上线，对外地址 web.frp.cikian.cn', detail: '', raw: '2026-09-24 20:29:29.520 [I] [client/control.go:174] [0ab2ace915055641] [web] start proxy success', kind: 'proxy_up' },
    { seq: 5, time: '20:29:29', level: 'info', scope: 'mysql', summary: '代理已上线，对外端口 20001', detail: '', raw: '2026-09-24 20:29:29.612 [I] [client/control.go:174] [0ab2ace915055641] [mysql] start proxy success', kind: 'proxy_up' },
    { seq: 6, time: '20:30:39', level: 'warn', scope: 'tcp-demo', summary: '健康检查失败：本机 127.0.0.1:22 拒绝连接', detail: '该代理配置了健康检查，但本机 22 端口没有服务在监听。在服务启动前，这条代理不会被发布出去。', raw: '2026-09-24 20:30:39.421 [W] [health/health.go:128] [0ab2ace915055641] [tcp-demo] do one health check failed: dial tcp 127.0.0.1:22: connectex: No connection could be made because the target machine actively refused it.', kind: 'health_fail' },
    { seq: 7, time: '20:31:02', level: 'error', scope: 'tcpmux-demo', summary: '启动失败：服务端未开启 TCPMUX 功能', detail: '服务端没有配置 tcpmuxHTTPConnectPort，无法创建 tcpmux 类型的代理。请联系管理员开启，或删除这条代理。', raw: '2026-09-24 20:31:02.691 [W] [client/control.go:172] [0ab2ace915055641] [tcpmux-demo] start error: tcpmux with multiplexer httpconnect not supported because this feature is not enabled in server', kind: 'proxy_error' },
    { seq: 8, time: '20:33:14', level: 'info', scope: 'nas', summary: '代理已停用，服务端已下线该代理', detail: '', raw: '2026-09-24 20:33:14.118 [I] [client/control.go:174] [0ab2ace915055641] [nas] stop proxy success', kind: '' },
    { seq: 9, time: '20:35:41', level: 'debug', scope: '', summary: '心跳正常，延迟 23ms', detail: '', raw: '2026-09-24 20:35:41.002 [D] [client/control.go:240] [0ab2ace915055641] heartbeat to server, latency 23ms', kind: '' },
  ];

  let proxyState = PROXIES.map((p) => Object.assign({}, p));
  let visitorState = VISITORS.map((v) => Object.assign({}, v));

  const API = {
    GetState: () => ok(Object.assign({}, STATE)),
    ListProxies: () => ok(proxyState.map((p) => Object.assign({}, p))),
    ListVisitors: () => ok(visitorState.map((v) => Object.assign({}, v))),
    GetConfig: () => ok(Object.assign({}, CONFIG)),
    // 配置向导第 2 步会调它。没有这个方法的话向导走不到第 3 步，
    // 预览工装就没法验证「配置完成」那一页。
    // 令牌允许为空 —— 服务端没开认证时客户端也留空，两边都空才连得上。
    SaveConfig: (c) => {
      if (c && typeof c.authToken === 'string') { CONFIG.authToken = c.authToken; }
      if (c && c.serverAddr) { CONFIG.serverAddr = c.serverAddr; }
      // 模拟真实时序：frpc 要几秒后才登录成功
      setTimeout(() => {
        STATE.processRunning = true;
        STATE.state = 'connected';
        STATE.stateText = '已连接到服务端';
        STATE.needSetup = false;
      }, 3000);
      return ok(null);
    },
    GetAboutInfo: () => ok({
      appName: 'frp 客户端',
      version: '1.0.0',
      buildTime: '2026/9/25 3:40:12',
      author: 'Cikian',
      email: 'cikian@126.com',
      website: 'https://cikian.cn',
      github: 'https://github.com/Cikian',
      gitee: 'https://gitee.com/Cikian',
      tagline: 'COURAGE ZENITH JOURNEY',
      frpVersion: '0.71.0',
      goVersion: 'go1.25.0',
      wailsVersion: 'v2.16.0',
      platform: 'windows / amd64',
      processArch: STATE.processArch,
      nativeArch: STATE.nativeArch,
      emulated: STATE.emulated,
      vcsRevision: '本地构建',
      vcsTime: '2026/9/25 3:40:12',
      license: 'Apache License 2.0',
      baseDir: 'D:\\Program Files\\frpc-gui',
      exePath: 'D:\\Program Files\\frpc-gui\\frp客户端.exe',
      configPath: 'D:\\Program Files\\frpc-gui\\frpc.toml',
      uptime: '2 小时 13 分',
      frpState: '已连接到服务端',
      proxyCount: 6,
      visitorCount: 2,
    }),
    OpenExternal: () => ok(null),
    GetPanelInfo: () => ok({
      url: 'http://127.0.0.1:7400',
      user: 'frpcgui',
      password: 'oFW1vIPOPZGrRSUg4zPlL5Yx',
      addr: '127.0.0.1',
      port: 7400,
      running: true,
      localOnly: true,
    }),
    OpenOfficialUI: () => ok(null),
    GetSettings: () => ok({ closeToTray: false, autoRestart: true }),
    GetAutoStart: () => ok(true),
    SaveSettings: () => ok(null),
    SetAutoStart: () => ok(null),
    GetLogs: (after) => ok(LOGS.filter((l) => l.seq > after)),
    ClearLogs: () => ok(null),
    GetConfigText: () => ok(
      'serverAddr = "frp.cikian.cn"\n' +
      'serverPort = 7500\n\n' +
      'user = "Cikian"\n' +
      'clientID = "stargis-025"\n\n' +
      '# 这是访问服务器的钥匙，请勿外传。\n' +
      'auth.method = "token"\n' +
      'auth.token = "k7Jf2mQ9xLp3vN8sT5wY1zA6bC4dE0fG"\n\n' +
      'loginFailExit = false\n\n' +
      'webServer.addr = "127.0.0.1"\n' +
      'webServer.port = 7400\n' +
      'webServer.user = "frpcgui"\n' +
      'webServer.pprofEnable = false\n\n' +
      'store.path = "frpc-store.json"\n\n' +
      'log.to = "console"\n' +
      'log.level = "info"\n' +
      'log.maxDays = 3\n'),
    CheckLocalPort: (port) => ok(port === 8080 || port === 3306 || port === 3389),
    StartFrp: () => { STATE.processRunning = true; STATE.state = 'starting'; STATE.stateText = '正在连接…'; return ok(null); },
    StopFrp: () => { STATE.processRunning = false; STATE.state = 'stopped'; STATE.stateText = '未运行'; return ok(null); },
    RestartFrp: () => ok(null),
    StopExternalFrp: () => { STATE.externalRunning = false; return ok(null); },
    ToggleProxy: (name, on) => {
      const p = proxyState.find((x) => x.name === name);
      if (p) { p.enabled = on; p.status = on ? 'running' : 'disabled'; p.statusText = on ? '运行中' : '已停用'; }
      return ok(null);
    },
    ToggleVisitor: (name, on) => {
      const v = visitorState.find((x) => x.name === name);
      if (v) { v.enabled = on; v.status = on ? 'enabled' : 'disabled'; v.statusText = on ? '已启用' : '已停用'; }
      return ok(null);
    },
    DeleteProxy: (name) => { proxyState = proxyState.filter((x) => x.name !== name); return ok(null); },
    DeleteVisitor: (name) => { visitorState = visitorState.filter((x) => x.name !== name); return ok(null); },
    SaveProxy: () => ok(null),
    SaveVisitor: () => ok(null),
    MigrateFileProxies: () => ok({ moved: 1, failed: [] }),
    Diagnose: () => ok([
      { name: '程序文件', ok: true, warn: false, message: 'frpc.exe 就位' },
      { name: '配置', ok: true, warn: false, message: '服务器 frp.cikian.cn:7500，身份 Cikian' },
      { name: '服务端连接', ok: true, warn: false, message: '已连接，认证通过' },
      { name: '代理 tcp-demo', ok: false, warn: false, message: '本机 127.0.0.1:22 拒绝连接，健康检查未通过' },
    ]),
    OpenOfficialUI: () => ok(null),
    OpenConfigFolder: () => ok(null),
    ExportLogs: () => ok('D:\\frp-client\\frpc-logs.txt'),
  };

  window.go = { main: { App: {} } };
  Object.keys(API).forEach((k) => { window.go.main.App[k] = API[k]; });

  // 告诉界面"这里是预览工装"，这样它才会认 URL 里的 hash 来切页
  window.__PREVIEW__ = true;

  window.runtime = {
    EventsOn: function () {},
    EventsEmit: function () {},
  };

  // 预览特殊处理：按 hash 打开对应弹窗
  window.addEventListener('DOMContentLoaded', function () {
    const h = (location.hash || '').replace('#', '');
    setTimeout(function () {
      if (h === 'modal-tcp') openProxyModal(null);
      else if (h === 'modal-http') openProxyModal(proxyState[0]);
      else if (h === 'modal-stcp') openProxyModal({
        name: 'private-db', type: 'stcp', enabled: true, status: 'running', statusText: '运行中',
        err: '', errText: '', localAddr: '127.0.0.1:3306', remoteAddr: '', fromFile: false,
        def: { name: 'private-db', type: 'stcp', stcp: { localIP: '127.0.0.1', localPort: 3306, secretKey: 'Zx8kQ2mN5pR7tV1w', allowUsers: ['zhangsan'] } },
      });
      else if (h === 'visitor-modal') openVisitorModal(visitorState[0]);
      else if (h === 'wizard') openWizard();
      else if (h === 'donate') openModal('donate-modal');
      else if (h === 'panel') openPanelModal();
    }, 600);
  });
})();
