<script setup lang="ts">
import { t, systemText } from "../i18n";
import { ref, computed, reactive, onUnmounted } from "vue";
import { agentConfig, settingsError } from "../agent-config";
import { api, type Node } from "../api";
const emit = defineEmits<{ close: []; added: [] }>();
const name = ref("");
const group = ref("");
const created = ref<{ node: Node; token: string }>();
const error = ref("");
const busy = ref(false);
const controller = new AbortController();
onUnmounted(() => controller.abort());
const settings = reactive({
  hub: window.location.origin,
  cleanup: "/var/log",
  scan: "/",
  panels: "",
});
const generatedSettings = ref({ ...settings });
const validation = computed(() => settingsError(settings));
const config = computed(() =>
  created.value
    ? JSON.stringify(
        agentConfig(
          generatedSettings.value,
          created.value.node.id,
          created.value.token,
        ),
        null,
        2,
      )
    : "",
);
async function create() {
  if (busy.value || validation.value) return;
  generatedSettings.value = { ...settings };
  busy.value = true;
  error.value = "";
  try {
    created.value = await api(
      "nodes",
      "POST",
      {
        name: name.value,
        group: group.value,
      },
      controller.signal,
    );
    emit("added");
  } catch (e) {
    if (!controller.signal.aborted) error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function download() {
  const url = URL.createObjectURL(
    new Blob([config.value], { type: "application/json" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = "nodesweep-agent.json";
  a.click();
  URL.revokeObjectURL(url);
}
</script>
<template>
  <div class="modal-backdrop">
    <section
      class="modal card"
      role="dialog"
      aria-modal="true"
      :aria-label="t('添加节点')"
    >
      <div class="toolbar spread">
        <h2>{{ t("添加服务器") }}</h2>
        <button @click="emit('close')" :aria-label="t('关闭')">×</button>
      </div>
      <p v-if="error" class="error">{{ systemText(error) }}</p>
      <form v-if="!created" @submit.prevent="create">
        <label
          >{{ t("节点名称")
          }}<input
            v-model="name"
            required
            maxlength="100"
            :placeholder="t('香港 VPS')"
            autofocus
        /></label>
        <label
          >{{ t("节点分组") }}<input v-model="group" maxlength="64"
        /></label>
        <label
          >{{ t("管理端 HTTPS 地址")
          }}<input v-model="settings.hub" required maxlength="2048"
        /></label>
        <label
          >{{ t("允许清理的目录")
          }}<textarea
            v-model="settings.cleanup"
            required
            rows="3"
            maxlength="16384"
          />
        </label>
        <label
          >{{ t("允许扫描的目录")
          }}<textarea
            v-model="settings.scan"
            required
            rows="2"
            maxlength="16384"
          />
        </label>
        <label
          >{{ t("自定义面板安装目录（可选）")
          }}<textarea v-model="settings.panels" rows="2" maxlength="16384" />
        </label>
        <p class="hint">
          {{
            t(
              "每行一个目录。面板安装目录仅用于识别，不会自动扩大清理权限；请只允许日志目录。",
            )
          }}
        </p>
        <p class="hint">
          {{
            t(
              "宝塔填写 panel 目录（例如 /www/server/panel）；1Panel 安装位置从 1pctl 静态配置识别。",
            )
          }}
        </p>
        <p v-if="validation" class="error" role="alert">{{ t(validation) }}</p>
        <p class="hint">{{ t("每台服务器使用独立凭证，可随时撤销。") }}</p>
        <button class="primary" :disabled="busy || !!validation">
          {{ t("生成节点配置") }}
        </button>
      </form>
      <div v-else>
        <p>{{ t("将配置保存到 VPS，设置文件权限为 600，再启动 Agent。") }}</p>
        <pre>{{ config }}</pre>
        <button @click="download">{{ t("下载配置") }}</button>
        <pre>
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json -check
./nodesweep -config nodesweep-agent.json</pre>
        <p class="hint">
          {{
            t(
              "凭证只在这里显示一次。远程接入需要 HTTPS；若当前通过 localhost 访问，请把 hub 改为服务器的 HTTPS 地址。",
            )
          }}
        </p>
        <button class="primary" @click="emit('close')">{{ t("完成") }}</button>
      </div>
    </section>
  </div>
</template>
