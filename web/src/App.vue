<script setup lang="ts">
import { ref, computed, onUnmounted } from "vue";
import { api, setToken, online, type Node } from "./api";
import Overview from "./components/Overview.vue";
import DiskExplorer from "./components/DiskExplorer.vue";
import RulesPanel from "./components/RulesPanel.vue";
import EnrollNode from "./components/EnrollNode.vue";
import TaskHistory from "./components/TaskHistory.vue";
const logged = ref(false);
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
  nodes.value = await api<Node[]>("nodes");
  if (!current.value) selected.value = nodes.value[0]?.id || "";
}
async function login() {
  error.value = "";
  setToken(password.value);
  try {
    await reload();
    logged.value = true;
    password.value = "";
    timer = setInterval(
      () => reload().catch((e) => (error.value = e.message)),
      5000,
    );
  } catch (e) {
    error.value = (e as Error).message;
  }
}
function logout() {
  clearInterval(timer);
  setToken("");
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
  try {
    await api("nodes/" + current.value.id, "DELETE");
    revoking.value = false;
    await reload();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
onUnmounted(() => clearInterval(timer));
</script>
<template>
  <div v-if="!logged" class="login-page">
    <form class="card login" @submit.prevent="login">
      <div class="brand"><span class="brand-icon">▦</span>NodeSweep</div>
      <span class="eyebrow">LESS CLUTTER. MORE CLARITY.</span>
      <h1>服务器空间管理</h1>
      <p>连接你的节点，让每一份磁盘空间都有迹可循。</p>
      <label
        >管理员访问令牌<input
          v-model="password"
          type="password"
          required
          autocomplete="current-password"
          placeholder="配置文件中的 adminToken"
      /></label>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button class="primary">进入控制台 →</button
      ><small>轻量部署 · 多节点管理 · 按规则清理</small>
    </form>
  </div>
  <div v-else class="app-shell">
    <aside>
      <div class="brand"><span class="brand-icon">▦</span>NodeSweep</div>
      <span class="eyebrow">WORKSPACE</span>
      <nav>
        <button
          :class="{ active: page === 'overview' }"
          @click="page = 'overview'"
        >
          ◫　节点总览</button
        ><button :class="{ active: page === 'disk' }" @click="page = 'disk'">
          ▦　磁盘分析</button
        ><button :class="{ active: page === 'rules' }" @click="page = 'rules'">
          ♧　清理方案</button
        ><button :class="{ active: page === 'tasks' }" @click="page = 'tasks'">
          ≡　任务记录
        </button>
      </nav>
      <div class="sidebar-bottom">
        <span class="status up"
          >{{ nodes.filter(online).length }} 个节点在线</span
        >
        <p>NodeSweep / v0.1 alpha</p>
        <button @click="logout">退出登录</button>
      </div>
    </aside>
    <main>
      <header>
        <div>
          <span class="eyebrow">NODE OPERATIONS</span>
          <h1>
            {{
              page === "overview"
                ? "节点总览"
                : page === "disk"
                  ? "磁盘分析"
                  : page === "rules"
                    ? "清理方案"
                    : "任务记录"
            }}
          </h1>
        </div>
        <button class="primary" @click="enrolling = true">＋ 添加节点</button>
      </header>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div
        v-if="page === 'disk' || page === 'rules'"
        class="node-selector toolbar"
      >
        <label
          >当前节点<select v-model="selected">
            <option v-for="n in nodes" :key="n.id" :value="n.id">
              {{ n.name }} · {{ online(n) ? "在线" : "离线" }}
            </option>
          </select></label
        ><span v-if="current" class="hint">{{ current.metrics.host }}</span
        ><button
          v-if="current && current.id !== 'local'"
          @click="revoking = true"
        >
          撤销节点
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
      <TaskHistory v-else-if="page === 'tasks'" />
      <div v-else class="card empty">先添加一台服务器，开始管理空间。</div>
      <footer>NodeSweep · 看清占用，安心清理</footer>
    </main>
    <EnrollNode v-if="enrolling" @close="enrolling = false" @added="reload" />
    <div v-if="revoking" class="modal-backdrop">
      <div class="modal card">
        <h2>撤销 {{ current?.name }}？</h2>
        <p>该节点的凭证将立即失效。已经在节点执行的任务不会被远程中止。</p>
        <button class="danger" @click="revoke">确认撤销</button>
        <button @click="revoking = false">取消</button>
      </div>
    </div>
  </div>
</template>
