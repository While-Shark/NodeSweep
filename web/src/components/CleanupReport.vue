<script setup lang="ts">
import { t, systemText } from "../i18n";
import { size, date, type CleanupResult } from "../api";
defineProps<{ result: CleanupResult }>();
</script>
<template>
  <section class="card cleanup-report">
    <h3>
      {{ t("清理效果报告") }}
      <span v-if="result.ruleName">· {{ result.ruleName }}</span>
    </h3>
    <p v-if="result.root">
      <code>{{ result.root }}</code>
    </p>
    <div class="toolbar">
      <span>{{ t("已删除") }}: {{ result.deleted }}</span
      ><strong>{{ size(result.bytes) }}</strong
      ><span>{{ t("跳过或失败") }}: {{ result.skipped.length }}</span>
    </div>
    <p v-if="result.started && result.finished" class="hint">
      {{ date(result.started) }} → {{ date(result.finished) }}
    </p>
    <p class="hint">
      {{ t("显示已删除文件的分配空间；不等同于磁盘可用空间净增长。") }}
    </p>
    <div v-if="result.items?.length" class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>{{ t("文件路径") }}</th>
            <th>{{ t("状态") }}</th>
            <th>{{ t("分配空间") }}</th>
            <th>{{ t("判定原因") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in result.items.slice(0, 5000)" :key="item.path">
            <td>
              <code>{{ item.path }}</code>
            </td>
            <td>
              {{
                item.status === "deleted"
                  ? t("已删除")
                  : item.status === "failed"
                    ? t("失败")
                    : t("已跳过")
              }}
            </td>
            <td>{{ size(item.bytes) }}</td>
            <td>{{ item.reason ? systemText(item.reason) : "—" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <ul v-else>
      <li v-for="item in result.skipped" :key="item">{{ item }}</li>
    </ul>
  </section>
</template>
