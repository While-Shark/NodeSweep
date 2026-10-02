<script setup lang="ts">
import { t, systemText } from "../i18n";
import { ref } from "vue";
import { api } from "../api";
const props = defineProps<{
  disabled: boolean;
  hasRules: boolean;
  schemes: { value: string; label: string }[];
}>();
const emit = defineEmits<{ import: [bundle: unknown] }>();
const fileInput = ref<HTMLInputElement>();
const working = ref(false);
const selectedScheme = ref("");
const error = ref("");
async function exportRules() {
  working.value = true;
  error.value = "";
  try {
    const bundle = await api(
      "rules/export" +
        (selectedScheme.value
          ? "?scheme=" + encodeURIComponent(selectedScheme.value)
          : ""),
    );
    const blob = new Blob([JSON.stringify(bundle, null, 2) + "\n"], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "nodesweep-cleanup-rules.json";
    link.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    working.value = false;
  }
}
async function readFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file || props.disabled) return;
  working.value = true;
  error.value = "";
  try {
    if (file.size > 256 * 1024) throw new Error("方案文件不能超过 256 KB。");
    const text = await file.text();
    let bundle;
    try {
      bundle = JSON.parse(text);
    } catch {
      throw new Error("方案文件不是有效的 JSON。");
    }
    if (
      bundle?.format !== "nodesweep.rules" ||
      bundle?.version !== 1 ||
      !Array.isArray(bundle.rules) ||
      bundle.rules.length < 1 ||
      bundle.rules.length > 100
    )
      throw new Error("请选择 NodeSweep 导出的方案文件，支持 1–100 条规则。");
    emit("import", bundle);
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    working.value = false;
  }
}
</script>
<template>
  <div class="card rule-transfer">
    <div>
      <h3>{{ t("复用清理方案") }}</h3>
      <p class="hint">
        {{
          t("导入会添加新规则并跳过完全相同的规则。节点白名单保持本机控制。")
        }}
      </p>
    </div>
    <div class="toolbar">
      <label
        >{{ t("导出范围")
        }}<select v-model="selectedScheme" :disabled="disabled || working">
          <option value="">{{ t("全部方案") }}</option>
          <option
            v-for="scheme in schemes"
            :key="scheme.value"
            :value="scheme.value"
          >
            {{ scheme.label }}
          </option>
        </select></label
      >
      <button :disabled="disabled || working" @click="fileInput?.click()">
        {{ t("导入方案") }}
      </button>
      <button :disabled="disabled || working || !hasRules" @click="exportRules">
        {{ t("导出方案") }}
      </button>
      <input
        ref="fileInput"
        type="file"
        accept=".json,application/json"
        hidden
        @change="readFile"
      />
    </div>
    <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
  </div>
</template>
