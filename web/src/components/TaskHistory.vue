<script setup lang="ts">
import { t, systemText, taskKind, taskStatus } from "../i18n";
import { onMounted, ref } from "vue";
import { api, date, type Task } from "../api";
const tasks = ref<Task[]>([]);
const error = ref("");
const detail = ref("");
async function load() {
  try {
    tasks.value = await api<Task[]>("tasks");
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function inspect(t: Task) {
  try {
    detail.value = JSON.stringify(await api("tasks/" + t.id), null, 2);
  } catch (e) {
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
              <button @click="inspect(job)">{{ t("查看") }}</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!tasks.length" class="empty">{{ t("还没有任务记录") }}</p>
    </div>
    <pre v-if="detail" class="card detail">{{ detail }}</pre>
  </section>
</template>
