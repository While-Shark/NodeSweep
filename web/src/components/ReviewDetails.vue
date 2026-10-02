<script setup lang="ts">
import { t } from "../i18n";
import type { MessageKey } from "../i18n/messages";
import type { Review } from "../api";
defineProps<{ review: Review }>();
const reasons: Record<string, MessageKey> = {
  eligible: "符合清理条件",
  excluded: "命中排除规则",
  symlink: "符号链接",
  mount: "跨挂载点",
  not_regular: "非普通文件",
  hardlink: "多个硬链接",
  active_or_not_archive: "活跃日志或非归档文件",
  pattern: "未匹配文件名模式",
  retention: "未超过保留天数",
  open: "文件正在使用",
};
const label = (key: string) => (reasons[key] ? t(reasons[key]) : key);
</script>
<template>
  <details class="review-details">
    <summary>{{ t("规则判定说明") }}</summary>
    <p class="hint">
      {{ t("统计已访问条目；排除目录不展开。示例最多 40 项。") }}
    </p>
    <div class="toolbar">
      <span v-for="(count, reason) in review.counts" :key="reason" class="badge"
        >{{ label(String(reason)) }}: {{ count }}</span
      >
    </div>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>{{ t("文件路径") }}</th>
            <th>{{ t("判定原因") }}</th>
            <th>{{ t("匹配模式") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in review.examples.slice(0, 40)" :key="item.path">
            <td>
              <code>{{ item.path }}</code>
            </td>
            <td>{{ label(item.reason) }}</td>
            <td>{{ item.pattern || "—" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </details>
</template>
