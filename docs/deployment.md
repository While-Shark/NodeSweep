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
