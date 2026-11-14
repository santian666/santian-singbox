# T01 审查意见（第 1 轮）

> 审查者：Claude　｜　分支：`fix/t01-killswitch-dns`　｜　结论：**需要修改后再合并**

整体完成度不错：子任务都有对应改动，测试注入点（`runFirewallCommand` / `checkAdminPrivileges`）设计合理，watchdog 也接上了。
gofmt 带来的 `RuntimeState`、`SingBoxManager` 字段对齐属于格式化副作用，可以接受。

但 Kill Switch 还有 3 处会导致“整机断网且没有任何记录可以自动恢复”，必须修复。

---

## R1【高】恢复失败时仍然删除状态文件

位置：`killswitch_windows.go` `disableKillSwitch`

现状：`restoreProfilePolicy` 失败只记日志，之后照样执行 `removeKillSwitchRules()` 和 `os.Remove(stateFile)`。
后果：策略仍为 BlockOutbound，但放行规则和状态文件都没了，下次启动自检发现不了，sing-box 也被拦，用户彻底断网。

要求：
- `restoreProfilePolicy` 改为返回 error。
- 只要有任意一个 profile 恢复失败（包括回退默认策略那条命令失败）：**不删除状态文件、不删除放行规则**，`disableKillSwitch` 返回 error。
- 全部恢复成功后，才删除规则和状态文件，并把 `killSwitchActive` 置为 false。

## R2【高】enableKillSwitch 不是幂等的

位置：`killswitch_windows.go` `enableKillSwitch`；调用方 `app.go` `SaveSettings`（运行中每次保存设置都会调用）

现状：
1. 已经激活时再次调用，会先 `removeKillSwitchRules()`，而此时策略还是 BlockOutbound，sing-box 和主程序的出站在重新加规则之前会被短暂拦截，已有连接可能断开。
2. 已激活状态下重新加规则失败时，`cleanupOnFailure` 删除了规则和状态文件，**但没有恢复策略**，结果同 R1。

要求：
- 已激活（`killSwitchActive == true`，或者状态文件 `active=true` 且 Core/App/LAN 三条规则都存在）→ 直接返回 nil，不重复操作。
- `cleanupOnFailure` 必须先判断当前策略是否已经是阻断（即 alreadyActive 分支）：如果是，先按状态文件中的 originalPolicy 恢复，恢复成功后再删状态文件；恢复失败则保留状态文件（与 R1 一致）。

## R3【中】旧版本残留会被记成“原始策略”

位置：`enableKillSwitch` 读取原始策略；`app.go` `cleanupStartupResiduals`

现状：T01 之前的版本崩溃后，残留只有 `SanTianKillSwitch_*` 规则和 BlockOutbound 策略，**没有状态文件**。
- 启动自检只检查状态文件 → 不会清理。
- 之后再次开启时，`queryFirewallPolicy` 会读到 BlockOutbound 并当作原始策略保存，以后“恢复”时恢复的还是阻断。

要求：
- 启动自检条件改为：`fileExists(stateFile) || hasAnyKillSwitchRule()`。
- `enableKillSwitch` 在没有有效状态文件时，如果查询到某个 profile 的出站是 BlockOutbound，并且 `hasAnyKillSwitchRule()` 为真，则把该 profile 记为 `<原入站>,AllowOutbound`，并记录 Warn 日志“检测到旧版 Kill Switch 残留，原始出站策略按放行处理”。

## R4【低】报告中的“实测”与实际不符

T01-6 只做了 `sing-box check` 和单元测试，没有真正在 TUN + 直连模式下上网并检查运行日志。
请把 report 第 4 节改成如实描述（“已通过配置校验，未做真机 TUN 运行验证”）。
T01-6 的 DNS 方案会在 T02 中重新设计，这一轮**不需要**补真机测试。

---

## 需要补充的测试（`killswitch_windows_test.go`）

1. disable：某个 profile 恢复命令失败 → 状态文件仍存在、没有执行 `delete rule`、返回 error。
2. enable：已激活时再次调用 → 不执行任何 `delete rule` 或 `add rule` 命令。
3. enable：alreadyActive 状态下 Core 规则添加失败 → 先执行恢复原始策略的命令；恢复成功后状态文件被删除。
4. enable：无状态文件、查询结果为 BlockOutbound、存在旧规则 → 写入状态文件的原始策略为 AllowOutbound。
5. 启动自检：只有旧规则、没有状态文件时也会调用恢复逻辑（如果 `cleanupStartupResiduals` 不方便测，把判断条件抽成小函数单独测）。

## 完成后

在同一分支上追加提交（commit message 以 `T01-r1:` 开头），然后重新生成：
- `review/T01.diff`（仍然是 `git diff main...fix/t01-killswitch-dns`）
- `review/T01-test.txt`（请改用 `go test -v ./... > review/T01-test.txt 2>&1`，便于确认新测试确实运行了）
- 在 `review/T01-report.md` 末尾追加“第 1 轮修改”一节，按 R1–R4 逐条说明。
