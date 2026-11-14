# T04：批量测试改为“流式结果 + 可取消”，解决卡顿和结果成批出现

> 执行者：Antigravity　｜　审查者：Claude　｜　分支：`feat/t04-batch-test-streaming`
> **前置条件：T03 已合并到 `main`。**

## 0. 工作规则

1. 从合并 T03 之后的 `main` 新建分支 `feat/t04-batch-test-streaming`。
2. 只改本任务列出的内容，不升级依赖，不修改 `dist/`、`bin/`、`build/`、`.gocache2/`。
3. Go 代码执行 `gofmt -w`，并运行 `go test -v ./...`；前端执行 `npm run build`（在 `frontend/` 下），两者都必须成功。
4. **提交分成两个**（顺序不能变）：
   - 第 1 个提交 `T04-0: format main.js`：只对 `frontend/src/main.js` 做格式化（见 T04-0），**不改任何逻辑**；
   - 第 2 个提交 `T04: ...`：所有功能修改。
5. 完成后生成：
   - `review/T04-format.diff`：`git diff main...<第1个提交>`，只用于确认没有逻辑改动；
   - `review/T04.diff`：`git diff <第1个提交>...feat/t04-batch-test-streaming`，即只包含功能修改；
   - `review/T04-test.txt`（`go test -v ./...`）、`review/T04-report.md`。
6. report 只写实际做过的验证，没做的写“未做”。

---

## 1. 现状与问题（已在代码中确认）

### 1.1 结果“成批出现”、整体慢

- 前端 `Qt()` 启动 64 个 worker（`Et=64`），每个 worker 对单个节点调用 `TestRawNode`。
- `raw-test-batch.js` 把 8 毫秒内的调用合并成一次 `TestRawNodes`。
- 后端 `TestRawNodes` 为这一批节点**启动一个临时 sing-box 内核**，**等这一批全部测完**才一次性返回。
- 于是 64 个 worker 同时拿到结果，又同时发起下一批 → 又启动一个新内核……

后果：
- N 个节点要启动约 N/64 次内核；每一批都要等最慢的那个节点超时（约 3 秒）才能整体返回；
- 界面上结果是一批一批跳出来的，中间长时间没有变化，看起来像卡住了；
- 测完延迟后，“出口 IP 回填”（`Yr`/`zr`）又用同样的方式再跑一整轮，**再启动一遍临时内核**。

### 1.2 界面卡顿

- `se()` 每次调用都**重建整张测试表的 innerHTML**。
- `Ue()` 对每个节点在开始和结束时各调用一次 `se()`，加上 `ne()`。几百个节点 × 64 并发，就是成千上万次整表重建。

### 1.3 “停止”不是真停止

- 前端停止只是 `testRunId += 1`，后端的临时内核和网络请求会继续跑完，占用 CPU、端口和带宽，还会和下一次测试抢资源。

### 1.4 一个坏节点拖垮一整批

- 临时内核配置里只要有一个节点的 outbound 无效，整个内核就启动失败，然后这一批全部退回“兼容模式”（每个节点单独起内核，并发只有 6），非常慢。

### 1.5 临时内核 DNS 仍使用 `local`

- `buildBatchDelayConfigWithUpstream` 的 DNS 是 `local`。T02 已经有了可靠的直连 DNS 检测，这里应该统一使用它。

---

## 2. 子任务

### T04-0　先让 main.js 可读（单独提交，不改逻辑）

`frontend/src/main.js` 目前是压缩后的代码（函数名是单字母，单行最长 3 万多字符），无法审查。

- 用 Prettier 格式化：`npx prettier@3 --write src/main.js --print-width 120`；
- **只格式化，不改名、不改逻辑**；
- 格式化后执行 `npm run build` 必须成功，并在本机打开程序，确认主界面和测试页能正常显示；
- 单独提交（`T04-0: format main.js`）。

> 如果你知道这个项目原本有未压缩的前端源码（例如某个旧目录或旧提交里），**不要自己去找或替换**，在 report 里写出来，由用户决定。

### T04-1　后端：批量测试任务（流式事件 + 可取消）

新建 `batch_test_job.go`（核心逻辑与平台无关，必须可测试）。

**新 API**（挂在 `App` 上，供前端调用）：

```go
type BatchTestOptions struct {
    Metric        string        // "delay" 或 "ping"
    WithIP        bool          // 延迟成功后，同一节点立即顺带查询出口 IP
    SpeedTestOptions            // 复用现有选项（超时、是否经系统代理等）
}
type BatchTestStartResult struct { JobID string; Total int }

func (a *App) StartBatchTest(raws []string, options BatchTestOptions) (BatchTestStartResult, error)
func (a *App) CancelBatchTest(jobID string) error
```

**事件**（Wails `EventsEmit`）：
- `batch-test-result`：`{jobId, index, phase: "delay"|"ip", result: SpeedResult, error: string}`，**每个节点、每个阶段完成后立即发送**；
- `batch-test-done`：`{jobId, total, ok, failed, cancelled}`。

**行为要求：**

1. 同一时间只允许一个任务。启动新任务时，先取消旧任务。
2. 解析所有 raw；解析失败的节点立即发送 error 事件，不进入队列。
3. **一个全局 worker 池，没有批次屏障。** worker 数量使用 `batchNodeTestConcurrency`。所有节点放进同一个队列，worker 测完一个立刻取下一个。**慢节点不能拖住其他节点的结果。**
4. 需要临时内核的节点按每 300 个分组（复用 `temporaryCoreDelayTestBatchSize`），每组一个临时内核：
   - 同时最多运行 2 个临时内核：当前组的队列快空时，预先启动下一组的内核；
   - 某组节点全部完成后，立即关闭该组内核。
5. `WithIP == true` 时：同一个 worker 在延迟测试成功后，**立即**用同一个 SOCKS 端口调用 `testOutboundInfo`，先发 delay 事件，再发 ip 事件。延迟失败的节点不查 IP。**不再有单独的第二轮 IP 回填。**
6. 取消：所有 worker、HTTP 请求和内核启动都受同一个 `context` 控制。取消后不再发送 result 事件；临时内核在 2 秒内被结束；最后发送 `done{cancelled:true}`。
7. 临时内核启动失败时（修 1.4）：
   - 运行 `sing-box check -c`，从错误输出中解析出出错的 outbound 序号（写一个纯函数 `parseFailingOutboundIndex(output string) (int, bool)`，用真实的 sing-box 错误文本做测试样例）；
   - 把该节点标记为“节点配置无效：<错误摘要>”，从配置中移除后重试；**最多重试 3 次**；
   - 仍然失败 → 该组剩余节点退回现有的兼容模式，记 Error 日志。其他组不受影响。
8. 临时内核配置的 DNS（修 1.5）：`buildBatchDelayConfigWithUpstream` 增加一个直连 DNS 参数，使用 `resolveDirectDNSPlanFunc(logger).Direct.Server`，生成 `{"tag":"direct-dns","type":"udp","server":<ip>}`，`final` 和 `default_domain_resolver` 都指向它。
9. 保留现有的 `TestRawNode`、`TestRawNodes`，不要删除（单节点重测、测速和其他地方还在用）。

### T04-2　前端：批量测试流程改用新 API

在格式化后的 `main.js` 中：

1. 批量测试入口（原 `Qt()`）：
   - 解析和选择测试方式的流程保持不变；
   - 不再启动 64 个 worker，改为调用 `StartBatchTest(全部 raw, {metric, withIP: true, ...原有选项})`；
   - 订阅 `batch-test-result` 和 `batch-test-done`，按 `index` 更新 `s.testNodes[index]` 的对应字段和状态文字。字段含义与现有 `Ue()` 中的处理保持一致；
   - 只处理 `jobId` 等于当前任务的事件，旧任务的事件一律忽略。
2. “停止”按钮（原 `ea()`）：调用 `CancelBatchTest(当前 jobId)`，然后保留现有的界面复位逻辑。
3. 删除批量流程中对 `Yr()`（出口 IP 回填）的调用。函数本身如果别处没有用到，可以删除。
4. **渲染节流（修 1.2）：**
   - 把现有的整表渲染函数改名为 `renderTestNodesNow()`；
   - 新的 `se()` 改为调度函数：标记需要渲染，**最多每 150 毫秒真正渲染一次**（用 `setTimeout` 或 `requestAnimationFrame` 合并）；
   - 这样所有调用 `se()` 的地方会自动被节流，不需要逐个修改；
   - 任务结束（done 事件）时立即强制渲染一次。
5. 进度显示：在测试页已有的状态区域显示“已完成 x / 总数 y，可用 z”，随事件更新（同样受节流控制）。
6. 不改单节点的“重测”和“测速”按钮的行为。

### T04-3　不改的内容

- 不改主界面、代理节点列表的测速（分组测速走的是另一条路径，本任务不动）；
- 不改 MTR、iperf3、本地测速、工具箱（这些在 T05 处理）；
- 不改 `TestRawNode` / `TestRawNodes` 的签名和行为。

---

## 3. 必须新增的测试（Go）

把“测一个节点”的函数通过注入替换成假实现（可以控制每个节点的耗时和成败），然后测试：

1. **无屏障：** 64 个 worker、200 个节点，其中 1 个节点耗时 3 秒，其他 10 毫秒 → 其他 199 个节点的 result 事件全部在 1 秒内发出。
2. **WithIP：** 延迟成功的节点依次收到 delay 事件和 ip 事件；延迟失败的节点只有 delay 事件（带 error），没有 ip 事件。
3. **取消：** 取消后不再有 result 事件；最后收到一次 `done{cancelled:true}`；假的“内核关闭”函数被调用。
4. **新任务取消旧任务：** 启动第二个任务后，第一个任务收到 `cancelled:true`，并且之后不再有第一个任务的 result 事件。
5. **`parseFailingOutboundIndex`：** 至少 3 个真实的 sing-box 错误文本样例（请在本机故意构造一个错误节点，运行 `sing-box check` 获取真实输出，原样贴进测试）。
6. **坏节点隔离：** 假内核在包含某个特定节点时启动失败 → 该节点被标记为配置无效，其余节点正常出结果，重试次数 ≤ 3。
7. **DNS：** `buildBatchDelayConfigWithUpstream` 生成的配置中不再出现 `local`，并且使用了传入的直连 DNS。

## 4. 真机验证（请实际做，并把结果写进 report）

准备一份约 300 个节点的列表（有效和无效混合即可）：

| 项目 | 改动前 | 改动后 |
|---|---|---|
| 点击开始到第一个结果出现的时间 | | |
| 全部完成的总时间（含出口 IP） | | |
| 测试过程中滚动表格是否流畅（主观描述） | | |
| 点击停止后，任务管理器中临时 sing-box 进程消失的时间 | | |

## 5. 验收标准

- [ ] `review/T04-format.diff` 只有格式变化
- [ ] 第 3 节测试全部存在并通过
- [ ] 第 4 节表格已填写真实数据
- [ ] 停止后 2 秒内没有残留的临时 sing-box 进程
- [ ] 单节点重测、测速、主界面分组测速行为不变
