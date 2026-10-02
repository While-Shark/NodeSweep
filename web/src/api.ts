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
  lastSeen: string;
  roots: string[];
  scanRoots: string[];
  metrics: {
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
  at: string;
}
export interface Plan {
  id: string;
  rule: Rule;
  created: string;
  bytes: number;
  files: { path: string; size: number }[];
}
export interface Task {
  id: string;
  node: string;
  request: { kind: string };
  status: string;
  error?: string;
  created: string;
}
let token = "";
export function setToken(value: string) {
  token = value;
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch("/api/" + path, {
    method,
    headers: {
      Authorization: "Bearer " + token,
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "请求失败");
  return data;
}
export async function task<T>(node: string, request: unknown): Promise<T> {
  const created = await api<Task>("tasks", "POST", { node, request });
  for (let i = 0; i < 130; i++) {
    await new Promise((r) => setTimeout(r, 1000));
    const current = await api<Task & { result: T }>("tasks/" + created.id);
    if (current.status === "succeeded") return current.result;
    if (["failed", "interrupted"].includes(current.status))
      throw new Error(current.error || "任务失败");
  }
  throw new Error("等待超时。请到任务记录查看状态，不要重复执行清理。");
}
export function size(n: number) {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), 4);
  return (n / 1024 ** i).toFixed(i > 1 ? 1 : 0) + " " + u[i];
}
export function online(n: Node) {
  return Date.now() - new Date(n.lastSeen).getTime() < 45000;
}
export function date(v: string) {
  return new Date(v).toLocaleString("zh-CN");
}
