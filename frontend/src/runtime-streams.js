let recentLogs = [];

function formatBytes(value) {
  let size = Number(value || 0);
  if (!Number.isFinite(size) || size <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  const digits = unit === 0 || size >= 100 ? 0 : size >= 10 ? 1 : 2;
  return `${size.toFixed(digits)} ${units[unit]}`;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function updateTraffic(snapshot = {}) {
  const upload = `${formatBytes(snapshot.uploadRate)}/s`;
  const download = `${formatBytes(snapshot.downloadRate)}/s`;
  const uploadElement = document.getElementById("trafficUpload");
  const downloadElement = document.getElementById("trafficDownload");
  const monitor = document.getElementById("trafficMonitor");
  if (uploadElement) uploadElement.textContent = `上传 ${upload}`;
  if (downloadElement) downloadElement.textContent = `下载 ${download}`;
  if (monitor) monitor.title = `实时流量：上传 ${upload} / 下载 ${download}`;
}

function renderLogPreview() {
  const target = document.getElementById("logPreviewItems");
  if (!target) return;
  const logs = recentLogs.slice(-8);
  if (!logs.length) {
    target.className = "log-preview-items empty";
    target.textContent = "暂无日志";
    return;
  }
  target.className = "log-preview-items";
  target.innerHTML = logs.map((item) => `
    <div class="log-preview-line ${item.level === "ERROR" ? "error" : ""}">
      <span>[${escapeHTML(item.level || "INFO")}]</span>
      <code>${escapeHTML(item.text)}</code>
    </div>
  `).join("");
  target.scrollTop = target.scrollHeight;
}

function appendLogs(messages) {
  const items = Array.isArray(messages) ? messages : [messages];
  recentLogs = recentLogs.concat(items.filter(Boolean)).slice(-800);
  renderLogPreview();
}

function installRuntimeStreams(retry = 0) {
  const runtime = window.runtime;
  const app = window.go?.main?.App;
  if (!runtime?.EventsOnMultiple || !app) {
    if (retry < 100) {
      window.setTimeout(() => installRuntimeStreams(retry + 1), 50);
    }
    return;
  }
  runtime.EventsOnMultiple("traffic-snapshot", updateTraffic, -1);
  runtime.EventsOnMultiple("logs-added", appendLogs, -1);
  if (app.Logs) {
    app.Logs().then((logs) => {
      recentLogs = Array.isArray(logs) ? logs.slice(-800) : [];
      renderLogPreview();
    }).catch(() => {});
  }
}

installRuntimeStreams();
