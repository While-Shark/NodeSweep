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
- 只有 `.gz` / `.xz` / `.bz2` 或 `.log.` 后接数字/连字符的归档才可能匹配；普通 `.log` 不处理。
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
