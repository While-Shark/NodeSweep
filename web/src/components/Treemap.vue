<script setup lang="ts">
import { t } from "../i18n";
import { computed } from "vue";
import { hierarchy, treemap, treemapSquarify } from "d3-hierarchy";
import type { Entry } from "../api";
import { size } from "../api";
const props = defineProps<{ entry: Entry }>();
const emit = defineEmits<{ open: [entry: Entry] }>();
const cells = computed(() => {
  const children = (props.entry.children || []).filter((x) => x.bytes > 0);
  const root = hierarchy<Entry>({
    ...props.entry,
    children: children.map((c) => ({ ...c, children: undefined })),
  })
    .sum((d) => (d.children ? 0 : d.bytes))
    .sort((a, b) => (b.value || 0) - (a.value || 0));
  return treemap<Entry>()
    .tile(treemapSquarify)
    .size([1000, 430])
    .paddingInner(4)(root)
    .leaves()
    .filter((x) => x.depth > 0);
});
const colors = [
  "#246b57",
  "#367e68",
  "#479981",
  "#65966f",
  "#779b50",
  "#a19655",
  "#658b94",
  "#487185",
];
</script>
<template>
  <div class="treemap" :aria-label="t('目录占用矩形树图')">
    <button
      v-for="(cell, index) in cells"
      :key="cell.data.path"
      class="tile"
      :style="{
        left: cell.x0 / 10 + '%',
        top: cell.y0 / 4.3 + '%',
        width: (cell.x1 - cell.x0) / 10 + '%',
        height: (cell.y1 - cell.y0) / 4.3 + '%',
        background: colors[index % colors.length],
      }"
      :title="cell.data.path + ' · ' + size(cell.data.bytes)"
      :aria-label="cell.data.path + ' · ' + size(cell.data.bytes)"
      @click="emit('open', cell.data)"
    >
      <span v-if="cell.x1 - cell.x0 > 75 && cell.y1 - cell.y0 > 35"
        >{{ cell.data.name }}<small>{{ size(cell.data.bytes) }}</small></span
      >
    </button>
    <p v-if="!cells.length" class="empty">
      {{ t("此目录没有已分配空间的文件") }}
    </p>
  </div>
</template>
