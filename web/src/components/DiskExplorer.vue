<script setup lang="ts">
import { t, systemText } from "../i18n";
import { ref, computed } from "vue";
import { task, size, date, type Node, type Scan, type Entry } from "../api";
import Treemap from "./Treemap.vue";
const props = defineProps<{ node: Node }>();
const path = ref(props.node.scanRoots?.[0] || "/");
const scan = ref<Scan>();
const trail = ref<Entry[]>([]);
const busy = ref(false);
const error = ref("");
const current = computed(() => trail.value.at(-1));
async function run() {
  busy.value = true;
  error.value = "";
  try {
    scan.value = await task<Scan>(props.node.id, {
      kind: "scan",
      path: path.value,
    });
    trail.value = [scan.value.tree];
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function open(e: Entry) {
  if (e.directory) trail.value.push(e);
}
</script>
<template>
  <section>
    <div class="section-heading">
      <div>
        <h2>{{ t("磁盘空间透视") }}</h2>
        <p>{{ t("从最大的方块开始，逐层找到空间去向。") }}</p>
      </div>
      <span class="badge">{{ t("按需扫描 · 实际分配空间") }}</span>
    </div>
    <div class="card">
      <form class="toolbar" @submit.prevent="run">
        <label class="grow"
          >{{ t("扫描目录")
          }}<input v-model="path" placeholder="/var/log" required /></label
        ><button class="primary" :disabled="busy">
          {{ busy ? t("扫描中…") : t("扫描目录") }}
        </button>
      </form>
      <p class="hint">
        {{
          t("扫描白名单：{paths}。不跨挂载点；最多扫描 100,000 个条目。", {
            paths: node.scanRoots?.join(", ") || "—",
          })
        }}
      </p>
      <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
    </div>
    <div v-if="current && scan" class="card">
      <div class="toolbar spread">
        <nav class="breadcrumbs">
          <button
            v-for="(part, index) in trail"
            :key="part.path"
            @click="trail = trail.slice(0, index + 1)"
          >
            {{ index ? " / " : "" }}{{ part.name }}
          </button>
        </nav>
        <strong>{{ size(current.bytes) }}</strong>
      </div>
      <Treemap :entry="current" @open="open" />
      <p class="hint">
        {{
          t("{time} · {count} 个条目 · 跳过 {skipped} 项", {
            time: date(scan.at),
            count: scan.files,
            skipped: scan.skipped,
          })
        }}
        <strong v-if="scan.truncated"
          >· {{ t("已达到上限，结果不完整，请缩小扫描范围") }}</strong
        >
      </p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{{ t("名称") }}</th>
              <th>{{ t("类型") }}</th>
              <th>{{ t("磁盘占用") }}</th>
              <th>{{ t("占比") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in current.children" :key="item.path">
              <td>
                <button
                  class="text-button"
                  :disabled="!item.directory"
                  @click="open(item)"
                >
                  {{ item.directory ? "▣" : "▤" }} {{ item.name }}
                </button>
              </td>
              <td>{{ item.directory ? t("目录") : t("文件") }}</td>
              <td>{{ size(item.bytes) }}</td>
              <td>
                <meter :value="item.bytes" :max="current.bytes || 1" />
                {{ ((100 * item.bytes) / (current.bytes || 1)).toFixed(1) }}%
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div v-else class="card empty">
      <span class="empty-icon">▦</span>
      <h3>{{ t("让空间占用一目了然") }}</h3>
      <p>{{ t("选择目录开始扫描。结果会显示在可点击的矩形树图中。") }}</p>
    </div>
  </section>
</template>
