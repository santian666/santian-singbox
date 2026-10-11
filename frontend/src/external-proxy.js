const externalProxyState = {
  endpoints: [],
  testing: new Map(),
  saving: false,
  loading: false,
  reloadPending: false,
  reloadTimer: 0,
};

function externalProxyAPI(name) {
  return window.go?.main?.App?.[name];
}

function endpointKey(endpoint) {
  return `${String(endpoint.host || "").toLowerCase()}:${Number(endpoint.port || 0)}`;
}

function endpointAddress(endpoint) {
  const host = String(endpoint.host || "");
  return host.includes(":") ? `[${host}]:${endpoint.port}` : `${host}:${endpoint.port}`;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function errorText(error) {
  return error?.message ? error.message : String(error || "操作失败");
}

function setPanelStatus(text, type = "") {
  const element = document.getElementById("externalProxyStatus");
  if (!element) return;
  element.textContent = text || "";
  element.dataset.type = type;
}

function setSaving(value) {
  externalProxyState.saving = value;
  const addButton = document.getElementById("externalProxyAdd");
  if (addButton) {
    addButton.disabled = value;
    addButton.textContent = value ? "保存中" : "添加";
  }
  renderExternalProxyEndpoints();
}

function renderExternalProxyEndpoints() {
  const list = document.getElementById("externalProxyList");
  const count = document.getElementById("externalProxyCount");
  if (!list || !count) return;
  count.textContent = `${externalProxyState.endpoints.length} 个`;
	if (!externalProxyState.endpoints.length) {
		list.className = "external-proxy-list empty";
		list.textContent = "尚未配置，BitBrowser 的远程 SOCKS 将按当前节点转发";
    return;
  }
  list.className = "external-proxy-list";
  list.innerHTML = externalProxyState.endpoints.map((endpoint, index) => {
    const key = endpointKey(endpoint);
    const status = externalProxyState.testing.get(key);
    const statusText = status?.text || "未检测";
    const statusType = status?.type || "";
    return `
      <div class="external-proxy-item" data-external-proxy-index="${index}">
        <label class="external-proxy-enabled" title="启用入口直连">
          <input type="checkbox" data-external-proxy-toggle="${index}" ${endpoint.enabled ? "checked" : ""} ${externalProxyState.saving ? "disabled" : ""} />
          <span aria-hidden="true"></span>
        </label>
        <strong title="${escapeHTML(endpointAddress(endpoint))}">${escapeHTML(endpointAddress(endpoint))}</strong>
        <span class="external-proxy-result" data-type="${escapeHTML(statusType)}" title="${escapeHTML(statusText)}">${escapeHTML(statusText)}</span>
        <button class="ghost small" data-external-proxy-test="${index}" type="button" ${externalProxyState.saving || !endpoint.enabled || statusType === "testing" ? "disabled" : ""}>${statusType === "testing" ? "检测中" : "检测"}</button>
        <button class="circle-close external-proxy-delete" data-external-proxy-delete="${index}" type="button" aria-label="删除外部代理入口" title="删除" ${externalProxyState.saving ? "disabled" : ""}>×</button>
      </div>`;
  }).join("");
  bindExternalProxyListActions();
}

async function loadExternalProxyEndpoints() {
  const stateAPI = externalProxyAPI("State");
  if (!stateAPI) return;
  if (externalProxyState.saving || externalProxyState.loading) {
    externalProxyState.reloadPending = true;
    return;
  }
  externalProxyState.loading = true;
  try {
    const state = await stateAPI();
    externalProxyState.endpoints = Array.isArray(state?.settings?.externalProxyEndpoints)
      ? state.settings.externalProxyEndpoints.map(endpoint => ({ ...endpoint }))
      : [];
    window.__sanTianExternalProxyEndpoints = externalProxyState.endpoints.map(endpoint => ({ ...endpoint }));
    renderExternalProxyEndpoints();
  } catch (error) {
    setPanelStatus(errorText(error), "error");
  } finally {
    externalProxyState.loading = false;
    if (externalProxyState.reloadPending && !externalProxyState.saving) {
      externalProxyState.reloadPending = false;
      scheduleExternalProxyReload(0);
    }
  }
}

function scheduleExternalProxyReload(delay = 220) {
  clearTimeout(externalProxyState.reloadTimer);
  externalProxyState.reloadTimer = window.setTimeout(() => {
    externalProxyState.reloadTimer = 0;
    loadExternalProxyEndpoints();
  }, delay);
}

async function saveExternalProxyEndpoints(endpoints, successText) {
  const saveAPI = externalProxyAPI("SaveExternalProxyEndpoints");
  if (!saveAPI) throw new Error("外部代理入口接口未就绪，请重启开发版");
  setSaving(true);
  try {
    const saved = await saveAPI(endpoints);
    externalProxyState.endpoints = Array.isArray(saved) ? saved.map(endpoint => ({ ...endpoint })) : [];
    window.__sanTianExternalProxyEndpoints = externalProxyState.endpoints.map(endpoint => ({ ...endpoint }));
    setPanelStatus(successText, "success");
    renderExternalProxyEndpoints();
  } finally {
    setSaving(false);
    if (externalProxyState.reloadPending) {
      externalProxyState.reloadPending = false;
      scheduleExternalProxyReload(0);
    }
  }
}

async function testExternalProxyEndpoint(index) {
  const endpoint = externalProxyState.endpoints[index];
  const testAPI = externalProxyAPI("TestExternalProxyEndpoint");
  if (!endpoint || !testAPI) return;
  const key = endpointKey(endpoint);
  externalProxyState.testing.set(key, { type: "testing", text: "正在检测远程 SOCKS5" });
  renderExternalProxyEndpoints();
  try {
    const result = await testAPI(endpoint);
    const text = `${result.message} · ${result.path} · ${result.latencyMs} ms`;
    externalProxyState.testing.set(key, { type: "success", text });
    setPanelStatus(text, "success");
  } catch (error) {
    const text = errorText(error);
    externalProxyState.testing.set(key, { type: "error", text });
    setPanelStatus(text, "error");
  }
  renderExternalProxyEndpoints();
}

function confirmExternalProxyDelete(endpoint) {
  const dialog = document.getElementById("externalProxyDeleteDialog");
  const message = document.getElementById("externalProxyDeleteMessage");
  if (!dialog || !message) return Promise.resolve(false);
  message.textContent = `确定删除“${endpointAddress(endpoint)}”？`;
  dialog.classList.remove("hidden");
  return new Promise(resolve => {
    const cancel = document.getElementById("externalProxyDeleteCancel");
    const confirm = document.getElementById("externalProxyDeleteConfirm");
    const finish = value => {
      dialog.classList.add("hidden");
      cancel.onclick = null;
      confirm.onclick = null;
      resolve(value);
    };
    cancel.onclick = () => finish(false);
    confirm.onclick = () => finish(true);
  });
}

function bindExternalProxyListActions() {
  document.querySelectorAll("[data-external-proxy-toggle]").forEach(input => {
    input.onchange = async () => {
      const index = Number(input.dataset.externalProxyToggle);
      const endpoints = externalProxyState.endpoints.map(endpoint => ({ ...endpoint }));
      endpoints[index].enabled = input.checked;
      try {
        await saveExternalProxyEndpoints(endpoints, input.checked ? "外部代理入口直连已启用" : "外部代理入口直连已停用");
      } catch (error) {
        setPanelStatus(errorText(error), "error");
        await loadExternalProxyEndpoints();
      }
    };
  });
  document.querySelectorAll("[data-external-proxy-test]").forEach(button => {
    button.onclick = () => testExternalProxyEndpoint(Number(button.dataset.externalProxyTest));
  });
  document.querySelectorAll("[data-external-proxy-delete]").forEach(button => {
    button.onclick = async () => {
      const index = Number(button.dataset.externalProxyDelete);
      const endpoint = externalProxyState.endpoints[index];
      if (!endpoint || !await confirmExternalProxyDelete(endpoint)) return;
      const endpoints = externalProxyState.endpoints.filter((_, itemIndex) => itemIndex !== index);
      try {
        await saveExternalProxyEndpoints(endpoints, "外部代理入口已删除");
        externalProxyState.testing.delete(endpointKey(endpoint));
      } catch (error) {
        setPanelStatus(errorText(error), "error");
      }
    };
  });
}

async function addExternalProxyEndpoint() {
  const hostInput = document.getElementById("externalProxyHost");
  const portInput = document.getElementById("externalProxyPort");
  const host = String(hostInput?.value || "").trim();
  const port = Number(portInput?.value || 0);
  if (!host || !Number.isInteger(port) || port < 1 || port > 65535) {
    setPanelStatus("请输入正确的代理主机和端口", "error");
    return;
  }
  const endpoint = { host, port, enabled: true, resolvedIps: [] };
  if (externalProxyState.endpoints.some(item => endpointKey(item) === endpointKey(endpoint))) {
    setPanelStatus("该外部代理入口已经存在", "error");
    return;
  }
  try {
    await saveExternalProxyEndpoints([...externalProxyState.endpoints, endpoint], "外部代理入口已添加并启用直连");
    hostInput.value = "";
    portInput.value = "";
    const index = externalProxyState.endpoints.findIndex(item => endpointKey(item) === endpointKey(endpoint));
    if (index >= 0) await testExternalProxyEndpoint(index);
  } catch (error) {
    setPanelStatus(errorText(error), "error");
  }
}

function mountExternalProxyPanel() {
  const page = document.getElementById("proxyMapPage");
  const mapPanel = page?.querySelector(".proxy-map-panel");
  if (!page || !mapPanel || document.getElementById("externalProxyPanel")) return false;
  const panel = document.createElement("section");
  panel.id = "externalProxyPanel";
  panel.className = "panel external-proxy-panel";
  panel.innerHTML = `
    <div class="panel-head external-proxy-head">
      <div>
        <h2>指纹浏览器外部代理入口</h2>
		<span id="externalProxyStatus" class="external-proxy-status">不会自动读取 BitBrowser，请填写其中使用的远程 SOCKS 主机和端口</span>
      </div>
      <span id="externalProxyCount" class="muted">0 个</span>
    </div>
    <div class="external-proxy-form">
      <input id="externalProxyHost" type="text" placeholder="远程 SOCKS 域名或 IP" autocomplete="off" />
      <input id="externalProxyPort" type="number" min="1" max="65535" placeholder="端口" />
      <button id="externalProxyAdd" class="primary" type="button">添加</button>
    </div>
	<div id="externalProxyList" class="external-proxy-list empty">尚未配置，BitBrowser 的远程 SOCKS 将按当前节点转发</div>`;
  mapPanel.before(panel);

  const dialog = document.createElement("div");
  dialog.id = "externalProxyDeleteDialog";
  dialog.className = "confirm-dialog hidden";
  dialog.setAttribute("role", "dialog");
  dialog.setAttribute("aria-modal", "true");
  dialog.innerHTML = `
    <div class="confirm-card">
      <div class="confirm-content">
        <h2>删除外部代理入口</h2>
        <p id="externalProxyDeleteMessage"></p>
        <span>删除后该主机和端口将恢复使用当前代理节点。</span>
      </div>
      <div class="confirm-actions">
        <button id="externalProxyDeleteCancel" class="ghost" type="button">取消</button>
        <button id="externalProxyDeleteConfirm" class="danger" type="button">删除</button>
      </div>
    </div>`;
  document.body.appendChild(dialog);

  document.getElementById("externalProxyAdd").onclick = addExternalProxyEndpoint;
  document.getElementById("externalProxyPort").onkeydown = event => {
    if (event.key === "Enter") addExternalProxyEndpoint();
  };
  return true;
}

function initExternalProxyPanel(retry = 0) {
  if (!mountExternalProxyPanel()) {
    if (retry < 100) {
      setTimeout(() => initExternalProxyPanel(retry + 1), 100);
    }
    return;
  }
  loadExternalProxyEndpoints();
  const events = window.runtime?.EventsOnMultiple;
  if (events) events("state-changed", () => scheduleExternalProxyReload(), -1);
}

window.addEventListener("load", initExternalProxyPanel, { once: true });
