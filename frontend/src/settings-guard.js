const settingsTimeoutMs = 20_000;

function installSettingsGuard(retry = 0) {
  const appApi = window.go?.main?.App;
  if (!appApi?.SaveSettings) {
    if (retry < 100) {
      window.setTimeout(() => installSettingsGuard(retry + 1), 50);
    }
    return;
  }
  if (appApi.SaveSettings.settingsGuardInstalled) {
    return;
  }

  const saveSettings = appApi.SaveSettings.bind(appApi);
  const guardedSaveSettings = (settings) => {
    let timer = 0;
    const timeout = new Promise((_, reject) => {
      timer = window.setTimeout(() => {
        reject(new Error("应用设置超时，已解除界面锁定；代理后台将继续保持当前状态"));
      }, settingsTimeoutMs);
    });
    return Promise.race([saveSettings(settings), timeout])
      .finally(() => window.clearTimeout(timer));
  };
  guardedSaveSettings.settingsGuardInstalled = true;
  appApi.SaveSettings = guardedSaveSettings;
}

installSettingsGuard();
