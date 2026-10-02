<div align="center">

# NodeSweep

**服务器空间，一目了然。**

轻量级多服务器磁盘分析与日志清理面板。用可下钻的矩形树图找到空间去向，使用内置或自定义方案预览并清理过期日志归档。

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [繁體中文](README.zh-TW.md) · [部署说明](docs/deployment.md) · [架构与安全边界](docs/architecture.md) · [开发路线](docs/roadmap.md)

</div>

> **v0.1 Alpha**：已实现单机/多节点、实时状态、空间树图、方案规则和归档日志清理。请先在测试 VPS 验证规则。本版本不直接清空活跃日志、不删除 Docker 日志文件、不处理数据库与备份。

## 界面预览

![磁盘分析界面](docs/images/disk-analysis.png)

截图来自本地测试环境：一个管理端、两个独立 Agent 和测试日志目录。

## 为什么做 NodeSweep

磁盘告警只告诉你“空间快满了”。NodeSweep 把下一步也放进面板：

1. 在一个页面查看多台 VPS 的 CPU、内存、负载、磁盘容量和 inode。
2. 点击最大的方块，逐层进入目录，找到真正占空间的文件。
3. 选择清理方案，先看清单和预计处理空间，再确认执行。
4. 在任务记录中查看成功、失败、跳过和执行结果。

## 当前功能

加入持久化节点分组、重命名及总览和批量页面分组筛选；指标上报保留管理员修改。 加入不删除文件的批量扫描和规则预览，每批最多 20 台在线节点、同时处理 2 台；结果与错误按节点显示，排除离线节点，单台失败不阻断其他节点。 保留节点本地允许目录与单节点清理确认；离开批量页面停止后续提交和浏览器轮询，已提交任务仍可在任务记录查看。

| 功能 | v0.1 |
|---|---|
| 单机 / 中央管理端 / Agent | ✅ 同一个可执行文件，三种模式 |
| 多节点状态 | ✅ 5 秒采样上报，45 秒未上报显示离线 |
| CPU、内存、负载、磁盘与 inode | ✅ Linux 真实数据 |
| 可下钻矩形树图 | ✅ d3 布局，目录列表、返回上级 |
| 清理方案 | ✅ 一个方案多条规则，自定义路径/匹配/排除/保留天数 |
| 常用目录预设 | ✅ Linux、Nginx、宝塔网站/面板、1Panel 默认日志目录 |
| 环境识别 | ✅ 解析常见目录、1Panel 安装路径和宝塔静态日志配置 |
| 清理安全检查 | ✅ 白名单、预览到期、打开文件检查、文件身份核验 |
| 节点凭证 | ✅ 独立凭证，服务端保存摘要，可撤销 |
| 任务记录 | ✅ SQLite 持久化，重启不重放任务 |
| 方案 JSON 导入导出 | ✅ 全量/单方案导出、整体校验、事务导入、重复跳过 |
| 原生日志轮转、journal / Docker 清理 | 后续版本 |
| 定时方案、批量节点清理执行、历史指标曲线 | 后续版本 |
| 自定义面板安装路径自动解析 | ✅ 静态路径识别，可配置 panelRoots |

预设只生成规则，不会修改 Agent 的本地权限白名单。没有对应目录的预设不会直接启用。

## 快速开始

从 [Releases](https://github.com/While-Shark/NodeSweep/releases) 下载对应 Linux amd64 / arm64 压缩包，核对 `SHA256SUMS` 后解压：

```bash
./nodesweep -init
./nodesweep -config config.json
```

默认仅监听 `127.0.0.1:9780`。读取 `config.json` 中的 `adminToken` 登录。令牌不会打印到启动日志，也不会保存到浏览器本地存储。

从本地电脑访问 VPS 可以使用 SSH 隧道：

```bash
ssh -L 9780:127.0.0.1:9780 root@你的服务器
```

再打开 `http://127.0.0.1:9780`。多节点部署请通过现有 Nginx、宝塔或 1Panel 配置 HTTPS 反向代理。NodeSweep 不占用 443。

若尚无发布包，使用源码构建：

```bash
git clone https://github.com/While-Shark/NodeSweep.git
cd NodeSweep
cd web && npm ci && npm run build && cd ..
go build -trimpath -ldflags='-s -w' -o nodesweep ./cmd/nodesweep
./nodesweep -init
./nodesweep
```

构建推荐受支持且已修补的 Go（CI 使用 1.27.x）、Node.js 22.18+（或 24+）。运行只需要编译后的 `nodesweep`，无需 Node.js、外部数据库、Redis 或消息队列。

## 添加其他 VPS

1. 面板点击 **添加节点**，下载独立 Agent 配置。
2. 把 `hub` 改成中央面板的 HTTPS 地址。
3. 上传同一份对应 CPU 架构的二进制和配置文件到目标 VPS。
4. 设置清理白名单，启动 Agent：

```bash
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json
```

Agent 主动向管理端发起 HTTPS 请求，无需为节点开放入站端口。默认最多每 5 秒取一次任务，执行时仍持续上报状态。

## 界面语言

支持简体中文、English、日本語、한국어、繁體中文。登录页和控制台右上角均可切换；首次按浏览器语言选择，之后记住手动选择。切换无需刷新，日期与数字使用当前语言格式。

节点名、规则名、方案名、文件路径和配置 JSON 保持原值，方便跨语言协作和排查。常见操作错误已翻译；任务详情 JSON 和无法识别的系统诊断保留原文。

## 清理之前

默认清理白名单只有 `/var/log`。例如确认宝塔日志路径后，可修改节点配置：

```json
"cleanupRoots": [
  "/var/log",
  "/www/wwwlogs",
  "/www/server/panel/logs"
]
```

重启该 Agent 后，在面板识别环境、添加方案并预览。1Panel 会读取 `1pctl` 中的静态安装路径；宝塔会读取 Nginx 站点配置中的静态日志路径。自定义宝塔安装目录可在节点设置 `panelRoots`，详见 [目录识别](docs/deployment.md#面板目录识别)。识别不会扩大清理白名单。不要把站点根目录、数据库目录或整个 `/opt` 当成日志目录。

v0.1 使用保守规则：普通文件、单硬链接、未被打开、超过保留天数，并且文件名属于压缩归档、数字轮转日志或严格校验过的 Lumberjack 时间戳归档。活跃 `.log`、符号链接、跨挂载点、排除目录不参与清理。每条规则最多预览 5,000 个候选文件。

完整安全边界、权限要求和中断处理见 [架构文档](docs/architecture.md)。

## 自动发布

Nightly 仅在 master 有新提交且 CI 通过后自动更新，供测试使用。版本 Release 根据 `VERSION` 自动创建；修改版本号后产生新版本，已发布版本不覆盖。也支持匹配 `VERSION` 的 v* 标签。两个渠道提供 Linux amd64/arm64 压缩包、SHA256SUMS、多语言文档和构建信息。运行 `./nodesweep -version` 查看版本、提交和构建时间。

配置必须是权限 0600 的普通单硬链接文件。目录逐层固定文件描述符并拒绝链接；需要 `/proc/self/fd` 可用。安全审计和剩余边界见 [security.md](docs/security.md)。

发布说明会包含五种语言的更新摘要。修改内容时同步维护 `docs/release-notes.json`，其中版本须与 `VERSION` 一致；发布时自动生成 Release 页面说明并放入新构建的压缩包。

## 安装、报告与告警

解压后运行 `sudo bash install.sh --start`。也可用 `sudo bash install.sh --download v0.1.0-alpha.3 --start` 校验并安装指定版本，测试可选择 nightly。升级保留配置、备份标准停机状态，启动失败自动恢复旧二进制；`--rollback` 只切换二进制，不覆盖当前数据库与配置。自定义数据路径需自行备份，旧快照需确认后手动清理。

规则试运行解释匹配和排除原因，不创建可执行预览；清理报告显示逐文件结果、分配空间与执行时间，失败任务的部分结果可在任务记录查看。分配空间不等于磁盘可用空间净增加。

可在“磁盘告警”开启容量、inode 和离线告警，支持重复间隔、恢复事件和最近 100 条记录。通知需在受保护的管理端配置加入 `webhookURL` 并重启，仅支持公开 HTTPS 的通用 JSON 接收端，浏览器不会获取地址。请同步升级管理端与 Agent，详见[部署说明](docs/deployment.md)。


## 开发与验证

```bash
cd web && npm ci && npm run build && cd ..
go test -race ./...
go vet ./...
go build ./cmd/nodesweep
```

安全测试覆盖路径逃逸、符号链接、硬链接、打开文件、预览过期/重放、文件被修改/替换、节点凭证隔离和重启不重放。运行沙箱可能限制 `/proc`；测试通过受控检查器验证当前测试进程的真实文件描述符，生产环境不存在跳过检查的配置开关。

## 致谢

设计参考 [Beszel](https://github.com/henrygd/beszel) 的轻量多节点监控方向、[gdu](https://github.com/dundee/gdu) 的磁盘探索体验。NodeSweep 独立实现，不包含这两个项目的源码。矩形布局使用 [d3-hierarchy](https://github.com/d3/d3-hierarchy)。

## 许可证

[MIT](LICENSE)
