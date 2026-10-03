<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { canAdmin } from "../access";
import {
  api,
  task,
  online,
  date,
  size,
  type Node,
  type Rule,
  type Plan,
} from "../api";
import { t, systemText } from "../i18n";
import ReviewDetails from "./ReviewDetails.vue";
interface Schedule {
  id: string;
  node: string;
  rule: Rule;
  hours: number;
  enabled: boolean;
  next: string;
  phase?: string;
  task?: string;
  outcome?: string;
}
const props = defineProps<{ nodes: Node[] }>();
const schedules = ref<Schedule[]>([]),
  rules = ref<Rule[]>([]);
const node = ref(""),
  rule = ref(""),
  hours = ref(24),
  error = ref(""),
  busy = ref(false),
  confirmed = ref(false),
  now = ref(Date.now());
const review = ref<{
  id: string;
  plan: Plan;
  task: string;
  received: number;
}>();
const controller = new AbortController();
let generation = 0,
  timer: ReturnType<typeof setInterval> | undefined;
const reviewReady = computed(
  () =>
    review.value &&
    now.value - review.value.received < 5 * 60000 &&
    props.nodes.some(
      (n) =>
        n.id === schedules.value.find((s) => s.id === review.value?.id)?.node &&
        online(n),
    ),
);
async function load() {
  const current = ++generation;
  try {
    const [list, saved] = await Promise.all([
      api<Schedule[]>("schedules", "GET", undefined, controller.signal),
      api<Rule[]>("rules", "GET", undefined, controller.signal),
    ]);
    if (current !== generation) return;
    schedules.value = list;
    rules.value = saved;
    if (review.value && !list.some((s) => s.id === review.value?.id))
      clearReview();
  } catch (e) {
    if (!controller.signal.aborted && current === generation)
      error.value = (e as Error).message;
  }
}
function clearReview() {
  review.value = undefined;
  confirmed.value = false;
}
async function action(run: () => Promise<unknown>) {
  if (busy.value || !canAdmin.value) return;
  busy.value = true;
  error.value = "";
  try {
    await run();
    if (!controller.signal.aborted) await load();
  } catch (e) {
    if (!controller.signal.aborted) error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function create() {
  clearReview();
  await action(() =>
    api(
      "schedules",
      "POST",
      { node: node.value, ruleId: rule.value, hours: hours.value },
      controller.signal,
    ),
  );
}
async function preview(schedule: Schedule) {
  clearReview();
  await action(async () => {
    let id = "";
    const plan = await task<Plan>(
      schedule.node,
      { kind: "preview", rule: schedule.rule },
      controller.signal,
      (job) => {
        id = job.id;
      },
    );
    if (!controller.signal.aborted)
      review.value = { id: schedule.id, plan, task: id, received: Date.now() };
  });
}
async function enable() {
  if (!reviewReady.value || !confirmed.value || !review.value || busy.value)
    return;
  const selected = review.value;
  clearReview();
  await action(() =>
    api(
      "schedules/" + selected.id,
      "PATCH",
      { enabled: true, confirm: true, previewTask: selected.task },
      controller.signal,
    ),
  );
}
async function pause(schedule: Schedule) {
  clearReview();
  await action(() =>
    api(
      "schedules/" + schedule.id,
      "PATCH",
      { enabled: false },
      controller.signal,
    ),
  );
}
async function remove(schedule: Schedule) {
  clearReview();
  await action(() =>
    api("schedules/" + schedule.id, "DELETE", undefined, controller.signal),
  );
}
watch(
  () => props.nodes.map((n) => n.id).join(","),
  () => {
    if (review.value && !reviewReady.value) clearReview();
  },
);
onMounted(() => {
  void load();
  timer = setInterval(() => {
    now.value = Date.now();
  }, 1000);
});
onUnmounted(() => {
  ++generation;
  controller.abort();
  clearInterval(timer);
});
const outcomes: Record<string, Parameters<typeof t>[0]> = {
  paused: "已暂停",
  enabled: "已启用",
  running: "执行中",
  succeeded: "已完成",
  empty: "没有符合条件的文件",
  offline: "节点离线，已跳过本轮",
  busy: "节点忙，已跳过本轮",
  failed: "失败，需重新核对",
  review_required: "需重新核对",
  restart_review_required: "重启后需重新核对",
  revoked: "节点已撤销",
};
</script>
<template>
  <section class="card schedules-panel">
    <h2>{{ t("自动清理计划") }}</h2>
    <p class="hint">
      {{
        t(
          "默认关闭。保存规则快照，修改原规则不会改变计划；每轮重新预览。重启或失败后暂停，错过的运行不补做。",
        )
      }}
    </p>
    <p class="hint">
      {{ t("暂停或删除计划只阻止后续提交，已提交的清理继续执行。") }}
    </p>
    <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
    <form class="toolbar" @submit.prevent="create">
      <label
        >{{ t("当前节点")
        }}<select v-model="node" required :disabled="busy || !canAdmin">
          <option value="" disabled>{{ t("选择节点") }}</option>
          <option v-for="n in nodes" :key="n.id" :value="n.id">
            {{ n.name }}
          </option>
        </select></label
      >
      <label
        >{{ t("清理规则")
        }}<select v-model="rule" required :disabled="busy || !canAdmin">
          <option value="" disabled>{{ t("选择规则") }}</option>
          <option v-for="r in rules" :key="r.id" :value="r.id">
            {{ r.name }} · {{ r.root }}
          </option>
        </select></label
      >
      <label
        >{{ t("间隔（小时）")
        }}<input
          v-model.number="hours"
          type="number"
          min="1"
          max="720"
          required
          :disabled="busy || !canAdmin"
      /></label>
      <button class="primary" :disabled="busy || !node || !rule || !canAdmin">
        {{ t("创建已暂停计划") }}
      </button>
      <button type="button" :disabled="busy" @click="load">
        {{ t("刷新") }}
      </button>
    </form>
  </section>
  <section v-for="s in schedules" :key="s.id" class="card schedules-panel">
    <h3>
      {{ nodes.find((n) => n.id === s.node)?.name || s.node }} ·
      {{ s.rule.name }}
    </h3>
    <p>
      <code>{{ s.rule.root }}</code> ·
      {{ t("保留 {days} 天", { days: s.rule.keepDays }) }} ·
      {{ t("每 {hours} 小时", { hours: s.hours }) }}
    </p>
    <p>
      <code>{{ s.rule.patterns.join(", ") }}</code> / {{ t("排除") }}:
      <code>{{ s.rule.excludes?.join(", ") || "—" }}</code>
    </p>
    <p>
      {{ t(s.enabled ? "已启用" : "已暂停") }} ·
      {{ s.outcome && outcomes[s.outcome] ? t(outcomes[s.outcome]) : "—" }} ·
      {{ t("下次运行") }}: {{ s.enabled ? date(s.next) : "—" }}
    </p>
    <p v-if="s.task">
      {{ t("任务编号") }}: <code>{{ s.task }}</code>
    </p>
    <div class="toolbar">
      <button
        :disabled="
          busy ||
          !canAdmin ||
          s.enabled ||
          !!s.phase ||
          !nodes.some((n) => n.id === s.node && online(n))
        "
        @click="preview(s)"
      >
        {{ t("核对后启用") }}</button
      ><button :disabled="busy || !canAdmin || !s.enabled" @click="pause(s)">
        {{ t("暂停计划") }}</button
      ><button :disabled="busy || !canAdmin" @click="remove(s)">
        {{ t("删除计划") }}
      </button>
    </div>
    <div v-if="review?.id === s.id">
      <p>
        {{
          t("预览 {count} 个文件，共 {bytes}", {
            count: review.plan.files.length,
            bytes: size(review.plan.bytes),
          })
        }}
      </p>
      <ReviewDetails v-if="review.plan.review" :review="review.plan.review" />
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{{ t("文件路径") }}</th>
              <th>{{ t("大小") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in review.plan.files.slice(0, 100)" :key="f.path">
              <td>
                <code>{{ f.path }}</code>
              </td>
              <td>{{ size(f.size) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="hint">
        {{
          t("列表显示前 100 项；计划会自动永久删除未来符合此规则的归档日志。")
        }}
      </p>
      <label
        ><input
          v-model="confirmed"
          type="checkbox"
          :disabled="busy || !reviewReady"
        />{{
          t("我已核对节点与规则，允许后续自动永久删除符合条件的文件。")
        }}</label
      >
      <button
        class="danger"
        :disabled="busy || !confirmed || !reviewReady"
        @click="enable"
      >
        {{ t("确认启用自动清理") }}
      </button>
    </div>
  </section>
</template>
