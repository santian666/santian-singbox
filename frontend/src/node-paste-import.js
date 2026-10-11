const pasteImportState = {
  importing: false,
};

function appApi(name) {
  return window.go?.main?.App?.[name];
}

function toast(message) {
  const box = document.querySelector("#toast");
  if (!box) return;
  box.innerHTML = `<span class="toast-icon" aria-hidden="true">OK</span><span>${escapeHtml(message)}</span>`;
  box.classList.add("show");
  clearTimeout(window.__toastTimer);
  window.__toastTimer = setTimeout(
    () => box.classList.remove("show"),
    String(message).length > 24 ? 6000 : 2600,
  );
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function errorText(error) {
  return error?.message || String(error || "导入失败");
}

function isEditable(target) {
  if (!(target instanceof Element)) return false;
  return Boolean(target.closest("input, textarea, select, [contenteditable='true']"));
}

function nodeListVisible() {
  const page = document.querySelector("#homePage");
  const nodeList = document.querySelector("#nodes");
  return Boolean(page && nodeList && !page.classList.contains("hidden"));
}

function dialogOpen() {
  return Boolean(document.querySelector("[aria-modal='true']:not(.hidden), dialog[open]"));
}

function activeGroupId() {
  return document.querySelector(".group-card.active")?.dataset.groupOpen || "default";
}

function looksLikeNodeLinks(text) {
  const value = String(text || "").trim();
  if (!value) return false;
  if (/^\s*\{[\s\S]*"type"\s*:/i.test(value)) return true;
  if (/^(?:ss|vmess|vless|trojan|hysteria2|hy2|socks|socks5|tuic|anytls|wireguard|naive|naive\+https|naive\+quic):\/\//im.test(value)) {
    return true;
  }
  if (/^https?:\/\/(?:[^\s/@]+(?::[^\s/@]*)?@)?(?:\[[^\]]+\]|[^\s/:]+):\d{1,5}(?:[/?#]|$)/im.test(value)) {
    return true;
  }
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .every((line) => /^(?:\[[^\]]+\]|[^\s:]+):\d{1,5}:[^\s:]+:.+$/.test(line));
}

function confirmDuplicates(count) {
  const dialog = document.querySelector("#confirmDialog");
  const ok = document.querySelector("#confirmOk");
  const cancel = document.querySelector("#confirmCancel");
  if (!dialog || !ok || !cancel) return Promise.resolve(false);

  document.querySelector("#confirmTitle").textContent = "发现重复节点";
  document.querySelector("#confirmMessage").textContent = `检测到 ${count} 个重复节点，是否仍然导入？`;
  document.querySelector("#confirmDesc").textContent = "重复节点可能位于当前分组或其他分组。取消后本次不导入任何节点。";
  ok.textContent = "确认导入";
  cancel.textContent = "取消";
  dialog.classList.remove("hidden");
  ok.focus();

  return new Promise((resolve) => {
    const finish = (confirmed) => {
      dialog.classList.add("hidden");
      ok.removeEventListener("click", confirm);
      cancel.removeEventListener("click", reject);
      resolve(confirmed);
    };
    const confirm = () => finish(true);
    const reject = () => finish(false);
    ok.addEventListener("click", confirm);
    cancel.addEventListener("click", reject);
  });
}

async function importClipboardNodes() {
  if (pasteImportState.importing) return;
  const clipboardText = appApi("ClipboardText");
  const importNodes = appApi("ImportNodesToGroupWithDuplicates");
  if (!clipboardText || !importNodes) return;

  pasteImportState.importing = true;
  try {
    const text = String(await clipboardText() || "").trim();
    if (!looksLikeNodeLinks(text)) {
      toast("剪贴板没有可导入的节点链接");
      return;
    }

    const groupId = activeGroupId();
    let result = await importNodes(text, groupId, false);
    if (result?.requiresConfirmation) {
      const confirmed = await confirmDuplicates(Number(result.duplicateCount || 0));
      if (!confirmed) {
        toast("已取消导入重复节点");
        return;
      }
      result = await importNodes(text, groupId, true);
    }

    const duplicateCount = Number(result?.duplicateCount || 0);
    toast(`已粘贴导入 ${Number(result?.added || 0)} 个节点${duplicateCount ? `，其中重复 ${duplicateCount} 个` : ""}`);
  } catch (error) {
    toast(errorText(error));
  } finally {
    pasteImportState.importing = false;
  }
}

if (!window.__sanTianNodePasteImportBound) {
  window.__sanTianNodePasteImportBound = true;
  document.addEventListener("keydown", (event) => {
    if (
      event.key.toLowerCase() !== "v" ||
      !event.ctrlKey ||
      event.altKey ||
      event.metaKey ||
      event.shiftKey ||
      event.repeat ||
      isEditable(event.target) ||
      !nodeListVisible() ||
      dialogOpen()
    ) {
      return;
    }
    event.preventDefault();
    importClipboardNodes();
  });
}
