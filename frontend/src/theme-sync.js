let lastTheme = "";
let transitionTimer = 0;

function disableThemeTransition() {
  document.documentElement.classList.add("theme-switching");
  clearTimeout(transitionTimer);
  transitionTimer = setTimeout(() => {
    document.documentElement.classList.remove("theme-switching");
  }, 180);
}

function setNativeTitleTheme(theme) {
  const runtime = window.runtime;
  if (!runtime) {
    return;
  }
  try {
    if (theme === "dark" && typeof runtime.WindowSetDarkTheme === "function") {
      runtime.WindowSetDarkTheme();
      return;
    }
    if (typeof runtime.WindowSetLightTheme === "function") {
      runtime.WindowSetLightTheme();
    }
  } catch {
    // 标题栏同步失败不影响前端主题。
  }
}

function updateThemeButton(theme) {
  const button = document.getElementById("themeToggle");
  if (!button) {
    return;
  }
  button.textContent = theme === "dark" ? "浅色" : "黑色";
  button.title = theme === "dark" ? "切换浅色主题" : "切换黑色主题";
  button.setAttribute("aria-pressed", String(theme === "dark"));
}

function rememberTheme(theme) {
  try {
    localStorage.setItem("santian-theme", theme);
  } catch {
    // 本地缓存不可用时仍允许切换当前界面主题。
  }
}

function applyThemeNow(theme, save) {
  theme = theme === "dark" ? "dark" : "light";
  if (theme === lastTheme) {
    updateThemeButton(theme);
    return;
  }
  lastTheme = theme;
  disableThemeTransition();
  document.documentElement.dataset.theme = theme;
  document.body.dataset.theme = theme;
  if (save) {
    rememberTheme(theme);
  }
  updateThemeButton(theme);
  setNativeTitleTheme(theme);
}

function syncTheme() {
  applyThemeNow(document.body?.dataset?.theme === "dark" ? "dark" : "light", false);
}

function initialTheme() {
  try {
    return localStorage.getItem("santian-theme") === "dark" ? "dark" : "light";
  } catch {
    return "light";
  }
}

applyThemeNow(initialTheme(), false);

document.addEventListener(
  "click",
  event => {
    const button = event.target?.closest?.("#themeToggle");
    if (!button) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    event.stopImmediatePropagation();
    const nextTheme = document.body.dataset.theme === "dark" ? "light" : "dark";
    applyThemeNow(nextTheme, true);
  },
  true
);

new MutationObserver(syncTheme).observe(document.body, {
  attributes: true,
  attributeFilter: ["data-theme"],
});
