# 部署

## 三种模式

- `standalone`：Web 面板 + 本机采集/扫描/清理，可继续添加远程 Agent。
- `hub`：仅中央管理端，不采集或清理本机。
- `agent`：只向管理端上报、获取任务，不开启监听端口。

## systemd

发布压缩包中的 `install.sh` 安装当前目录的二进制，保留已有配置：

```bash
sudo bash install.sh
```

安装位置：

- `/usr/local/bin/nodesweep`
- `/etc/nodesweep/config.json`（0600）
- `/var/lib/nodesweep`（0700，工作目录）
- `/etc/systemd/system/nodesweep.service`

新安装默认是 standalone。检查配置后启动：

```bash
sudo systemctl enable --now nodesweep
sudo systemctl status nodesweep
sudo journalctl -u nodesweep -n 50 --no-pager
```

若安装远程 Agent，先将面板生成的配置保存到 `/etc/nodesweep/config.json`，确认 `hub` 是可访问的 HTTPS 地址，然后启动服务。

升级：停止服务，备份配置和数据目录，替换二进制，重新启动。备份 SQLite 时停服务，复制整个数据目录，不要只复制运行中的 `.db` 文件而遗漏 WAL。

## 现有 Nginx / 宝塔 / 1Panel

创建子域名并配置 TLS，反向代理到 `http://127.0.0.1:9780`：

```nginx
location / {
    proxy_pass http://127.0.0.1:9780;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Real-IP $remote_addr;
    client_max_body_size 32m;
    proxy_read_timeout 45s;
}
```

Agent 使用短轮询，不需要 WebSocket。端口 9780 可修改；无需修改既有 443 网站。不要将明文 HTTP 暴露到公网。若反向代理自身在容器内，容器的 `127.0.0.1` 并非宿主机，请按现有网络拓扑配置宿主访问地址，并通过防火墙限制后端端口。

## 权限

仅监控/扫描时可使用普通用户，无法读取的目录会跳过。执行归档清理要求对目标目录有写权限，并能完整检查 `/proc/*/fd`。推荐宿主机 Agent；容器 PID namespace、hidepid、SELinux 或其他安全策略可能让检查失败，此时清理会拒绝执行。

服务示例使用 root 以便访问系统日志。权限仍受 Agent 配置白名单约束。不要为解决拒绝而关闭宿主机安全机制；可以只使用监控功能或选择权限明确的专用主机。

配置白名单只在节点本机修改，重启后生效；Web 管理员不能通过 API 扩大白名单。

## 规则语义

- `patterns`：匹配**文件名**的 Go filepath glob，例如 `*.log.*`、`*.gz`。
- `excludes`：匹配文件或目录名；匹配目录时整个子树跳过。
- `keepDays`：基于文件修改时间，保留最近 N × 24 小时。
- `scheme`：相同名称的规则在 UI 组合为一个方案。
- 支持 `.gz` / `.xz` / `.bz2` 压缩归档、`.log.` 后接数字/连字符（须含数字），以及 `name-YYYY-MM-DDTHH-mm-ss.SSS.log` 格式的 Lumberjack 归档。时间戳必须是有效日期和时间；普通 `.log` 不处理。
- 1Panel 预设包含时间戳归档模式；其他应用可在 `patterns` 添加 `*-????-??-??T??-??-??.???.log`。保留天数始终依据修改时间，不以文件名时间计算。
- 压缩后缀本身不证明文件是日志；用户必须把规则限制在确认过的日志目录。
- 预览十分钟过期、只执行一次。方案规则顺序执行，不是跨规则事务；重叠规则可能产生“文件已不存在”的跳过项。

## 数据保留

任务记录默认保留 30 天，界面显示最新 100 条。每分钟清理过期数据并将旧扫描结果压缩为摘要，仅保留最近 10 次成功扫描的完整结果。SQLite 删除记录不立即缩小数据库文件，空闲页可复用。

目录扫描最多 100,000 条目，达到上限会标记结果不完整。单任务最长 90 秒；断连超过任务有效期显示中断，不能据此认定清理没有发生。请查看 Agent 和任务记录，重新扫描再决定下一步。

## 故障排查

- `path is outside the agent allowlist`：检查目标节点 `cleanupRoots` / `scanRoots`，不要仅修改中央面板配置。
- `permission denied` / `cannot inspect process`：无法验证文件占用，拒绝清理属于预期行为。
- 节点离线：检查 HTTPS 地址、证书、代理限制、节点凭证和 Agent 日志。
- 重启后预览失效：预览只保存在节点内存中，重新预览。
- 删除量与磁盘空闲变化不一致：并发写入、文件系统快照、稀疏文件、压缩/COW 均可能影响实际回收空间；UI 汇总的是被处理文件的分配块数。

## 清理方案导入导出

清理方案页面支持导出全部规则或单个方案。JSON 使用 `format: "nodesweep.rules"` 和 `version: 1`，单份文件最多 100 条规则、256 KB。规则较多时按方案分别导出。

导入先校验整份文件，再在 SQLite 事务中一次性保存；任意规则或写入失败则整份不生效。文件中的规则 ID 会重新生成，已有规则不会被覆盖，完全相同的规则会跳过。导入只写规则，不执行清理，也不修改节点的 `cleanupRoots`。在目标节点重新预览后再执行。

## 面板目录识别

“识别环境”返回常见目录预设，并读取节点上的以下配置：

- 1Panel：`/usr/local/bin/1pctl` 或 `/usr/bin/1pctl` 的静态 `BASE_DIR`，生成 `<BASE_DIR>/1panel/log` 规则。
- 宝塔：`/www/server/panel/vhost/nginx/*.conf` 中的静态 `access_log` / `error_log` 路径，为每个日志文件生成对应归档规则并排除当前日志。

自定义宝塔安装位置，在 **Agent 或 standalone 本机配置** 中加入面板目录。例如：

```json
"panelRoots": ["/data/server/panel"],
"cleanupRoots": ["/var/log", "/data/server/panel/logs", "/data/site-logs"]
```

`panelRoots` 最多 16 项，只用于读取配置，不授权删除。重新启动节点使配置生效，确认识别结果及对应 `cleanupRoots` 后再预览。规则仍受归档命名、保留天数和占用检查限制；识别出目录不代表所有历史日志格式均可清理。

检测只读取配置，不运行或 source 脚本，不展开变量、命令、Nginx include 或 syslog 地址。单个文件最多 256 KB，每个 Nginx 配置目录最多读取 128 项，总结果最多 100 条。复杂配置、Apache、符号链接目录或不支持的文件名请手动设置规则；是否可以清理由预览中的白名单和路径检查决定。

解析依据：[1Panel 安装器的 1pctl](https://github.com/1Panel-dev/installer/blob/v2/1pctl)，以及宝塔的 Nginx 站点配置。尚需在真实面板版本和 VPS 上验证，不能把配置夹具测试视作实机兼容性认证。

时间戳归档格式参考：[Lumberjack 官方源码](https://github.com/natefinch/lumberjack/blob/v2.0/lumberjack.go)。这项兼容的是文件命名格式，不能单凭该格式证明文件已停用；占用检查和管理员确认仍然必要。

## 界面语言与翻译维护

登录页与控制台右上角的语言菜单支持 `zh-CN`、`en`、`ja`、`ko`、`zh-TW`。首次优先匹配浏览器语言；不支持的语言回退简体中文。手动选择保存为浏览器的 `nodesweep.language`，不向节点传播，也不保存登录令牌。禁用本地存储时仍能切换，但刷新后按浏览器语言重新选择。

翻译表位于 `web/src/i18n/messages.ts`，每条文案对应英、日、韩、繁体四列，简体中文使用原始键。参数通过 `{name}` 等占位符插入，并由 Vue 按普通文本渲染。增加或修改文案后运行：

```bash
cd web
npm test
npm run format:check
npm run build
```

CI 会检查五种语言的条目、占位符和语言回退。浏览器回归脚本使用 API 夹具验证界面流程，不操作实际日志目录；开发环境安装 Playwright 及 Chromium 后可运行 `npm run test:browser`，也可使用 `PLAYWRIGHT_MODULE` / `CHROMIUM_EXECUTABLE_PATH` 指定现有安装。脚本检查五种语言的登录、扫描、预览确认、导出、节点接入、偏好保存和 360/390px 布局。

用户保存的节点、规则、方案名称以及路径、导出 JSON 不自动翻译；环境识别的预设说明和常见 API 错误在展示时翻译。未知系统错误、任务详情 JSON 保留原文，便于定位问题。

## 安装、升级与回滚（Alpha.2）

已解压发布包时，在包目录运行：

```bash
sudo bash install.sh --start
```

首次安装会生成 `/etc/nodesweep/config.json` 并监听本机 9780。Agent 可预先放入自己的配置；已有配置和 systemd 单元不会被覆盖。管理端和 Agent 请同步升级。

也可使用本地 `install.sh` 下载指定的发布版本。脚本仅访问本仓库的 HTTPS 发布地址，自动识别 amd64/arm64，下载后检查 SHA256SUMS：

```bash
sudo bash install.sh --download v0.1.0-alpha.2 --start
# 测试最新代码时才选 nightly：
sudo bash install.sh --download nightly --start
```

升级会停止已运行的服务，备份配置及标准 `/var/lib/nodesweep/data`，原子替换二进制并重启。短暂启动检查失败会恢复旧二进制并尝试重启旧服务。自定义 `data` 路径需要自行备份。快照位于 `/var/lib/nodesweep/backups`，确认升级成功后可手动删除不需要的旧快照。

```bash
sudo bash install.sh --rollback
```

回滚只切换到上一个二进制，保留当前配置、数据库和任务；不会自动用旧快照覆盖新数据。若未来版本发生不兼容的配置或数据库迁移，需要按该版本说明恢复备份。`--root` 仅供隔离安装测试使用，不用于生产部署。

## 规则试运行与清理报告

清理方案中的“规则试运行”只检查规则，不创建可执行预览，不删除文件。面板展示已访问条目的判定数量，以及最多 40 个示例和命中的模式。排除目录不会展开，因此统计不代表目录内全部文件。

正式执行仍须重新预览并确认。清理报告展示已删除、安全跳过、操作失败的文件、计划总量、分配空间和开始/结束时间；失败或中断任务的部分结果可在任务记录查看。分配空间并非磁盘可用空间净增长，例如其他进程同时写入或文件系统共享数据块时会有差异。

## 磁盘与离线告警

告警默认关闭。在“磁盘告警”页面开启，可设置磁盘/inode 使用阈值、离线等待和重复通知间隔。管理端每五秒检测一次，持久化状态以避免重启后重复首报，恢复时也记录事件。最多保留最近 100 条。

没有 Webhook 时仅记录到面板。需要通知时，在管理端的权限 0600 配置文件加入：

```json
"webhookURL": "https://your-public-receiver.example/hooks/your-secret"
```

重新启动管理端后生效。Webhook 为通用 `POST application/json`：包含 `node`、`name`、`path`、`kind`（disk/inode/offline）、`percent`、`resolved` 和 `at`。默认 event 格式需由接收端处理；webhookFormat 可选择 slack 或 discord 平台格式，详见 [通知适配](notifications.md)。地址只保存在管理端文件，不返回浏览器。仅支持公开 HTTPS 目标，拒绝私网、保留地址和重定向，并在连接时校验解析后的 IP，避免 DNS 重绑定。网络错误只记录通知失败，不展示含密钥的 URL。

通知包含节点名称与路径，请使用你信任的接收端。不内置 SMTP 或自动升级；邮件可经可信 HTTPS Relay 转发。自动清理计划另需管理员核对并启用，默认关闭。

## Agent 配置向导与本地检查

“添加节点”可以设置管理端 HTTPS 地址、逐行填写清理及扫描白名单、自定义面板安装目录。识别面板不会自动扩展清理权限；请只允许日志目录，不要把整个面板安装目录当作清理范围。宝塔的 panelRoots 填写实际 panel 目录（如 `/www/server/panel`），1Panel 安装位置沿用静态 1pctl 配置识别。自定义目录需在实际 VPS 上确认存在。

```bash
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json -check
```

命令读取受保护配置并输出不含凭证和管理端 URL 的 JSON。检查目录是否能在不跟随符号链接的情况下打开、可见进程描述符是否可读取；任一检查失败返回非零退出码。不启动服务、不连接管理端、不创建数据库、不修改文件；不能与 `-init` 或 `-version` 同时使用。该检查不验证网络连通性、TLS、数据库、Webhook 或清理目录的写入权限，启动和正式预览仍需验证这些条件。

进程检查只覆盖当前可见命名空间，不能证明能看到宿主机所有进程；容器内 Agent 仍不适合清理宿主机日志。SELinux/hidepid、宝塔/1Panel 自定义目录和 arm64 实机验证仍是待办。

## 扫描进度、取消与资源预算

先升级 Hub，再升级 Agent。新 Agent 的每次上报会携带扫描控制能力；旧 Agent 保持原有扫描/清理功能，但不提供取消。管理端令牌可以通过 `POST /api/tasks/{id}/cancel` 取消等待或运行中的扫描；不支持中止清理、预览或其他任务。单机取消在本地生效，远程取消经 Agent 下一次轮询传达，正常情况下约 5 秒；断网、请求超时或阻塞文件系统 I/O 可能延迟。任务完成与取消竞争时，已经完成的结果可能仍显示成功。

进度是已访问和已纳入树图的条目数、已累计的分配空间，不是已知总量的百分比。取消扫描保留部分结果；未达到完整扫描时不应将其当作完整磁盘统计。离开网页只停止浏览器等待，不取消远程任务，真正取消需使用取消按钮。

可在实际执行扫描的节点配置文件加入：

```json
"scanBudget": {
  "entries": 100000,
  "seconds": 60,
  "depth": 64,
  "treeBytes": 8388608,
  "pauseMillis": 5
}
```

省略或零值使用默认预算。条目最多 100000、深度最多 64、时间最多 80 秒、保守树图预算最多 16 MiB，每 128 项暂停 1–100 毫秒（默认 5）。目录按 256 项读取。达到限制显示截断原因；8 MiB 是保守 JSON 转义估算，实际可保留条目数可能低于 100000。预算只在 Agent 本地配置，Hub 任务不能扩大预算。

成功、失败、中断扫描合计只保留最近 10 份完整载荷，过期树图会清除，任务元数据、进度和错误保留原有 30 天策略。回滚到不认识 `scanBudget` 的旧版前，应先备份并移除该新增配置字段。

### Metric history

The overview includes authenticated per-node CPU, memory, disk and inode utilization history. Select a node (the overview group filter also limits the selector), a mount and the last 24 hours or seven days. The data table is available for keyboard and screen-reader access. Refresh explicitly to obtain the latest history.

The hub stores the first accepted heartbeat per node per receipt minute, independently of the Agent timestamp. SQLite keeps up to seven days and 20,000 samples globally, so many nodes may have a shorter history. Each sample retains numeric utilization and up to 16 mount paths; path length and a conservative 4 KiB payload budget bound storage. Missing and invalid measurements remain unavailable, and offline periods are not filled with zeros. History uses 15-minute means for 24 hours and hourly means for seven days; a partly observed bucket averages its available samples, with the count shown in the table. Long gaps break chart lines. Revoking a node removes its metric history.

`GET /api/metrics/{node}?period=24h` (or `7d`) uses a dashboard access credential (viewer, operator or administrator); Agent credentials cannot read another node's history. History contains mount paths but no hostnames, credentials, process details or cleanup authorization. It survives hub restarts. Retention deletes rows for reuse; SQLite may keep the allocated database file size.

### Cleanup failure notifications

Enable alerts and the separate **Notify cleanup failures** option to record failed or interrupted `execute` tasks created within the last 24 hours. Both controls default to off. Enabling the option can report recent failures; scans and previews are excluded. Records contain node name/ID, task ID, event kind and time. Raw task errors, rule/log paths, task results and credentials are omitted; inspect the authenticated task page for details. A failed execution may have deleted some files before failing. The notification never retries cleanup or authorizes another deletion.

The existing `webhookURL` sends the same JSON event to a public HTTPS receiver, with `kind: "cleanup_failure"` and `task` holding the task ID. Private addresses, DNS rebinding and redirects remain blocked. No URL is exposed through the browser API. A custom relay can transform this JSON into an email or platform message; native SMTP is not included; Slack and Discord adapters are available (see [notifications.md](notifications.md)). Configure relay credentials outside NodeSweep, and accept only the event fields your relay needs.

Each pass processes at most ten unreported failures, within a 20-second processing context. The task stores a deduplication marker atomically with the event before sending. Restarting or receiving duplicate Agent results cannot replay the notification. Delivery failures are recorded without raw errors and are not retried automatically; a crash after recording may leave delivery incomplete. Events share the existing latest-100 retention, and the deduplication marker expires with the task's normal retention. Cleanup failures have no synthetic recovery event.

### Independent credentials and roles

`adminToken` remains the primary administrator credential. On the hub or standalone instance, optionally configure `accessTokens` as a list of `{ "name": "observer", "role": "viewer", "token": "<a-new-random-token-of-at-least-32-characters>" }` objects. Supported roles are `viewer`, `operator`, and `admin`; at most 32 additional identities are accepted. Names and tokens must be unique, and `admin` is reserved for the primary credential's name. Generate a separate strong random token for each identity; protect the configuration with mode `0600`, restart after edits, and remove or rotate a credential to revoke it. Agent configurations cannot contain dashboard access tokens. No credential is returned by the session, node or audit APIs.

| Role | Allowed operations |
| --- | --- |
| Viewer | Read nodes, metric history, rules/export, task details and alerts |
| Operator | Viewer access plus creation of scan, check, detection, preview and execution tasks; cancel scans |
| Administrator | Operator access plus node/rule/alert settings and the audit trail |

Roles apply to the entire hub, including all nodes and stored paths. This is not per-node or tenant isolation. Operators can permanently delete eligible archives through a fresh preview, so grant the role deliberately. The browser identifies the current role and disables unavailable actions; server authorization remains authoritative even when requests are made outside the UI. Tokens stay in browser memory and are not saved locally. All externally accessible sessions require HTTPS.

`GET /api/session` identifies the authenticated role and configured identity name. The additional credentials are read from local protected configuration; neither agents nor the Web UI can expand the credential list. An older binary rejects `accessTokens`, so remove that field before rolling back.

### Audit semantics

The administrator-only audit page and `GET /api/audit` report authenticated mutation requests and role denials. The hub records the configured actor name, role, route category, a validated target ID when present, receipt time and HTTP outcome. It excludes credentials, request bodies, log contents, raw paths, URLs and query strings. Invalid credentials do not allocate audit entries. Node/rule/task actions are recorded as request categories such as `nodes.post`, `rules.delete` or `tasks.cancel`.

A durable accepted entry is written before processing a mutation. If this write fails, the mutation is refused. The HTTP result is added afterwards; status `0` explicitly means the outcome was not recorded (for example, a crash or a failed final write). HTTP `200` for a task means the task was accepted, not that deletion succeeded: inspect the separate task record for its eventual execution result. Audit entries are bounded to the latest 1,000 and at most 30 days when new entries arrive. They survive hub restarts and cannot be deleted through the Web API. Retention reuses SQLite storage; this is a protected local operational log, not cryptographic tamper evidence against the host administrator.

### Batch cleanup

Operators and administrators can preview one selected rule on up to 20 visible online nodes, then review and confirm each node separately. Every node returns its own plan: plans from another node cannot authorize deletion. Expand the candidate list (first 100 displayed) or export the full list for local review. Exports contain node identity, rule, time and candidates, but no executable plan ID or credentials; they may contain private paths and should be handled accordingly.

The batch workspace imposes a five-minute receipt-age limit, stricter than the engine's independent ten-minute plan expiry. It rechecks the online selection and age before each submission; the node still enforces its own plan expiry, allowlist and file identity. Changing the selection, group or rule invalidates prior confirmations. Nodes without successful eligible previews cannot be confirmed.

Only confirmed nodes are dispatched, with at most two workers. All confirmations are consumed before dispatch, including stopped or unsubmitted rows: any follow-up requires new previews. **Stop further submissions** prevents subsequent nodes from being admitted; it cannot cancel cleanup already accepted by the hub. Leaving the page or logging out stops browser submission/polling, while accepted tasks continue and can be inspected in task history. Errors and timeouts never automatically retry deletion. A failed task may have partially deleted its candidates; inspect its record before creating a new preview. Hub or Agent restarts cannot replay destructive work.

### Keyboard interaction

The console provides a skip-to-main link, current-page navigation state, focus outlines and full path/size labels for treemap buttons. Dialogs contain Tab/Shift+Tab focus, accept Escape to close, make background controls inert and restore focus to the opener when it still exists. A node metadata save keeps its dialog open until the save finishes. Native buttons, selects, checkboxes and detail summaries support keyboard operation; metric history also has a readable data table. Five-language browser regressions cover dialog focus, restoration, background state, roles and mobile widths. These checks are functional coverage, not a claim of a full accessibility certification.

## Metric sampling limits

Each local or Agent sample uses a cooperative one-second deadline, bounded `/proc` reads, at most 4,096 mount lines and 128 supported-filesystem probe attempts, including duplicates or failures. A conservative 32 KiB metric payload budget and a separate 32 KiB configured root-metadata limit bound reporting. Sampling reports partial data when these limits or read errors occur. CPU needs two valid counter readings; initial or regressed counters are unavailable rather than a measured zero, and new unavailable readings are excluded from history. Old Agents without this flag retain their previous semantics.

These limits are checked between filesystem calls. Kernel-blocked reads or `statfs` cannot be forcibly interrupted; the sampler does not spawn abandoned timeout goroutines. Upgrade the hub before Agents.

## Scheduled cleanup

Only administrators can create, enable, pause or delete schedules. Creation selects one registered node and one existing rule, saves an immutable rule snapshot and starts paused. Changing or deleting the original rule does not change the schedule. To enable, obtain a new matching preview on that same node, review its rule and candidate list, and explicitly authorize future permanent archive deletion. The server checks ownership, rule equality, success and a five-minute preview age; it also checks that the node is online and idle.

Intervals are 1–720 hours, with the first run one interval after enabling. Up to 32 schedules are saved and two preview/execution pipelines run concurrently. Each due run consumes its interval before submitting a fresh node-owned preview. Execution uses only that preview's one-use plan. Node allowlists, archive age, exclusions, managed-log protection, open-file and identity checks still apply. Audit is persisted before dispatch; failure to record it prevents dispatch. Tasks and cleanup-failure alerts use the existing protocol.

Offline/busy runs are skipped without catch-up or retries. Failed, interrupted, expired or mismatched previews/execution pause the schedule. Every hub restart pauses all schedules, including ones between preview and execution; inspect any uncertain submitted execution and review again before enabling. Pausing, deleting a schedule or revoking a node cannot undo an already submitted cleanup. A schedule's stored task ID points to its execution evidence.

Application cleanup schedules are independent of releases. Release/nightly workflows have no daily cron and run only for new checked commits. Native journald/Docker retention remains an operator action described in [the retention guide](log-retention.md).

## Retained scan reuse

Disk analysis can explicitly load the latest retained terminal scan for the same node and requested directory, including partial failed scans. It labels the result as historical and shows the scan timestamp. Loading it creates no task and consumes no cleanup plan. The existing newest-ten-payload/30-day metadata policy bounds it; a pruned result is unavailable. A historical tree never authorizes deletion: create a new preview for cleanup.

## Host acceptance

See [acceptance.md](acceptance.md) for isolated scan stress/soak commands and the pending real-host evidence matrix. Release packages include the tool; it never connects to your installed Hub or authorizes cleanup.
