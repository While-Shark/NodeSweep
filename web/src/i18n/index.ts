import { ref, watch } from "vue";
import { messages, type MessageKey } from "./messages";
import { errorKeys } from "./errors";

export const languages = [
  { value: "zh-CN", label: "简体中文" },
  { value: "en", label: "English" },
  { value: "ja", label: "日本語" },
  { value: "ko", label: "한국어" },
  { value: "zh-TW", label: "繁體中文" },
] as const;
export type Locale = (typeof languages)[number]["value"];
export type Params = Record<string, string | number>;
export interface Notice {
  key: MessageKey;
  params?: Params;
}
const storageKey = "nodesweep.language";
export function isLocale(value: unknown): value is Locale {
  return languages.some((item) => item.value === value);
}
export function detectLocale(preferences: readonly string[]): Locale {
  for (const preference of preferences) {
    const code = preference.toLowerCase();
    if (code === "zh" || code.startsWith("zh-"))
      return /(?:hant|tw|hk|mo)/.test(code) ? "zh-TW" : "zh-CN";
    for (const supported of ["en", "ja", "ko"] as const)
      if (code === supported || code.startsWith(supported + "-"))
        return supported;
  }
  return "zh-CN";
}
function initialLocale(): Locale {
  try {
    const saved = localStorage.getItem(storageKey);
    if (isLocale(saved)) return saved;
  } catch {
    /* Language still works when storage is unavailable. */
  }
  return detectLocale(
    typeof navigator === "undefined" ? [] : navigator.languages,
  );
}
export const locale = ref<Locale>(initialLocale());
export function setLocale(value: string) {
  if (isLocale(value)) locale.value = value;
}
watch(
  locale,
  (value) => {
    if (typeof document !== "undefined") document.documentElement.lang = value;
    try {
      localStorage.setItem(storageKey, value);
    } catch {
      /* Optional persistence. */
    }
  },
  { immediate: true },
);

const columns = { en: 0, ja: 1, ko: 2, "zh-TW": 3 } as const;
const chineseErrors: Partial<Record<MessageKey, string>> = {
  "invalid administrator token": "管理员访问令牌无效",
  "node offline": "节点已离线",
  "node already has an active task": "该节点已有执行中的任务",
  "path is outside the agent allowlist": "路径不在节点白名单内",
  "symlink paths are not allowed": "不允许符号链接路径",
  "preview missing, expired or already consumed":
    "预览不存在、已过期或已使用，请重新预览",
  "preview expired": "预览已过期，请重新预览",
  "too many previews; wait for expiry": "预览数量过多，请等待已有预览过期",
  "server shutting down": "服务器正在关闭",
  "name required, max 100 characters": "请输入名称（最多 100 个字符）",
  "node not found": "找不到节点",
  "task not found": "找不到任务",
};
export function number(value: number): string {
  return new Intl.NumberFormat(locale.value).format(value);
}
export function t(key: MessageKey, params: Params = {}): string {
  const template =
    locale.value === "zh-CN"
      ? chineseErrors[key] || key
      : messages[key][columns[locale.value]];
  return template.replace(/\{(\w+)\}/g, (match, name: string) => {
    const value = params[name];
    return value === undefined
      ? match
      : typeof value === "number"
        ? number(value)
        : value;
  });
}
// Only system-owned text uses this helper. User names, paths and JSON stay intact.
export function systemText(value: string): string {
  if (Object.hasOwn(errorKeys, value)) return t(errorKeys[value]);
  const ruleError = /^rule (\d+): (.+)$/.exec(value);
  if (ruleError)
    return t("规则 {index}：{detail}", {
      index: Number(ruleError[1]),
      detail: systemText(ruleError[2]),
    });
  if (Object.prototype.hasOwnProperty.call(messages, value))
    return t(value as MessageKey);
  const prefix = "从 Nginx 静态日志指令识别；保留当前日志 ";
  if (value.startsWith(prefix))
    return t("从 Nginx 静态日志指令识别；保留当前日志 {name}", {
      name: value.slice(prefix.length),
    });
  return value;
}
export function taskKind(kind: string): string {
  const keys: Record<string, MessageKey> = {
    scan: "目录扫描",
    trial: "规则试运行",
    preview: "清理预览",
    execute: "执行清理",
    detect: "环境识别",
  };
  return Object.hasOwn(keys, kind) ? t(keys[kind]) : kind;
}
export function taskStatus(status: string): string {
  const keys: Record<string, MessageKey> = {
    pending: "待执行",
    running: "执行中",
    succeeded: "成功",
    failed: "失败",
    interrupted: "已中断",
  };
  return Object.hasOwn(keys, status) ? t(keys[status]) : status;
}

watch(
  locale,
  () => {
    if (typeof document !== "undefined")
      document.title = "NodeSweep · " + t("服务器空间管理");
  },
  { immediate: true },
);

export function scanReason(reason: string): string {
  const keys: Record<string, MessageKey> = {
    entries: "条目数量上限",
    tree_bytes: "结果大小上限",
    depth: "目录深度上限",
    time: "扫描时间上限",
    cancelled: "扫描已取消",
  };
  return keys[reason] ? t(keys[reason]) : reason;
}
