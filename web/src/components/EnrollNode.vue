<script setup lang="ts">
import { ref, computed } from "vue";
import { api, type Node } from "../api";
const emit = defineEmits<{ close: []; added: [] }>();
const name = ref("");
const created = ref<{ node: Node; token: string }>();
const error = ref("");
const busy = ref(false);
const config = computed(() =>
  JSON.stringify(
    {
      mode: "agent",
      hub: window.location.origin,
      node: created.value?.node.id,
      token: created.value?.token,
      cleanupRoots: ["/var/log"],
      scanRoots: ["/"],
    },
    null,
    2,
  ),
);
async function create() {
  busy.value = true;
  try {
    created.value = await api("nodes", "POST", { name: name.value });
    emit("added");
  } catch (e) {
    error.value = (e as Error).message;
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
      aria-label="添加节点"
    >
      <div class="toolbar spread">
        <h2>添加服务器</h2>
        <button @click="emit('close')" aria-label="关闭">×</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
      <form v-if="!created" @submit.prevent="create">
        <label
          >节点名称<input
            v-model="name"
            required
            maxlength="100"
            placeholder="香港 VPS"
            autofocus
        /></label>
        <p class="hint">每台服务器使用独立凭证，可随时撤销。</p>
        <button class="primary" :disabled="busy">生成节点配置</button>
      </form>
      <div v-else>
        <p>将配置保存到 VPS，设置文件权限为 600，再启动 Agent。</p>
        <pre>{{ config }}</pre>
        <button @click="download">下载配置</button>
        <pre>
chmod 600 nodesweep-agent.json
./nodesweep -config nodesweep-agent.json</pre>
        <p class="hint">
          凭证只在这里显示一次。远程接入需要 HTTPS；若当前通过 localhost
          访问，请把 hub 改为服务器的 HTTPS 地址。
        </p>
        <button class="primary" @click="emit('close')">完成</button>
      </div>
    </section>
  </div>
</template>
