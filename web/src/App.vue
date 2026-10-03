<script setup lang="ts">
import { canAdmin, role, setRole, type Role } from "./access";
import AuditPanel from "./components/AuditPanel.vue";
import { t, systemText } from "./i18n";
import { ref, computed, onUnmounted } from "vue";
import { api, setToken, online, type Node } from "./api";
import LanguagePicker from "./components/LanguagePicker.vue";
import BatchOperations from "./components/BatchOperations.vue";
import Overview from "./components/Overview.vue";
import DiskExplorer from "./components/DiskExplorer.vue";
import RulesPanel from "./components/RulesPanel.vue";
import EnrollNode from "./components/EnrollNode.vue";
import AlertsPanel from "./components/AlertsPanel.vue";
import TaskHistory from "./components/TaskHistory.vue";
const logged = ref(false);
const loggingIn = ref(false);
let authGeneration = 0;
let reloadGeneration = 0;
const password = ref("");
const nodes = ref<Node[]>([]);
const selected = ref("");
const page = ref("overview");
const error = ref("");
const enrolling = ref(false);
const revoking = ref(false);
const current = computed(() =>
  nodes.value.find((n) => n.id === selected.value),
);
let timer: ReturnType<typeof setInterval> | undefined;
async function reload() {
  const generation = ++reloadGeneration;
  const updated = await api<Node[]>("nodes");
  if (generation !== reloadGeneration) return;
  nodes.value = updated;
  if (!current.value) selected.value = nodes.value[0]?.id || "";
}
async function login() {
  if (loggingIn.value) return;
  const generation = ++authGeneration;
  loggingIn.value = true;
  error.value = "";
  setRole("viewer");
  setToken(password.value);
  try {
    const session = await api<{ role: Role }>("session");
    if (generation !== authGeneration) return;
    setRole(session.role);
    await reload();
    if (generation !== authGeneration) return;
    logged.value = true;
    password.value = "";
    timer = setInterval(
      () =>
        reload().catch((e) => {
          if (logged.value && e.name !== "AbortError") error.value = e.message;
        }),
      5000,
    );
  } catch (e) {
    if (generation === authGeneration) {
      setToken("");
      error.value = (e as Error).message;
    }
  } finally {
    if (generation === authGeneration) loggingIn.value = false;
  }
}
function logout() {
  ++authGeneration;
  ++reloadGeneration;
  clearInterval(timer);
  timer = undefined;
  loggingIn.value = false;
  enrolling.value = false;
  revoking.value = false;
  password.value = "";
  error.value = "";
  setToken("");
  setRole("viewer");
  logged.value = false;
  nodes.value = [];
  selected.value = "";
  page.value = "overview";
}
function select(id: string) {
  selected.value = id;
  page.value = "disk";
}
async function revoke() {
  if (!current.value) return;
  const generation = authGeneration;
  try {
    await api("nodes/" + current.value.id, "DELETE");
    if (generation !== authGeneration) return;
    revoking.value = false;
    await reload();
  } catch (e) {
    if (generation === authGeneration && logged.value)
      error.value = (e as Error).message;
  }
}
onUnmounted(logout);
</script>
<template>
  <div v-if="!logged" class="login-page">
    <form class="card login" @submit.prevent="login">
      <div class="brand"><span class="brand-icon">▦</span>NodeSweep</div>
      <LanguagePicker />
      <h1>{{ t("服务器空间管理") }}</h1>
      <p>{{ t("连接你的节点，让每一份磁盘空间都有迹可循。") }}</p>
      <label
        >{{ t("访问令牌")
        }}<input
          v-model="password"
          type="password"
          required
          autocomplete="current-password"
          :placeholder="t('配置文件中的 adminToken 或 accessTokens')"
      /></label>
      <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
      <button class="primary" :disabled="loggingIn">
        {{ loggingIn ? t("登录中…") : t("进入控制台 →") }}</button
      ><small>{{ t("轻量部署 · 多节点管理 · 按规则清理") }}</small>
    </form>
  </div>
  <div v-else class="app-shell">
    <aside>
      <div class="brand"><span class="brand-icon">▦</span>NodeSweep</div>
      <span class="eyebrow">{{ t("工作空间") }}</span>
      <nav>
        <button
          :class="{ active: page === 'overview' }"
          @click="page = 'overview'"
        >
          ◫　{{ t("节点总览") }}</button
        ><button :class="{ active: page === 'disk' }" @click="page = 'disk'">
          ▦　{{ t("磁盘分析") }}</button
        ><button :class="{ active: page === 'rules' }" @click="page = 'rules'">
          ♧　{{ t("清理方案") }}</button
        ><button :class="{ active: page === 'tasks' }" @click="page = 'tasks'">
          ≡　{{ t("任务记录") }}</button
        ><button
          :class="{ active: page === 'alerts' }"
          @click="page = 'alerts'"
        >
          ⚑　{{ t("磁盘告警") }}</button
        ><button :class="{ active: page === 'batch' }" @click="page = 'batch'">
          ▤　{{ t("批量操作") }}
        </button>
        <button
          v-if="canAdmin"
          :class="{ active: page === 'audit' }"
          @click="page = 'audit'"
        >
          ≡　{{ t("操作审计") }}
        </button>
      </nav>
      <div class="sidebar-bottom">
        <span class="status up">{{
          t("{count} 个节点在线", { count: nodes.filter(online).length })
        }}</span>
        <p>
          {{
            t(
              role === "admin"
                ? "管理员"
                : role === "operator"
                  ? "操作员"
                  : "只读",
            )
          }}
        </p>
        <p>NodeSweep / v0.1 alpha</p>
        <button @click="logout">{{ t("退出登录") }}</button>
      </div>
    </aside>
    <main>
      <header>
        <div>
          <span class="eyebrow">{{ t("节点运维") }}</span>
          <h1>
            {{
              page === "overview"
                ? t("节点总览")
                : page === "disk"
                  ? t("磁盘分析")
                  : page === "rules"
                    ? t("清理方案")
                    : page === "batch"
                      ? t("批量操作")
                      : page === "alerts"
                        ? t("磁盘告警")
                        : page === "audit"
                          ? t("操作审计")
                          : t("任务记录")
            }}
          </h1>
        </div>
        <div class="header-actions">
          <LanguagePicker /><button
            v-if="canAdmin"
            class="primary"
            @click="enrolling = true"
          >
            ＋ {{ t("添加节点") }}
          </button>
        </div>
      </header>
      <p v-if="error" class="error" role="alert">{{ systemText(error) }}</p>
      <div
        v-if="page === 'disk' || page === 'rules'"
        class="node-selector toolbar"
      >
        <label
          >{{ t("当前节点")
          }}<select v-model="selected">
            <option v-for="n in nodes" :key="n.id" :value="n.id">
              {{ n.name }} · {{ online(n) ? t("在线") : t("离线") }}
            </option>
          </select></label
        ><span v-if="current" class="hint">{{ current.metrics.host }}</span
        ><button
          v-if="canAdmin && current && current.id !== 'local'"
          @click="revoking = true"
        >
          {{ t("撤销节点") }}
        </button>
      </div>
      <Overview v-if="page === 'overview'" :nodes="nodes" @select="select" />
      <DiskExplorer
        v-else-if="page === 'disk' && current"
        :key="'disk' + current.id"
        :node="current"
      />
      <RulesPanel
        v-else-if="page === 'rules' && current"
        :key="'rules' + current.id"
        :node="current"
      />
      <BatchOperations
        v-else-if="page === 'batch'"
        :nodes="nodes"
        @changed="reload"
        @select="select"
      />
      <AuditPanel v-else-if="page === 'audit' && canAdmin" />
      <TaskHistory v-else-if="page === 'tasks'" :nodes="nodes" /><AlertsPanel
        v-else-if="page === 'alerts'"
      />
      <div v-else class="card empty">
        {{ t("先添加一台服务器，开始管理空间。") }}
      </div>
      <footer>NodeSweep · {{ t("看清占用，安心清理") }}</footer>
    </main>
    <EnrollNode
      v-if="enrolling && canAdmin"
      @close="enrolling = false"
      @added="reload"
    />
    <div v-if="revoking && canAdmin" class="modal-backdrop">
      <div class="modal card">
        <h2>{{ t("撤销 {name}？", { name: current?.name || "" }) }}</h2>
        <p>
          {{
            t("该节点的凭证将立即失效。已经在节点执行的任务不会被远程中止。")
          }}
        </p>
        <button class="danger" @click="revoke">{{ t("确认撤销") }}</button>
        <button @click="revoking = false">{{ t("取消") }}</button>
      </div>
    </div>
  </div>
</template>
