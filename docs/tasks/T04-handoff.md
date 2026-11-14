# T04 交接：Claude 已完成代码修改，请 Antigravity 编译、测试、提交、打包

> 修改者：Claude　｜　执行者：Antigravity　｜　分支：`feat/t04-batch-test-streaming`
>
> Claude 已经直接修改了工作区里的文件（**尚未提交**）。你的任务是：编译、运行测试、提交、打包。
> **不要重写或“优化” Claude 的代码。** 只有编译失败或测试失败时才允许做最小修改，并且每一处都要写进报告。

---

## 1. Claude 改了什么

### 1.1 T03 修正（T03 已合并到 main，但实现与任务单第 2 版不一致）

文件：`direct_dns.go`、`direct_dns_test.go`

| 问题 | 修正 |
|---|---|
| 连续 **2** 次探测失败就重新选择 | 改为连续 **3** 次（常量 `directDNSFailThreshold`） |
| 没有“每小时最多 3 次重启”的上限 | 新增 `directDNSMaxRestartsPerHour = 3`；超过后暂停自动切换并记一条 Error 日志，网络指纹变化或系统唤醒时解除暂停 |
| 当前 DNS 仍可用时仍然会重新测速一轮 | 当前 DNS 可用就直接保持，不再 resolve |
| 缺少“稳定网络 1 小时 0 次重启”测试 | 新增 `StableNetworkOneHourZeroRestarts`、`FingerprintChangedButRunningReachableKeeps`、`HourlyRestartCapPausesUntilNetworkChange`；`ResumeForces…` 测试按新行为改写 |

### 1.2 T04 后端

| 文件 | 修改 |
|---|---|
| `batch_test_job.go` | **整体重写**（接口、事件格式与你之前定义的保持一致）。全局 worker 池无批次屏障；每 300 个节点一个临时内核，最多同时 2 个，当前组剩余 ≤50 时预启动下一组；WithIP 时同一 worker 立即查出口 IP；**Ping 模式也会对成功节点补查出口 IP**；取消时立即关闭所有临时内核；内核启动时**同时监听进程退出**，配置错误能立刻发现，不必等端口超时；按 outbound/endpoint 序号精确定位坏节点并剔除重试（最多 3 次），仍失败的该组走兼容模式（6 并发）；节点按 `PreferCore` 决定是否使用原生客户端（与原 `TestNodeMetric…` 逻辑一致，修复了 TUN 下原生节点误走 TUN 的问题）；`cmd.WaitDelay` 保证结束进程后不会卡住 |
| `batch_test_job_test.go` | **新增**，10 个测试：无屏障、WithIP 顺序、取消、最多 2 个内核且全部关闭、错误文本解析、坏节点隔离、重试用尽转兼容模式、Ping+IP、原生节点不起内核、初始失败数计入 done |
| `delay_batch.go` | 新增 `buildBatchDelayConfigWithOwners`，返回每个 outbound/endpoint 属于哪个节点（WireGuard 节点在 endpoints 里，原来的推算会对错号）；`buildBatchDelayConfigWithUpstream` 改为调用它，行为不变 |
| `delay_batch_test.go` | 新增 2 个测试：序号映射（含 WireGuard 和上游代理）、临时内核 DNS 不再使用 `local`；补充 `encoding/json` 导入，并用 gofmt 去掉了原文件末尾的多余空行 |
| `app.go` | `StartBatchTest`：校验 metric；接受前端传入的 `jobId`；**按节点调用 `speedTestOptionsForCurrentSettings`**；解析失败数计入 done；任务结束后清理 `currentBatchJob`；加了 `recoverError`。`CancelBatchTest` 加了 `recoverError`。新增 `sanitizeBatchJobID`、`cancelBatchTestJob`；**程序退出时**（`cleanupAllProxyEffects`）取消批量测试，避免残留临时内核 |

### 1.3 T04 前端（`frontend/src/main.js`）

| 问题 | 修正 |
|---|---|
| 测试表格每 150ms 整体重建，**滚动容器也被重建，测试中往下滚会被拉回顶部** | 只替换 `thead` / `tbody`，保留 `.test-table-wrap`，滚动位置不变 |
| 每次渲染都给每一行重新绑定 3 种点击事件 | 改为在 `#testNodes` 容器上绑定一次（事件委托） |
| 任务 ID 要等调用返回才知道，只能先缓存事件 | 前端生成 `jobId` 传给后端，删除 `pendingBatch*` 缓存逻辑 |
| 活动栏显示“完成后显示输出” | 改为“批量测试中 x/y” |
| 重新解析节点后仍显示上一次的“已完成 x / 总数 y” | `Vt()` 中重置计数 |
| 节点数显示为“N 节点” | 恢复为“N 个节点” |

---

## 2. Claude 已做的验证（在 Claude 那边，无法连接你的依赖）

- 所有改动的 Go 文件通过 `gofmt`。
- 用标准库 + 第三方包空桩做了**整个包的 Windows 类型检查（含测试文件）**：除第三方包外没有任何类型错误。
- `direct_dns` 和 `batch_test_job` 的全部测试在隔离环境中用 `-race` 跑通；批量任务的并发测试重复 15 次稳定通过。
- 配置生成器的 2 个新测试用项目里原样抽出的真实代码跑通。
- 临时内核启动逻辑用假内核程序验证了三种情况：配置错误立即退出（<0.5 秒识别并定位到节点）、正常就绪后关闭（进程退出、配置文件删除）、取消（0.7 秒内返回）。
- `main.js` 通过 `node --check`；ESLint `no-undef` 检查没有在改动区域发现未定义变量（报出的 28 处都是原有的页面元素 ID 全局变量等）。
- **没做的：** 在 Windows 上完整编译、完整 `go test ./...`、`npm run build`、真机运行。这些请你来做。

---

## 3. 你要做的事（按顺序）

### 3.1 确认工作区
```
git status
git branch --show-current
```
- 必须在 `feat/t04-batch-test-streaming` 分支上。
- 预期的改动文件：`app.go`、`batch_test_job.go`（新）、`batch_test_job_test.go`（新）、`delay_batch.go`、`delay_batch_test.go`、`direct_dns.go`、`direct_dns_test.go`、`frontend/src/main.js`。
- **如果还有其他文件被改动**（例如你之前做了一半的修改，或 `frontend/wailsjs/` 下的绑定文件），不要提交它们，先在报告中列出，由用户决定。`frontend/wailsjs/` 下的绑定文件由 `wails build` 自动生成，可以随打包一起提交。

### 3.2 编译和测试
```
gofmt -l .
go vet ./...
go test -v ./... > review/T04-test.txt 2>&1
cd frontend && npm run build && cd ..
```
- `gofmt -l .` 应该没有输出。
- 如果编译或测试失败：**只做最小修改**，在报告中逐条写明：失败信息原文、改了哪个文件哪一行、为什么。
- 特别留意：`TestBatchJob*`、`TestParseFailingConfigEntry`、`TestBuildBatchDelayConfigWithOwners…`、`TestBuildBatchDelayConfigUsesDirectDNSNotLocal`、`TestDirectDNSHealthCheckerScenarios`（包括 9 个子测试）必须出现在输出里并通过。

### 3.3 补充一个真实的 sing-box 错误样例
在本机构造一个错误节点配置（例如 vmess 的 uuid 写成 `abc`），执行 `bin\sing-box.exe check -c <文件>`，把**原始输出**加到 `batch_test_job_test.go` 的 `TestParseFailingConfigEntry` 用例里（期望的 kind/index/summary 按实际输出填写）。如果实际格式与现有用例都对不上，说明 `failingConfigEntryRegexp` 需要调整：**不要自己改正则**，在报告中贴出原始输出，交给 Claude 处理。

### 3.4 提交（两个提交，顺序不能变）
```
git add direct_dns.go direct_dns_test.go
git commit -m "T03-fix: direct DNS fail threshold 3, hourly restart cap, keep reachable DNS (by Claude)"

git add app.go batch_test_job.go batch_test_job_test.go delay_batch.go delay_batch_test.go frontend/src/main.js
git commit -m "T04: streaming batch test job with cancel, bad-node isolation, scroll-preserving table (by Claude)"
```
然后生成：
- `review/T04.diff`：`git diff <T04-0 那个提交>..HEAD`
- `review/T04-report.md`：按第 5 节的格式写。

### 3.5 打包（只打包，不发布）
```
build.bat
```
- `build.bat` 会自动把版本号加 1（当前 `data/build-version.json` 是 7.0.9），并调用 `scripts\ensure-latest-core.ps1` **下载最新的 sing-box 内核**。请在报告中写明打包后的版本号，以及 `bin\sing-box.exe version` 的输出。
- 打包完成后确认存在 `dist\SanTian-Singbox-windows-amd64.zip`。
- **不要运行 `scripts\publish_release.py`。** 它会在 GitHub 上创建正式 Release，而且里面的更新说明还是旧的（例如写着“default_domain_resolver 从 local 升级为 china”，现在已经是 `direct-dns`）。是否发布、发布说明怎么写，由用户决定。
- **不要合并到 main**，等用户真机测试通过后再合并。

---

## 4. 交给用户的真机测试清单（请原样放进报告末尾）

用打包出来的程序测试：

1. **批量测试流畅度**：粘贴约 300 个节点，选“真连接延迟”开始测试。
   - 第一个结果出现用了多少秒？
   - 结果是陆续出现，还是一批一批出现？
   - 测试过程中往下滚动表格，会不会被拉回顶部？
   - 全部完成（含出口 IP）用了多少秒？
2. **停止**：测试进行中点“停止”，打开任务管理器，看临时的 sing-box 进程是否在 2 秒内消失（主内核那个会保留）。
3. **Ping 测试**：选“Ping 延迟”，完成后 Ping 成功的节点应该也有出口 IP。
4. **坏节点**：在节点列表里混入一个故意写错的节点（例如 uuid 改成 `abc`），其他节点应该正常出结果，这个节点显示“节点配置无效：…”。
5. **网络切换（T03）**：开着 TUN，从 Wi-Fi 切到手机热点再切回来；日志里应该看到“检测到网络变化”，代理在 30 秒内恢复；网络稳定时不应出现“直连 DNS 已切换”。

---

## 5. 报告格式（`review/T04-report.md`）

1. `git status` 的结果（提交前），以及有没有预期之外的改动文件
2. `gofmt` / `go vet` / `go test` / `npm run build` 的结果（通过 / 失败及原文）
3. 你做的修改（如果有），逐条：文件、行、原因
4. 3.3 中真实 sing-box 错误输出的原文
5. 两个提交的哈希
6. 打包后的版本号、sing-box 版本、zip 文件路径和大小
7. 第 4 节的真机测试清单（原样，留给用户填写）
