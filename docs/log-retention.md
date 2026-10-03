# Native log retention

The **Log rotation checks** page runs a read-only task on standalone or upgraded Agents. It inspects fixed logrotate, journald and Docker configuration locations without running scripts, reading log contents, changing files or contacting the Docker socket. It returns only supported rotation settings, never complete configuration text.

Checks use pinned directories, refuse symlinks and nonregular files, limit each file to 64 KiB, directory listings to 16 entries each, input to roughly 1 MiB and output to 32 KiB, with a cooperative one-second deadline. Missing sources and limits are identified. Kernel-blocked filesystem calls may delay the deadline. A source list is not an effective configuration calculation: verify includes, overrides, rootless/custom configuration, startup flags, journal namespaces and running containers on the actual host.

Ordinary cleanup refuses known journal/audit directories and Docker container storage. A bounded, safe read of Docker's standard daemon configuration also protects a static custom `data-root/containers`. Journal filenames and common Docker JSON archive names are rejected independently of custom patterns. Nonstandard Docker configuration needs manual verification; do not allow its storage directories. These checks do not broaden cleanup allowlists.

## Docker

Use Docker's retention mechanism. Merge these keys into the existing daemon configuration, preserving other settings:

```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "3" }
}
```

Option values are strings. The `local` driver is another rotation-capable choice. Verify the configuration and plan service disruption before applying it; new daemon defaults do not change existing containers. Check each container's logging settings and recreate it through its deployment process when needed. NodeSweep does not delete Docker-managed files or automatically restart services.

Sources: [Docker configuration](https://docs.docker.com/engine/logging/configure/), [JSON driver](https://docs.docker.com/engine/logging/drivers/json-file/), [local driver](https://docs.docker.com/engine/logging/drivers/local/).

## Journald

For persistent journals, a reviewed drop-in such as `/etc/systemd/journald.conf.d/90-retention.conf` can contain:

```ini
[Journal]
SystemMaxUse=500M
SystemKeepFree=1G
MaxRetentionSec=14day
```

Choose limits for your retention requirements and review overrides before applying them. Volatile journals use separate runtime settings.

On the server, first inspect usage:

```bash
sudo journalctl --disk-usage
```

After reviewing the permanent deletion, use the native operation, for example:

```bash
sudo journalctl --vacuum-time=14d
```

Vacuum removes archived journals; active journals can keep total usage above a size target. Rotation and vacuum have different effects. NodeSweep shows guidance and static settings; it does not execute journalctl or offer a remote command runner. Actual retention and service changes remain operator actions on the host.

Sources: [journalctl](https://www.freedesktop.org/software/systemd/man/journalctl.html), [journald configuration](https://www.freedesktop.org/software/systemd/man/journald.conf.html), [upstream manual source](https://github.com/systemd/systemd/blob/main/man/journalctl.xml).

## Logrotate

The checker reports supported interval, count, size and compression directives. It skips action scripts and marks the presence of includes without evaluating them. Hints from multiple stanzas are not a merged policy. Verify the actual application's reopen behavior, ownership and retention before changing a rule; NodeSweep never runs logrotate hooks.
