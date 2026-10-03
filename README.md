# NodeSweep

**See where server space goes. Clean archived logs with confidence.**

A lightweight, self-hosted Linux VPS dashboard: one binary for standalone, central hub and outbound agents. Monitor multiple servers, drill into a rectangular disk treemap, and preview cleanup plans before permanently deleting old log archives.

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [繁體中文](README.zh-TW.md)

> **Alpha:** validate on a test VPS first. Active logs, Docker logs, databases and backups are not cleanup targets. Real VPS deployment validation remains necessary.

![Disk analysis dashboard](docs/images/disk-analysis.png)

The screenshot shows a local test hub, two agents and test log directories. The UI supports all five languages above.

History charts cover CPU, memory, disks and inodes with bounded seven-day retention. Opt-in cleanup failure notifications reuse the safe HTTPS webhook and omit raw errors and credentials.

## Features

Scans show aggregate progress and support cancellation on standalone and upgraded Agents. Local scan budgets cap entries, depth, time and output size. Incomplete results are labeled; cancelled scans retain partial trees. Upgrade the hub before Agents. See [completion goals](docs/completion-plan.md) for remaining work.

Switching groups clears the selection. Batch actions target visible online nodes, with up to 20 selected. Use **Stop further submissions** to leave the remaining nodes unsubmitted; already submitted tasks finish normally and remain in task history.

Persistent node groups and renaming, with group filters in the overview and batch workspace. Metric reports preserve administrator edits. Non-destructive batch scans and rule previews for up to 20 online nodes, processing two at a time. Each node reports its own result or error; offline nodes are excluded and one failure does not block the others. Each node enforces its local directory allowlist, and cleanup requires individual confirmation. Leaving the batch page stops further submissions and browser polling; already submitted tasks remain available in task history.

- CPU, memory, load, disk capacity and inode usage across multiple VPS nodes.
- Directory drilldown with a rectangular treemap and file/directory lists.
- Cleanup plans with multiple rules: paths, patterns, exclusions and retention days.
- Linux/Nginx, BaoTa and 1Panel presets; static installation/log path discovery, including configurable `panelRoots`.
- JSON plan import/export with validation, duplicate handling and transactional import.
- Independent revocable node credentials, hashed storage and persistent task history.
- Local cleanup allowlists, expiring previews, file identity checks, symlink/hardlink rejection and open-file inspection.

Presets and discovery never expand an agent's local cleanup allowlist. Scheduled cleanup, bulk execution, historical metric charts and native journal/Docker cleanup are future work.

## Quick start

Download the Linux amd64 or arm64 archive from [Releases](https://github.com/While-Shark/NodeSweep/releases), verify `SHA256SUMS`, extract it, then run:

```bash
./nodesweep -version
./nodesweep -init
./nodesweep -config config.json
```

The default listener is `127.0.0.1:9780`. Read `adminToken` from `config.json` to sign in; it is not printed in startup logs or stored in browser local storage. Keep secret configuration files at mode `0600`.

For a remote VPS, open an SSH tunnel from your computer:

```bash
ssh -L 9780:127.0.0.1:9780 root@your-server
```

Open `http://127.0.0.1:9780`. For public/multi-node access, use an existing HTTPS reverse proxy; NodeSweep does not need port 443 itself. See [deployment](docs/deployment.md) (Chinese).

## Multiple nodes

Add an Agent configuration wizard for hub HTTPS address, cleanup/scan allowlists and optional custom panel directories. Panel discovery never expands cleanup permissions.

```bash
./nodesweep -config nodesweep-agent.json -check
```

Add the read-only -check command for local configuration, directory access without symlinks and visible process descriptor access. It does not start services, connect to the hub or create the database, and reports no credentials or hub URLs.

1. Add a node in the dashboard and download its independent agent configuration.
2. Set `hub` to the central dashboard's HTTPS URL.
3. Upload the architecture-appropriate binary and configuration to the VPS.
4. Review local `cleanupRoots`, then start the agent:

```bash
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json
```

Agents poll outbound every five seconds and keep reporting metrics during tasks. They need no inbound listening port. Hub URLs cannot contain credentials, query strings or fragments; HTTP is permitted only on loopback for development. Redirects are rejected.

## Cleanup safety

Signing out clears the browser token and view state and cancels outstanding browser requests. Already submitted server tasks continue; check task history after signing in again.

The default cleanup allowlist is `/var/log`. Add only verified log directories, such as BaoTa's `/www/wwwlogs` or `/www/server/panel/logs`, and restart the agent. Never allow a website root, database directory or all of `/opt`.

Only old regular archives with one hard link and no detected open descriptors are candidates: compressed archives, numbered log rotations or validated Lumberjack timestamp archives. Active `.log` files, symlinks and cross-mount entries are skipped. Preview at most 5,000 candidates per rule; previews expire after ten minutes and cannot be replayed. Execution checks file identity again and uses a temporary quarantine step; quarantine is not a backup.

Linux `/proc/self/fd` and visibility into other processes' descriptors are required. If open-file inspection is unavailable, cleanup fails closed. Another application can still open a file after inspection; coordinate retention with its logging policy. See [security audit and boundaries](docs/security.md) and [architecture](docs/architecture.md).

## Languages

English, Japanese, Korean, Simplified Chinese and Traditional Chinese can be switched without refreshing. Initial selection follows browser language; only the chosen locale is persisted. User-defined names, paths and configuration keys remain unchanged. Known operation errors are translated; raw task JSON and unknown system diagnostics retain their original text.

## Nightly and version releases

- **Nightly:** rolling prerelease, updated after a new commit to `master` passes CI. Use for testing.
- **Version Release:** automatically created from `VERSION` after checks pass. Published versions are never overwritten; bump `VERSION` for a new release. A matching `v*` tag can also trigger publishing. Prerelease versions remain marked as prereleases.
- Both channels provide Linux amd64/arm64 archives, `SHA256SUMS`, multilingual READMEs and build metadata. `./nodesweep -version` prints the version, commit and build time.

CI checks frontend formatting/tests, Go race tests/vet, standalone/multi-node smoke tests, dependency vulnerabilities and both architectures. Fork pull requests cannot start the privileged publisher. Release builds use read-only repository permissions; only the final publishing job has write access.

Release descriptions include reviewed change summaries in all five languages. Update `docs/release-notes.json` alongside changes; its version must match `VERSION`. The publisher renders these notes for release pages and includes them in newly built archives.

## Installation, reports and alerts

Run `sudo bash install.sh --start` from an extracted bundle. To download a checked build, use `sudo bash install.sh --download v0.1.0-alpha.7 --start` (or `nightly` for testing). Upgrades preserve configuration, snapshot standard stopped state and restore the old binary if activation fails. `sudo bash install.sh --rollback` swaps binaries without restoring old database/configuration snapshots. Custom data paths require an operator backup; clean up obsolete snapshots after verification.

Rule dry runs explain matching/exclusion decisions without creating an executable preview. Cleanup reports show file outcomes, allocated space and execution times, including partial failed tasks in task history. Allocated space is not net free-space growth.

Optional alerts cover disk/inode thresholds and offline nodes, with cooldowns, recovery events and the latest 100 records. Enable them in the dashboard. For generic JSON notifications, add `webhookURL` to the hub's protected config and restart it; only public HTTPS destinations are supported. The URL is never returned to the browser. Upgrade hub and agents together. See [deployment details](docs/deployment.md).


## Build and verify

Use a supported, patched Go release (CI uses Go 1.27.x), Node.js 22.18+ or 24+, and Python 3 for smoke/publishing checks. Runtime requires only the binary; no Node.js, external database, Redis or queue.

```bash
git clone https://github.com/While-Shark/NodeSweep.git
cd NodeSweep
cd web && npm ci && npm run format:check && npm test && npm run build && cd ..
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go build -o nodesweep ./cmd/nodesweep
python3 scripts/smoke.py ./nodesweep
bash scripts/package.sh
```

## Credits and license

Inspired by [Beszel](https://github.com/henrygd/beszel)'s lightweight monitoring and [gdu](https://github.com/dundee/gdu)'s disk exploration. Independently implemented; no source from either project is included. Treemap layout uses [d3-hierarchy](https://github.com/d3/d3-hierarchy). [MIT license](LICENSE).
