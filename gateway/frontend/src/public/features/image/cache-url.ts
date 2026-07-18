export function absoluteImageCacheURL(value: unknown, origin: string): string {
  const rawURL = String(value || "").trim();
  if (!rawURL) return "";
  try {
    return new URL(rawURL, origin).toString();
  } catch {
    return "";
  }
}
