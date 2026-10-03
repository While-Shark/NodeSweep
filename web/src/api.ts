import { locale } from "./i18n";
export interface Disk {
  path: string;
  total: number;
  available: number;
  inodes: number;
  freeInodes: number;
}
export interface Node {
  id: string;
  name: string;
  group?: string;
  scanControl?: boolean;
  logChecks?: boolean;
  lastSeen: string;
  roots: string[];
  scanRoots: string[];
  metrics: {
    cpuAvailable?: boolean;
    partial?: boolean;
    host: string;
    cpu: number;
    memoryTotal: number;
    memoryAvailable: number;
    load: string;
    uptime: string;
    disks: Disk[];
  };
}
export interface Rule {
  id: string;
  name: string;
  scheme?: string;
  root: string;
  patterns: string[];
  excludes: string[];
  keepDays: number;
}
export interface Entry {
  name: string;
  path: string;
  bytes: number;
  directory: boolean;
  children?: Entry[];
}
export interface Scan {
  tree: Entry;
  files: number;
  skipped: number;
  truncated: boolean;
  reason?: string;
  at: string;
}
export interface Review {
  counts: Record<string, number>;
  examples: { path: string; reason: string; pattern?: string }[];
}
export interface CleanupResult {
  root?: string;
  ruleName?: string;
  deleted: number;
  bytes: number;
  skipped: string[];
  planned?: number;
  plannedBytes?: number;
  started?: string;
  finished?: string;
  items?: { path: string; status: string; reason?: string; bytes: number }[];
}
export interface Plan {
  review?: Review;
  id: string;
  rule: Rule;
  created: string;
  bytes: number;
  files: { path: string; size: number }[];
}
export interface ScanProgress {
  visited: number;
  files: number;
  bytes: number;
  limit: number;
  elapsedMillis: number;
}
export interface Task {
  progress?: ScanProgress;
  cancelRequested?: boolean;
  id: string;
  node: string;
  request: { kind: string };
  status: string;
  error?: string;
  created: string;
  result?: unknown;
}
let token = "";
let session = new AbortController();
export function setToken(value: string) {
  session.abort();
  session = new AbortController();
  token = value;
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const activeSignal = AbortSignal.any([
    session.signal,
    ...(signal ? [signal] : []),
  ]);
  activeSignal.throwIfAborted();
  try {
    const response = await fetch("/api/" + path, {
      method,
      signal: activeSignal,
      headers: {
        Authorization: "Bearer " + token,
        "Content-Type": "application/json",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data = await response.json();
    activeSignal.throwIfAborted();
    if (!response.ok) throw new Error(data.error || "请求失败");
    return data;
  } catch (error) {
    activeSignal.throwIfAborted();
    throw error;
  }
}
export async function task<T>(
  node: string,
  request: unknown,
  signal?: AbortSignal,
  onUpdate?: (task: Task) => void,
): Promise<T> {
  signal = AbortSignal.any([session.signal, ...(signal ? [signal] : [])]);
  const created = await api<Task>("tasks", "POST", { node, request }, signal);
  onUpdate?.(created);
  for (let i = 0; i < 130; i++) {
    await new Promise((r) => setTimeout(r, 1000));
    signal?.throwIfAborted();
    const current = await api<Task & { result: T }>(
      "tasks/" + created.id,
      "GET",
      undefined,
      signal,
    );
    onUpdate?.(current);
    if (current.status === "succeeded") {
      if (current.result == null)
        throw new Error("任务结果已过期，请重新扫描或预览。");
      return current.result;
    }
    if (["failed", "interrupted"].includes(current.status))
      throw new Error(current.error || "任务失败");
  }
  throw new Error("等待超时。请到任务记录查看状态，不要重复执行清理。");
}
export function size(n: number) {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), 4);
  return (
    new Intl.NumberFormat(locale.value, {
      minimumFractionDigits: i > 1 ? 1 : 0,
      maximumFractionDigits: i > 1 ? 1 : 0,
    }).format(n / 1024 ** i) +
    " " +
    u[i]
  );
}
export function online(n: Node) {
  return Date.now() - new Date(n.lastSeen).getTime() < 45000;
}
export function date(v: string) {
  return new Date(v).toLocaleString(locale.value);
}
