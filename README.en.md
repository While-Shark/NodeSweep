# NodeSweep

**See your server storage. Clean with confidence.**

A lightweight Linux server dashboard with multi-node monitoring, drill-down disk treemaps, and preview-first archived-log cleanup.

[简体中文](README.md)

## v0.1 Alpha

- One Go binary: standalone, hub-only, or outbound agent mode.
- CPU, memory, load, disk capacity and inode metrics.
- Allocated-space treemap with directory drill-down and file table.
- Named cleanup schemes containing multiple reusable rules.
- JSON scheme import/export with transactional validation and duplicate skipping.
- Common Linux, Nginx, BaoTa and 1Panel directory presets.
- Agent-local allowlists, short-lived previews, open-file checks and identity verification.
- SQLite-backed task history; no external database or message broker.

This release only removes eligible expired archives. Scheduled cleanup, metrics history, journal/Docker-native rotation, and automatic custom-panel path discovery are planned. The current UI is Chinese; English UI is planned.

## Run

Download a matching Linux amd64/arm64 archive from [Releases](https://github.com/While-Shark/NodeSweep/releases), verify its checksum, then:

```bash
./nodesweep -init
./nodesweep -config config.json
```

The default bind is `127.0.0.1:9780`. Sign in using `adminToken` from the generated configuration. Use an SSH tunnel locally or an HTTPS reverse proxy for remote access. To add a server, create a node in the dashboard and run the same binary with its generated agent configuration. Agents require HTTPS except on loopback.

## Build

Requires Go 1.25+ and Node.js 22.18+ / 24+.

```bash
cd web && npm ci && npm run build && cd ..
go test -race ./...
go vet ./...
go build -trimpath -ldflags='-s -w' -o nodesweep ./cmd/nodesweep
```

Runtime requires only the binary and its local configuration/database. Host-level cleanup requires sufficient `/proc` visibility; the agent refuses cleanup if it cannot establish open-file safety. There is no bypass switch. Read [security boundaries](docs/architecture.md) before deploying.

Inspired by Beszel's lightweight monitoring model and gdu's disk exploration UX; independently implemented. MIT license.
