import type { MessageKey } from "./i18n/messages";
export interface AgentSettings {
  hub: string;
  cleanup: string;
  scan: string;
  panels: string;
}
export function roots(text: string) {
  return [
    ...new Set(
      text
        .split(/\r?\n/)
        .map((v) => v.trim())
        .filter(Boolean),
    ),
  ];
}
export function settingsError(settings: AgentSettings): MessageKey | undefined {
  try {
    const url = new URL(settings.hub.trim());
    if (
      url.username ||
      url.password ||
      /[?#]/.test(settings.hub) ||
      url.pathname.includes("%") ||
      !url.hostname ||
      !(
        url.protocol === "https:" ||
        (url.protocol === "http:" &&
          ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname))
      )
    )
      return "管理端地址需要 HTTPS，且不能包含凭证、查询参数或编码路径。";
  } catch {
    return "管理端地址需要 HTTPS，且不能包含凭证、查询参数或编码路径。";
  }
  const lists = [
    roots(settings.cleanup),
    roots(settings.scan),
    roots(settings.panels),
  ];
  if (
    !lists[0].length ||
    !lists[1].length ||
    lists[0].length > 64 ||
    lists[1].length > 64 ||
    lists[2].length > 16 ||
    lists
      .flat()
      .some(
        (p) =>
          !p.startsWith("/") ||
          p.length > 4096 ||
          /[\x00-\x1f\x7f]/.test(p) ||
          p.split("/").some((v) => v === "." || v === ".."),
      )
  )
    return "目录必须是绝对路径，每行一个；扫描及清理最多 64 项，面板最多 16 项。";
  if (
    lists[0].some((p) =>
      ["/", "/etc", "/proc", "/sys", "/dev"].includes(
        p.replace(/\/+/g, "/").replace(/\/+$/, "") || "/",
      ),
    ) ||
    lists[2].some((p) => !p.replace(/\//g, ""))
  )
    return "不能将系统根目录或受保护目录设为清理范围。";
}
export function agentConfig(
  settings: AgentSettings,
  node: string,
  token: string,
) {
  if (settingsError(settings)) throw new Error("invalid agent settings");
  const url = new URL(settings.hub.trim());
  return {
    mode: "agent",
    hub: url.toString().replace(/\/+$/, ""),
    node,
    token,
    cleanupRoots: roots(settings.cleanup),
    scanRoots: roots(settings.scan),
    ...(roots(settings.panels).length
      ? { panelRoots: roots(settings.panels) }
      : {}),
  };
}
