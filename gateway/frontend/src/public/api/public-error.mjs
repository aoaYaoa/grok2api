export function sanitizePublicError(message, fallback = "请求失败，请稍后重试") {
  const value = String(message || "").replace(/\s+/g, " ").trim();
  if (!value) return fallback;
  const lower = value.toLowerCase();
  if (lower.includes("<!doctype html") || lower.includes("<html") || lower.includes("just a moment") || lower.includes("challenge-platform")) {
    return "上游安全验证暂时未通过，请稍后重试";
  }
  if (lower.includes("constraint failed") || lower.includes("chk_media_jobs_input_json") || lower.includes("sqlite") || lower.includes("sqlstate")) {
    return "服务暂时无法处理该任务，请稍后重试";
  }
  const characters = Array.from(value);
  return characters.length > 180 ? `${characters.slice(0, 180).join("")}...` : value;
}
