<script setup lang="ts">
import { canAdmin, canOperate } from "../access";
import { computed, onUnmounted, ref, watch } from "vue";
import {
  api,
  task,
  online,
  size,
  type Node,
  type Rule,
  type Scan,
  type Plan,
  type CleanupResult,
} from "../api";
import { t, systemText } from "../i18n";
import CleanupReport from "./CleanupReport.vue";
import ReviewDetails from "./ReviewDetails.vue";
const props = defineProps<{ nodes: Node[] }>();
const emit = defineEmits<{ changed: []; select: [id: string] }>();
const group = ref("*");
const groups = computed(() =>
  [...new Set(props.nodes.map((n) => n.group || ""))].sort(),
);
const filtered = computed(() =>
  props.nodes.filter(
    (n) => group.value === "*" || (n.group || "") === group.value.slice(2),
  ),
);
const selected = ref<string[]>([]);
watch(group, () => {
  selected.value = [];
});
const selectedOnline = computed(() =>
  filtered.value.filter((n) => selected.value.includes(n.id) && online(n)),
);
const stopQueued = ref(false);
const rules = ref<Rule[]>([]);
const ruleID = ref("");
const path = ref("/var/log");
const busy = ref(false);
const error = ref("");
const editing = ref<string>();
const name = ref("");
const editGroup = ref("");
const saving = ref(false);
const controller = new AbortController();
onUnmounted(() => controller.abort());
interface Row {
  id: string;
  name: string;
  status: "pending" | "running" | "succeeded" | "failed" | "not_submitted";
  scan?: Scan;
  plan?: Plan;
  receivedAt?: number;
  confirmed?: boolean;
  consumed?: boolean;
  cleanup?: CleanupResult;
  error?: string;
}
const rows = ref<Row[]>([]);
const clock = ref(Date.now());
const ageTimer = setInterval(() => (clock.value = Date.now()), 1000);
onUnmounted(() => clearInterval(ageTimer));
function ready(row: Row) {
  return (
    !!row.plan?.id &&
    !row.consumed &&
    !!row.receivedAt &&
    clock.value - row.receivedAt < 300000 &&
    selectedOnline.value.some((n) => n.id === row.id)
  );
}
const approved = computed(() =>
  rows.value.filter((row) => ready(row) && row.confirmed),
);
watch(
  [group, ruleID, selected],
  () => {
    if (busy.value || !canOperate.value) return;
    for (const row of rows.value) {
      row.confirmed = false;
      row.consumed = true;
    }
  },
  { deep: true },
);

api<Rule[]>("rules", "GET", undefined, controller.signal)
  .then((v) => {
    rules.value = v;
    ruleID.value = v[0]?.id || "";
  })
  .catch((e) => {
    if (!controller.signal.aborted) error.value = e.message;
  });
function edit(n: Node) {
  editing.value = n.id;
  name.value = n.name;
  editGroup.value = n.group || "";
}
async function save() {
  saving.value = true;
  error.value = "";
  try {
    await api(
      "nodes/" + editing.value,
      "PATCH",
      { name: name.value, group: editGroup.value },
      controller.signal,
    );
    editing.value = undefined;
    emit("changed");
  } catch (e) {
    if (!controller.signal.aborted) error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
function selectVisible() {
  selected.value = filtered.value
    .filter(online)
    .slice(0, 20)
    .map((n) => n.id);
}
async function run(kind: "scan" | "preview") {
  if (busy.value) return;
  error.value = "";
  const targets = selectedOnline.value;
  if (!targets.length || targets.length > 20) {
    error.value = t("请选择 1–20 台在线节点");
    return;
  }
  const rule = rules.value.find((r) => r.id === ruleID.value);
  if (kind === "preview" && !rule) return;
  const request =
    kind === "scan"
      ? { kind, path: path.value }
      : { kind, rule: JSON.parse(JSON.stringify(rule)) };
  rows.value = targets.map((n) => ({
    id: n.id,
    name: n.name,
    status: "pending",
  }));
  busy.value = true;
  stopQueued.value = false;
  let next = 0;
  async function worker() {
    while (
      !controller.signal.aborted &&
      !stopQueued.value &&
      next < rows.value.length
    ) {
      const row = rows.value[next++];
      row.status = "running";
      try {
        const result = await task<Scan | Plan>(
          row.id,
          request,
          controller.signal,
        );
        if (controller.signal.aborted) return;
        if (kind === "scan") row.scan = result as Scan;
        else {
          row.plan = result as Plan;
          row.receivedAt = Date.now();
          row.confirmed = false;
        }
        row.status = "succeeded";
      } catch (e) {
        if (controller.signal.aborted) return;
        row.status = "failed";
        row.error = (e as Error).message;
      }
    }
  }
  try {
    await Promise.all([worker(), worker()]);
  } finally {
    for (const row of rows.value) {
      if (row.status === "pending") row.status = "not_submitted";
    }
    busy.value = false;
  }
}

function exportPreview(row: Row) {
  if (!row.plan) return;
  const data = {
    node: row.id,
    name: row.name,
    rule: row.plan.rule,
    created: row.plan.created,
    bytes: row.plan.bytes,
    files: row.plan.files,
  };
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(data, null, 2) + "\n"], {
      type: "application/json",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = "nodesweep-preview-" + row.id + ".json";
  link.click();
  URL.revokeObjectURL(url);
}
async function executeApproved() {
  if (busy.value || !canOperate.value) return;
  const targets = approved.value.slice();
  if (!targets.length || targets.length > 20) return;
  // Snapshot each node's own plan before dispatch. Every approval is consumed
  // immediately, including unsubmitted rows, so any retry requires a new preview.
  const jobs = targets.map((row) => ({ row, planID: row.plan!.id }));
  for (const row of rows.value) {
    row.confirmed = false;
    row.consumed = true;
  }
  busy.value = true;
  stopQueued.value = false;
  error.value = "";
  for (const { row } of jobs) {
    row.status = "pending";
    row.error = undefined;
  }
  let next = 0;
  async function worker() {
    while (
      !controller.signal.aborted &&
      !stopQueued.value &&
      next < jobs.length
    ) {
      const { row, planID } = jobs[next++];
      // Recheck online/visible membership just before admission. Engine-side
      // plan age, allowlist and file identity remain authoritative.
      if (
        !selectedOnline.value.some((n) => n.id === row.id) ||
        Date.now() - (row.receivedAt || 0) >= 300000
      ) {
        row.status = "not_submitted";
        continue;
      }
      row.status = "running";
      try {
        const result = await task<CleanupResult>(
          row.id,
          { kind: "execute", planId: planID },
          controller.signal,
        );
        if (controller.signal.aborted) return;
        row.cleanup = result;
        row.status = "succeeded";
      } catch (e) {
        if (controller.signal.aborted) return;
        row.error = (e as Error).message;
        row.status = "failed";
      }
    }
  }
  try {
    await Promise.all([worker(), worker()]);
  } finally {
    for (const { row } of jobs) {
      if (row.status === "pending") row.status = "not_submitted";
    }
    busy.value = false;
  }
}
</script>
<template>
  <section class="batch-operations">
    <p class="hint">
      {{
        t(
          "每批最多 20 台，同时处理 2 台。扫描和预览不删除文件；离开页面后已提交任务继续执行，可在任务记录查看。",
        )
      }}
    </p>
    <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
    <div class="toolbar">
      <span class="hint">{{
        t("已选择 {count} 台在线节点", { count: selectedOnline.length })
      }}</span>
      <button v-if="busy" :disabled="stopQueued" @click="stopQueued = true">
        {{ t("停止后续提交") }}
      </button>
      <span v-if="busy && stopQueued" class="hint">{{
        t("等待已提交任务完成，剩余节点不会提交。")
      }}</span>
    </div>
    <div class="card">
      <div class="toolbar">
        <label
          >{{ t("节点分组")
          }}<select v-model="group" :disabled="busy">
            <option value="*">{{ t("全部节点") }}</option>
            <option v-for="g in groups" :key="g" :value="'g:' + g">
              {{ g || t("未分组") }}
            </option>
          </select></label
        >
        <button :disabled="busy" @click="selectVisible">
          {{ t("选择本组在线节点") }}
        </button>
      </div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{{ t("选择") }}</th>
              <th>{{ t("节点名称") }}</th>
              <th>{{ t("节点分组") }}</th>
              <th>{{ t("节点状态与空间") }}</th>
              <th>{{ t("操作") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="n in filtered" :key="n.id">
              <td>
                <input
                  v-model="selected"
                  type="checkbox"
                  :value="n.id"
                  :disabled="
                    busy ||
                    !online(n) ||
                    (selected.length >= 20 && !selected.includes(n.id))
                  "
                  :aria-label="n.name"
                />
              </td>
              <td>{{ n.name }}</td>
              <td>{{ n.group || t("未分组") }}</td>
              <td>{{ online(n) ? t("在线") : t("离线") }}</td>
              <td>
                <button :disabled="busy || !canAdmin" @click="edit(n)">
                  {{ t("编辑节点") }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="card">
      <div class="toolbar">
        <label
          >{{ t("扫描路径")
          }}<input v-model="path" :disabled="busy" maxlength="4096" /></label
        ><button
          class="primary"
          :disabled="
            busy ||
            !canOperate ||
            !selectedOnline.length ||
            !path.startsWith('/')
          "
          @click="run('scan')"
        >
          {{ t("批量扫描") }}
        </button>
      </div>
      <div class="toolbar">
        <label
          >{{ t("清理规则")
          }}<select v-model="ruleID" :disabled="busy">
            <option v-for="r in rules" :key="r.id" :value="r.id">
              {{ r.name }} · {{ r.root }}
            </option>
          </select></label
        ><button
          :disabled="busy || !canOperate || !selectedOnline.length || !ruleID"
          @click="run('preview')"
        >
          {{ t("批量预览") }}
        </button>
      </div>
      <p class="hint">
        {{
          t(
            "所有节点使用相同规则，但预览和授权独立。逐台核对清单并确认后才可批量清理。",
          )
        }}
      </p>
    </div>
    <div v-if="rows.some((row) => row.plan)" class="card batch-confirm">
      <p class="hint">
        {{
          t(
            "预览仅供本次确认，五分钟后需重新预览；失败或停止后不得复用。已提交的清理不能取消。",
          )
        }}
      </p>
      <button
        class="danger"
        :disabled="busy || !canOperate || !approved.length"
        @click="executeApproved"
      >
        {{ t("清理已确认的 {count} 台节点", { count: approved.length }) }}
      </button>
    </div>
    <article v-for="row in rows" :key="row.id" class="card">
      <div class="toolbar spread">
        <h3>{{ row.name }}</h3>
        <span>{{
          row.status === "not_submitted" ? t("未提交") : systemText(row.status)
        }}</span
        ><button @click="emit('select', row.id)">{{ t("磁盘分析") }}</button>
      </div>
      <p v-if="row.error" class="error">{{ systemText(row.error) }}</p>
      <p v-if="row.scan">
        {{ row.scan.tree.path }} · {{ size(row.scan.tree.bytes) }} ·
        {{ t("文件数量") }}: {{ row.scan.files }} · {{ t("跳过条目") }}:
        {{ row.scan.skipped }}
        <span v-if="row.scan.truncated" class="error">{{
          t("扫描达到上限，结果不完整")
        }}</span>
      </p>
      <div v-if="row.plan">
        <p>
          {{ row.plan.rule.root }} · {{ t("符合清理条件") }}:
          {{ row.plan.files.length }} · {{ size(row.plan.bytes) }}
        </p>
        <label class="check" v-if="!row.consumed"
          ><input
            v-model="row.confirmed"
            type="checkbox"
            :disabled="
              busy || !canOperate || !ready(row) || !row.plan.files.length
            "
          />{{
            t("我已核对 {name} 的清单，确认永久删除", { name: row.name })
          }}</label
        >
        <p v-if="!ready(row) && !busy" class="hint">
          {{ t("此预览不可执行，请重新预览。") }}
        </p>
        <details>
          <summary>{{ t("查看清理清单") }}</summary>
          <p class="hint">{{ t("页面显示前 100 项；可导出完整清单核对。") }}</p>
          <button @click="exportPreview(row)">{{ t("导出完整清单") }}</button>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{{ t("文件路径") }}</th>
                  <th>{{ t("大小") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="file in row.plan.files.slice(0, 100)"
                  :key="file.path"
                >
                  <td>
                    <code>{{ file.path }}</code>
                  </td>
                  <td>{{ size(file.size) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </details>
        <ReviewDetails v-if="row.plan.review" :review="row.plan.review" />
      </div>
      <CleanupReport v-if="row.cleanup" :result="row.cleanup" />
    </article>
    <div v-if="editing" class="modal-backdrop">
      <form
        v-dialog="
          () => {
            if (!saving) editing = undefined;
          }
        "
        class="modal card"
        role="dialog"
        aria-modal="true"
        :aria-label="t('编辑节点')"
        @submit.prevent="save"
      >
        <h2>{{ t("编辑节点") }}</h2>
        <label
          >{{ t("节点名称")
          }}<input v-model="name" required maxlength="100" /></label
        ><label
          >{{ t("节点分组") }}<input v-model="editGroup" maxlength="64"
        /></label>
        <p v-if="error" class="error">{{ systemText(error) }}</p>
        <button class="primary" :disabled="saving">{{ t("保存") }}</button
        ><button type="button" :disabled="saving" @click="editing = undefined">
          {{ t("取消") }}
        </button>
      </form>
    </div>
  </section>
</template>
