# NodeSweep completion goals

The goal is a lightweight, usable release with explicit safety boundaries. A feature is complete only when its implementation, failure cases, five-language UI where applicable, documentation and release checks pass. Real VPS evidence is tracked separately.

| Work | Acceptance | State |
| --- | --- | --- |
| Scan budgets | Chunked directory reads; entry, depth, output-size and time limits; cancellable pacing; partial results identified | Complete |
| Scan progress and cancellation | Standalone and outbound agents; ownership checks; cancel scans only; retain partial results; refuse unsupported agents | Complete |
| Large-directory checks | Repeatable fixture coverage for wide/deep trees, limits, cancellation, symlinks and bounded result payloads | Complete |
| Metric sampling | Bounded proc reads, mount attempts and payloads; cooperative deadline and unavailable CPU indication | Complete |
| History charts | Bounded SQLite retention, aggregation, node filtering, disk/inode/CPU/memory charts | Complete |
| Cleanup failure notifications | Opt-in terminal execution alerts, persistent deduplication, bounded processing and redacted payloads | Complete |
| Notification adapters | Slack/Discord formats; optional email via trusted HTTPS relay, no embedded SMTP; protected destination | Complete (payload and transport tests; live destination needs configuration) |
| Access and audit | Read-only/operator/admin boundaries, independent credentials, protected bounded audit trail | Complete |
| Cleanup orchestration | Node-specific fresh previews, explicit confirmation and bounded execution; never replay destructive work after restart | Complete |
| Scheduled cleanup | Disabled by default, immutable node/rule snapshot, fresh consent, overlap limits and pre-dispatch audit; pause after restart | Complete |
| Log integration | Bounded read-only configuration checks; native journal/Docker operator guide; managed-log exclusions, no remote commands | Complete (native maintenance remains a host operator action) |
| Agent onboarding | HTTPS and directory wizard, protected downloaded config and local read-only preflight | Complete |
| Keyboard/accessibility | Keyboard navigation, dialogs, focus restoration, mobile layouts, five languages | Complete |
| Retained scan reuse | Same-node/path terminal trees, visible timestamp, no new task or cleanup authorization | Complete |
| VPS acceptance | BaoTa/1Panel defaults and custom paths, arm64, SELinux/hidepid, low disk and extended uptime | Requires real hosts |

Explicit reuse of retained scans shows the scan timestamp and historical status, remains bounded by existing retention, and cannot authorize cleanup. Panel-version and additional rotation-pattern claims require fixtures or real-host evidence. Release workflows run for new checked commits, with no daily schedule.

VPS acceptance requires an actual deployment. Cross-compilation, browser fixtures and short smoke tests do not establish real-host or long-running stability.

Repeatable isolated scan/soak tooling and the host evidence matrix are in [acceptance.md](acceptance.md). The short test runs in CI and against the packaged amd64 binary; host matrix items remain pending until actual results are recorded.
