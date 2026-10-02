<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { api, date } from "../api";
import { t, systemText } from "../i18n";
interface Settings {
  enabled: boolean;
  diskPercent: number;
  inodePercent: number;
  offlineSeconds: number;
  cooldownSeconds: number;
}
interface Alert {
  id: number;
  name: string;
  path: string;
  kind: string;
  percent: number;
  resolved: boolean;
  at: string;
  delivery: string;
}
const settings = ref<Settings>({
  enabled: false,
  diskPercent: 85,
  inodePercent: 85,
  offlineSeconds: 60,
  cooldownSeconds: 1800,
});
const events = ref<Alert[]>([]);
const configured = ref(false),
  busy = ref(false),
  error = ref(""),
  saved = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;
async function history() {
  const data = await api<{
    settings: Settings;
    events: Alert[];
    webhookConfigured: boolean;
  }>("alerts");
  events.value = data.events;
  configured.value = data.webhookConfigured;
  return data;
}
async function load() {
  try {
    settings.value = (await history()).settings;
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function save() {
  busy.value = true;
  error.value = "";
  saved.value = false;
  try {
    await api("alerts", "PUT", settings.value);
    saved.value = true;
    await history();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
onMounted(() => {
  void load();
  timer = setInterval(
    () => history().catch((e) => (error.value = e.message)),
    5000,
  );
});
onUnmounted(() => clearInterval(timer));
</script>
<template>
  <section>
    <form class="card" @submit.prevent="save">
      <h2>{{ t("磁盘告警") }}</h2>
      <label class="check"
        ><input v-model="settings.enabled" type="checkbox" />{{
          t("启用告警")
        }}</label
      >
      <div class="form-grid">
        <label
          >{{ t("磁盘使用阈值 (%)")
          }}<input
            v-model.number="settings.diskPercent"
            type="number"
            min="1"
            max="100"
            required
        /></label>
        <label
          >{{ t("inode 使用阈值 (%)")
          }}<input
            v-model.number="settings.inodePercent"
            type="number"
            min="1"
            max="100"
            required
        /></label>
        <label
          >{{ t("离线等待 (秒)")
          }}<input
            v-model.number="settings.offlineSeconds"
            type="number"
            min="45"
            max="86400"
            required
        /></label>
        <label
          >{{ t("重复告警间隔 (秒)")
          }}<input
            v-model.number="settings.cooldownSeconds"
            type="number"
            min="60"
            max="86400"
            required
        /></label>
      </div>
      <p class="hint">
        {{
          configured
            ? t("Webhook 已配置")
            : t("仅面板记录；在管理端配置文件添加 webhookURL 可发送通知。")
        }}
      </p>
      <p class="hint">
        {{ t("支持公开 HTTPS Webhook；私网地址和重定向会被拒绝。") }}
      </p>
      <button class="primary" :disabled="busy">{{ t("保存告警设置") }}</button>
      <p v-if="saved" class="success">{{ t("告警设置已保存") }}</p>
    </form>
    <p v-if="error" class="error">{{ systemText(error) }}</p>
    <div class="card table-wrap">
      <h3>{{ t("最近告警") }}</h3>
      <p class="hint">{{ t("保留最近 100 条，恢复时也会记录并通知。") }}</p>
      <table>
        <thead>
          <tr>
            <th>{{ t("节点") }}</th>
            <th>{{ t("类型") }}</th>
            <th>{{ t("文件路径") }}</th>
            <th>{{ t("状态") }}</th>
            <th>{{ t("时间") }}</th>
            <th>{{ t("通知状态") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in events" :key="item.id">
            <td>{{ item.name }}</td>
            <td>
              {{
                item.kind === "offline"
                  ? t("离线")
                  : item.kind === "inode"
                    ? "inode"
                    : t("磁盘分析")
              }}
              <span v-if="item.kind !== 'offline'"
                >{{ item.percent.toFixed(1) }}%</span
              >
            </td>
            <td>
              <code>{{ item.path || "—" }}</code>
            </td>
            <td>{{ item.resolved ? t("已恢复") : t("告警中") }}</td>
            <td>{{ date(item.at) }}</td>
            <td>
              {{
                item.delivery === "sent"
                  ? t("通知已发送")
                  : item.delivery === "failed"
                    ? t("通知失败")
                    : t("未配置通知")
              }}
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!events.length">{{ t("暂无告警") }}</p>
    </div>
  </section>
</template>
