const localSpeedtestState = {
  running: false,
  output: [],
  progressIndex: -1,
  latencyPhase: ""
};

let localSpeedtestSubscribed = false;
let localSpeedtestObserver = null;

function localSpeedtestApp() {
  return window.go?.main?.App;
}

function localSpeedtestRuntime() {
  return window.runtime;
}

function localSpeedtestToast(message) {
  const toast = document.getElementById("toast");
  if (!toast) {
    return;
  }
  toast.innerHTML = `<span class="toast-icon" aria-hidden="true">OK</span><span>${escapeLocalSpeedtestText(message)}</span>`;
  toast.classList.add("show");
  clearTimeout(window.__toastTimer);
  window.__toastTimer = setTimeout(() => toast.classList.remove("show"), String(message).length > 24 ? 6000 : 2600);
}

function escapeLocalSpeedtestText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function appendLocalSpeedtestOutput(line) {
  const text = formatLocalSpeedtestLine(line);
  if (!text) {
    return;
  }
  if (isLocalSpeedtestProgress(text)) {
    if (localSpeedtestState.progressIndex >= 0 && localSpeedtestState.progressIndex < localSpeedtestState.output.length) {
      localSpeedtestState.output[localSpeedtestState.progressIndex] = text;
    } else {
      localSpeedtestState.output.push(text);
      localSpeedtestState.progressIndex = localSpeedtestState.output.length - 1;
    }
  } else {
    localSpeedtestState.output.push(text);
    localSpeedtestState.progressIndex = -1;
  }
  if (text.startsWith("下载速度：")) {
    localSpeedtestState.latencyPhase = "download";
  } else if (text.startsWith("上传速度：")) {
    localSpeedtestState.latencyPhase = "upload";
  } else if (text.startsWith("下载延迟：") || text.startsWith("上传延迟：") || text.startsWith("丢包率：")) {
    localSpeedtestState.latencyPhase = "";
  }
  if (localSpeedtestState.output.length > 600) {
    localSpeedtestState.output = localSpeedtestState.output.slice(-600);
    localSpeedtestState.progressIndex = -1;
  }
  if (text.includes("本地测速已结束") || text.includes("已停止") || text.includes("执行失败")) {
    localSpeedtestState.running = false;
    updateLocalSpeedtestButtons();
  }
  renderLocalSpeedtestOutput();
}

function isLocalSpeedtestProgress(text) {
  return text.startsWith("下载进度：") || text.startsWith("上传进度：");
}

function resetSpeedtestMetricCards() {
  const dl = document.getElementById("speedtestDownloadVal");
  if (dl) dl.innerHTML = `-- <span class="unit">Mbps</span>`;
  const ul = document.getElementById("speedtestUploadVal");
  if (ul) ul.innerHTML = `-- <span class="unit">Mbps</span>`;
  const ping = document.getElementById("speedtestPingVal");
  if (ping) ping.innerHTML = `-- <span class="unit">ms</span>`;
  const server = document.getElementById("speedtestServerVal");
  if (server) server.textContent = `--`;
}

function updateSpeedtestMetric(type, value) {
  if (type === "download") {
    const el = document.getElementById("speedtestDownloadVal");
    if (el) el.innerHTML = `${value} <span class="unit">Mbps</span>`;
  } else if (type === "upload") {
    const el = document.getElementById("speedtestUploadVal");
    if (el) el.innerHTML = `${value} <span class="unit">Mbps</span>`;
  } else if (type === "latency") {
    const el = document.getElementById("speedtestPingVal");
    if (el) el.innerHTML = `${value} <span class="unit">ms</span>`;
  } else if (type === "server") {
    const el = document.getElementById("speedtestServerVal");
    if (el) el.textContent = value;
  }
}

function formatLocalSpeedtestLine(line) {
  const raw = String(line || "").replace(/\u001b\[[0-?]*[ -/]*[@-~]/g, "").trim();
  if (!raw) {
    return "";
  }
  let match = raw.match(/^Server:\s*(.+?)\s*\(id:\s*(\d+)\)/i);
  if (match) {
    updateSpeedtestMetric("server", match[1]);
    return `测速服务器：${match[1]}（ID：${match[2]}）`;
  }
  match = raw.match(/^ISP:\s*(.+)$/i);
  if (match) {
    const sEl = document.getElementById("speedtestServerVal");
    if (sEl && sEl.textContent !== "--") {
      sEl.textContent = `${sEl.textContent} / ${match[1]}`;
    } else {
      updateSpeedtestMetric("server", match[1]);
    }
    return `运营商：${match[1]}`;
  }
  match = raw.match(/^Idle Latency:\s*([0-9.]+)\s*ms(.*)$/i);
  if (match) {
    updateSpeedtestMetric("latency", match[1]);
    return `空闲延迟：${formatLocalSpeedtestLatencyDetail(match[1] + " ms", match[2] || "")}`;
  }
  match = raw.match(/^Download:\s*([0-9.]+)\s*Mbps.*?(\d+)%/i);
  if (match) {
    updateSpeedtestMetric("download", match[1]);
    return `下载进度：${match[2]}% / ${match[1]} Mbps`;
  }
  match = raw.match(/^Upload:\s*([0-9.]+)\s*Mbps.*?(\d+)%/i);
  if (match) {
    updateSpeedtestMetric("upload", match[1]);
    return `上传进度：${match[2]}% / ${match[1]} Mbps`;
  }
  match = raw.match(/^Download:\s*([0-9.]+)\s*Mbps(.*)$/i);
  if (match) {
    updateSpeedtestMetric("download", match[1]);
    return `下载速度：${match[1]} Mbps${match[2] || ""}`;
  }
  match = raw.match(/^Upload:\s*([0-9.]+)\s*Mbps(.*)$/i);
  if (match) {
    updateSpeedtestMetric("upload", match[1]);
    return `上传速度：${match[1]} Mbps${match[2] || ""}`;
  }
  match = raw.match(/^Packet Loss:\s*(.+)$/i);
  if (match) {
    return `丢包率：${match[1]}`;
  }
  match = raw.match(/^([0-9.]+\s*ms)\s+\(jitter:\s*([0-9.]+ms),\s*low:\s*([0-9.]+ms),\s*high:\s*([0-9.]+ms)\)$/i);
  if (match) {
    const label = localSpeedtestState.latencyPhase === "upload" ? "上传延迟" : "下载延迟";
    return `${label}：${match[1]}（抖动：${match[2]}，最低：${match[3]}，最高：${match[4]}）`;
  }
  match = raw.match(/^Result URL:\s*(.+)$/i);
  if (match) {
    return `结果链接：${match[1]}`;
  }
  if (/License acceptance recorded/i.test(raw)) {
    return "许可已确认，继续测速";
  }
  if (/Speedtest by Ookla/i.test(raw)) {
    return "Ookla Speedtest 已启动";
  }
  if (/^\[error\]/i.test(raw)) {
    return `提示：${raw.replace(/^\[error\]\s*/i, "")}`;
  }
  return raw
    .replace(/^Error:/i, "错误：")
    .replace(/^Cannot read:/i, "无法读取：")
    .replace(/^Unknown error/i, "未知错误");
}

function formatLocalSpeedtestLatencyDetail(value, detail) {
  const match = String(detail || "").match(/^\s*\(jitter:\s*([0-9.]+ms),\s*low:\s*([0-9.]+ms),\s*high:\s*([0-9.]+ms)\)$/i);
  if (!match) {
    return `${value}${detail || ""}`;
  }
  return `${value}（抖动：${match[1]}，最低：${match[2]}，最高：${match[3]}）`;
}

function renderLocalSpeedtestOutput() {
  const output = document.getElementById("localSpeedtestOutput");
  if (!output) {
    return;
  }
  output.textContent = localSpeedtestState.output.length ? localSpeedtestState.output.join("\n") : "暂无输出";
  output.scrollTop = output.scrollHeight;
}

function updateLocalSpeedtestButtons() {
  const start = document.getElementById("startLocalSpeedtest");
  const stop = document.getElementById("stopLocalSpeedtest");
  if (start) {
    start.disabled = localSpeedtestState.running;
    start.textContent = localSpeedtestState.running ? "测速中" : "开始测速";
  }
  if (stop) {
    stop.disabled = !localSpeedtestState.running;
  }
}

function getLocalSpeedtestNetworkMode() {
  return document.getElementById("localSpeedtestNetworkMode")?.value || "direct";
}

function localSpeedtestNetworkModeName(mode = getLocalSpeedtestNetworkMode()) {
  if (mode === "proxy") {
    return "当前网络/TUN";
  }
  if (mode === "http_proxy") {
    return "HTTP代理";
  }
  return "本机直连";
}

function resetLocalSpeedtestServerSelect(message = "") {
  const select = document.getElementById("localSpeedtestServerSelect");
  if (select) {
    select.innerHTML = '<option value="">自动选择测速服务器</option>';
  }
  if (message) {
    appendLocalSpeedtestOutput(message);
  }
}

async function startLocalSpeedtest() {
  const app = localSpeedtestApp();
  if (!app?.StartLocalSpeedtest || localSpeedtestState.running) {
    return;
  }
  localSpeedtestState.running = true;
  localSpeedtestState.output = [];
  localSpeedtestState.progressIndex = -1;
  localSpeedtestState.latencyPhase = "";
  resetSpeedtestMetricCards();
  appendLocalSpeedtestOutput("正在启动本地测速");
  updateLocalSpeedtestButtons();
  try {
    const result = await app.StartLocalSpeedtest({
      serverId: document.getElementById("localSpeedtestServerSelect")?.value || "",
      networkMode: getLocalSpeedtestNetworkMode()
    });
    appendLocalSpeedtestOutput(`命令：${result?.command || ""}`);
    localSpeedtestToast("本地测速已开始");
  } catch (error) {
    localSpeedtestState.running = false;
    appendLocalSpeedtestOutput(`启动失败：${error?.message || error || "操作失败"}`);
    updateLocalSpeedtestButtons();
    localSpeedtestToast(error?.message || String(error || "本地测速启动失败"));
  }
}

function renderLocalSpeedtestServers(servers) {
  const select = document.getElementById("localSpeedtestServerSelect");
  if (!select) {
    return;
  }
  select.innerHTML = '<option value="">自动选择测速服务器</option>';
  for (const server of servers || []) {
    const option = document.createElement("option");
    option.value = server.id || "";
    option.textContent = formatLocalSpeedtestServerName(server);
    select.appendChild(option);
  }
}

function formatLocalSpeedtestServerName(server) {
  const name = translateLocalSpeedtestServerName(server?.name);
  const location = translateLocalSpeedtestLocation(server?.location);
  const country = translateLocalSpeedtestCountry(server?.country);
  return `${name} - ${location}（${country}，ID：${server?.id || ""}）`;
}

function translateLocalSpeedtestServerName(value) {
  const text = String(value || "").trim();
  const normalized = text.toLowerCase();
  const names = {
    "zhejiang telecom": "浙江电信",
    "china telecom": "中国电信",
    "china mobile": "中国移动",
    "china unicom": "中国联通",
    "fpt telecom": "FPT 电信",
    "viettel idc": "Viettel 数据中心",
    "viettel network": "Viettel 网络",
    "cmc telecom": "CMC 电信",
    "mobifone": "MobiFone",
    "vnpt-net": "VNPT 网络",
    "vietpn co, ltd": "VIETPN 公司",
    "vpsttt group company ltd": "VPSTTT 集团"
  };
  return names[normalized] || text || "未知服务器";
}

function translateLocalSpeedtestLocation(value) {
  const text = String(value || "").trim();
  const normalized = text.toLowerCase().replace(/[\s_-]+/g, "");
  const locations = {
    hangzhou: "杭州",
    haiphong: "海防",
    hanoi: "河内",
    hochiminh: "胡志明市",
    danang: "岘港",
    guangzhou: "广州",
    shanghai: "上海",
    beijing: "北京",
    shenzhen: "深圳",
    hongkong: "香港",
    taipei: "台北",
    tokyo: "东京",
    osaka: "大阪",
    seoul: "首尔",
    singapore: "新加坡",
    bangkok: "曼谷",
    kualalumpur: "吉隆坡",
    manila: "马尼拉",
    jakarta: "雅加达",
    losangeles: "洛杉矶",
    sanjose: "圣何塞",
    newyork: "纽约",
    london: "伦敦",
    frankfurt: "法兰克福",
    paris: "巴黎"
  };
  return locations[normalized] || text || "未知地区";
}

function translateLocalSpeedtestCountry(value) {
  const text = String(value || "").trim();
  const normalized = text.toLowerCase().replace(/[\s_-]+/g, "");
  const countries = {
    china: "中国",
    vietnam: "越南",
    hongkong: "中国香港",
    taiwan: "中国台湾",
    japan: "日本",
    southkorea: "韩国",
    korea: "韩国",
    singapore: "新加坡",
    thailand: "泰国",
    malaysia: "马来西亚",
    philippines: "菲律宾",
    indonesia: "印度尼西亚",
    unitedstates: "美国",
    usa: "美国",
    unitedkingdom: "英国",
    germany: "德国",
    france: "法国",
    canada: "加拿大",
    australia: "澳大利亚",
    india: "印度",
    russia: "俄罗斯"
  };
  return countries[normalized] || text || "未知国家";
}

async function loadLocalSpeedtestServers() {
  const app = localSpeedtestApp();
  const button = document.getElementById("loadLocalSpeedtestServers");
  if (!app?.ListLocalSpeedtestServers || button?.disabled) {
    return;
  }
  if (button) {
    button.disabled = true;
    button.textContent = "获取中";
  }
  const mode = getLocalSpeedtestNetworkMode();
  appendLocalSpeedtestOutput(`正在按「${localSpeedtestNetworkModeName(mode)}」获取测速服务器`);
  try {
    const servers = await app.ListLocalSpeedtestServers({
      networkMode: mode
    });
    renderLocalSpeedtestServers(servers);
    localSpeedtestToast(`已按${localSpeedtestNetworkModeName(mode)}获取 ${servers?.length || 0} 个测速服务器`);
  } catch (error) {
    const message = error?.message || String(error || "获取服务器失败");
    appendLocalSpeedtestOutput(`获取服务器失败：${message}`);
    localSpeedtestToast(message);
  } finally {
    if (button) {
      button.disabled = false;
      button.textContent = "获取服务器";
    }
  }
}

async function stopLocalSpeedtest() {
  const app = localSpeedtestApp();
  if (!app?.StopLocalSpeedtest || !localSpeedtestState.running) {
    return;
  }
  try {
    await app.StopLocalSpeedtest();
  } catch (error) {
    localSpeedtestToast(error?.message || String(error || "停止本地测速失败"));
  }
}

function showLocalSpeedtestTab() {
  document.querySelectorAll("[data-tester-tab]").forEach((button) => {
    button.classList.toggle("active", button.dataset.testerTab === "localSpeedtest");
  });
  document.querySelectorAll(".tester-tab-page").forEach((page) => {
    page.classList.toggle("hidden", page.id !== "localSpeedtestTab");
  });
}

function subscribeLocalSpeedtestOutput() {
  if (localSpeedtestSubscribed) {
    return;
  }
  localSpeedtestRuntime()?.EventsOnMultiple?.("local-speedtest-output", appendLocalSpeedtestOutput, -1);
  localSpeedtestSubscribed = true;
}

function bindLocalSpeedtestControls() {
  const tabButton = document.getElementById("localSpeedtestTabButton");
  const panel = document.getElementById("localSpeedtestTab");
  if (!tabButton || !panel) {
    return false;
  }
  if (tabButton.dataset.localSpeedtestBound !== "1") {
    tabButton.dataset.localSpeedtestBound = "1";
    tabButton.addEventListener("click", showLocalSpeedtestTab);
  }
  const start = document.getElementById("startLocalSpeedtest");
  if (start && start.dataset.localSpeedtestBound !== "1") {
    start.dataset.localSpeedtestBound = "1";
    start.addEventListener("click", startLocalSpeedtest);
  }
  const stop = document.getElementById("stopLocalSpeedtest");
  if (stop && stop.dataset.localSpeedtestBound !== "1") {
    stop.dataset.localSpeedtestBound = "1";
    stop.addEventListener("click", stopLocalSpeedtest);
  }
  const clear = document.getElementById("clearLocalSpeedtestOutput");
  if (clear && clear.dataset.localSpeedtestBound !== "1") {
    clear.dataset.localSpeedtestBound = "1";
    clear.addEventListener("click", () => {
      localSpeedtestState.output = [];
      localSpeedtestState.progressIndex = -1;
      localSpeedtestState.latencyPhase = "";
      resetSpeedtestMetricCards();
      renderLocalSpeedtestOutput();
    });
  }
  const loadServers = document.getElementById("loadLocalSpeedtestServers");
  if (loadServers && loadServers.dataset.localSpeedtestBound !== "1") {
    loadServers.dataset.localSpeedtestBound = "1";
    loadServers.addEventListener("click", loadLocalSpeedtestServers);
  }
  const serverSelect = document.getElementById("localSpeedtestServerSelect");
  if (serverSelect && serverSelect.dataset.localSpeedtestBound !== "1") {
    serverSelect.dataset.localSpeedtestBound = "1";
  }
  const networkMode = document.getElementById("localSpeedtestNetworkMode");
  if (networkMode && networkMode.dataset.localSpeedtestBound !== "1") {
    networkMode.dataset.localSpeedtestBound = "1";
    networkMode.addEventListener("change", () => {
      resetLocalSpeedtestServerSelect(`已切换到「${localSpeedtestNetworkModeName()}」，请重新点击获取服务器`);
    });
  }
  subscribeLocalSpeedtestOutput();
  return true;
}

function installLocalSpeedtestUI() {
  return bindLocalSpeedtestControls();
}

function initLocalSpeedtest() {
  const installed = installLocalSpeedtestUI();
  if (installed) {
    localSpeedtestObserver?.disconnect();
    localSpeedtestObserver = null;
  }
}

function watchLocalSpeedtestUI() {
  initLocalSpeedtest();
  if (document.getElementById("localSpeedtestTabButton")) {
    return;
  }
  localSpeedtestObserver?.disconnect();
  localSpeedtestObserver = new MutationObserver(initLocalSpeedtest);
  localSpeedtestObserver.observe(document.documentElement, {
    childList: true,
    subtree: true
  });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", watchLocalSpeedtestUI);
} else {
  watchLocalSpeedtestUI();
}
