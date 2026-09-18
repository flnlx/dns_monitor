# DNS Monitor

本地运行的 Windows IPv4 DNS 监测工具。Go 后端、原生网页、SQLite 存储，使用同目录 DOGGO 发送 DNS 请求。无需安装运行时，无在线网页依赖。

## 启动与移动

1. 解压 `dist/DNSMonitor-windows-amd64.zip`，或进入 `dist/DNSMonitor`。
2. 双击 `start.cmd`。网页默认地址 `http://127.0.0.1:8080`，默认监听 `0.0.0.0:8080`。
3. 首次运行会生成 `data/access-key.txt`。将里面的密钥粘贴至登录页；同一密钥用于本机和局域网访问。会话仅保存在浏览器内存中，刷新后重新登录，12 小时过期。
4. 添加服务器与供应商；在服务器表单勾选你认可的可信 DNS；配置域名后开始监测。

数据与配置保存在 `data/monitor.db`，滚动日志在 `logs`。退出程序后复制**整个文件夹**即可迁移。备份前也应先正常停止；运行期间 SQLite 可能同时使用 `.db-wal`、`.db-shm`。不要只复制正在使用的 `.db`。

仅面向 Windows 10/11、Windows Server 2016 及以上的 amd64 系统与 IPv4 传输。不创建 Windows 防火墙例外。局域网是否可访问取决于现有防火墙配置；默认 HTTP 适用于可信本地网络，跨不可信网络应通过已有 HTTPS 反向代理或 VPN。

## 监测与统计

- 监测服务器的可达情况、平均与 P95 时延、查询成功率；按协议、供应商、污染状态、评级筛选，按时延排序。
- 支持 24 小时、7 日、30 日视图。原始探测记录、回答、可信源证据保留滚动 30 天，程序启动及每小时清理超期数据。清理后 SQLite 文件空间可复用，不保证文件立即变小。
- 收到 DNS 响应代表可达；NOERROR 且有相应记录、或明确的 NXDOMAIN 负回答代表查询成功。无可识别响应码的空回答不计成功。记录不使用 ICMP，不把 DNS 超时称为实际网络丢包率。
- 可用率按已观察时间加权；查询成功率按实际查询次数计算。失败退避期间不会凭空生成成功记录；停止运行后的未覆盖时间保留为空档。覆盖率帮助区分“尚未监测”与“服务故障”。
- 时延使用 DOGGO 提供的查询耗时；P95 使用向上取整至 1ms 的直方图统计，误差不超过 1ms。趋势图使用聚合数据；原始记录支持 CSV 导出。

## 探测配置

默认周期 300 秒、超时 3 秒、并发 2。并发范围 **0–50**，**0 表示暂停所有新探测**，包括手动触发。已有请求会在超时界限内结束。

智能惩罚对连续失败的服务器逐步延长周期，最大退避 **0–24 小时**，默认 **1 小时**。**0 表示关闭退避**。退避上限不低于基础周期，调度加入 0–10% 的负向抖动，最短不低于 5 秒。错峰、抖动、有界任务队列和同服务器不重叠机制限制瞬时负载。长时间离线后首次恢复会在约 30 秒内复核，然后回归基础周期。更改监听地址需重启生效，其余配置热生效。

支持多个探测域名和记录类型。仅 IPv4 限制的是访问 DNS 的网络传输与服务器地址。域名应填写 ASCII/Punycode，不接受 URL；不支持 IPv6 上游。

地址使用 AdGuard Home 常用上游写法，例如：

```text
1.1.1.1
8.8.8.8:53
udp://9.9.9.9:53
tcp://1.1.1.1
tls://dns.google
https://cloudflare-dns.com/dns-query
h3://dns.google/dns-query
quic://dns.adguard-dns.com
sdns://...
```

DNS stamp 支持普通 UDP、IPv4 DNSCrypt，以及不附带固定 IP、证书 hash 和 bootstrap 约束的 DoH/DoT/DoQ。随包 DOGGO 不能正确保留这些加密 stamp 约束，遇到它们会明确拒绝，建议填写对应标准上游 URL。AdGuard Home 的分域路由指令（如 `[/example.com/]...`）、特殊 `#` 系统默认上游和不安全 TLS 选项不属于单台被测服务器地址，保存时会明确拒绝。域名上游启动解析依赖主机可用的 IPv4 DNS；本项目不修改系统 DNS。探测隔离继承的 DOGGO 配置和 HTTP(S) 代理环境变量，直接测量 DNS 访问路径。加密协议额外绑定 IPv4 来源，保留 TLS 证书校验。

## 污染判定与人工修正

在服务器管理中，由用户勾选可信 DNS。系统对同一域名和记录类型收集有效可信回答并缓存：可信源有效回答一致、被测回答与其不同，直接标为“污染”；可信源失败、无可比较答案或互相冲突时显示“无法判定”。不会让被测服务器用自己的答案证明自己清白。参考查询也受全局并发上限限制，并保留实际查询证据。

判定按规范化的回答集合比较，忽略 TTL 和顺序。CDN、地域调度和过滤策略可能导致真实答案差异，因此这里的“污染”是**与用户可信源不一致的产品判定**，不等同于证明网络攻击。可点击域名标记，手动定罪、取消定罪，或恢复自动判断；人工决定持续作用于该服务器、域名、记录类型，保留说明和审计记录，不覆盖原始探测证据。

DOGGO 的 `--do`、`--ad` 与权威查询不足以自行证明 DNSSEC 完整签名链已验证，因此本版不会依据这些标志自动创建“系统可信 DNS”。需要用户指定可信源。

评级优先级：所选时间范围内存在有效污染判定则 **F**；其余按可用率 45%、查询成功率 35%、P95 时延 20% 评分得到 A–D（A ≥95，B ≥85，C ≥70，其余 D；P95 ≤50ms 获得满额时延分，≥2000ms 为零，中间线性递减）。少于 10 个样本或覆盖不足 30 分钟为“待评估”。无可信源时保留“未检测/无法判定”提示，不把它显示为已经验证清白。

## Windows 服务

网页“Windows 服务”页与 `manage-service.cmd` 提供安装、卸载、启动、停止、重启、状态查询。管理时使用 Windows 原生管理员授权，安装不会自动启动，也不会自动改动防火墙。

服务名 `DNSMonitor`，延迟自动启动，使用 Windows 默认 LocalSystem 服务账户。要切换到服务运行，请先安装，然后正常退出当前便携进程，再通过 `manage-service.cmd` 启动服务，避免端口和数据文件被两个实例占用。服务运行时也可以访问相同网页。管理结果写入 `logs/service-action.log`。

**移动目录前先停止并卸载服务，移动后重新安装**；服务注册了绝对路径。普通便携运行不需要服务。

管理员终端也可使用：

```powershell
.\dns-monitor.exe service install
.\dns-monitor.exe service start
.\dns-monitor.exe service status
.\dns-monitor.exe service stop
.\dns-monitor.exe service uninstall
```

可选启动参数必须置于子命令之前，例如：

```powershell
.\dns-monitor.exe --listen 127.0.0.1:8088 --open
.\dns-monitor.exe --data-dir D:\DNSData --doggo D:\Tools\doggo.exe service install
```

## 开发与构建

Go 1.25+，Windows amd64。无需 Node.js 或 C 编译器；SQLite 使用 Go 驱动。已有 DOGGO 位于 `doggo`。首次构建需要联网下载 Go 模块。

```powershell
.\build.ps1
```

构建脚本执行测试和 `go vet`，生成便携目录与 ZIP，并收集第三方许可证。工具链如不在 PATH，可放入 `.tools/go`。`.tools`、`data`、`logs`、`dist` 均不进入 Git。

主要代码：`cmd/dns-monitor` 启动与生命周期，`internal/httpapi` 接口与会话，`internal/monitor` 探测调度，`internal/store` 存储与统计，`internal/web` 网页，`internal/winservice` Windows SCM 管理。

CPU 和内存开销随服务器数量、周期、并发、协议变化。默认并发 2，每次只启动有界数量 DOGGO 子进程；图表查询聚合历史并短暂缓存，日志限量滚动。批量探测量约为 `服务器数 × 域名数 × 86400 / 周期秒数` 次/日，另加有缓存的可信源查询。原始证据保留意味着磁盘用量随探测量增长。为限制异常子进程输出，每次 DOGGO 的 stdout/stderr 各最多保留 256 KiB；超限记录错误，不用于污染比对。

DOGGO 是随包独立程序，许可证见 `doggo/LICENSE`；其他依赖许可证见发行包 `licenses`。
