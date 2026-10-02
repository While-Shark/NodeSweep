# Contributing

Keep changes focused: engine logic, transport, persistence and UI have separate modules. Do not introduce arbitrary shell execution or weaken node-local allowlists to make a cleanup succeed.

Build the frontend before Go compilation. Run `go test -race ./...`, `go vet ./...`, and `npm run build` in `web`. Cleanup tests must use temporary directories only. Add a regression test whenever modifying deletion or authentication boundaries.

Document partial-completion and failure behavior. Report reproducible compatibility details: distribution, filesystem, panel version, configured paths and redacted errors. Never include tokens or private logs in issues.
