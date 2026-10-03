<script setup lang="ts">
import MetricsHistory from "./MetricsHistory.vue";
import { t, number } from "../i18n";
import { computed, ref } from "vue";
import { size, online, type Node } from "../api";
const props = defineProps<{ nodes: Node[] }>();
const emit = defineEmits<{ select: [id: string] }>();
const group = ref("*");
const groups = computed(() =>
  [...new Set(props.nodes.map((n) => n.group || ""))].sort(),
);
const visible = computed(() =>
  props.nodes.filter(
    (n) => group.value === "*" || (n.group || "") === group.value.slice(2),
  ),
);
const active = computed(() => props.nodes.filter(online).length);
const total = computed(() =>
  props.nodes.reduce(
    (s, n) => s + (n.metrics.disks || []).reduce((v, d) => v + d.total, 0),
    0,
  ),
);
const free = computed(() =>
  props.nodes.reduce(
    (s, n) => s + (n.metrics.disks || []).reduce((v, d) => v + d.available, 0),
    0,
  ),
);
</script>
<template>
  <div>
    <div class="hero">
      <div>
        <span class="eyebrow">{{ t("节点状态与空间") }}</span>
        <h2>{{ t("服务器空间，心中有数。") }}</h2>
        <p>{{ t("查看节点状态，定位占用，按规则释放空间。") }}</p>
      </div>
      <span class="hero-mark">▦</span>
    </div>
    <div class="stats">
      <div class="card stat">
        <span>{{ t("在线节点") }}</span
        ><strong
          >{{ number(active) }}
          <small>/ {{ number(nodes.length) }}</small></strong
        >
      </div>
      <div class="card stat">
        <span>{{ t("磁盘总容量") }}</span
        ><strong>{{ size(total) }}</strong>
      </div>
      <div class="card stat">
        <span>{{ t("可用空间") }}</span
        ><strong>{{ size(free) }}</strong>
      </div>
    </div>
    <div class="section-heading">
      <h2>{{ t("全部节点") }}</h2>
      <label
        >{{ t("节点分组")
        }}<select v-model="group">
          <option value="*">{{ t("全部节点") }}</option>
          <option v-for="g in groups" :key="g" :value="'g:' + g">
            {{ g || t("未分组") }}
          </option>
        </select></label
      >
      <span class="hint">{{ t("每 5 秒更新 · 离线指标保留最后一次值") }}</span>
    </div>
    <div class="node-grid">
      <button
        v-for="n in visible"
        :key="n.id"
        class="card node-card"
        @click="emit('select', n.id)"
      >
        <div class="toolbar spread">
          <h3>
            {{ n.name }}<small v-if="n.group"> · {{ n.group }}</small>
          </h3>
          <span :class="['status', online(n) ? 'up' : 'down']">{{
            online(n) ? t("在线") : t("离线")
          }}</span>
        </div>
        <p class="hint">{{ n.metrics.host || t("等待 Agent 接入") }}</p>
        <p v-if="n.metrics.partial" class="hint">
          {{ t("指标采样不完整；部分指标或挂载点缺失。") }}
        </p>
        <div class="node-metrics">
          <span
            >CPU<strong>{{
              n.metrics.cpuAvailable === false
                ? "—"
                : (n.metrics.cpu?.toFixed(1) || "0") + "%"
            }}</strong></span
          ><span
            >{{ t("内存")
            }}<strong
              >{{
                n.metrics.memoryTotal
                  ? (
                      100 *
                      (1 - n.metrics.memoryAvailable / n.metrics.memoryTotal)
                    ).toFixed(0)
                  : "—"
              }}%</strong
            ></span
          ><span
            >{{ t("负载") }}<strong>{{ n.metrics.load || "—" }}</strong></span
          >
        </div>
        <div v-for="d in n.metrics.disks" :key="d.path" class="disk-line">
          <div class="toolbar spread">
            <span>{{ d.path }}</span
            ><span
              >{{ size(d.total - d.available) }} / {{ size(d.total) }}</span
            >
          </div>
          <progress :value="d.total - d.available" :max="d.total" /><small>{{
            t("可用 {space} · inode 已用 {percent}%", {
              space: size(d.available),
              percent: d.inodes
                ? (100 * (1 - d.freeInodes / d.inodes)).toFixed(1)
                : "0",
            })
          }}</small>
        </div>
      </button>
    </div>
    <MetricsHistory :nodes="visible" />
    <div v-if="!nodes.length" class="card empty">
      <h3>{{ t("接入你的第一台服务器") }}</h3>
      <p>{{ t("点击右上角“添加节点”，生成这台 VPS 的独立配置。") }}</p>
    </div>
  </div>
</template>
