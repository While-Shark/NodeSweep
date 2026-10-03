import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import ts from "typescript";
const source = readFileSync(
  new URL("../src/agent-config.ts", import.meta.url),
  "utf8",
);
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;
const { agentConfig, settingsError } = await import(
  "data:text/javascript;base64," + Buffer.from(compiled).toString("base64")
);
const settings = {
  hub: "https://hub.example/nodesweep/",
  cleanup: "/var/log\n/var/log",
  scan: "/",
  panels: "/www",
};
assert.equal(settingsError(settings), undefined);
assert.deepEqual(agentConfig(settings, "node-id", "test-token"), {
  mode: "agent",
  hub: "https://hub.example/nodesweep",
  node: "node-id",
  token: "test-token",
  cleanupRoots: ["/var/log"],
  scanRoots: ["/"],
  panelRoots: ["/www"],
});
for (const hub of [
  "http://remote.example",
  "https://user:secret@example.com",
  "https://example.com?",
  "https://example.com/#",
  "https://example.com/a%2fb",
]) {
  assert.ok(settingsError({ ...settings, hub }), hub);
}
for (const cleanup of [
  "/",
  "//etc//",
  "/proc",
  "relative",
  "/var/log/../etc",
  "",
]) {
  assert.ok(settingsError({ ...settings, cleanup }), cleanup);
}
assert.ok(
  settingsError({
    ...settings,
    panels: Array.from({ length: 17 }, (_, i) => "/panel" + i).join("\n"),
  }),
);
assert.equal(
  settingsError({ ...settings, hub: "http://[::1]:9780" }),
  undefined,
);
assert.ok(
  settingsError({
    ...settings,
    scan: Array.from({ length: 64 }, (_, i) => "/" + i + "x".repeat(1000)).join(
      "\n",
    ),
  }),
);
console.log(
  "PASS Agent wizard: URL restrictions, protected roots, directory limits, deduplication and panel discovery does not extend cleanup",
);

assert.equal(
  settingsError({
    ...settings,
    cleanup: Array.from(
      { length: 4 },
      (_, i) => "/logs/" + i + "<".repeat(2000),
    ).join("\n"),
  }),
  "目录元数据不能超过 32 KB。",
);
