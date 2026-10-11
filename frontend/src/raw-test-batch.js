const batchDelayMs = 8;

function installRawTestBatcher(retry = 0) {
  const app = window.go?.main?.App;
  if (!app?.TestRawNode || !app?.TestRawNodes || app.__sanTianRawTestBatcher) {
    if ((!app?.TestRawNode || !app?.TestRawNodes) && retry < 100) {
      window.setTimeout(() => installRawTestBatcher(retry + 1), 50);
    }
    return;
  }

  const testSingle = app.TestRawNode.bind(app);
  let queue = [];
  let timer = 0;

  const flush = () => {
    timer = 0;
    const pending = queue;
    queue = [];
    const groups = new Map();
    for (const item of pending) {
      const options = item.options || {};
      const key = `${item.metric}\n${JSON.stringify(options)}`;
      const group = groups.get(key) || { metric: item.metric, options, items: [] };
      group.items.push(item);
      groups.set(key, group);
    }
    for (const group of groups.values()) {
      if (group.items.length === 1) {
        const item = group.items[0];
        testSingle(item.raw, item.metric, item.options).then(item.resolve, item.reject);
        continue;
      }
      app.TestRawNodes(group.items.map((item) => item.raw), group.metric, group.options)
        .then((results) => {
          const resultByIndex = new Map((results || []).map((item) => [Number(item.index), item]));
          group.items.forEach((item, index) => {
            const batchResult = resultByIndex.get(index);
            if (!batchResult) {
              item.reject(new Error("节点测试未返回结果"));
            } else if (batchResult.error) {
              item.reject(new Error(batchResult.error));
            } else {
              item.resolve(batchResult.result || {});
            }
          });
        })
        .catch((error) => group.items.forEach((item) => item.reject(error)));
    }
  };

  app.TestRawNode = (raw, metric, options) => {
    if (metric === "speed") {
      return testSingle(raw, metric, options);
    }
    return new Promise((resolve, reject) => {
      queue.push({ raw, metric, options: options || {}, resolve, reject });
      if (!timer) {
        timer = window.setTimeout(flush, batchDelayMs);
      }
    });
  };
  app.__sanTianRawTestBatcher = true;
}

installRawTestBatcher();
