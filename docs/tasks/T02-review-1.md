# T02 审查意见（第 1 轮）

> 审查者：Claude　｜　分支：`fix/t02-direct-dns`　｜　结论：**小改一轮后可以合并**

整体做得很好：
- `china` 已全部替换为 `direct-dns`，DNS 引用一致性测试也写了；
- 探测时通过 `IP_UNICAST_IF` 绑定物理网卡，字节序正确；
- 死代码清理干净，前端没有改动；
- 报告如实区分了“已做”和“未做”。

只有选择逻辑上的 3 个小问题，一轮就能改完。

---

## R1【中】主网卡的 DNS 应该优先，而不是和其他网卡一起比速度

位置：`direct_dns.go` `chooseDirectDNS`、`direct_dns_windows.go` `detectPhysicalDNSServers`

现状：默认路由网卡的 DNS 和其他 Up 网卡的 DNS 合并成一个列表，然后取最快的。
问题：电脑上常有 Hyper-V vEthernet、VMware、VirtualBox、公司 VPN 等虚拟网卡，它们也可能带 DNS，而且响应可能更快（比如本机虚拟交换机），结果会选中一个并不代表用户真实上网环境的 DNS。
（这一点是我任务单写得不严谨，不是你的执行问题。）

要求：
- `detectPhysicalDNSServers` 改为分别返回两组：`primary`（默认路由网卡）和 `secondary`（其他网卡），两组各自经过 `filterDirectDNSServers` 过滤。函数名和签名可以调整，比如 `detectPhysicalDNSServers() (primary, secondary []string)`。
- 选择优先级：**primary 中可用且最快的 > secondary 中可用且最快的 > fallbacks 中可用且最快的 > `fallbacks[0]`**。
- source 区分为 `physical`（primary）、`physical-secondary`、`fallback`、`default`；日志分别写“网卡”、“其他网卡”、“公共候选测速”、“默认”。

## R2【低】三组改为同时探测，控制总耗时

现状：先探测 physical，全部失败后再探测 fallbacks，最坏约 2 秒。加上 R1 的 secondary 之后，最坏会变成 3 秒。而 `startLocked` 执行时持有 `m.mu`，这期间界面的状态刷新（`Running()`）会被卡住。

要求：primary、secondary、fallbacks 三组**一次性并发探测**（每个地址只探测一次，同一个地址出现在多组时去重），所有结果到齐（或者整体 1.2 秒超时）后，再按 R1 的优先级挑选。最坏耗时应约等于单次探测超时。

## R3【低】“全部失败”的结果不要缓存

现状：全部探测失败返回 `fallbacks[0]`（source = default），这个结果也会缓存 60 秒。
问题：开机后网络还没就绪时启动，会在 60 秒内一直沿用这个兜底结果。

要求：source 为 `default` 时不写缓存。

---

## 需要调整或补充的测试（`direct_dns_test.go`）

1. 修改 `TestChooseDirectDNS`，覆盖以下情况：
   - primary 可用但比 secondary 慢 → 仍然选 primary，source 为 physical；
   - primary 全部不可用、secondary 可用 → 选 secondary，source 为 physical-secondary；
   - primary 和 secondary 都不可用 → fallback 中最快的；
   - 全部不可用 → `fallbacks[0]`，source 为 default；
   - 同一地址同时出现在 primary 和 fallbacks 中 → probe 只被调用一次（用计数器断言）。
2. 新增：全部探测失败（default）后，第二次调用会再次触发 probe（不走缓存）。
3. 新增：probe 中有一个会卡住超过整体超时的地址时，`chooseDirectDNS` 仍在约 1.2 秒内返回（测试里可以把超时做成可注入的参数，用较短的值来测）。

## 完成后

在同一分支上追加提交（`T02-r1:` 开头），重新生成 `review/T02.diff`、`review/T02-test.txt`（`go test -v`），并在 `review/T02-report.md` 末尾追加“第 1 轮修改”一节。
