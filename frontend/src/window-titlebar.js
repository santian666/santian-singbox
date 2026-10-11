function callRuntime(name) {
  const fn = window.runtime?.[name];
  if (typeof fn === "function") {
    fn();
  }
}

function createTitlebar() {
  if (document.querySelector(".window-titlebar")) {
    return;
  }
  const titlebar = document.createElement("div");
  titlebar.className = "window-titlebar";
  titlebar.innerHTML = `
    <div class="window-titlebar-drag">
      <img class="window-titlebar-icon" src="/favicon.png" alt="" />
      <span class="window-titlebar-title">SanTian-Singbox</span>
      <span id="windowAppVersion" class="window-titlebar-version"></span>
    </div>
    <div class="window-titlebar-actions">
      <button class="window-control" type="button" data-window-action="minimise" title="最小化" aria-label="最小化">-</button>
      <button class="window-control" type="button" data-window-action="toggleMaximise" title="最大化/还原" aria-label="最大化或还原">□</button>
      <button class="window-control close" type="button" data-window-action="hide" title="隐藏到托盘" aria-label="隐藏到托盘">×</button>
    </div>
  `;
  document.body.prepend(titlebar);
  loadTitlebarVersion();
}

function formatVersion(version) {
  const text = String(version || "").trim();
  if (!text) {
    return "";
  }
  return text.toLowerCase().startsWith("v") ? text : `v${text}`;
}

function loadTitlebarVersion(retry = 0) {
  const versionEl = document.getElementById("windowAppVersion");
  const stateFn = window.go?.main?.App?.State;
  if (!versionEl) {
    return;
  }
  if (typeof stateFn !== "function") {
    if (retry < 10) {
      setTimeout(() => loadTitlebarVersion(retry + 1), 300);
    }
    return;
  }
  stateFn()
    .then(state => {
      const version = formatVersion(state?.appVersion);
      versionEl.textContent = version;
      if (version) {
        document.title = `SanTian-Singbox ${version}`;
      }
    })
    .catch(() => {
      if (retry < 3) {
        setTimeout(() => loadTitlebarVersion(retry + 1), 500);
      }
    });
}

document.addEventListener(
  "click",
  event => {
    const button = event.target?.closest?.("[data-window-action]");
    if (!button) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    const action = button.dataset.windowAction;
    if (action === "minimise") {
      callRuntime("WindowMinimise");
    } else if (action === "toggleMaximise") {
      callRuntime("WindowToggleMaximise");
    } else if (action === "hide") {
      callRuntime("WindowHide");
    }
  },
  true
);

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", createTitlebar, { once: true });
} else {
  createTitlebar();
}
