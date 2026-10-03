<script setup lang="ts">
import { canOperate } from "../access";
import { t, systemText, taskKind, taskStatus } from "../i18n";
import { onMounted, onUnmounted, ref } from "vue";
import CleanupReport from "./CleanupReport.vue";
import ReviewDetails from "./ReviewDetails.vue";
import type { CleanupResult, Plan, Node } from "../api";
import { api, date, size, type Task } from "../api";
const props = defineProps<{ nodes: Node[] }>();
const cancelling = ref("");
function canCancel(job: Task) {
  if (!canOperate.value) return false;
  return (
    job.request.kind === "scan" &&
    ["pending", "running"].includes(job.status) &&
    !job.cancelRequested &&
    (job.node === "local" ||
      props.nodes.some((n) => n.id === job.node && n.scanControl))
  );
}
async function cancelScan(job: Task) {
  cancelling.value = job.id;
  try {
    await api(
      "tasks/" + job.id + "/cancel",
      "POST",
      undefined,
      controller.signal,
    );
    await load();
  } catch (e) {
    if (!controller.signal.aborted) error.value = (e as Error).message;
  } finally {
    cancelling.value = "";
  }
}
const controller = new AbortController();
onUnmounted(() => controller.abort());
let loadGeneration = 0;
let detailGeneration = 0;
const tasks = ref<Task[]>([]);
const error = ref("");
const detail = ref("");
const report = ref<CleanupResult>();
const review = ref<Plan>();
async function load() {
  const generation = ++loadGeneration;
  try {
    const updated = await api<Task[]>(
      "tasks",
      "GET",
      undefined,
      controller.signal,
    );
    if (generation !== loadGeneration) return;
    tasks.value = updated;
    error.value = "";
  } catch (e) {
    if (!controller.signal.aborted && generation === loadGeneration)
      error.value = (e as Error).message;
  }
}
async function inspect(t: Task) {
  const generation = ++detailGeneration;
  detail.value = "";
  report.value = undefined;
  review.value = undefined;
  error.value = "";
  try {
    const data = await api<Task>(
      "tasks/" + t.id,
      "GET",
      undefined,
      controller.signal,
    );
    if (generation !== detailGeneration) return;
    report.value =
      data.request.kind === "execute" && data.result
        ? (data.result as CleanupResult)
        : undefined;
    review.value =
      ["preview", "trial"].includes(data.request.kind) && data.result
        ? (data.result as Plan)
        : undefined;
    detail.value = JSON.stringify(data, null, 2);
  } catch (e) {
    if (!controller.signal.aborted && generation === detailGeneration)
      error.value = (e as Error).message;
  }
}
onMounted(load);
</script>
<template>
  <section>
    <div class="section-heading">
      <div>
        <h2>{{ t("任务记录") }}</h2>
        <p>{{ t("每一次扫描与清理，都有迹可循。显示最近 100 条。") }}</p>
      </div>
      <button @click="load">{{ t("刷新") }}</button>
    </div>
    <p v-if="error" class="error">{{ systemText(error) }}</p>
    <div class="card table-wrap">
      <table>
        <thead>
          <tr>
            <th>{{ t("任务") }}</th>
            <th>{{ t("节点") }}</th>
            <th>{{ t("时间") }}</th>
            <th>{{ t("状态") }}</th>
            <th>{{ t("详情") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="job in tasks" :key="job.id">
            <td>{{ taskKind(job.request.kind) }}</td>
            <td>
              {{ job.node === "local" ? t("本机") : job.node.slice(0, 8) }}
            </td>
            <td>{{ date(job.created) }}</td>
            <td>
              <span class="badge">{{ taskStatus(job.status) }}</span
              ><small v-if="job.error" class="error">{{
                systemText(job.error)
              }}</small>
            </td>
            <td>
              <button @click="inspect(job)">{{ t("查看") }}</button
              ><button
                v-if="canCancel(job)"
                :disabled="!!cancelling"
                @click="cancelScan(job)"
              >
                {{ t("取消扫描") }}
              </button>
              <span
                v-if="job.cancelRequested && job.status === 'running'"
                class="hint"
                >{{ t("正在取消扫描…") }}</span
              >
              <small v-if="job.progress">{{
                t("已访问 {visited} 项 · 已统计 {files} 项 · {space}", {
                  visited: job.progress.visited,
                  files: job.progress.files,
                  space: size(job.progress.bytes),
                })
              }}</small>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!tasks.length" class="empty">{{ t("还没有任务记录") }}</p>
    </div>
    <CleanupReport v-if="report" :result="report" />
    <ReviewDetails v-if="review?.review" :review="review.review" />
    <pre v-if="detail" class="card detail">{{ detail }}</pre>
  </section>
</template>
