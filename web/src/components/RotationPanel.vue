<script setup lang="ts">
import { ref, onUnmounted } from "vue";
import { canOperate } from "../access";
import { task, online, date, type Node } from "../api";
import { t, systemText } from "../i18n";
defineProps<{ node: Node }>();
interface Report {
  at: string;
  partial: boolean;
  sources: {
    kind: string;
    path: string;
    status: string;
    settings: Record<string, string>;
  }[];
}
const report = ref<Report>(),
  busy = ref(false),
  error = ref("");
const controller = new AbortController();
onUnmounted(() => controller.abort());
async function inspect(node: Node) {
  if (busy.value || !canOperate.value || !node.logChecks || !online(node))
    return;
  busy.value = true;
  error.value = "";
  report.value = undefined;
  try {
    report.value = await task<Report>(
      node.id,
      { kind: "rotation" },
      controller.signal,
    );
  } catch (e) {
    if (!controller.signal.aborted) error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="card">
    <h2>{{ t("日志轮转检查") }}</h2>
    <p class="hint">
      {{
        t(
          "只读检查 logrotate、journald 和 Docker 的静态轮转参数，不修改配置、不执行脚本、不读取日志内容。",
        )
      }}
    </p>
    <button
      class="primary"
      :disabled="busy || !canOperate || !online(node) || !node.logChecks"
      @click="inspect(node)"
    >
      {{ busy ? t("执行中") : t("检查轮转配置") }}
    </button>
    <p v-if="!node.logChecks" class="hint">
      {{ t("请升级 Agent 后使用日志检查") }}
    </p>
    <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
    <div v-if="report">
      <p>{{ date(report.at) }}</p>
      <p class="hint">
        {{
          t(
            "参数按来源展示，不代表实际生效配置；包含文件、覆盖项、启动参数和现有容器需在服务器核对。",
          )
        }}
      </p>
      <p v-if="report.partial" class="hint">
        {{ t("部分配置不存在、不可读取或超过检查预算。") }}
      </p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{{ t("来源") }}</th>
              <th>{{ t("状态") }}</th>
              <th>{{ t("轮转参数") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="source in report.sources" :key="source.path">
              <td>
                {{ source.kind }}<br /><code>{{ source.path }}</code>
              </td>
              <td>
                {{
                  t(
                    source.status === "observed"
                      ? "已读取参数"
                      : source.status === "not_configured"
                        ? "未发现受支持参数"
                        : "配置不可用",
                  )
                }}
              </td>
              <td>
                <div v-for="(value, key) in source.settings" :key="key">
                  <code
                    >{{ key }} =
                    {{ value === "present" ? t("已发现") : value }}</code
                  >
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
  <section class="card">
    <h3>{{ t("原生日志保留策略") }}</h3>
    <p>
      {{
        t(
          "Docker 日志由 Docker 管理，建议设置 local 驱动或 json-file 的 max-size 与 max-file。新默认值不会自动应用到现有容器。",
        )
      }}
    </p>
    <pre>
{"log-driver":"json-file","log-opts":{"max-size":"10m","max-file":"3"}}</pre>
    <p>
      {{
        t(
          "journal 使用 journald 的保留配置或 journalctl 原生 vacuum；不要加入普通归档删除规则。vacuum 只清理已归档日志，永久删除前请核对保留需求。",
        )
      }}
    </p>
    <pre>
[Journal]
SystemMaxUse=500M
SystemKeepFree=1G
MaxRetentionSec=14day</pre>
    <a
      href="https://github.com/While-Shark/NodeSweep/blob/master/docs/log-retention.md"
      target="_blank"
      rel="noopener noreferrer"
      >{{ t("查看保留策略指南") }}</a
    >
  </section>
</template>
