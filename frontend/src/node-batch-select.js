const batchState = {
  enabled: false,
  selected: new Set(),
  testingIds: new Set(),
  groupId: "",
  testing: false,
};

const qs = (selector) => document.querySelector(selector);
const qsa = (selector, root = document) => Array.from(root.querySelectorAll(selector));

function appApi(name) {
  return window.go?.main?.App?.[name];
}

function toast(message) {
  const box = qs("#toast");
  if (!box) return;
  box.innerHTML = `<span class="toast-icon" aria-hidden="true">OK</span><span>${escapeHtml(message)}</span>`;
  box.classList.add("show");
  clearTimeout(window.__toastTimer);
  window.__toastTimer = setTimeout(() => box.classList.remove("show"), String(message).length > 24 ? 6000 : 2600);
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function escapeAttr(value) {
  return escapeHtml(value).replaceAll("'", "&#39;");
}

function cssEscape(value) {
  return window.CSS?.escape ? CSS.escape(String(value)) : String(value).replace(/["\\]/g, "\\$&");
}

function activeGroupId() {
  return qs(".group-card.active")?.dataset.groupOpen || "default";
}

function visibleNodeIds() {
  return qsa("#nodes [data-node-open]").map((item) => item.dataset.nodeOpen).filter(Boolean);
}

function currentGroupNodes() {
  const groupId = activeGroupId();
  return (window.__sanTianLastNodes || []).filter((node) => (node.groupId || "default") === groupId);
}

function currentGroupNodeIds() {
  return currentGroupNodes().map((node) => node.id).filter(Boolean);
}

function selectedIds() {
  const available = new Set(currentGroupNodeIds());
  return Array.from(batchState.selected).filter((id) => available.has(id));
}

function selectedNodes() {
  const ids = new Set(selectedIds());
  return currentGroupNodes().filter((node) => ids.has(node.id));
}

function exitBatchMode() {
  batchState.enabled = false;
  batchState.selected.clear();
  renderBatchSelect();
}

async function rawLinks(nodes) {
  const ids = nodes.map((node) => node.id).filter(Boolean);
  if (!ids.length) return [];
  const shareLinksApi = appApi("ShareLinks");
  if (shareLinksApi) return (await shareLinksApi(ids)).map((link) => String(link || "").trim()).filter(Boolean);
  const shareLinkApi = appApi("ShareLink");
  if (!shareLinkApi) return [];
  const links = [];
  for (const id of ids) {
    const link = String(await shareLinkApi(id) || "").trim();
    if (link) links.push(link);
  }
  return links;
}

function ensureToolbar() {
  const actions = qs(".node-list-head-actions");
  if (!actions || qs("#nodeBatchSelectBtn")) return;
  actions.insertAdjacentHTML("afterbegin", `
    <button id="nodeBatchSelectBtn" class="ghost small node-batch-select-btn" type="button">多选</button>
    <div id="nodeBatchToolbar" class="node-batch-toolbar hidden">
      <button id="nodeBatchCancelBtn" class="ghost small" type="button">取消选择</button>
      <button id="nodeBatchAllBtn" class="ghost small" type="button">全选</button>
      <button id="nodeBatchTestBtn" class="ghost small" type="button">批量测速</button>
      <button id="nodeBatchCopyBtn" class="ghost small" type="button">复制链接</button>
      <button id="nodeBatchExportBtn" class="ghost small" type="button" title="导出选中节点 Excel（含节点二维码）">导出 Excel</button>
      <button id="nodeBatchMoveBtn" class="ghost small" type="button">转移分组</button>
      <button id="nodeBatchDeleteBtn" class="ghost small danger-action" type="button">删除选中</button>
      <button id="nodeBatchDeleteAllBtn" class="ghost small danger-action" type="button">删除全部</button>
      <span id="nodeBatchCount" class="muted">已选 0</span>
    </div>
  `);
  qs("#nodeBatchSelectBtn").addEventListener("click", () => {
    batchState.enabled = true;
    batchState.groupId = activeGroupId();
    renderBatchSelect();
  });
  qs("#nodeBatchCancelBtn").addEventListener("click", () => {
    batchState.enabled = false;
    batchState.selected.clear();
    renderBatchSelect();
  });
  qs("#nodeBatchAllBtn").addEventListener("click", () => {
    currentGroupNodeIds().forEach((id) => batchState.selected.add(id));
    renderBatchSelect();
  });
  qs("#nodeBatchTestBtn").addEventListener("click", testSelectedNodes);
  qs("#nodeBatchCopyBtn").addEventListener("click", copySelectedLinks);
  qs("#nodeBatchExportBtn").addEventListener("click", exportSelectedLinks);
  qs("#nodeBatchMoveBtn").addEventListener("click", moveSelectedNodes);
  qs("#nodeBatchDeleteBtn").addEventListener("click", deleteSelectedNodes);
  qs("#nodeBatchDeleteAllBtn").addEventListener("click", deleteAllCurrentNodes);
}

function renderBatchSelect() {
  ensureToolbar();
  const groupId = activeGroupId();
  if (batchState.groupId && batchState.groupId !== groupId && !batchState.testing) {
    batchState.enabled = false;
    batchState.selected.clear();
  }
  batchState.groupId = groupId;
  const available = new Set(currentGroupNodeIds());
  batchState.selected = new Set(Array.from(batchState.selected).filter((id) => available.has(id)));
  document.body.classList.toggle("node-batch-mode", batchState.enabled);
  qs("#nodeBatchSelectBtn")?.classList.toggle("hidden", batchState.enabled);
  qs("#nodeBatchToolbar")?.classList.toggle("hidden", !batchState.enabled);
  qs("#exportGroupLinksBtn")?.classList.toggle("hidden", batchState.enabled);
  qsa("#nodes [data-node-open]").forEach((item) => renderCheckbox(item));
  const count = selectedIds().length;
  const total = available.size;
  const countEl = qs("#nodeBatchCount");
  if (countEl) {
    const testingCount = Array.from(batchState.testingIds).filter((id) => available.has(id)).length;
    countEl.textContent = batchState.testing ? `测试中 ${testingCount} / 已选 ${count}` : `已选 ${count} / ${total}`;
  }
  ["nodeBatchTestBtn", "nodeBatchCopyBtn", "nodeBatchExportBtn", "nodeBatchMoveBtn", "nodeBatchDeleteBtn"].forEach((id) => {
    const btn = qs(`#${id}`);
    if (btn) btn.disabled = count === 0 || batchState.testing;
  });
  const testBtn = qs("#nodeBatchTestBtn");
  if (testBtn) testBtn.textContent = batchState.testing ? "测试中" : "批量测速";
  const selectBtn = qs("#nodeBatchSelectBtn");
  if (selectBtn) {
    selectBtn.disabled = batchState.testing;
    selectBtn.textContent = batchState.testing ? "测速中" : "多选";
  }
  const allBtn = qs("#nodeBatchAllBtn");
  if (allBtn) allBtn.textContent = count > 0 && count === total ? "已全选" : "全选";
  const deleteAllBtn = qs("#nodeBatchDeleteAllBtn");
  if (deleteAllBtn) deleteAllBtn.disabled = total === 0 || batchState.testing;
}

function renderCheckbox(item) {
  let box = item.querySelector(".node-batch-check");
  const id = item.dataset.nodeOpen;
  item.classList.toggle("batch-testing", batchState.testingIds.has(id));
  if (!batchState.enabled) {
    box?.remove();
    item.classList.remove("batch-picked");
    return;
  }
  if (!box) {
    box = document.createElement("label");
    box.className = "node-batch-check";
    box.innerHTML = `<input type="checkbox" /><span></span>`;
    item.appendChild(box);
    box.addEventListener("click", (event) => event.stopPropagation());
    box.querySelector("input").addEventListener("change", (event) => {
      if (event.target.checked) batchState.selected.add(id);
      else batchState.selected.delete(id);
      renderBatchSelect();
    });
  }
  const checked = batchState.selected.has(id);
  box.querySelector("input").checked = checked;
  item.classList.toggle("batch-picked", checked);
}

async function copySelectedLinks() {
  const links = await rawLinks(selectedNodes());
  if (!links.length) {
    toast("选中节点没有可复制的协议链接");
    return;
  }
  await window.runtime?.ClipboardSetText?.(links.join("\n"));
  toast(`已复制 ${links.length} 条链接`);
  exitBatchMode();
}

async function testSelectedNodes() {
  const ids = selectedIds();
  if (!ids.length) {
    toast("请先勾选节点");
    return;
  }
  if (window.__sanTianPrepareStoredNodeTest) {
    const ready = await window.__sanTianPrepareStoredNodeTest(selectedNodes());
    if (!ready) return;
  }
  const mode = window.__sanTianChooseDelayMetric
    ? await window.__sanTianChooseDelayMetric({ title: "选择批量测速方式", message: "请选择本次批量测速的延迟类型。" })
    : "tcp";
  if (!mode) return;
  const modeApi = appApi("TestNodesWithMode");
  const api = modeApi || appApi("TestNodes");
  if (!api) {
    toast("批量测试接口未就绪，请重启开发模式");
    return;
  }
  const idsSnapshot = [...ids];
  batchState.testing = true;
  batchState.testingIds = new Set(idsSnapshot);
  batchState.enabled = false;
  batchState.selected.clear();
  toast(`已开始${mode === "real" ? "真连接延迟" : "TCP延迟"}测试 ${idsSnapshot.length} 个节点，已退出多选，任务将在后台继续`);
  renderBatchSelect();
  try {
    const result = modeApi ? await modeApi(idsSnapshot, mode) : await api(idsSnapshot);
    toast(`并发测试完成：成功 ${result?.tested || 0} 个，失败 ${result?.failed || 0} 个`);
    const state = await appApi("State")?.();
    if (state) {
      window.__sanTianLastState = state;
      window.__sanTianLastNodes = state?.nodes || [];
    }
  } finally {
    batchState.testing = false;
    batchState.testingIds.clear();
    renderBatchSelect();
  }
}

async function exportSelectedLinks() {
  const ids = selectedIds();
  if (!ids.length) {
    toast("请先勾选节点");
    return;
  }
  const api = appApi("ExportNodesLinks");
  if (!api) {
    toast("导出接口未就绪，请重启开发模式");
    return;
  }
  const path = await api(ids);
  toast(path ? "选中节点 Excel 已导出，包含节点二维码" : "已取消导出");
  if (path) exitBatchMode();
}

async function moveSelectedNodes() {
  const ids = selectedIds();
  if (!ids.length) {
    toast("请先勾选节点");
    return;
  }
  const groups = (window.__sanTianLastState?.groups || []).filter((group) => group?.id);
  const current = activeGroupId();
  const targets = groups.filter((group) => group.id !== current);
  if (!targets.length) {
    toast("没有可转移的其它分组");
    return;
  }
  const targetId = await chooseMoveTargetGroup(targets, ids.length);
  if (!targetId) return;
  const api = appApi("MoveNodesToGroup");
  if (!api) {
    toast("转移接口未就绪，请重启开发模式");
    return;
  }
  await api(ids, targetId);
  exitBatchMode();
  const target = groups.find((group) => group.id === targetId);
  toast(`已转移 ${ids.length} 个节点到 ${target?.name || "目标分组"}`);
  await refreshState(current);
}

function chooseMoveTargetGroup(groups, count) {
  return new Promise((resolve) => {
    let selectedId = "";
    const overlay = document.createElement("div");
    overlay.className = "node-move-dialog";
    overlay.innerHTML = `
      <div class="node-move-panel" role="dialog" aria-modal="true">
        <div class="node-move-head">
          <strong>转移节点</strong>
          <button class="node-move-close" type="button" aria-label="关闭">×</button>
        </div>
        <div class="node-move-tip">选择目标分组，将 ${count} 个节点移动过去。</div>
        <div class="node-move-list">
          ${groups.map((group) => `
            <button class="node-move-group" type="button" data-group-id="${escapeAttr(group.id)}" title="${escapeAttr(group.name || "未命名组")}">
              <span class="node-move-name">${escapeHtml(group.name || "未命名组")}</span>
            </button>
          `).join("")}
        </div>
        <div class="node-move-actions">
          <button class="node-move-cancel" type="button">取消</button>
          <button class="node-move-submit" type="button" disabled>转移</button>
        </div>
      </div>
    `;
    const close = (value = "") => {
      overlay.remove();
      resolve(value);
    };
    overlay.querySelector(".node-move-close")?.addEventListener("click", () => close(""));
    overlay.querySelector(".node-move-cancel")?.addEventListener("click", () => close(""));
    overlay.querySelector(".node-move-submit")?.addEventListener("click", () => close(selectedId));
    overlay.querySelectorAll("[data-group-id]").forEach((button) => {
      button.addEventListener("click", () => {
        selectedId = button.dataset.groupId || "";
        overlay.querySelectorAll("[data-group-id]").forEach((item) => item.classList.toggle("selected", item === button));
        const submit = overlay.querySelector(".node-move-submit");
        if (submit) submit.disabled = !selectedId;
      });
    });
    (document.getElementById("app") || document.body).appendChild(overlay);
  });
}

function showBatchConfirm({ title, message, desc, okText = "删除" }) {
  const dialog = qs("#confirmDialog");
  const ok = qs("#confirmOk");
  const cancel = qs("#confirmCancel");
  if (!dialog || !ok || !cancel) return Promise.resolve(false);
  qs("#confirmTitle").textContent = title;
  qs("#confirmMessage").textContent = message;
  qs("#confirmDesc").textContent = desc;
  ok.textContent = okText;
  cancel.textContent = "取消";
  dialog.classList.remove("hidden");
  ok.focus();
  return new Promise((resolve) => {
    const close = (value) => {
      dialog.classList.add("hidden");
      ok.removeEventListener("click", confirm);
      cancel.removeEventListener("click", dismiss);
      window.removeEventListener("keydown", onKey);
      resolve(value);
    };
    const confirm = () => close(true);
    const dismiss = () => close(false);
    const onKey = (event) => {
      if (event.key === "Escape") dismiss();
    };
    ok.addEventListener("click", confirm);
    cancel.addEventListener("click", dismiss);
    window.addEventListener("keydown", onKey);
  });
}

async function refreshState(keepGroupId = "") {
  const state = await appApi("State")?.();
  if (!state) return;
  window.__sanTianLastState = state;
  window.__sanTianLastNodes = state?.nodes || [];
  syncStateFromDom();
  if (keepGroupId) {
    queueMicrotask(() => {
      const current = activeGroupId();
      const target = qs(`[data-group-open="${cssEscape(keepGroupId)}"]`);
      if (target && current !== keepGroupId) target.click();
    });
  }
}

async function deleteSelectedNodes() {
  const ids = selectedIds();
  if (!ids.length) {
    toast("请先勾选节点");
    return;
  }
  const confirmed = await showBatchConfirm({
    title: "批量删除节点",
    message: `确定删除选中的 ${ids.length} 个节点？`,
    desc: "删除后不能恢复，请仔细确认。",
  });
  if (!confirmed) return;
  await deleteNodes(ids, `已删除 ${ids.length} 个节点`);
}

async function deleteAllCurrentNodes() {
  const ids = currentGroupNodeIds();
  if (!ids.length) {
    toast("当前分组没有节点");
    return;
  }
  const confirmed = await showBatchConfirm({
    title: "删除分组全部节点",
    message: `确定删除当前节点组的全部 ${ids.length} 个节点？`,
    desc: "该操作会清空当前分组，删除后不能恢复。",
  });
  if (!confirmed) return;
  await deleteNodes(ids, `已删除当前分组 ${ids.length} 个节点`);
}

async function deleteNodes(ids, message) {
  const api = appApi("DeleteNodes");
  if (!api) {
    toast("删除接口未就绪，请重启开发模式");
    return;
  }
  await api(ids);
  exitBatchMode();
  toast(message);
  await refreshState();
}

function syncStateFromDom() {
  const state = window.__sanTianLastState;
  if (state?.nodes) window.__sanTianLastNodes = state.nodes;
  renderBatchSelect();
}

function patchStateRenderer() {
  const descriptor = Object.getOwnPropertyDescriptor(window, "__sanTianBatchStatePatched");
  if (descriptor?.value) return true;
  const original = window.go?.main?.App?.State;
  if (!original) return false;
  Object.defineProperty(window, "__sanTianBatchStatePatched", { value: true });
  window.go.main.App.State = async (...args) => {
    const state = await original(...args);
    window.__sanTianLastState = state;
    window.__sanTianLastNodes = state?.nodes || [];
    queueMicrotask(syncStateFromDom);
    return state;
  };
  return true;
}

function boot() {
  patchStateRenderer();
  ensureToolbar();
  appApi("State")?.().then((state) => {
    window.__sanTianLastState = state;
    window.__sanTianLastNodes = state?.nodes || [];
    syncStateFromDom();
  }).catch(() => {});
  const nodes = qs("#nodes");
  if (nodes) {
    new MutationObserver(() => renderBatchSelect()).observe(nodes, { childList: true, subtree: true });
  }
  let retries = 0;
  const timer = setInterval(() => {
    const patched = patchStateRenderer();
    ensureToolbar();
    renderBatchSelect();
    retries += 1;
    if ((patched && qs("#nodeBatchSelectBtn")) || retries >= 25) {
      clearInterval(timer);
    }
  }, 400);
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", boot);
} else {
  boot();
}
