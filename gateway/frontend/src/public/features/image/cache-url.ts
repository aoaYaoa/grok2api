export function absoluteImageCacheURL(value: unknown, origin: string): string {
  const rawURL = String(value || "").trim();
  if (!rawURL) return "";
  try {
    return new URL(rawURL, origin).toString();
  } catch {
    return "";
  }
}

export function cachedImageReference(source: string, cacheKey: string, sourceURL: string): string {
  if (source === "mediaAsset") return `grok2api-media://image/${cacheKey}`;
  return sourceURL;
}
