<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { api, task, size, type Rule, type Node, type Plan } from "../api";
const props = defineProps<{ node: Node }>();
const rules = ref<Rule[]>([]);
const busy = ref(false);
const error = ref("");
const notice = ref("");
const plans = ref<Plan[]>([]);
const selectedScheme = ref("");
const confirm = ref(false);
const draft = ref<Rule>({
  id: "",
  name: "",
  scheme: "自定义方案",
  root: "/var/log",
  patterns: ["*.log.*", "*.gz"],
  excludes: ["audit", "journal"],
  keepDays: 14,
});
const patterns = ref("*.log.*, *.gz");
const excludes = ref("audit, journal");
const schemes = computed(() =>
  Object.entries(
    rules.value.reduce<Record<string, Rule[]>>((groups, r) => {
      (groups[r.scheme || "默认方案"] ??= []).push(r);
      return groups;
    }, Object.create(null)),
  ),
);
async function reload() {
  rules.value = await api<Rule[]>("rules");
}
async function action(fn: () => Promise<void>) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function save() {
  await action(async () => {
    await api("rules", "POST", {
      ...draft.value,
      patterns: patterns.value
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean),
      excludes: excludes.value
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean),
    });
    await reload();
    draft.value.id = "";
    notice.value = "规则已保存，可在任意节点预览；节点仍会检查本地白名单。";
  });
}
async function detect() {
  await action(async () => {
    const found = await task<
      { name: string; detected: boolean; rule: Rule; note: string }[]
    >(props.node.id, { kind: "detect" });
    presets.value = found;
  });
}
const presets = ref<
  { name: string; detected: boolean; rule: Rule; note: string }[]
>([]);
async function adopt(p: { name: string; rule: Rule }) {
  await action(async () => {
    await api("rules", "POST", { ...p.rule, scheme: p.name });
    await reload();
    notice.value = "方案已添加，请先预览。";
  });
}
function edit(r: Rule) {
  draft.value = { ...r };
  patterns.value = r.patterns.join(", ");
  excludes.value = r.excludes.join(", ");
}
async function preview(name: string, rs: Rule[]) {
  plans.value = [];
  confirm.value = false;
  selectedScheme.value = name;
  await action(async () => {
    const complete: Plan[] = [];
    for (const r of rs)
      complete.push(
        await task<Plan>(props.node.id, { kind: "preview", rule: r }),
      );
    plans.value = complete;
  });
}
async function execute() {
  await action(async () => {
    let bytes = 0,
      deleted = 0;
    const skipped: string[] = [];
    const executing = plans.value;
    plans.value = [];
    confirm.value = false;
    for (const p of executing) {
      const result = await task<{
        bytes: number;
        deleted: number;
        skipped: string[];
      }>(props.node.id, { kind: "execute", planId: p.id });
      bytes += result.bytes;
      deleted += result.deleted;
      skipped.push(...result.skipped);
    }
    notice.value = `已删除 ${deleted} 个归档文件，处理分配空间 ${size(bytes)}。跳过 ${skipped.length} 项。详细结果见任务记录。`;
  });
}
onMounted(() => action(reload));
</script>
<template>
  <section>
    <div class="section-heading">
      <div>
        <h2>清理方案</h2>
        <p>内置规则快速开始，自定义方案适配自己的服务。</p>
      </div>
      <button :disabled="busy" @click="detect">识别服务器环境</button>
    </div>
    <div class="notice">
      当前节点允许清理：{{
        node.roots?.join("、") || "尚未配置"
      }}。第一版仅删除过期归档，活跃日志不参与清理。
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="notice" class="success" role="status">{{ notice }}</p>
    <p v-if="busy" class="hint" role="status">任务执行中，请稍候…</p>
    <div v-if="presets.length" class="card">
      <h3>环境识别结果</h3>
      <p class="hint">
        依据常见目录检测，不代表已确认面板安装。非默认路径请使用自定义规则。
      </p>
      <div v-for="p in presets" :key="p.name" class="preset">
        <div>
          <strong>{{ p.name }}</strong
          ><small
            >{{ p.rule.root }} · {{ p.detected ? "目录存在" : "未发现目录"
            }}<br />{{ p.note }}</small
          >
        </div>
        <button :disabled="busy || !p.detected" @click="adopt(p)">
          添加方案
        </button>
      </div>
    </div>
    <div class="rule-layout">
      <div>
        <div v-for="[scheme, group] in schemes" :key="scheme" class="card">
          <div class="toolbar spread">
            <h3>{{ scheme }}</h3>
            <button
              class="primary"
              :disabled="busy"
              @click="preview(scheme, group || [])"
            >
              预览清理
            </button>
          </div>
          <div v-for="rule in group" :key="rule.id" class="rule-row">
            <div>
              <strong>{{ rule.name }}</strong
              ><small
                >{{ rule.root }} · 保留 {{ rule.keepDays }} 天<br />{{
                  rule.patterns.join("、")
                }}</small
              >
            </div>
            <button :disabled="busy" @click="edit(rule)">编辑</button
            ><button
              :disabled="busy"
              @click="
                action(async () => {
                  await api('rules/' + rule.id, 'DELETE');
                  await reload();
                })
              "
            >
              移除
            </button>
          </div>
        </div>
        <div v-if="!rules.length" class="card empty">
          <h3>还没有清理方案</h3>
          <p>识别服务器环境，或在右侧创建第一条规则。</p>
        </div>
      </div>
      <form class="card rule-form" @submit.prevent="save">
        <h3>{{ draft.id ? "编辑规则" : "自定义规则" }}</h3>
        <label
          >所属方案<input
            v-model="draft.scheme"
            required
            maxlength="100" /></label
        ><label
          >规则名称<input
            v-model="draft.name"
            required
            maxlength="100"
            placeholder="应用历史日志" /></label
        ><label
          >日志目录<input
            v-model="draft.root"
            required
            placeholder="/var/log/myapp" /></label
        ><label
          >文件名匹配（逗号分隔）<input v-model="patterns" required /></label
        ><label>排除文件/目录名<input v-model="excludes" /></label
        ><label
          >保留天数<input
            v-model.number="draft.keepDays"
            type="number"
            min="1"
            max="3650"
            required /></label
        ><button :disabled="busy" class="primary">保存规则</button>
      </form>
    </div>
    <div v-if="plans.length" class="card preview">
      <div class="toolbar spread">
        <div>
          <h3>{{ selectedScheme }} · 清理预览</h3>
          <p>当前节点：{{ node.name }} · 预览有效期 10 分钟</p>
        </div>
        <strong>{{ size(plans.reduce((s, p) => s + p.bytes, 0)) }}</strong>
      </div>
      <div v-for="p in plans" :key="p.id">
        <h4>{{ p.rule.name }} · {{ p.files.length }} 个文件</h4>
        <div class="candidate-list">
          <div v-for="f in p.files" :key="f.path">
            <code>{{ f.path }}</code
            ><span>{{ size(f.size) }}</span>
          </div>
          <p v-if="!p.files.length" class="hint">没有符合规则的归档文件。</p>
        </div>
      </div>
      <label class="check"
        ><input
          v-model="confirm"
          type="checkbox"
        />我已核对清单，确认永久删除这些归档文件</label
      ><button
        class="danger"
        :disabled="busy || !confirm || !plans.some((p) => p.files.length)"
        @click="execute"
      >
        执行已预览的方案
      </button>
    </div>
  </section>
</template>
