<script setup lang="ts">
import { computed, onUnmounted, ref } from "vue";
import {
  api,
  task,
  online,
  size,
  type Node,
  type Rule,
  type Scan,
  type Plan,
} from "../api";
import { t, systemText } from "../i18n";
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
  status: "pending" | "running" | "succeeded" | "failed";
  scan?: Scan;
  plan?: Plan;
  error?: string;
}
const rows = ref<Row[]>([]);
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
  error.value = "";
  const targets = props.nodes.filter(
    (n) => selected.value.includes(n.id) && online(n),
  );
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
  let next = 0;
  async function worker() {
    while (!controller.signal.aborted && next < rows.value.length) {
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
        else row.plan = result as Plan;
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
                  :disabled="busy || !online(n)"
                  :aria-label="n.name"
                />
              </td>
              <td>{{ n.name }}</td>
              <td>{{ n.group || t("未分组") }}</td>
              <td>{{ online(n) ? t("在线") : t("离线") }}</td>
              <td>
                <button :disabled="busy" @click="edit(n)">
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
          :disabled="busy || !selected.length || !path.startsWith('/')"
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
          :disabled="busy || !selected.length || !ruleID"
          @click="run('preview')"
        >
          {{ t("批量预览") }}
        </button>
      </div>
      <p class="hint">
        {{
          t(
            "所有节点使用相同路径或规则，各节点仍按本地允许目录校验。清理请进入单节点页面重新预览并确认。",
          )
        }}
      </p>
    </div>
    <article v-for="row in rows" :key="row.id" class="card">
      <div class="toolbar spread">
        <h3>{{ row.name }}</h3>
        <span>{{ systemText(row.status) }}</span
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
        <ReviewDetails v-if="row.plan.review" :review="row.plan.review" />
      </div>
    </article>
    <div v-if="editing" class="modal-backdrop">
      <form
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
