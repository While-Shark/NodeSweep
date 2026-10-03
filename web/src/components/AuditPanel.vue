<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { api, date } from "../api";
import { t, systemText } from "../i18n";
interface Event {
  id: number;
  at: string;
  actor: string;
  role: string;
  action: string;
  target?: string;
  status: number;
}
const events = ref<Event[]>([]),
  busy = ref(false),
  error = ref("");
const controller = new AbortController();
async function load() {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    events.value = await api<Event[]>(
      "audit",
      "GET",
      undefined,
      controller.signal,
    );
  } catch (e) {
    if (!controller.signal.aborted) error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
onMounted(load);
onUnmounted(() => controller.abort());
</script>
<template>
  <section class="card table-wrap">
    <div class="toolbar spread">
      <h2>{{ t("操作审计") }}</h2>
      <button :disabled="busy" @click="load">{{ t("刷新") }}</button>
    </div>
    <p class="hint">
      {{ t("最多保留 1000 条、30 天；不记录令牌或请求正文。") }}
    </p>
    <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
    <table>
      <thead>
        <tr>
          <th>{{ t("时间") }}</th>
          <th>{{ t("操作人") }}</th>
          <th>{{ t("角色") }}</th>
          <th>{{ t("操作") }}</th>
          <th>{{ t("目标编号") }}</th>
          <th>{{ t("状态") }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in events" :key="e.id">
          <td>{{ date(e.at) }}</td>
          <td>{{ e.actor }}</td>
          <td>
            {{
              t(
                e.role === "admin"
                  ? "管理员"
                  : e.role === "operator"
                    ? "操作员"
                    : "只读",
              )
            }}
          </td>
          <td>
            <code>{{ e.action }}</code>
          </td>
          <td>
            <code>{{ e.target || "—" }}</code>
          </td>
          <td>{{ e.status === 0 ? t("结果未记录") : e.status }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>
