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
      let settings = {
        enabled: false,
        diskPercent: 85,
        inodePercent: 85,
        offlineSeconds: 60,
        cooldownSeconds: 1800,
      };
      const node = {
        id: "local",
        name: "我的服务器 <img src=x onerror=window.__xss=1>",
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
      let fixtureNodes = [node];
      const submitted = [];
      let cancellableScan = false;
      let fixtureRole = "admin";
      let failRemote = true;
      let holdDetail = false,
        releaseDetail,
        notifyHeld;
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
        if (endpoint.startsWith("metrics/"))
          return send([
            {
              at: new Date(Date.now() - 3600000).toISOString(),
              samples: 5,
              cpu: 20,
              memory: 40,
              disks: [{ path: "/", used: 60, inodes: 10 }],
            },
            {
              at: new Date(Date.now() - 1800000).toISOString(),
              samples: 4,
              cpu: 80,
              memory: 50,
              disks: [{ path: "/", used: 61, inodes: 11 }],
            },
          ]);
        if (endpoint === "session") return send({ role: fixtureRole });
        if (endpoint === "audit")
          return send([
            {
              id: 1,
              at: new Date().toISOString(),
              actor: "runner",
              role: "operator",
              action: "tasks.post",
              status: 200,
            },
          ]);
        if (endpoint === "nodes")
          return send(
            request.method() === "POST"
              ? {
                  node: { ...node, id: "remote", name: body.name },
                  token: "fixture-token-not-a-real-credential",
                }
              : fixtureNodes,
          );
        if (endpoint.startsWith("nodes/") && request.method() === "PATCH") {
          Object.assign(
            fixtureNodes.find((n) => n.id === endpoint.split("/")[1]),
            body,
          );
          return send({ ok: true });
        }
        if (endpoint === "alerts") {
          if (request.method() === "PUT") {
            settings = body;
            return send({ ok: true });
          }
          return send({
            settings,
            webhookConfigured: false,
            events: [
              {
                id: 1,
                name: "我的服务器",
                path: "/",
                kind: "disk",
                percent: 91.2,
                resolved: false,
                at: new Date().toISOString(),
                delivery: "not_configured",
              },
            ],
          });
        }
        if (endpoint === "rules") return send([rule]);
        if (endpoint === "rules/export")
          return send({
            format: "nodesweep.rules",
            version: 1,
            rules: [{ ...rule, id: "" }],
          });
        if (
          endpoint.startsWith("tasks/") &&
          endpoint.endsWith("/cancel") &&
          request.method() === "POST"
        ) {
          const job = tasks.get(endpoint.split("/")[1]);
          job.status = "failed";
          job.error = "context canceled";
          job.cancelRequested = true;
          job.result.truncated = true;
          job.result.reason = "cancelled";
          return send(job);
        }
        if (endpoint === "tasks" && request.method() === "POST") {
          submitted.push(body);
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
          if (["preview", "trial"].includes(body.request.kind))
            result = {
              id: "plan-" + body.node,
              rule,
              created: new Date().toISOString(),
              bytes: 4096,
              files: [{ path: "app.log.1", size: 4096 }],
              review: {
                counts: { eligible: 1, active_or_not_archive: 1 },
                examples: [
                  { path: "app.log.1", reason: "eligible", pattern: "*.log.*" },
                  { path: "active.log", reason: "active_or_not_archive" },
                ],
              },
            };
          if (body.request.kind === "execute") {
            assert.equal(
              body.request.planId,
              "plan-" + body.node,
              "plan crossed nodes",
            );
            executions++;
            result = {
              deleted: 1,
              bytes: 4096,
              skipped: [],
              items: [{ path: "app.log.1", status: "deleted", bytes: 4096 }],
            };
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
            node: body.node,
            status:
              failRemote && body.node === "remote"
                ? "failed"
                : cancellableScan && body.request.kind === "scan"
                  ? "running"
                  : "succeeded",
            progress:
              cancellableScan && body.request.kind === "scan"
                ? {
                    visited: 100,
                    files: 90,
                    bytes: 4096,
                    limit: 100000,
                    elapsedMillis: 1000,
                  }
                : undefined,
            error:
              failRemote && body.node === "remote" ? "node offline" : undefined,
            created: new Date().toISOString(),
            request: body.request,
            result,
          });
          return send({ id });
        }
        if (endpoint === "tasks") return send([...tasks.values()]);
        if (endpoint.startsWith("tasks/")) {
          if (holdDetail && endpoint === "tasks/t1") {
            await new Promise((resolve) => {
              releaseDetail = resolve;
              notifyHeld();
            });
            holdDetail = false;
          }
          return send(tasks.get(endpoint.split("/")[1]));
        }
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
      await page
        .getByRole("link", {
          name: label(locale, "跳转到主要内容"),
          exact: true,
        })
        .focus();
      await page.keyboard.press("Enter");
      assert.equal(
        await page.evaluate(() => document.activeElement.id),
        "main-content",
      );
      assert.equal(
        await page.locator("aside nav button[aria-current=page]").count(),
        1,
      );
      assert.ok(
        (await page.locator("main").innerText()).includes("我的服务器"),
      );
      assert.equal(
        await page.locator("header h1").innerText(),
        label(locale, "节点总览"),
      );
      assert.equal(await page.locator("main img").count(), 0);
      assert.equal(await page.evaluate(() => window.__xss), undefined);
      assert.ok(
        (await page.locator("main").innerText()).includes(
          "<img src=x onerror=window.__xss=1>",
        ),
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
        .getByRole("button", { name: label(locale, "规则试运行"), exact: true })
        .click();
      await page.locator(".review-details").first().waitFor();
      await page.locator(".review-details summary").first().click();
      assert.ok(
        (await page.locator("main").innerText()).includes(
          label(locale, "活跃日志或非归档文件"),
        ),
      );
      assert.equal(executions, 0);
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
      await page.locator(".cleanup-report").waitFor();
      assert.ok(
        (await page.locator(".cleanup-report").innerText()).includes(
          "app.log.1",
        ),
      );
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
      holdDetail = true;
      const heldDetail = new Promise((resolve) => {
        notifyHeld = resolve;
      });
      await page.locator("table tbody button").nth(0).click();
      await heldDetail;
      await page.locator("table tbody button").nth(1).click();
      await page.waitForFunction(() =>
        document.querySelector(".detail")?.textContent.includes('"id": "t2"'),
      );
      const oldDetailResponse = page.waitForResponse((r) =>
        r.url().endsWith("/api/tasks/t1"),
      );
      releaseDetail();
      await oldDetailResponse;
      await page.evaluate(
        () => new Promise((resolve) => setTimeout(resolve, 0)),
      );
      assert.equal(
        JSON.parse(await page.locator(".detail").innerText()).id,
        "t2",
      );
      await page
        .getByRole("button", { name: label(locale, "添加节点"), exact: false })
        .click();
      await page.locator(".modal").waitFor();
      assert.ok(
        await page.evaluate(() =>
          document.querySelector(".modal").contains(document.activeElement),
        ),
      );
      assert.ok(await page.locator("main").evaluate((el) => el.inert));
      for (let i = 0; i < 12; i++) {
        await page.keyboard.press("Tab");
        assert.ok(
          await page.evaluate(() =>
            document.querySelector(".modal").contains(document.activeElement),
          ),
        );
      }
      await page.keyboard.press("Escape");
      await page.locator(".modal").waitFor({ state: "hidden" });
      assert.ok(
        await page.evaluate(
          (name) => document.activeElement.textContent.includes(name),
          label(locale, "添加节点"),
        ),
      );
      assert.ok(!(await page.locator("main").evaluate((el) => el.inert)));
      await page
        .getByRole("button", {
          name: "＋ " + label(locale, "添加节点"),
          exact: true,
        })
        .click();
      await page.locator(".modal input").first().fill("自定义 VPS");
      await page
        .getByLabel(label(locale, "管理端 HTTPS 地址"), { exact: true })
        .fill("http://remote.example");
      assert.ok(
        await page
          .getByRole("button", {
            name: label(locale, "生成节点配置"),
            exact: true,
          })
          .isDisabled(),
      );
      await page
        .getByLabel(label(locale, "管理端 HTTPS 地址"), { exact: true })
        .fill("https://hub.example/nodesweep");
      await page
        .getByLabel(label(locale, "允许清理的目录"), { exact: true })
        .fill("//etc//");
      assert.ok(
        await page
          .getByRole("button", {
            name: label(locale, "生成节点配置"),
            exact: true,
          })
          .isDisabled(),
      );
      await page
        .getByLabel(label(locale, "允许清理的目录"), { exact: true })
        .fill("/var/log");
      await page
        .getByLabel(label(locale, "自定义面板安装目录（可选）"), {
          exact: true,
        })
        .fill("/www/custom");
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
      assert.equal(config.hub, "https://hub.example/nodesweep");
      assert.deepEqual(config.panelRoots, ["/www/custom"]);
      assert.deepEqual(config.cleanupRoots, ["/var/log"]);
      await page
        .getByRole("button", { name: label(locale, "完成"), exact: true })
        .click();
      await page.locator("aside nav button").nth(4).click();
      await page.locator(".form-grid").waitFor();
      await page.locator("form input[type=checkbox]").first().check();
      await page.locator("form input[type=checkbox]").nth(1).check();
      await page
        .getByRole("button", {
          name: label(locale, "保存告警设置"),
          exact: true,
        })
        .click();
      await page.locator(".success").waitFor();
      assert.equal(settings.enabled, true);
      assert.ok((await page.locator("main").innerText()).includes("91.2%"));
      node.group = "production";
      fixtureNodes = [
        node,
        { ...node, id: "remote", name: "second-vps" },
        {
          ...node,
          id: "offline",
          name: "offline-vps",
          lastSeen: "2000-01-01T00:00:00Z",
        },
      ];
      await page.locator("aside nav button").nth(5).click();
      await page.locator("table tbody tr").nth(2).waitFor();
      assert.ok(
        await page
          .getByRole("checkbox", { name: "offline-vps", exact: true })
          .isDisabled(),
      );
      await page
        .getByRole("button", {
          name: label(locale, "选择本组在线节点"),
          exact: true,
        })
        .click();
      await page
        .locator("main > section select")
        .first()
        .selectOption("g:production");
      await page.waitForFunction(() =>
        [
          ...document.querySelectorAll("main > section input[type=checkbox]"),
        ].every((n) => !n.checked),
      );
      assert.ok(
        await page
          .getByRole("button", { name: label(locale, "批量扫描"), exact: true })
          .isDisabled(),
      );
      await page
        .getByRole("button", {
          name: label(locale, "选择本组在线节点"),
          exact: true,
        })
        .click();
      const batchStart = submitted.length;
      await page
        .getByRole("button", { name: label(locale, "批量扫描"), exact: true })
        .click();
      await page
        .locator("article")
        .filter({ hasText: "1,234" })
        .or(page.locator("article").filter({ hasText: "1234" }))
        .first()
        .waitFor();
      await page.locator("article .error").waitFor();
      assert.deepEqual(
        submitted
          .slice(batchStart)
          .map((x) => x.node)
          .sort(),
        ["local", "remote"],
      );
      assert.ok(
        submitted.slice(batchStart).every((x) => x.request.kind === "scan"),
      );
      await page
        .getByRole("button", { name: label(locale, "批量预览"), exact: true })
        .click();
      await page.locator("article .review-details").waitFor();
      assert.equal(executions, 1);
      await page
        .getByRole("button", { name: label(locale, "编辑节点"), exact: true })
        .first()
        .click();
      await page.locator(".modal").waitFor();
      await page.keyboard.press("Shift+Tab");
      assert.ok(
        await page.evaluate(() =>
          document.querySelector(".modal").contains(document.activeElement),
        ),
      );
      await page.keyboard.press("Escape");
      await page.locator(".modal").waitFor({ state: "hidden" });
      assert.ok(
        await page.evaluate(
          (name) => document.activeElement.textContent.includes(name),
          label(locale, "编辑节点"),
        ),
      );
      await page
        .getByRole("button", { name: label(locale, "编辑节点"), exact: true })
        .first()
        .click();
      await page.locator(".modal input").nth(0).fill("renamed-vps");
      await page.locator(".modal input").nth(1).fill("*");
      await page
        .getByRole("button", { name: label(locale, "保存"), exact: true })
        .click();
      await page.locator(".modal").waitFor({ state: "hidden" });
      await page.locator("main > section select").first().selectOption("g:*");
      await page.waitForFunction(
        () =>
          document.querySelectorAll("main > section > div.card table tbody tr")
            .length === 1,
      );
      assert.equal(
        await page.locator("main > section > div.card table tbody tr").count(),
        1,
      );
      assert.ok(
        (
          await page
            .locator("main > section > div.card table tbody tr")
            .innerText()
        ).includes("renamed-vps"),
      );
      await page.locator("aside nav button").nth(0).click();
      await page.locator(".section-heading select").selectOption("g:*");
      await page.waitForFunction(
        () => document.querySelectorAll(".node-card").length === 1,
      );
      assert.equal(await page.locator(".node-card").count(), 1);
      await page.locator(".history-charts figure").first().waitFor();
      assert.equal(await page.locator(".history-charts figure").count(), 4);
      assert.equal(
        await page
          .locator(".history-charts figure")
          .first()
          .locator(".chart-line")
          .count(),
        2,
      );
      await page
        .getByText(label(locale, "查看指标数据"), { exact: true })
        .click();
      assert.equal(await page.locator(".history-table tbody tr").count(), 2);
      await page.locator(".history-controls select").nth(1).selectOption("7d");
      await page.waitForFunction(
        () =>
          document.querySelectorAll(
            ".history-charts figure:first-child .chart-line",
          ).length === 1,
      );
      cancellableScan = true;
      await page.locator("aside nav button").nth(1).click();
      await page
        .getByRole("button", { name: label(locale, "扫描目录"), exact: true })
        .click();
      await page
        .getByRole("button", { name: label(locale, "取消扫描"), exact: true })
        .click();
      await page.locator(".treemap .tile").waitFor();
      assert.ok(
        (await page.locator("main").innerText()).includes(
          label(locale, "任务已取消"),
        ),
      );
      assert.ok(
        (await page.locator("main").innerText()).includes(
          label(locale, "扫描已取消"),
        ),
      );
      cancellableScan = false;
      fixtureNodes[2].lastSeen = new Date().toISOString();
      await page.locator("aside nav button").nth(5).click();
      await page.waitForFunction(
        () =>
          !document.querySelector('input[aria-label="offline-vps"]').disabled,
      );
      await page
        .getByRole("button", {
          name: label(locale, "选择本组在线节点"),
          exact: true,
        })
        .click();
      const stopStart = submitted.length;
      await page
        .getByRole("button", { name: label(locale, "批量扫描"), exact: true })
        .click();
      await page
        .getByRole("button", {
          name: label(locale, "停止后续提交"),
          exact: true,
        })
        .click();
      await page
        .locator("article")
        .filter({ hasText: label(locale, "未提交") })
        .waitFor();
      assert.equal(submitted.length - stopStart, 2); // Two workers, third node remains unsubmitted.
      assert.ok(
        submitted.slice(stopStart).every((x) => x.request.kind === "scan"),
      );
      assert.equal(executions, 1);
      const leaveStart = submitted.length;
      await page
        .getByRole("button", { name: label(locale, "批量扫描"), exact: true })
        .click();
      await page.locator("aside nav button").nth(3).click();
      await page.waitForTimeout(1200); // Allow old workers to reach the next queue item if cancellation is broken.
      assert.equal(submitted.length - leaveStart, 2);
      failRemote = false;
      await page.locator("aside nav button").nth(5).click();
      await page
        .getByRole("button", {
          name: label(locale, "选择本组在线节点"),
          exact: true,
        })
        .click();
      await page
        .getByRole("button", { name: label(locale, "批量预览"), exact: true })
        .click();
      await page
        .getByRole("button", {
          name: label(locale, "停止后续提交"),
          exact: true,
        })
        .waitFor({ state: "hidden" });
      assert.equal(
        await page.locator("article input[type=checkbox]").count(),
        3,
      );
      assert.ok(await page.locator(".batch-confirm button").isDisabled());
      await page.locator("article details summary").first().click();
      const listDownload = page.waitForEvent("download");
      await page
        .getByRole("button", {
          name: label(locale, "导出完整清单"),
          exact: true,
        })
        .first()
        .click();
      const savedList = await listDownload;
      const previewList = JSON.parse(
        fs.readFileSync(await savedList.path(), "utf8"),
      );
      assert.equal(previewList.files.length, 1);
      assert.equal(previewList.node, fixtureNodes[0].id);
      assert.equal(previewList.id, undefined);
      await page.locator("article input[type=checkbox]").first().check();
      assert.ok(!(await page.locator(".batch-confirm button").isDisabled()));
      await page.locator("main > section select").first().selectOption("g:*");
      assert.ok(
        await page.locator(".batch-confirm button").isDisabled(),
        "group change kept old authorization",
      );
      await page.locator("main > section select").first().selectOption("*");
      await page
        .getByRole("button", {
          name: label(locale, "选择本组在线节点"),
          exact: true,
        })
        .click();
      await page
        .getByRole("button", { name: label(locale, "批量预览"), exact: true })
        .click();
      await page
        .getByRole("button", {
          name: label(locale, "停止后续提交"),
          exact: true,
        })
        .waitFor({ state: "hidden" });
      for (const checkbox of await page
        .locator("article input[type=checkbox]")
        .all())
        await checkbox.check();
      const cleanupStart = submitted.length;
      await page.locator(".batch-confirm button").click();
      await page
        .getByRole("button", {
          name: label(locale, "停止后续提交"),
          exact: true,
        })
        .click();
      await page
        .locator("article")
        .filter({ hasText: label(locale, "未提交") })
        .waitFor();
      assert.equal(submitted.length - cleanupStart, 2);
      assert.ok(
        submitted
          .slice(cleanupStart)
          .every(
            (x) =>
              x.request.kind === "execute" &&
              x.request.planId === "plan-" + x.node,
          ),
      );
      assert.ok(
        await page.locator(".batch-confirm button").isDisabled(),
        "cleanup previews were reused",
      );
      assert.equal(
        await page.locator("article input[type=checkbox]").count(),
        0,
      );
      for (const width of [390, 360]) {
        await page.setViewportSize({ width, height: 844 });
        for (let i = 0; i < 7; i++) {
          await page.locator("aside nav button").nth(i).click();
          const overflow = await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          );
          assert.equal(overflow, false, locale + " page " + i + " at " + width);
        }
      }
      await page.setViewportSize({ width: 1280, height: 900 });
      await page
        .getByRole("button", { name: label(locale, "退出登录"), exact: true })
        .click();
      await page.locator(".login").waitFor();
      assert.equal(await page.locator(".error").count(), 0);
      assert.equal(await page.locator("input[type=password]").inputValue(), "");
      for (const testedRole of ["viewer", "operator"]) {
        fixtureRole = testedRole;
        await page
          .locator("input[type=password]")
          .fill(testedRole + "-fixture");
        await page
          .getByRole("button", {
            name: label(locale, "进入控制台 →"),
            exact: true,
          })
          .click();
        await page.locator(".app-shell").waitFor();
        assert.equal(
          await page
            .getByRole("button", {
              name: label(locale, "添加节点"),
              exact: false,
            })
            .count(),
          0,
        );
        assert.equal(
          await page
            .getByRole("button", {
              name: label(locale, "操作审计"),
              exact: false,
            })
            .count(),
          0,
        );
        await page.locator("aside nav button").nth(1).click();
        assert.equal(
          await page
            .getByRole("button", {
              name: label(locale, "扫描目录"),
              exact: true,
            })
            .isDisabled(),
          testedRole === "viewer",
        );
        await page.locator("aside nav button").nth(2).click();
        await page.locator(".rule-row").first().waitFor();
        assert.equal(
          await page
            .getByRole("button", { name: label(locale, "编辑"), exact: true })
            .first()
            .isDisabled(),
          true,
        );
        assert.equal(await page.locator(".rule-form").count(), 0);
        assert.equal(
          await page
            .getByRole("button", {
              name: label(locale, "预览清理"),
              exact: true,
            })
            .first()
            .isDisabled(),
          testedRole === "viewer",
        );
        await page.locator("aside nav button").nth(4).click();
        await page.locator(".form-grid").waitFor();
        assert.equal(
          await page.locator("form input[type=checkbox]").first().isDisabled(),
          true,
        );
        await page
          .getByRole("button", { name: label(locale, "退出登录"), exact: true })
          .click();
        await page.locator(".login").waitFor();
      }
      fixtureRole = "admin";
      await page.reload();
      await page.locator(".login").waitFor();
      assert.equal(await page.locator("html").getAttribute("lang"), locale); // Saved preference.
      assert.equal(await page.locator("input[type=password]").inputValue(), ""); // No token persisted.
      assert.deepEqual(errors, [], locale);
      await context.close();
      console.log(
        "PASS browser: " +
          locale +
          " login, scan, dry run, cleanup report, alerts, node groups, batch isolation, queue stop, page exit, export, enrollment and mobile layouts",
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
