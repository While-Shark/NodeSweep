<script setup lang="ts">
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
const labels: Record<string, string> = {
  scan: "目录扫描",
  preview: "清理预览",
  execute: "执行清理",
  detect: "环境识别",
};
</script>
<template>
  <section>
    <div class="section-heading">
      <div>
        <h2>任务记录</h2>
        <p>每一次扫描与清理，都有迹可循。显示最近 100 条。</p>
      </div>
      <button @click="load">刷新</button>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="card table-wrap">
      <table>
        <thead>
          <tr>
            <th>任务</th>
            <th>节点</th>
            <th>时间</th>
            <th>状态</th>
            <th>详情</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tasks" :key="t.id">
            <td>{{ labels[t.request.kind] || t.request.kind }}</td>
            <td>{{ t.node === "local" ? "本机" : t.node.slice(0, 8) }}</td>
            <td>{{ date(t.created) }}</td>
            <td>
              <span class="badge">{{ t.status }}</span
              ><small v-if="t.error" class="error">{{ t.error }}</small>
            </td>
            <td><button @click="inspect(t)">查看</button></td>
          </tr>
        </tbody>
      </table>
      <p v-if="!tasks.length" class="empty">还没有任务记录</p>
    </div>
    <pre v-if="detail" class="card detail">{{ detail }}</pre>
  </section>
</template>
