// Exercise delayed responses that deliberately ignore AbortSignal.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import ts from "typescript";
const source = readFileSync(
  new URL("../src/api.ts", import.meta.url),
  "utf8",
).replace(
  'import { locale } from "./i18n";',
  'const locale = { value: "en" };',
);
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;
const { api, task, setToken } = await import(
  "data:text/javascript;base64," + Buffer.from(compiled).toString("base64")
);
const originalFetch = globalThis.fetch;
try {
  let finish;
  let requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url, init });
    return await new Promise((resolve) => {
      finish = resolve;
    });
  };
  setToken("old-token");
  const stale = api("nodes");
  const staleRejected = assert.rejects(stale, { name: "AbortError" });
  setToken("new-token");
  finish({ ok: true, json: async () => [{ id: "old-node" }] });
  await staleRejected;
  assert.equal(requests[0].init.headers.Authorization, "Bearer old-token");
  assert.ok(requests[0].init.signal.aborted);
  globalThis.fetch = async (url, init) => {
    requests.push({ url, init });
    return { ok: true, json: async () => ({ id: "fresh" }) };
  };
  assert.deepEqual(await api("nodes"), { id: "fresh" });
  assert.equal(requests[1].init.headers.Authorization, "Bearer new-token");
  const cancelled = new AbortController();
  cancelled.abort();
  await assert.rejects(api("nodes", "GET", undefined, cancelled.signal), {
    name: "AbortError",
  });
  assert.equal(requests.length, 2);
  let failOld;
  globalThis.fetch = async () =>
    new Promise((_resolve, reject) => {
      failOld = reject;
    });
  const failedOld = api("nodes");
  const failedOldRejected = assert.rejects(failedOld, { name: "AbortError" });
  setToken("new-token");
  failOld(new Error("old session network error"));
  await failedOldRejected;
  requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url, init });
    return { ok: true, json: async () => ({ id: "queued" }) };
  };
  const oldTask = task("node-a", { kind: "scan", path: "/var/log" });
  const oldTaskRejected = assert.rejects(oldTask, { name: "AbortError" });
  await new Promise(setImmediate);
  setToken("next-session-token");
  await oldTaskRejected;
  assert.equal(
    requests.length,
    1,
    "old task must not poll using the next session token",
  );
  assert.equal(requests[0].init.headers.Authorization, "Bearer new-token");
  console.log(
    "PASS API: stale responses discarded, external abort preserved, old task polling cannot cross sessions",
  );
} finally {
  globalThis.fetch = originalFetch;
  setToken("");
}
