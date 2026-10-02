# Security policy

NodeSweep is currently Alpha. Use the newest checked version and validate on a test VPS before managing production logs.

See [security boundaries and audit](docs/security.md). A successful vulnerability scan is not a guarantee that no vulnerabilities exist.

Do not post credentials, configuration files, database files, private server paths or exploitable details in public issues. Use GitHub's private vulnerability reporting if available on this repository, or arrange a private reporting channel with the repository owner before sharing details.

The application provides file metadata and constrained archive cleanup. It does not provide a terminal, arbitrary command execution, remote script installation or a reverse-shell operation. The hub and its administrator remain trusted: a compromised hub can request supported deletion operations within each agent's local allowlist. A compromised root account is outside the application's isolation boundary.
