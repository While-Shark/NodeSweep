<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { api, date, type Node } from "../api";
import { t, number, systemText } from "../i18n";
interface Point {
  at: string;
  samples: number;
  cpu: number | null;
  memory: number | null;
  disks: { path: string; used: number | null; inodes: number | null }[];
}
const props = defineProps<{ nodes: Node[] }>();
const node = ref("");
const period = ref("24h");
const mount = ref("");
const points = ref<Point[]>([]);
const busy = ref(false),
  error = ref("");
const end = ref(Date.now());
let request: AbortController | undefined;
let sequence = 0;
const mounts = computed(() =>
  [...new Set(points.value.flatMap((p) => p.disks.map((d) => d.path)))].sort(),
);
watch(mounts, (paths) => {
  if (!paths.includes(mount.value)) mount.value = paths[0] || "";
});
watch(
  () => props.nodes.map((n) => n.id).join(","),
  () => {
    if (!props.nodes.some((n) => n.id === node.value))
      node.value = props.nodes[0]?.id || "";
  },
  { immediate: true },
);
async function load() {
  request?.abort();
  request = new AbortController();
  const signal = request.signal,
    current = ++sequence;
  points.value = [];
  error.value = "";
  busy.value = !!node.value;
  if (!node.value) return;
  end.value = Date.now();
  try {
    const value = await api<Point[]>(
      `metrics/${encodeURIComponent(node.value)}?period=${period.value}`,
      "GET",
      undefined,
      signal,
    );
    if (current === sequence) points.value = value;
  } catch (e) {
    if (!signal.aborted && current === sequence)
      error.value = (e as Error).message;
  } finally {
    if (current === sequence) busy.value = false;
  }
}
watch([node, period], () => void load(), { immediate: true });
onUnmounted(() => {
  sequence++;
  request?.abort();
});
const duration = computed(() =>
  period.value === "7d" ? 7 * 86400000 : 86400000,
);
const start = computed(() => end.value - duration.value);
function value(p: Point, key: string): number | null {
  if (key === "cpu") return p.cpu;
  if (key === "memory") return p.memory;
  const d = p.disks.find((d) => d.path === mount.value);
  return (key === "disk" ? d?.used : d?.inodes) ?? null;
}
const charts = computed(() => [
  { key: "cpu", label: "CPU" },
  { key: "memory", label: t("内存") },
  { key: "disk", label: t("磁盘已用") },
  { key: "inode", label: t("inode 已用") },
]);
function paths(key: string): string[] {
  const output: string[] = [];
  let path = "",
    previous = 0;
  const bucket = period.value === "7d" ? 3600000 : 900000;
  for (const p of points.value) {
    const v = value(p, key),
      at = new Date(p.at).getTime();
    if (
      v === null ||
      !Number.isFinite(v) ||
      (previous && at - previous > bucket)
    ) {
      if (path) output.push(path);
      path = "";
    }
    if (v !== null && Number.isFinite(v)) {
      const x =
        40 +
        Math.max(0, Math.min(1, (at - start.value) / duration.value)) * 520;
      const y = 130 - Math.max(0, Math.min(100, v));
      path += `${path ? " L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`;
      // A tiny segment makes a solitary sample visible without joining gaps.
      if (!path.includes(" L"))
        path += ` L${(x + 0.4).toFixed(1)},${y.toFixed(1)}`;
    }
    previous = at;
  }
  if (path) output.push(path);
  return output;
}
const formatted = (v: number | null) =>
  v === null ? "—" : number(Math.round(v * 10) / 10) + "%";
</script>
<template>
  <section class="card metric-history" aria-labelledby="metric-history-title">
    <div class="toolbar spread">
      <h2 id="metric-history-title">{{ t("历史指标") }}</h2>
      <button :disabled="busy || !node" @click="load">{{ t("刷新") }}</button>
    </div>
    <div class="toolbar history-controls">
      <label
        >{{ t("节点")
        }}<select v-model="node">
          <option v-for="n in nodes" :key="n.id" :value="n.id">
            {{ n.name }}
          </option>
        </select></label
      >
      <label
        >{{ t("时间范围")
        }}<select v-model="period">
          <option value="24h">{{ t("最近 24 小时") }}</option>
          <option value="7d">{{ t("最近 7 天") }}</option>
        </select></label
      >
      <label v-if="mounts.length"
        >{{ t("磁盘占用")
        }}<select v-model="mount">
          <option v-for="path in mounts" :key="path" :value="path">
            {{ path }}
          </option>
        </select></label
      >
    </div>
    <p class="hint">{{ t("每分钟采样 · 七天保留 · 空白表示缺失数据") }}</p>
    <p v-if="error" role="alert" class="error">{{ systemText(error) }}</p>
    <p v-else-if="busy" role="status">{{ t("加载中…") }}</p>
    <p v-else-if="!points.length">{{ t("暂无历史数据，请等待采样。") }}</p>
    <template v-else>
      <div class="history-charts">
        <figure v-for="chart in charts" :key="chart.key">
          <figcaption>
            {{ chart.label }}
            <small v-if="chart.key === 'disk' || chart.key === 'inode'">{{
              mount
            }}</small>
          </figcaption>
          <svg
            viewBox="0 0 600 160"
            role="img"
            :aria-label="chart.label + ' · ' + t('历史指标')"
          >
            <title>{{ chart.label }} · {{ t("历史指标") }}</title>
            <path d="M40,30 H560 M40,80 H560 M40,130 H560" class="chart-grid" />
            <text x="0" y="34">100%</text>
            <text x="8" y="84">50%</text>
            <text x="14" y="134">0%</text>
            <path
              v-for="(path, i) in paths(chart.key)"
              :key="i"
              :d="path"
              class="chart-line"
            />
          </svg>
        </figure>
      </div>
      <div class="toolbar spread hint">
        <span>{{ date(new Date(start).toISOString()) }}</span
        ><span>{{ date(new Date(end).toISOString()) }}</span>
      </div>
      <details>
        <summary>{{ t("查看指标数据") }}</summary>
        <div class="history-table">
          <table>
            <thead>
              <tr>
                <th>{{ t("时间") }}</th>
                <th>{{ t("样本数") }}</th>
                <th v-for="chart in charts" :key="chart.key">
                  {{ chart.label }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in points" :key="p.at">
                <td>{{ date(p.at) }}</td>
                <td>{{ number(p.samples) }}</td>
                <td v-for="chart in charts" :key="chart.key">
                  {{ formatted(value(p, chart.key)) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </details>
    </template>
  </section>
</template>
<style scoped>
.metric-history {
  margin-top: 24px;
}
.history-controls {
  flex-wrap: wrap;
}
.history-controls label {
  display: grid;
  gap: 6px;
  min-width: 140px;
}
.history-charts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
figure {
  margin: 12px 0;
  min-width: 0;
}
figcaption {
  font-weight: 600;
  overflow-wrap: anywhere;
}
svg {
  width: 100%;
  height: auto;
}
svg text {
  font-size: 11px;
  fill: currentColor;
}
.chart-grid {
  stroke: #dce5e2;
  fill: none;
}
.chart-line {
  stroke: #147b63;
  stroke-width: 2;
  fill: none;
}
.history-table {
  overflow-x: auto;
}
@media (max-width: 700px) {
  .history-charts {
    grid-template-columns: 1fr;
  }
}
</style>
