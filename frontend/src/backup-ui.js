const appApi = () => window.go && window.go.main && window.go.main.App;

function toast(message) {
  const target = document.getElementById("toast");
  if (!target) {
    return;
  }
  target.innerHTML = `<span class="toast-icon" aria-hidden="true">OK</span><span>${escapeHtml(message)}</span>`;
  target.classList.add("show");
  clearTimeout(window.__toastTimer);
  window.__toastTimer = setTimeout(() => target.classList.remove("show"), String(message).length > 24 ? 6000 : 2600);
}

function escapeHtml(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function setBusy(button, busy, busyText, idleText) {
  if (!button) {
    return;
  }
  if (!button.dataset.idleText) {
    button.dataset.idleText = idleText || button.textContent;
  }
  button.disabled = busy;
  button.classList.toggle("is-busy", busy);
  button.textContent = busy ? busyText : button.dataset.idleText;
}

function confirmImport() {
  const dialog = document.getElementById("confirmDialog");
  const ok = document.getElementById("confirmOk");
  const cancel = document.getElementById("confirmCancel");
  if (!dialog || !ok || !cancel) {
    toast("确认弹窗未就绪，请重启软件后再试");
    return Promise.resolve(false);
  }
  document.getElementById("confirmTitle").textContent = "导入备份";
  document.getElementById("confirmMessage").textContent = "确定导入备份并覆盖当前节点分组和节点？";
  document.getElementById("confirmDesc").textContent = "导入后会恢复备份中的分组、节点和排序；不会覆盖代理开关、端口、主题和规则设置。";
  ok.textContent = "导入";
  cancel.textContent = "取消";
  dialog.classList.remove("hidden");
  ok.focus();
  return new Promise((resolve) => {
    const finish = (result) => {
      dialog.classList.add("hidden");
      ok.removeEventListener("click", onOk);
      cancel.removeEventListener("click", onCancel);
      window.removeEventListener("keydown", onKey);
      resolve(result);
    };
    const onOk = () => finish(true);
    const onCancel = () => finish(false);
    const onKey = (event) => event.key === "Escape" && finish(false);
    ok.addEventListener("click", onOk);
    cancel.addEventListener("click", onCancel);
    window.addEventListener("keydown", onKey);
  });
}

async function refreshState() {
  const app = appApi();
  let state = null;
  if (app && app.State) {
    try {
      state = await app.State();
      if (state && typeof window.__sanTianRenderState === "function") {
        window.__sanTianRenderState(state);
      }
    } catch (_) {}
  }
  if (typeof window.__sanTianRefreshState === "function") {
    try {
      await window.__sanTianRefreshState();
    } catch (_) {}
  } else if (window.runtime && window.runtime.EventsEmit) {
    window.runtime.EventsEmit("state-changed", state);
  }
}

function bindBackupButtons() {
  const exportButton = document.getElementById("exportBackupBtn");
  const importButton = document.getElementById("importBackupBtn");
  if (!exportButton || !importButton || exportButton.dataset.backupReady === "1") {
    return;
  }
  exportButton.dataset.backupReady = "1";
  importButton.dataset.backupReady = "1";

  exportButton.addEventListener("click", async () => {
    const app = appApi();
    if (!app || !app.ExportBackup) {
      toast("导出备份接口未就绪，请重启软件");
      return;
    }
    setBusy(exportButton, true, "导出中", "导出备份");
    try {
      const path = await app.ExportBackup();
      toast(path ? "节点备份已导出" : "已取消导出备份");
    } catch (error) {
      toast(error && error.message ? error.message : String(error || "导出备份失败"));
    } finally {
      setBusy(exportButton, false, "", "导出备份");
    }
  });

  importButton.addEventListener("click", async () => {
    const app = appApi();
    if (!app || !app.ImportBackup) {
      toast("导入备份接口未就绪，请重启软件");
      return;
    }
    if (!(await confirmImport())) {
      return;
    }
    setBusy(importButton, true, "导入中", "导入备份");
    try {
      const result = await app.ImportBackup();
      if (result && result.cancelled) {
        toast("已取消导入备份");
        return;
      }
      toast(`备份已导入：${result.nodes || 0} 个节点，${result.groups || 0} 个分组`);
      await refreshState();
    } catch (error) {
      toast(error && error.message ? error.message : String(error || "导入备份失败"));
    } finally {
      setBusy(importButton, false, "", "导入备份");
    }
  });
}

let retries = 0;
const timer = setInterval(() => {
  bindBackupButtons();
  retries += 1;
  if (document.getElementById("exportBackupBtn") || retries >= 50) {
    clearInterval(timer);
  }
}, 200);
