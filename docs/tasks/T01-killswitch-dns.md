# T01：修复 Kill Switch 逻辑错误 + TUN 模式 DNS 回环风险

> 执行者：Antigravity　｜　审查者：Claude　｜　分支：`fix/t01-killswitch-dns`

## 0. 工作规则（必须遵守）

1. 从 `main` 新建分支 `fix/t01-killswitch-dns`，只在该分支上改。
2. **只改本任务列出的问题**，不要顺手重构、改格式化风格、改无关文件、升级依赖。
3. 不要修改 `dist/`、`bin/`、`build/`、`frontend/`、`.gocache2/`。
4. 所有 Go 文件改完后执行 `gofmt -w`，并运行 `go test ./...`，全部通过。
5. 现有测试如果因本任务的行为变更而失败，可以修改测试，但必须在完成报告里逐个说明改了哪个测试、为什么。
6. 完成后：
   - 把 `review/` 加进 `.gitignore`；
   - 提交到分支（commit message 以 `T01:` 开头）；
   - 执行 `git diff main...fix/t01-killswitch-dns > review/T01.diff`；
   - 执行 `go test ./... > review/T01-test.txt 2>&1`；
   - 写 `review/T01-report.md`：每个子任务改了哪些函数、新增了哪些测试、有没有偏离本任务单的地方及原因。
7. 不确定的地方**不要猜**，在 report 里写出疑问，留给审查。

---

## 1. 背景：已确认的问题

| # | 问题 | 位置 |
|---|------|------|
| A | 程序崩溃/被强杀后，防火墙出站策略停留在 `blockoutbound`，重启电脑也不恢复，整机断网；启动自检和 watchdog 都不清理 | `killswitch_windows.go`，`app.go` `startup` / `cleanupStartupResiduals` |
| B | TUN 模式下开启 Kill Switch：应用流量发往 FakeIP（198.18.0.0/15）或真实 IP，不在放行列表，被防火墙拦截 → 断网 | `enableKillSwitch` |
| C | 正常点"启动"不会开启 Kill Switch（只在"运行中改设置"和熔断时开启）；点"停止"也不会关闭 | `app.go` `startLocked`（约 3515 行）、`stopProxyForUser`（约 546 行） |
| D | 放行了所有目标的 UDP 53，DNS 可以直接发到公网，防泄露失效 | `enableKillSwitch` 中 `%sDNS` 规则 |
| E | `disableKillSwitch` 无条件把出站策略设为 `allowoutbound`，每次退出都覆盖用户原有防火墙策略 | `disableKillSwitch` |
| F | 白名单规则添加失败（错误被 `_ =` 吞掉）后仍然设置 `blockoutbound`，可能把 sing-box 自己也拦掉 | `enableKillSwitch` |
| G | TUN 模式下 DNS 直连规则和启动配置的 DNS final 使用 `local`；Windows 上 `local` 读取系统 DNS，可能读到 TUN 网卡自己的 DNS，查询绕回 TUN 被 hijack，形成回环 | `singbox.go` `buildDNSConfig`（约 3343–3368 行） |

---

## 2. 子任务

### T01-1　保存并恢复原始防火墙策略（修 A、E）

1. 新增一个状态文件，路径为 `<root>/data/killswitch-state.json`（`root` 与 `App.root` 一致；`enableKillSwitch` / `disableKillSwitch` 需要能拿到这个路径，改成传参即可）。
   内容至少包括：
   ```json
   { "active": true, "createdAt": "RFC3339 时间", "pid": 1234,
     "originalPolicy": { "domain": "BlockInbound,AllowOutbound", "private": "...", "public": "..." } }
   ```
2. `enableKillSwitch` 在修改策略**之前**：
   - 用 `netsh advfirewall show <profile>profile firewallpolicy`（domain / private / public 三个）读取当前策略并解析；
   - 如果状态文件已存在且 `active=true`，**不要覆盖其中的 originalPolicy**（防止二次开启把"已阻断"当成原始策略保存）；
   - 先用 `atomicWriteFile`（已有函数）写状态文件，写成功后才允许修改策略。
3. `disableKillSwitch`：
   - **只有**状态文件存在、或者能检测到 `SanTianKillSwitch_*` 规则时才执行恢复；否则什么都不做、直接返回（修 E）；
   - 按状态文件里各 profile 的 originalPolicy 分别恢复；状态文件损坏或缺字段时，回退到 `blockinbound,allowoutbound`，并输出 Warn 日志；
   - 删除临时规则，删除状态文件。
4. 启动清理（修 A）：在 `cleanupStartupResiduals` 中（Windows 分支）加入：如果状态文件存在 → 调用恢复逻辑并输出 Info 日志"启动自检已解除残留 Kill Switch"。即使当前不是管理员也要尝试；失败时用 Error 日志写清楚"请以管理员身份运行一次以恢复网络"。
5. watchdog：阅读 `proxy_watchdog_windows.go`，如果 watchdog 在主进程异常退出时负责清理系统代理/TUN，就在同一处加入 Kill Switch 恢复。如果 watchdog 的结构不适合加，在 report 里说明原因，不要强行改。

### T01-2　白名单失败时不允许阻断（修 F）

`enableKillSwitch` 中 Core、App、LAN 三条放行规则，任意一条 `netsh` 返回错误 → 删除已加的规则、删除状态文件、**不设置** `blockoutbound`，返回 error，并输出 Error 日志。DHCP 规则失败只记 Warn，不中断。

### T01-3　Kill Switch 与 TUN 互斥（修 B）

- 当 `effectiveTun(settings)` 为真时，不修改防火墙策略：TUN 的 `strict_route` 已经负责防泄露。输出一次 Info 日志："TUN 模式已由 strict_route 防泄露，跳过防火墙 Kill Switch"。
- 熔断路径（`app.go` 约 2795 行 `enableKillSwitch` 调用处）也要遵守同样的规则：需要拿到当前 settings 判断。
- 如果从"系统代理 + Kill Switch 已生效"切换到 TUN，要先解除 Kill Switch。
- 建议把判断收敛成一个函数，比如 `shouldApplyFirewallKillSwitch(settings Settings, circuitBreaker bool) bool`，方便测试。

### T01-4　启动/停止时同步 Kill Switch（修 C）

- `startLocked` 在 `a.manager.Start(state)` 成功、`emitState` 之前：满足 `shouldApplyFirewallKillSwitch` 时调用 `enableKillSwitch`。开启失败**不要**让代理启动失败，只记 Error 日志（代理能用比防火墙规则更重要）。
- `stopProxyForUser`：停止内核后调用 `disableKillSwitch`，但熔断激活时除外（与 `app.go` 约 2592 行现有逻辑保持一致）。
- 现有 `app.go` 约 2586 行"运行中改设置"处的逻辑，改为同样调用 `shouldApplyFirewallKillSwitch`。

### T01-5　收紧 DNS 放行（修 D）

- 删除"放行所有目标 UDP 53"的 `%sDNS` 规则，同时从 `removeKillSwitchRules` 的列表里移除 `"DNS"`。但为了兼容旧版本残留，**删除旧规则时仍要尝试删除 `SanTianKillSwitch_DNS`**。
- 局域网 DNS（路由器）已经被 `%sLAN` 规则覆盖，不需要额外规则。
- 在 report 里写明：系统代理模式下，未走代理的应用仍可通过路由器 DNS 解析（属于已知限制）。

### T01-6　TUN 模式下 DNS 不使用 `local`（修 G）

在 `buildDNSConfig` 中：

- 当 `effectiveTun(settings)` 为真时：
  - `clash_mode: direct` 的 DNS 规则 `server` 改为 `"china"`；
  - `includeRemoteRuleSets == false` 时 `dnsFinal` 改为 `"china"`。
- 非 TUN 模式保持原样（仍然用 `local`）。
- `local` 服务器条目本身保留在 servers 列表中，不要删除（其他地方可能引用）。
- 改完后在本机实测一次：开 TUN，切到直连模式，访问几个网站，确认 sing-box 日志中没有 `loopback` 字样或 DNS 超时。把结果写进 report。

---

## 3. 可测试性要求

防火墙命令无法在单元测试中真实执行，请：

- 在 `killswitch_windows.go` 中把命令执行抽成包级变量，比如 `var runFirewallCommand = func(args ...string) (string, error) { return runCommandOutput(args[0], args[1:]...) }`，测试时替换成记录调用的假实现；
- `hasAdminPrivileges` 同理，可通过包级变量注入。

## 4. 必须新增的测试（新建 `killswitch_windows_test.go` 和/或补充 `singbox_config_test.go`）

1. `enable` 前读取策略并写状态文件；状态文件写入失败 → 不执行 `blockoutbound`。
2. 已有 `active=true` 状态文件时再次 `enable`，originalPolicy 不被覆盖。
3. Core 规则失败 → 不执行 `blockoutbound`，返回 error。
4. `disable` 在没有状态文件、没有规则时不执行任何 `set ... firewallpolicy` 命令。
5. `disable` 按状态文件恢复各 profile 的原始策略；状态文件损坏 → 回退默认值。
6. 生成的命令中不再包含 `remoteport=53` 的全局放行。
7. `shouldApplyFirewallKillSwitch`：TUN 开 → false；系统代理 + KillSwitch → true；熔断 + 非 TUN → true；熔断 + TUN → false。
8. `buildDNSConfig`：TUN + 直连规则 server == "china"；TUN + `includeRemoteRuleSets=false` 时 final == "china"；非 TUN 时两者仍为 "local"。

## 5. 验收标准（审查时逐条检查）

- [ ] 问题 A–G 都有对应代码修改，并且有测试覆盖
- [ ] `go test ./...` 全部通过，输出在 `review/T01-test.txt`
- [ ] 没有改动任务范围以外的文件（`.gitignore` 除外）
- [ ] `review/T01.diff`、`review/T01-report.md` 齐全
- [ ] report 里写了 T01-6 的实测结果
