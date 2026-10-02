import assert from "node:assert/strict";
import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
} from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { join, dirname } from "node:path";
import ts from "typescript";
const root = dirname(dirname(fileURLToPath(import.meta.url)));
const cache = join(root, "node_modules", ".cache");
mkdirSync(cache, { recursive: true });
const temp = mkdtempSync(join(cache, "nodesweep-i18n-"));
try {
  for (const name of ["index", "messages", "errors"]) {
    const source = readFileSync(join(root, "src/i18n", name + ".ts"), "utf8")
      .replaceAll('"./messages"', '"./messages.mjs"')
      .replaceAll('"./errors"', '"./errors.mjs"');
    writeFileSync(
      join(temp, name + ".mjs"),
      ts.transpileModule(source, {
        compilerOptions: {
          target: ts.ScriptTarget.ES2022,
          module: ts.ModuleKind.ESNext,
        },
      }).outputText,
    );
  }
  const { messages } = await import(pathToFileURL(join(temp, "messages.mjs")));
  const {
    t,
    locale,
    detectLocale,
    systemText,
    taskKind,
    taskStatus,
    setLocale,
  } = await import(pathToFileURL(join(temp, "index.mjs")));
  const placeholders = (s) =>
    [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
  for (const [key, translations] of Object.entries(messages)) {
    assert.equal(translations.length, 4, key);
    for (const translation of translations) {
      assert.ok(translation.trim().length > 0, key);
      assert.deepEqual(placeholders(translation), placeholders(key), key);
    }
  }
  assert.equal(detectLocale(["fr-FR", "ja-JP"]), "ja");
  assert.equal(detectLocale(["en-US"]), "en");
  assert.equal(detectLocale(["ko-KR"]), "ko");
  for (const code of ["zh-Hant", "zh-HK", "zh-MO", "zh-TW"])
    assert.equal(detectLocale([code]), "zh-TW");
  assert.equal(detectLocale(["zh-Hans-SG"]), "zh-CN");
  assert.equal(detectLocale(["de-DE"]), "zh-CN");
  for (const code of ["zh-CN", "en", "ja", "ko", "zh-TW"]) {
    setLocale(code);
    assert.ok(!t("{count} 个文件", { count: 1234 }).includes("{count}"));
    assert.equal(systemText("/var/log/我的服务.log"), "/var/log/我的服务.log");
    assert.equal(taskStatus("__proto__"), "__proto__");
    assert.equal(taskKind("unknown"), "unknown");
    assert.notEqual(taskStatus("succeeded"), "succeeded");
    assert.ok(
      systemText("从 Nginx 静态日志指令识别；保留当前日志 site.log").includes(
        "site.log",
      ),
    );
  }
  setLocale("en");
  assert.equal(
    systemText("invalid administrator token"),
    "Invalid administrator token",
  );
  assert.equal(t("撤销 {name}？", { name: "<script>" }), "Revoke <script>?"); // Vue renders this as text.
  setLocale("unsupported");
  assert.equal(locale.value, "en");
  console.log(
    `PASS: ${Object.keys(messages).length} messages, all five languages, placeholders and locale fallback`,
  );
} finally {
  rmSync(temp, { recursive: true, force: true });
}
