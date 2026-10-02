# Security audit and operational boundaries

This source review covers node authentication, task ownership, agent transport, file cleanup, secret/state storage and CI publishing. Regression tests accompany the fixes. It is not an independent penetration test or a claim of complete vulnerability elimination.

## Findings and fixes

| Area | Risk | Implemented control |
|---|---|---|
| Directory opening | A component replaced after a symlink check could redirect a root outside its intended path | Open every component relative to a pinned parent descriptor with `O_NOFOLLOW`; keep the root descriptor alive; read metadata through the pinned root |
| Agent protocol | A forged hub response could assign another node's task or unrecognized operation | Strict response schema, one JSON object, 1 MiB response cap, node/task identity and operation validation |
| Token transport | Redirects or credentials embedded in hub URLs could expose credentials | HTTPS except loopback; reject redirects, URL credentials, query strings, fragments and encoded paths |
| Node reporting | Oversized metadata and parallel polling could exhaust resources | Authenticate before reading; one in-flight poll per node, burst/rate budgets, bounded paths/metrics/errors and request bodies |
| Node isolation | One valid node could try to claim or complete another node's task | Header identity, payload identity and persisted task ownership checks; node credentials stored only as hashes |
| Secret configuration | Readable-by-others files, symlinks and hardlinks could expose or substitute secrets | Reject nonregular/link files, group/other permissions, oversized or unknown JSON fields |
| SQLite state | Loose permissions or special file names could expose data or alter connection options | Owned non-group/world-writable state directory; regular single-link owned database/sidecars at `0600`; URL-encoded database path |
| Error responses | Database failures could reveal internal details | Generic HTTP 500 response; detailed diagnostics only in server logs |
| Publishing | Untrusted PR artifacts or build scripts could access a write token | Only same-repository successful master push CI triggers publishing; fresh verification; read-only build job and separate write-enabled publishing job |

## Remaining boundaries

- One trusted administrator, not a tenant-isolated hosting service. Administrator tokens grant control over registered nodes.
- A compromised hub can request supported scans and archive deletion within agent-local allowlists. Keep those lists narrow. No allowlist expansion is accepted from the hub.
- A compromised agent can fabricate metrics or scan metadata; such data is not an attestation of server integrity.
- No application terminal, command runner or reverse-shell endpoint was found in this review. Restricting operation types does not protect a server whose root account or executable has already been compromised.
- Paths, hostnames and scan metadata are sensitive. The dashboard exposes them to its authenticated administrator. Protect reverse-proxy logs, backups and database files; never put tokens in URLs.
- Linux `/proc/self/fd` must exist. Open-file inspection needs a compatible PID namespace and descriptor visibility. It fails closed when inspection cannot complete. A file may be opened by another process after inspection; configure retention with its application.
- File checks do not isolate privileged bind-mount changes or hostile root users. Same-user/root filesystem replacement is outside the state-storage trust boundary.
- Poll budgets mitigate one node's flooding; they are not a global DDoS defense. Use reverse-proxy limits and infrastructure controls for public endpoints.
- Deletion is permanent. Quarantine prevents some file-replacement races; it is not a recoverable trash bin or backup.
- Nightly is mutable and intended for testing. Version releases are immutable in the publisher. SHA256SUMS detects accidental transfer corruption; it does not replace trust in the repository and its Actions credentials.

## Verification

Go race tests include root replacement, traversal, file replacement, hardlinks/open files, expired/replayed previews, strict configuration permissions, state file permissions, redirect rejection, cross-node task isolation, payload limits and poll budgets. Smoke tests exercise standalone and hub/agent flows. Frontend checks cover locale keys, formatting and build. CI also runs npm audit and govulncheck; new advisories can block future builds. This audit upgraded golang.org/x/sys to v0.44.0, addressing GO-2026-5024 even though its Windows-only package was not used by this Linux application. Scans with Go 1.27.1 found no remaining known vulnerabilities.

## Alpha.2 additions

Rule dry runs use the same allowlists and file checks but do not store an executable preview. Decision examples are bounded to 40 entries. Cleanup reports preserve file metadata, not file contents.

Webhook URLs are loaded only from the protected hub config. The admin API returns only a configured flag. HTTPS notifications resolve and validate public IP addresses during connection, connect to that pinned address, reject redirects and avoid logging URL-bearing transport errors. Alerts are disabled by default; event history is capped at 100.

Installer downloads trust this repository and verify SHA256SUMS. It snapshots the standard data path only after stopping the service, preserves configuration and custom units, and supports binary-only rollback. A short systemd startup check is not a guarantee of long-term health. Custom data paths and future incompatible migrations require operator-managed backups.
