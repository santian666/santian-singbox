# T02：直连 DNS 跟随用户真实网络（不再写死中国大陆 DNS）

> 执行者：Antigravity　｜　审查者：Claude　｜　分支：`fix/t02-direct-dns`
> **前置条件：T01 已合并到 `main`。** 如果 `main` 里还没有 T01 的提交，请停下，不要开始。

## 0. 工作规则（与 T01 相同）

1. 从合并 T01 之后的 `main` 新建分支 `fix/t02-direct-dns`，只在该分支上改。
2. **只改本任务列出的内容**，不要重构无关代码，不要升级依赖，不要修改 `dist/`、`bin/`、`build/`、`.gocache2/`。
3. **本任务不改前端**（`frontend/` 完全不要动）。原因见第 5 节。
4. 改完执行 `gofmt -w`，并运行 `go test -v ./...`，全部通过。
5. 如果现有测试因本任务的行为变更而失败，可以修改，但必须在 report 里逐个列出：测试名、原断言、新断言、原因。
6. 完成后：
   - 提交到分支，commit message 以 `T02:` 开头；
   - `git diff main...fix/t02-direct-dns > review/T02.diff`
   - `go test -v ./... > review/T02-test.txt 2>&1`
   - 写 `review/T02-report.md`：每个子任务改了哪些函数、新增了哪些测试、偏离任务单的地方和原因、遇到的疑问。
7. 不确定的地方不要猜，写进 report。**report 里只写实际做过的验证**，没做的就写"未做"。

---

## 1. 背景

现在所有"直连"的 DNS 解析都固定使用名为 `china` 的服务器，候选只有阿里、腾讯、百度（`chinaDNSCandidates`）。用到它的地方包括：

- 节点服务器域名的解析规则；
- `route.default_domain_resolver`（直连出站和代理出站的域名解析）；
- 自定义直连域名、Loyalsoldier direct/china 规则集、内置国内域名列表；
- T01 新加的：TUN 模式下直连模式规则，以及启动配置的 DNS final。

这个软件不只在中国大陆使用。海外用户用阿里 DNS 时：①连接可能很慢；②拿到的是面向国内的 CDN 地址，直连访问当地网站反而变慢。

**目标：** "直连 DNS"改为使用用户当前物理网络自身的 DNS（DHCP 或手动配置的那组），由程序检测出来，以明确的 IP 写进配置。检测不到或都不可用时，从一组不分地区的公共 DNS 中测速选最快的。

这样做也能保留 T01-6 的防回环效果：写进配置的是明确的物理网络 DNS IP，而不是 `local`。

另外，`resolveAutoDNSPlan` 目前没有任何地方调用，属于死代码。本任务用新逻辑替换它。

---

## 2. 子任务

### T02-1　检测物理网卡的 DNS 服务器

新建文件：
- `direct_dns.go`：与平台无关的纯函数、类型和候选列表；
- `direct_dns_windows.go`：Windows 检测实现；
- `direct_dns_other.go`：非 Windows 实现，直接返回 nil。

Windows 实现 `detectPhysicalDNSServers() []string`：

1. 用 `windows.GetAdaptersAddresses` 遍历网卡，读取每个网卡的 `FirstDnsServerAddress` 链表。
2. 跳过以下网卡，判断条件与 `direct_dialer_windows.go` 中 `findPhysicalNetworkInterface` 保持一致：
   - 未处于 Up 状态的；
   - TUN 网卡（按 `tunInterfaceName` 对应的 ifIndex、名称含 `santian`、描述含 `wintun`）；
   - loopback、TAP。
3. **排序：** 承载默认路由的网卡（`findPhysicalNetworkInterface` 返回的 ifIndex）的 DNS 排在最前；其他 Up 的物理网卡的 DNS 排在后面。建议把 `findPhysicalNetworkInterface` 里"找默认路由网卡"的部分复用，不要复制一份新逻辑。
4. 把结果交给 T02-2 的过滤函数，返回过滤后的列表。

### T02-2　过滤函数（纯函数，必须单测）

在 `direct_dns.go` 中实现 `filterDirectDNSServers(ips []string) []string`：

- 只保留合法的 **IPv4**（当前 TUN 是 `ipv4_only`，第一版不处理 IPv6 DNS）；
- 去掉：`0.0.0.0/8`、`127.0.0.0/8`、`169.254.0.0/16`、`172.19.0.0/16`（TUN 网段，用已有的 `isTunSubnetIP`）、`198.18.0.0/15`（FakeIP，用 `isFakeIPv4`）、组播和广播地址；
- 去重，保持原有顺序，最多保留 3 个。

> 已知限制（写进 report 即可，本任务不处理）：如果用户把系统 DNS 设成 `127.0.0.1`（例如本机运行 AdGuard 之类的 DNS 程序），会被过滤掉，然后走公共候选。

### T02-3　选择直连 DNS（测速 + 回退）

在 `direct_dns.go` 中：

1. 新增不分地区的公共候选：
   ```go
   var directDNSFallbackCandidates = []string{"223.5.5.5", "119.29.29.29", "1.1.1.1", "8.8.8.8", "9.9.9.9"}
   ```
2. 实现纯函数 `chooseDirectDNS(physical []string, fallbacks []string, probe func(server string) (time.Duration, bool)) (server string, source string)`：
   - 对 `physical` 中的所有地址**并发**探测；有可用的，取其中最快的一个，`source = "physical"`；
   - 都不可用（或 `physical` 为空）时，并发探测 `fallbacks`，取最快的一个，`source = "fallback"`；
   - 全部失败时返回 `fallbacks[0]`，`source = "default"`；
   - 整体耗时上限约 1.5 秒（每个探测自带超时，并发执行）。
3. 实际使用的探测函数：
   - 基于现有的 `probeUDPDNS` 改造，查询一个不分地区的域名，比如 `www.microsoft.com`；
   - **重点：** TUN 已经在运行时（例如运行中重启内核），直接发出的 UDP 53 会被 TUN 劫持，测速结果是假的。所以 Windows 上探测必须绑定物理网卡发出，复用 `direct_dialer_windows.go` 中绑定物理网卡的拨号逻辑（`newDirectPhysicalDialerForTarget` 或同类函数）。如果复用困难，在 report 里说明你的做法。
4. 结果缓存 60 秒，缓存键是默认路由网卡的 ifIndex，避免每次重启内核都重新测速。网卡变化时缓存自动失效。

### T02-4　替换 DNS 计划和配置生成

1. 修改 `autoDNSPlan`：把 `China dnsProbeCandidate` 改为 `Direct dnsProbeCandidate`（`Type` 固定为 `udp`），`Remote` 保持不变。同步修改 `defaultAutoDNSPlan` 和 `normalizeAutoDNSPlan`：默认的 Direct 为 `directDNSFallbackCandidates[0]`。
2. 新函数 `resolveDirectDNSPlan(logger *Logger) autoDNSPlan`，替换掉死代码 `resolveAutoDNSPlan` 和 `probeBestChinaDNS`：
   - 调用 T02-1 和 T02-3；
   - 输出 Info 日志：`直连 DNS：<ip>（来源：网卡 / 公共候选测速 / 默认）`。
3. `startLocked` 中的 `dnsPlan := defaultAutoDNSPlan()` 改为调用 `resolveDirectDNSPlan(m.logger)`。为了方便测试，通过包级变量注入，例如 `var resolveDirectDNSPlanFunc = resolveDirectDNSPlan`。
4. 在 `buildDNSConfig` 和 `buildSingBoxConfig` 中：
   - DNS 服务器 tag `china` **改名为 `direct-dns`**，server 使用 `dnsPlan.Direct.Server`；
   - 原来所有引用 `"china"` 的地方都改成 `"direct-dns"`，包括：节点域名规则、自定义直连域名规则、Loyalsoldier direct/china 规则、内置国内域名和关键词规则、`route.default_domain_resolver`，以及 T01 加的 TUN 直连规则和 TUN 启动 final；
   - 非 TUN 模式下，直连模式规则和启动 final 仍然使用 `local`，**保持不变**；
   - 删除 `chinaDNSCandidates`。如果别的地方还在用，先在 report 里列出来再决定怎么改，不要直接删。
5. 搜索整个项目里的 `"china"` 字符串（包括测试），确认没有遗漏的 DNS 引用。路由规则集的 tag（如 `loyalsoldier-china`、`geoip-cn`）**不要改**，那是规则集名称，不是 DNS 服务器。

### T02-5　不改的内容（避免范围扩大）

- 分流规则本身（`geoip-cn` 直连、国内域名直连）不改。它对海外用户是否合理，留到以后的任务单独处理。
- 远程 DNS（1.1.1.1 DoH，经代理）不改。
- 不新增任何 Settings 字段，不改前端。

---

## 3. 必须新增的测试

新建 `direct_dns_test.go`（纯函数测试，不要加 build tag，让它在任何平台都能运行）：

1. `filterDirectDNSServers`：
   - 输入 `["192.168.1.1", "127.0.0.1", "172.19.0.2", "198.18.0.5", "0.0.0.0", "169.254.1.1", "fe80::1", "192.168.1.1", "10.0.0.1", "8.8.8.8", "1.1.1.1"]`
   - 期望输出 `["192.168.1.1", "10.0.0.1", "8.8.8.8"]`
2. `chooseDirectDNS`（probe 用假函数）：
   - 物理 DNS 有两个可用 → 选更快的那个，source 为 physical；
   - 物理 DNS 全部不可用 → 从 fallbacks 中选最快的，source 为 fallback；
   - `physical` 为空 → 直接走 fallbacks；
   - 全部不可用 → `fallbacks[0]`，source 为 default。
3. 缓存：同一 ifIndex 在 60 秒内第二次调用不会触发 probe；ifIndex 变化后会重新 probe（时间和 ifIndex 用注入的方式控制）。

在 `singbox_config_test.go`（或新文件）中：

4. 生成的配置中存在 tag 为 `direct-dns` 的服务器，server 等于传入的 `dnsPlan.Direct.Server`；**不存在** tag 为 `china` 的服务器。
5. 节点域名规则、`route.default_domain_resolver.server` 都是 `direct-dns`。
6. TUN 模式：直连模式规则为 `direct-dns`；`includeRemoteRuleSets=false` 时 final 为 `direct-dns`。
7. 非 TUN 模式：直连模式规则和启动 final 仍为 `local`。
8. 配置中任何 DNS 规则或 resolver 引用的服务器 tag，都必须存在于 servers 列表里（写一个通用的一致性检查，TUN/非 TUN、有无规则集各跑一遍）。

## 4. 验收标准

- [ ] 第 2 节的子任务全部完成，第 3 节的测试全部存在且通过（`review/T02-test.txt` 中能看到测试名）
- [ ] 项目中没有 tag 为 `china` 的 DNS 服务器，也没有对它的引用
- [ ] `resolveAutoDNSPlan`、`probeBestChinaDNS`、`chinaDNSCandidates` 已被替换或删除
- [ ] `frontend/` 没有任何改动
- [ ] report 中列出了修改过的现有测试，并且如实写明做过哪些真机验证

## 5. 说明：为什么这次不加"自定义直连 DNS"设置

前端 `main.js` 的 `lt()` 每次保存设置时都会重新构造完整的 settings 对象，**不在其中的字段会被覆盖成默认值**。现在 `killSwitch` 就不在里面，所以界面上任何一次保存都会把它重置为 false。
新增设置项需要同时改前端，还要处理"前端没有传这个字段"的情况，单独放到下一个任务处理。
