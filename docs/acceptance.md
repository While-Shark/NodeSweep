# Host acceptance and stability evidence

Alpha.10 passed [CI](https://github.com/While-Shark/NodeSweep/actions/runs/37099725412) and [publishing](https://github.com/While-Shark/NodeSweep/actions/runs/37099818648) successfully. Software feature coverage is recorded in
[completion-plan.md](completion-plan.md); real host acceptance remains open. A passing
short automated test is not evidence of long uptime, all panel versions or arm64 execution.

## Isolated scan verification

Release packages include `scripts/stability.py` (Python 3 standard library only).
Run as an ordinary user on Linux, from an extracted release directory:

```bash
python3 scripts/stability.py ./nodesweep
```

The tool starts a separate loopback-only standalone process with a random credential,
temporary database, empty cleanup allowlist and synthetic scan directory. It does not
read your installed configuration, contact your Hub, edit systemd, send notifications,
preview or execute cleanup. It creates and later removes its own temporary fixtures.
All requests bypass environment HTTP proxies. Output contains aggregate counts and
timings, without credentials, fixture paths, raw errors or directory trees.

The default creates 2,000 empty files and checks three scans stop at the configured
1,000-entry budget, preserve a partial result, obey the payload limit, cancel a running
scan and admit another scan afterwards. CI and package verification run this short test.
RSS and file descriptor peaks are sampled during polling; unavailable measurements
remain `null`. A pass does not assert a memory ceiling or absence of slow leaks.

Optional large-directory and extended runs:

```bash
python3 scripts/stability.py ./nodesweep --files 1000000 --rounds 10
python3 scripts/stability.py ./nodesweep --files 20000 --seconds 86400
```

Million-file creation consumes inodes, directory metadata and temporary space even
with empty files. Choose a test host with adequate capacity; stop with Ctrl+C, which
terminates the isolated child and removes the fixture. SIGKILL/power loss cannot perform
cleanup. The million-file run tests bounded enumeration of a wide directory, not complete
enumeration of a million files. The duration starts after fixture creation; shutdown and
the final cancellation/recovery checks add time. This is repeated local scan evidence,
not a simulation of every production workload or real Agent/panel acceptance.

## Real host matrix

Record release version/commit, OS/kernel, architecture, panel version, execution user,
filesystem and relevant isolation settings. Keep raw private paths and diagnostics local.
Use the installed `-check` command from [deployment.md](deployment.md) before cleanup.
Do not disable SELinux/hidepid to turn a refusal into a pass.

| Environment | Evidence required | Current status |
| --- | --- | --- |
| BaoTa default/custom paths | Detection and rule preview agree with manually reviewed archives | Pending real host |
| 1Panel default/custom paths | Detection and rule preview agree with manually reviewed archives | Pending real host |
| Linux arm64 | Binary startup, preflight, scan and Hub–Agent round trip on hardware | Pending real host |
| SELinux / hidepid / container PID isolation | Restricted visibility refuses cleanup; monitoring remains usable | Pending real host |
| Large directory / long uptime | Isolated JSON report, elapsed duration and observed resource trend | Tool ready; host evidence pending |
| Low disk / inode exhaustion | Disposable VM or isolated quota filesystem; durable failures reject dispatch, service recovery inspected | Pending isolated host |
| Live notifications | Opt-in test receiver confirms payload format and destination restrictions | Pending configured receiver |

Do not fill a production filesystem to test exhaustion. Successful CI builds and unit
tests of storage errors do not establish behavior under a genuinely full disk. Review
partial cleanup task results before any retry; automated retries must not delete files.

## Development-environment observations (2026-10-03)

Linux x86_64, binary SHA-256 `9b9f834f278150cd18684fcdbfd8e1105765bf932e8c44354432ad7f8263933d`, built from Alpha.10 application code without release linker metadata:

| Fixture | Completed bounded scans | Largest scan duration | Cancellation completion | Result |
| --- | --- | --- | --- | --- |
| 2,000 empty files | 3 | 0.371 s | 0.006 s | Passed |
| 20,000 empty files, 10-second soak | 8 | 0.405 s | 0.077 s | Passed |
| 1,000,000 empty files | 10 | 0.374 s | 0.006 s | Passed |

These runs also verified scan admission after cancellation. RSS/descriptor values depend
on the execution environment's `/proc` visibility and are observations, not production
resource guarantees. Go race tests and the standalone/two-Agent smoke test also passed.
