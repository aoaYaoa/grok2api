import { publicEndpoints, publicFetch, publicSSE, publicSSERequest } from "@/public/api/client";
import { cacheDeletePayload } from "@/public/features/cache/cache-selection";
import { imageSource } from "@/public/lib/media";
import { absoluteImageCacheURL, cachedImageReference } from "./cache-url";

export type GeneratedImage = { id: string; url: string; prompt: string; parentPostID: string; sourceURL: string; requestSourceURL?: string; elapsedMS?: number; createdAt: number };
export type CacheSource = "legacy" | "mediaAsset" | "mediaJob";
export type CacheIdentity = { source: CacheSource; cacheKey: string };
export type CacheDeleteResult = { deleted: number; skipped: number; failed: number; deleted_keys: string[] };
export type CachedImage = GeneratedImage & CacheIdentity & { name: string; sizeBytes: number };
export type ImageEvent = Record<string, unknown>;

export async function startImage(key: string, body: { prompt: string; aspect_ratio: string; nsfw: boolean; pro: boolean }, signal?: AbortSignal) { return publicFetch<{ task_id: string }>(key, publicEndpoints.imagineStart, { method: "POST", body: JSON.stringify(body), signal }); }
export async function stopImages(key: string, taskIDs: string[]) { if (taskIDs.length) await publicFetch(key, publicEndpoints.imagineStop, { method: "POST", body: JSON.stringify({ task_ids: taskIDs }) }); }
export function streamImage(key: string, taskID: string, onEvent: (event: ImageEvent) => void, signal: AbortSignal) { return publicSSE<ImageEvent>(key, `${publicEndpoints.imagineSSE}?task_id=${encodeURIComponent(taskID)}`, ({ data }) => { if (data && typeof data === "object") onEvent(data); }, signal); }

export function generatedImage(event: ImageEvent, prompt: string): GeneratedImage | null {
  const url = imageSource(event); if (!url) return null;
  const id = String(event.image_id || event.parent_post_id || crypto.randomUUID());
  return { id, url, prompt: String(event.prompt || prompt), parentPostID: String(event.parent_post_id || event.image_id || ""), sourceURL: String(event.current_source_image_url || event.source_image_url || url), elapsedMS: Number(event.elapsed_ms || 0) || undefined, createdAt: Date.now() };
}

export async function editImage(key: string, body: Record<string, unknown>, onProgress?: (value: number, message: string) => void, signal?: AbortSignal) {
  let result: Record<string, unknown> | null = null; let failure = "";
  await publicSSERequest<Record<string, unknown>>(key, body.workbench ? publicEndpoints.workbenchEdit : publicEndpoints.imagineEdit, { method: "POST", body: JSON.stringify({ ...body, stream: true, workbench: undefined }) }, ({ event, data }) => {
    if (event === "progress") onProgress?.(Number(data.progress || 0), String(data.message || "编辑中"));
    if (event === "result") result = data;
    if (event === "error") failure = String(data.message || "编辑失败");
  }, signal);
  if (failure) throw new Error(failure); if (!result) throw new Error("编辑结果为空"); return result as Record<string, unknown>;
}

export function imageFromEdit(payload: Record<string, unknown>, prompt: string): GeneratedImage | null {
  const data = Array.isArray(payload.data) ? payload.data[0] as Record<string, unknown> : {};
  const url = imageSource({ ...data, current_source_image_url: payload.current_source_image_url }); if (!url) return null;
  const id = String(payload.current_parent_post_id || payload.generated_parent_post_id || crypto.randomUUID());
  return { id, url, prompt, parentPostID: id, sourceURL: String(payload.current_source_image_url || url), elapsedMS: Number(payload.elapsed_ms || 0) || undefined, createdAt: Date.now() };
}

export function cachedImage(payload: Record<string, unknown>): CachedImage {
  const name = String(payload.name || "缓存图片");
  const url = absoluteImageCacheURL(payload.view_url || payload.url, window.location.origin);
  const parentPostID = name.match(/[0-9a-fA-F]{8}-[0-9a-fA-F-]{24,28}/)?.[0] || "";
  const source = (String(payload.source || "legacy") === "mediaAsset" ? "mediaAsset" : "legacy") as "legacy" | "mediaAsset";
  const cacheKey = String(payload.cache_key || name);
  return {
    id: `${source}:${cacheKey}`, name, url, source, cacheKey, sourceURL: url,
    requestSourceURL: cachedImageReference(source, cacheKey, url), parentPostID,
    prompt: name, sizeBytes: Number(payload.size_bytes || 0), createdAt: Number(payload.mtime_ms || Date.now()),
  };
}

export async function deleteCachedImages(key: string, items: CacheIdentity[]) {
  return publicFetch<CacheDeleteResult>(key, publicEndpoints.imageCacheDelete, { method: "POST", body: JSON.stringify(cacheDeletePayload(items)) });
}

export function cachedReferenceSource(image: CachedImage) {
  return image.requestSourceURL || cachedImageReference(image.source, image.cacheKey, image.sourceURL);
}

export async function listCachedImages(key: string) {
  const items: Array<Record<string, unknown>> = [];
  let page = 1;
  let total: number;
  do {
    const payload = await publicFetch<{ items: Array<Record<string, unknown>>; total: number }>(key, `${publicEndpoints.imageCache}?page=${page}&page_size=200`);
    items.push(...(payload.items || []));
    total = Number(payload.total || 0);
    page += 1;
  } while (items.length < total && page <= 50);
  return { items, total };
}
