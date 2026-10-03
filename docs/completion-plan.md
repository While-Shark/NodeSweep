# NodeSweep completion goals

The goal is a lightweight, usable release with explicit safety boundaries. A feature is complete only when its implementation, failure cases, five-language UI where applicable, documentation and release checks pass. Real VPS evidence is tracked separately.

| Work | Acceptance | State |
| --- | --- | --- |
| Scan budgets | Chunked directory reads; entry, depth, output-size and time limits; cancellable pacing; partial results identified | Complete |
| Scan progress and cancellation | Standalone and outbound agents; ownership checks; cancel scans only; retain partial results; refuse unsupported agents | Complete |
| Large-directory checks | Repeatable fixture coverage for wide/deep trees, limits, cancellation, symlinks and bounded result payloads | Complete |
| History charts | Bounded SQLite retention, aggregation, node filtering, disk/inode/CPU/memory charts | Complete |
| Cleanup failure notifications | Opt-in terminal execution alerts, persistent deduplication, bounded processing and redacted payloads | Complete |
| Notification adapters | Optional email/platform forwarding without exposing secrets | Pending |
| Access and audit | Read-only/operator/admin boundaries, independent credentials, protected bounded audit trail | Complete |
| Cleanup orchestration | Node-specific fresh previews, explicit confirmation and bounded execution; never replay destructive work after restart | Pending |
| Scheduled cleanup | Disabled by default, explicit node/rule assignment, overlap prevention and audit; independent of release scheduling | Pending |
| Log integration | Inspect rotation configuration; journal native retention and Docker rotation guidance; no arbitrary shell commands | Pending |
| Agent onboarding | HTTPS and directory wizard, protected downloaded config and local read-only preflight | Complete |
| Keyboard/accessibility | Keyboard navigation, dialogs, focus restoration, mobile layouts, five languages | Pending |
| VPS acceptance | BaoTa/1Panel defaults and custom paths, arm64, SELinux/hidepid, low disk and extended uptime | Requires real hosts |

Scan caching will follow measured scan behavior: any cached tree must show its age and cannot authorize cleanup. Panel-version and additional rotation-pattern claims require fixtures or real-host evidence. Release workflows run for new checked commits, with no daily schedule.

VPS acceptance requires an actual deployment. Cross-compilation, browser fixtures and short smoke tests do not establish real-host or long-running stability.
