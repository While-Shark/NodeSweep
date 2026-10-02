import type { MessageKey } from "./messages";
// Known API diagnostics; unknown details remain available verbatim.
export const errorKeys: Record<string, MessageKey> = {
  "internal server error": "服务器内部错误，请查看服务端日志",
  "invalid preview identifier": "预览标识无效，请重新预览",
  "node poll rate exceeded": "节点请求过于频繁，请稍后重试",
  "absolute path required": "请输入绝对路径",
  "rule root must be an absolute path, max 4096 characters":
    "日志目录必须是绝对路径，最多 4096 个字符",
  "filesystem root cannot be a cleanup root":
    "不能将文件系统根目录作为清理目录",
  "scheme name too long": "方案名称过长",
  "rule name required (max 100 characters)": "请输入规则名称，最多 100 个字符",
  "keepDays must be 1–3650": "保留天数必须为 1–3650",
  "provide 1–20 filename patterns and at most 50 exclusions":
    "请提供 1–20 个文件名模式，排除项最多 50 个",
  "filename patterns must contain 1–255 characters":
    "文件名模式长度必须为 1–255 个字符",
  "patterns match filenames, not paths": "匹配模式只允许文件名，不允许目录路径",
  "unsupported rule bundle format or version": "不支持此方案文件格式或版本",
  "a bundle must contain 1–100 rules": "方案文件必须包含 1–100 条规则",
  "export requires 1–100 rules; select a smaller scheme":
    "导出需要 1–100 条规则，请选择更小的方案",
  "export exceeds file size limit; select a smaller scheme":
    "导出超过文件大小限制，请选择更小的方案",
  "preview exceeds 100000 entries; choose a narrower directory":
    "预览超过 100,000 个条目，请缩小目录范围",
  "more than 5000 candidates; narrow the rule before cleanup":
    "候选文件超过 5,000 个，请缩小规则范围",
  "context deadline exceeded": "任务超过执行时限，请查看任务记录",
  "context canceled": "任务已取消",
};
