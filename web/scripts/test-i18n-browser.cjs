// Optional browser regression: npm run build first, then node scripts/test-i18n-browser.cjs.
// Uses API fixtures; no server filesystem is scanned or cleaned.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const assert = require("node:assert/strict");
const ts = require("typescript");
const root = path.resolve(__dirname, "..");
const translations = ts.transpileModule(
  fs.readFileSync(path.join(root, "src/i18n/messages.ts"), "utf8"),
  { compilerOptions: { module: ts.ModuleKind.CommonJS } },
).outputText;
const moduleObject = { exports: {} };
new Function("exports", translations)(moduleObject.exports);
const messages = moduleObject.exports.messages;
const locales = ["zh-CN", "en", "ja", "ko", "zh-TW"];
const label = (locale, key) =>
  locale === "zh-CN" ? key : messages[key][locales.indexOf(locale) - 1];
const server = http.createServer((req, res) => {
  const file = path.join(
    root,
    "dist",
    req.url === "/" ? "index.html" : req.url.split("?")[0],
  );
  if (!fs.existsSync(file)) {
    res.writeHead(404).end();
    return;
  }
  res.setHeader(
    "Content-Type",
    file.endsWith(".js")
      ? "text/javascript"
      : file.endsWith(".css")
        ? "text/css"
        : "text/html",
  );
  fs.createReadStream(file).pipe(res);
});
(async () => {
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const base = "http://127.0.0.1:" + server.address().port;
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROMIUM_EXECUTABLE_PATH || undefined,
    args: [
      "--no-sandbox",
      "--disable-dev-shm-usage",
      "--use-gl=angle",
      "--use-angle=swiftshader",
    ],
  });
  try {
    for (const locale of locales) {
      const context = await browser.newContext({
        locale: "en-US",
        viewport: { width: 1280, height: 900 },
      });
      const page = await context.newPage();
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      const rule = {
        id: "r1",
        name: "我的日志",
        scheme: "我的方案",
        root: "/var/log",
        patterns: ["*.log.*"],
        excludes: [],
        keepDays: 14,
      };
      const tasks = new Map();
      let executions = 0;
      const node = {
        id: "local",
        name: "我的服务器",
        lastSeen: new Date().toISOString(),
        roots: ["/var/log"],
        scanRoots: ["/var/log"],
        metrics: {
          host: "fixture-vps",
          cpu: 7.1,
          memoryTotal: 1024,
          memoryAvailable: 512,
          load: "0.1",
          disks: [
            {
              path: "/",
              total: 1000000,
              available: 400000,
              inodes: 1000,
              freeInodes: 600,
            },
          ],
        },
      };
      await page.route("**/api/**", async (route) => {
        const request = route.request();
        const url = new URL(request.url());
        const endpoint = url.pathname.slice("/api/".length);
        const body = request.postDataJSON();
        const send = (data, status = 200) =>
          route.fulfill({
            status,
            contentType: "application/json",
            body: JSON.stringify(data),
          });
        if (request.headers().authorization === "Bearer bad")
          return send({ error: "invalid administrator token" }, 401);
        if (endpoint === "nodes")
          return send(
            request.method() === "POST"
              ? {
                  node: { ...node, id: "remote", name: body.name },
                  token: "fixture-token-not-a-real-credential",
                }
              : [node],
          );
        if (endpoint === "rules") return send([rule]);
        if (endpoint === "rules/export")
          return send({
            format: "nodesweep.rules",
            version: 1,
            rules: [{ ...rule, id: "" }],
          });
        if (endpoint === "tasks" && request.method() === "POST") {
          const id = "t" + (tasks.size + 1);
          let result;
          if (body.request.kind === "detect")
            result = [
              {
                name: "Linux 归档日志",
                detected: true,
                rule,
                note: "仅处理过期归档；清理前须在节点白名单允许该目录",
              },
            ];
          if (body.request.kind === "preview")
            result = {
              id: "p1",
              rule,
              created: new Date().toISOString(),
              bytes: 4096,
              files: [{ path: "app.log.1", size: 4096 }],
            };
          if (body.request.kind === "execute") {
            executions++;
            result = { deleted: 1, bytes: 4096, skipped: [] };
          }
          if (body.request.kind === "scan")
            result = {
              at: new Date().toISOString(),
              files: 1234,
              skipped: 0,
              truncated: false,
              tree: {
                name: "log",
                path: "/var/log",
                bytes: 4096,
                directory: true,
                children: [
                  {
                    name: "app.log.1",
                    path: "/var/log/app.log.1",
                    bytes: 4096,
                    directory: false,
                  },
                ],
              },
            };
          tasks.set(id, {
            id,
            node: "local",
            status: "succeeded",
            created: new Date().toISOString(),
            request: body.request,
            result,
          });
          return send({ id });
        }
        if (endpoint === "tasks") return send([...tasks.values()]);
        if (endpoint.startsWith("tasks/"))
          return send(tasks.get(endpoint.split("/")[1]));
        return send({ error: "unexpected fixture endpoint" }, 400);
      });
      await page.goto(base);
      assert.equal(await page.locator("html").getAttribute("lang"), "en"); // Browser detection.
      await page.locator(".language-picker select").selectOption(locale);
      assert.equal(
        await page.locator("h1").innerText(),
        label(locale, "服务器空间管理"),
      );
      await page.locator("input[type=password]").fill("bad");
      await page.locator("form.login button").click();
      await page.locator(".error").waitFor();
      const invalid =
        locale === "zh-CN"
          ? "管理员访问令牌无效"
          : label(locale, "invalid administrator token");
      assert.equal(await page.locator(".error").innerText(), invalid);
      const alternate = locale === "en" ? "ja" : "en";
      await page.locator(".language-picker select").selectOption(alternate);
      assert.equal(
        await page.locator(".error").innerText(),
        label(alternate, "invalid administrator token"),
      );
      await page.locator(".language-picker select").selectOption(locale);
      await page.locator("input[type=password]").fill("good");
      await page.locator("form.login button").click();
      await page.locator(".app-shell").waitFor();
      assert.ok(
        (await page.locator("main").innerText()).includes("我的服务器"),
      );
      assert.equal(
        await page.locator("header h1").innerText(),
        label(locale, "节点总览"),
      );
      await page.locator("aside nav button").nth(1).click();
      await page
        .locator("button")
        .filter({ hasText: new RegExp("^" + label(locale, "扫描目录") + "$") })
        .click();
      await page.locator(".treemap .tile").waitFor();
      assert.ok((await page.locator("main").innerText()).includes("/var/log"));
      await page.locator("aside nav button").nth(2).click();
      await page
        .getByRole("button", {
          name: label(locale, "识别服务器环境"),
          exact: true,
        })
        .click();
      await page.locator(".preset").waitFor();
      assert.equal(
        await page.locator(".preset strong").innerText(),
        label(locale, "Linux 归档日志"),
      );
      assert.ok(
        (await page.locator(".rule-row").innerText()).includes("我的日志"),
      );
      await page
        .getByRole("button", { name: label(locale, "预览清理"), exact: true })
        .click();
      await page.locator(".preview").waitFor();
      const execute = page.getByRole("button", {
        name: label(locale, "执行已预览的方案"),
        exact: true,
      });
      assert.ok(await execute.isDisabled());
      await page.locator(".language-picker select").selectOption(alternate);
      assert.ok(await page.locator(".preview").isVisible());
      assert.ok(
        (await page.locator(".preview").innerText()).includes(
          label(alternate, "我已核对清单，确认永久删除这些归档文件"),
        ),
      );
      assert.equal(
        await page.locator(".preview input[type=checkbox]").isChecked(),
        false,
      );
      await page.locator(".language-picker select").selectOption(locale);
      assert.ok(
        (await page.locator(".preview").innerText()).includes(
          label(locale, "我已核对清单，确认永久删除这些归档文件"),
        ),
      );
      await page.locator(".preview input[type=checkbox]").check();
      await execute.click();
      await page.locator(".success").waitFor();
      assert.equal(executions, 1);
      assert.ok(
        !(await page.locator(".success").innerText()).includes("{deleted}"),
      );
      await page
        .getByRole("button", { name: label(locale, "导入方案"), exact: true })
        .click();
      // Canceling a file picker does not change rules. Supply an invalid file to test translated validation.
      await page.locator("input[type=file]").setInputFiles({
        name: "invalid.json",
        mimeType: "application/json",
        buffer: Buffer.from("not JSON"),
      });
      await page.locator(".rule-transfer .error").waitFor();
      assert.equal(
        await page.locator(".rule-transfer .error").innerText(),
        label(locale, "方案文件不是有效的 JSON。"),
      );
      const downloadPromise = page.waitForEvent("download");
      await page
        .getByRole("button", { name: label(locale, "导出方案"), exact: true })
        .click();
      const download = await downloadPromise;
      const exported = JSON.parse(
        fs.readFileSync(await download.path(), "utf8"),
      );
      assert.equal(exported.rules[0].name, "我的日志");
      assert.equal(exported.rules[0].scheme, "我的方案");
      await page.locator("aside nav button").nth(3).click();
      await page.locator("table tbody .badge").first().waitFor();
      assert.ok(
        (await page.locator("table").innerText()).includes(
          label(locale, "成功"),
        ),
      );
      await page
        .getByRole("button", { name: label(locale, "添加节点"), exact: false })
        .click();
      await page.locator(".modal input").fill("自定义 VPS");
      await page
        .getByRole("button", {
          name: label(locale, "生成节点配置"),
          exact: true,
        })
        .click();
      await page.locator(".modal pre").first().waitFor();
      const config = JSON.parse(
        await page.locator(".modal pre").first().innerText(),
      );
      assert.equal(config.mode, "agent");
      assert.deepEqual(config.cleanupRoots, ["/var/log"]);
      await page
        .getByRole("button", { name: label(locale, "完成"), exact: true })
        .click();
      for (const width of [390, 360]) {
        await page.setViewportSize({ width, height: 844 });
        for (let i = 0; i < 4; i++) {
          await page.locator("aside nav button").nth(i).click();
          const overflow = await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          );
          assert.equal(overflow, false, locale + " page " + i + " at " + width);
        }
      }
      await page.reload();
      await page.locator(".login").waitFor();
      assert.equal(await page.locator("html").getAttribute("lang"), locale); // Saved preference.
      assert.equal(await page.locator("input[type=password]").inputValue(), ""); // No token persisted.
      assert.deepEqual(errors, [], locale);
      await context.close();
      console.log(
        "PASS browser: " +
          locale +
          " login, scan, discovery, cleanup confirmation, export, enrollment and mobile layouts",
      );
    }
    const blocked = await browser.newContext({ locale: "ko-KR" });
    await blocked.addInitScript(() => {
      Object.defineProperty(window, "localStorage", {
        get() {
          throw new Error("storage blocked");
        },
      });
    });
    const page = await blocked.newPage();
    await page.goto(base);
    await page.locator(".login").waitFor();
    assert.equal(await page.locator("html").getAttribute("lang"), "ko");
    await page.locator(".language-picker select").selectOption("ja");
    assert.equal(await page.locator("html").getAttribute("lang"), "ja");
    await blocked.close();
    console.log("PASS browser: language switching with blocked storage");
  } finally {
    await browser.close();
  }
})()
  .catch((error) => {
    console.error(error);
    process.exitCode = 1;
  })
  .finally(() => server.close());
