<script setup lang="ts">
import { computed } from "vue";
import { size, online, type Node } from "../api";
const props = defineProps<{ nodes: Node[] }>();
const emit = defineEmits<{ select: [id: string] }>();
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
        <span class="eyebrow">YOUR FLEET, IN FOCUS</span>
        <h2>服务器空间，心中有数。</h2>
        <p>查看节点状态，定位占用，按规则释放空间。</p>
      </div>
      <span class="hero-mark">▦</span>
    </div>
    <div class="stats">
      <div class="card stat">
        <span>在线节点</span
        ><strong
          >{{ active }} <small>/ {{ nodes.length }}</small></strong
        >
      </div>
      <div class="card stat">
        <span>磁盘总容量</span><strong>{{ size(total) }}</strong>
      </div>
      <div class="card stat">
        <span>可用空间</span><strong>{{ size(free) }}</strong>
      </div>
    </div>
    <div class="section-heading">
      <h2>全部节点</h2>
      <span class="hint">每 5 秒更新 · 离线指标保留最后一次值</span>
    </div>
    <div class="node-grid">
      <button
        v-for="n in nodes"
        :key="n.id"
        class="card node-card"
        @click="emit('select', n.id)"
      >
        <div class="toolbar spread">
          <h3>{{ n.name }}</h3>
          <span :class="['status', online(n) ? 'up' : 'down']">{{
            online(n) ? "在线" : "离线"
          }}</span>
        </div>
        <p class="hint">{{ n.metrics.host || "等待 Agent 接入" }}</p>
        <div class="node-metrics">
          <span
            >CPU<strong>{{ n.metrics.cpu?.toFixed(1) || "0" }}%</strong></span
          ><span
            >内存<strong
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
            >负载<strong>{{ n.metrics.load || "—" }}</strong></span
          >
        </div>
        <div v-for="d in n.metrics.disks" :key="d.path" class="disk-line">
          <div class="toolbar spread">
            <span>{{ d.path }}</span
            ><span
              >{{ size(d.total - d.available) }} / {{ size(d.total) }}</span
            >
          </div>
          <progress :value="d.total - d.available" :max="d.total" /><small
            >可用 {{ size(d.available) }} · inode 已用
            {{
              d.inodes ? (100 * (1 - d.freeInodes / d.inodes)).toFixed(1) : "0"
            }}%</small
          >
        </div>
      </button>
    </div>
    <div v-if="!nodes.length" class="card empty">
      <h3>接入你的第一台服务器</h3>
      <p>点击右上角“添加节点”，生成这台 VPS 的独立配置。</p>
    </div>
  </div>
</template>
