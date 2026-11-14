# T03：网络切换 / 休眠唤醒后自动更新直连 DNS

> 执行者：Antigravity　｜　审查者：Claude　｜　分支：`fix/t03-network-change`
> **前置条件：T02（含第 1 轮修改）已合并到 `main`。** 如果 `main` 里还没有 T02，请停下，不要开始。

## 0. 工作规则（与 T01/T02 相同）

1. 从合并 T02 之后的 `main` 新建分支 `fix/t03-network-change`，只在该分支上改。
2. 只改本任务列出的内容，**不改前端**，不升级依赖，不修改 `dist/`、`bin/`、`build/`、`.gocache2/`。
3. 改完执行 `gofmt -w`，并运行 `go test -v ./...`，全部通过。
4. 修改了现有测试的，在 report 里逐个说明。
5. 完成后提交（`T03:` 开头），生成 `review/T03.diff`、`review/T03-test.txt`（`go test -v`）、`review/T03-report.md`。report 中只写实际做过的验证。

---

## 1. 背景：T02 引入的风险

T02 之后，直连 DNS 用的是**当前网络**的 DNS，常见的是路由器地址，比如 `192.168.1.1`。这个值只在内核启动时确定一次。

如果用户在代理运行期间切换了网络：
- 笔记本从家里 Wi-Fi 换到公司网络、手机热点；
- 合上盖子休眠，到另一个地方唤醒；
- 网线和 Wi-Fi 之间切换；

那么配置里的 `direct-dns` 仍然指向旧网络的 DNS，新网络里这个地址不存在或不可达。结果是：
- 节点服务器域名解析失败，**所有用域名的节点都连不上**；
- 直连网站的解析也失败。

而现在的健康检查（`ensureProxyHealth` → `HealthCheck`）只检查本地端口和 Clash API 是否在线，**发现不了这种情况**，代理会一直坏着。

T02 之前用的是固定的 223.5.5.5，在大陆任何网络下都可达，所以不会出现这个问题。**这是 T02 带来的回归，必须在发布前修复。**

---

## 2. 子任务

### T03-1　记录当前运行使用的直连 DNS

- `SingBoxManager` 新增字段 `runningDirectDNS string`。在 `startLocked` 成功启动后写入本次使用的 `dnsPlan.Direct.Server`，停止时清空。
- 新增加锁的读取方法 `RunningDirectDNS() string`。

### T03-2　网络指纹

在 `direct_dns.go`（纯逻辑）以及 Windows/非 Windows 文件中：

- 定义 `type networkFingerprint struct { IfIndex uint32; LocalIP string; DNS string }`，其中 DNS 是 primary 和 secondary 合并后的字符串，例如用 `,` 连接，方便比较。
- Windows 实现 `currentNetworkFingerprint() networkFingerprint`：复用 `findPhysicalNetworkInterface` 和 `detectPhysicalDNSServers`。**这里不做任何网络探测**，只读本机信息，必须很快完成（目标 < 50ms）。
- 非 Windows 返回零值。
- 提供 `invalidateDirectDNSCache()`，清空 T02 的 60 秒缓存。

### T03-3　决策函数（纯函数，必须单测）

**设计原则：网络稳定时，重启次数必须为 0。** 只有“当前直连 DNS 确实不可用，并且找到了另一个可用的 DNS”时才允许重启。不能因为“另一个 DNS 更快”就切换，否则测速结果的正常波动会导致内核反复重启、代理掉线。

```go
type directDNSAction int
const (
    directDNSKeep directDNSAction = iota     // 什么都不做
    directDNSRestart                          // 需要重启内核切换直连 DNS
)

func decideDirectDNSAction(running string, runningReachable bool, resolved string, resolvedSource string) directDNSAction
```

规则（按顺序判断）：
1. `runningReachable == true` → `Keep`。**当前 DNS 还能用就绝不切换**，哪怕网络变了，或者有更快的 DNS。
2. `resolvedSource == "default"`（全部探测失败，说明网络可能整体不通）→ `Keep`。不要在断网时反复重启。
3. `resolved == running` → `Keep`。
4. 其他情况 → `Restart`。

### T03-4　在现有健康检查中接入

位置：`app.go` 的 `startProxyHealthWatcher`（每 10 秒一次）以及唤醒后的 `scheduleResumeHealthChecks` 路径。复用现有流程，**不要新开独立的 goroutine 或 ticker**。

在 `ensureProxyHealth` 中，内核运行且现有检查通过之后，增加：

1. **指纹检查（每次都做）：** 计算当前指纹，与上一次记录的指纹比较（记录在 `App` 字段里，加锁访问）。
   - 第一次没有记录时，只记录、不动作。
   - 指纹变化时：记 Info 日志 `检测到网络变化：<旧 IfIndex/IP> → <新 IfIndex/IP>`，然后等 **3 秒**让新网络的 DHCP/DNS 稳定，再执行下面的“重新选择”。
2. **直连 DNS 可用性检查（每 30 秒一次，即每 3 个周期）：** 用 `probeDirectUDPDNSFunc` 探测 `RunningDirectDNS()`。
   - 连续 2 次失败才执行“重新选择”；成功时计数清零。
3. **重新选择：**
   - 调用 `invalidateDirectDNSCache()`，然后 `resolveDirectDNSPlanFunc(logger)`；
   - 用 `decideDirectDNSAction` 决定是否重启；
   - 需要重启时：遵守现有的 `lastHealthRestart` 20 秒间隔限制，持有 `proxyOpMu`（使用 `TryLock`，与现有代码一致），调用 `a.manager.Restart(state, effectiveTun(state.Settings))`，记 Info 日志 `直连 DNS 已切换：<旧> → <新>，已重启内核`；
   - 不需要重启时：记一条 Info 日志说明原因（例如“网络可能未连通，暂不切换”），**同一原因 5 分钟内只记一次**，避免刷屏。
4. 系统唤醒路径（`onSystemResume`）：唤醒后的第一次健康检查要**强制执行一次“重新选择”**，不用等指纹变化，因为有些网络切换前后指纹可能相同（比如同一张网卡、同一个 IP 段）。

### T03-5　不改的内容

- 不改 DNS 配置生成逻辑（`buildDNSConfig`）。
- 不改 Kill Switch。
- 不改前端。

---

## 3. 必须新增的测试

1. `decideDirectDNSAction`：default → Keep；相同 → Keep；不同且 source 为 physical / physical-secondary / fallback → Restart。
2. 指纹比较：IfIndex、LocalIP、DNS 任意一项变化都判定为变化；完全相同判定为未变化。
3. 健康检查接入逻辑：把“指纹检查 + 可用性计数 + 重新选择”抽成一个可测试的方法（依赖通过函数变量注入：指纹、探测、resolve、restart、时间）。测试以下场景：
   - 指纹从 A 变为 B，resolve 返回新地址 → restart 被调用 1 次；
   - 指纹不变，探测连续失败 1 次 → 不 resolve；连续失败 2 次 → resolve；
   - resolve 返回 default → 不 restart；
   - 距离上次重启不足 20 秒 → 不 restart；
   - 唤醒后的第一次检查 → 即使指纹未变也会 resolve。
4. `RunningDirectDNS()`：启动后等于本次的 plan，停止后为空（如果直接测 `startLocked` 太重，可以只测赋值与清空的那段逻辑）。

## 4. 验收标准

- [ ] 第 2 节全部完成，第 3 节测试存在并通过
- [ ] 没有新增独立的 ticker 或 goroutine，复用现有的健康检查周期
- [ ] 断网（全部探测失败）时不会反复重启内核
- [ ] 前端、DNS 配置生成、Kill Switch 没有改动
- [ ] report 中如实写明做过哪些真机验证。如果有条件，请在真机上实际做一次“开着 TUN 从 Wi-Fi 切到手机热点”，并贴出相关日志行
