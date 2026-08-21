import { publicEndpoints, publicFetch, publicSSE } from "@/public/api/client";
import { sanitizePublicError } from "@/public/api/public-error.mjs";
import { cacheDeletePayload } from "@/public/features/cache/cache-selection";
import { videoURLFromText } from "@/public/lib/media";
import type { CacheDeleteResult, CacheIdentity } from "@/public/features/image/image-api";

export type VideoItem = { id: string; taskID: string; url: string; posterURL: string; prompt: string; progress: number; status: "queued" | "running" | "completed" | "failed"; postID: string; displayName: string; error?: string; createdAt: number; originalPostID?: string; source?: CacheIdentity["source"]; cacheKey?: string };
export type VideoStartBody = { prompt: string; aspect_ratio: string; video_length: number; resolution_name: string; provider?: "grok_web" | "grok_console"; concurrent?: number; image_url?: string; source_image_url?: string; image_references?: string[]; source_image_urls?: string[]; reference_items?: Array<Record<string, string>>; preset?: string; is_video_extension?: boolean; source_task_id?: string; extend_post_id?: string; video_extension_start_time?: number; original_post_id?: string; file_attachment_id?: string; stitch_with_extend?: boolean };

export async function startVideo(key: string, body: VideoStartBody, signal?: AbortSignal) { return publicFetch<{ task_id: string; task_ids?: string[] }>(key, publicEndpoints.videoStart, { method: "POST", body: JSON.stringify(body), signal }); }
export async function stopVideos(key: string, taskIDs: string[]) { if (taskIDs.length) await publicFetch(key, publicEndpoints.videoStop, { method: "POST", body: JSON.stringify({ task_ids: taskIDs }) }); }
export function streamVideo(key: string, taskID: string, onUpdate: (update: { progress?: number; url?: string; posterURL?: string; done?: boolean; error?: string }) => void, signal: AbortSignal) {
  let terminal = false;
  let resultSeen = false;
  return publicSSE<Record<string, unknown> | string>(key, `${publicEndpoints.videoSSE}?task_id=${encodeURIComponent(taskID)}`, ({ data }) => {
    if (data === "[DONE]") {
      if (!terminal) onUpdate(resultSeen ? { done: true } : { error: "视频任务结束但未返回结果", done: true });
      terminal = true;
      return;
    }
    if (terminal) return;
    if (!data || typeof data !== "object") return;
		if (data.error) { terminal = true; onUpdate({ error: sanitizePublicError(String(data.error), "视频生成失败，请稍后重试"), done: true }); return; }
    if (data.poster_url) onUpdate({ posterURL: String(data.poster_url) });
    const choice = (data.choices as Array<{ delta?: { content?: string }; finish_reason?: string }> | undefined)?.[0]; const content = choice?.delta?.content || ""; const url = videoURLFromText(content); const progress = Number(content.match(/(\d{1,3})%/)?.[1] || 0);
    if (url) resultSeen = true;
    if (choice?.finish_reason) {
      terminal = true;
      onUpdate(resultSeen ? { ...(url ? { url, progress: 100 } : {}), done: true } : { error: "视频任务结束但未返回结果", done: true });
      return;
    }
    onUpdate({ ...(progress ? { progress } : {}), ...(url ? { url, progress: 100 } : {}) });
  }, signal);
}
export async function listCachedVideos(key: string) { return publicFetch<{ items?: Array<Record<string, unknown>> }>(key, `${publicEndpoints.videoCache}?page=1&page_size=200`); }
export async function deleteCachedVideos(key: string, items: CacheIdentity[]) { return publicFetch<CacheDeleteResult>(key, publicEndpoints.videoCacheDelete, { method: "POST", body: JSON.stringify(cacheDeletePayload(items)) }); }
export async function renameVideo(key: string, item: VideoItem, displayName: string) { return publicFetch(key, publicEndpoints.videoRename, { method: "POST", body: JSON.stringify({ post_id: item.postID, name: item.id, display_name: displayName }) }); }
export function videoPostID(value: string) { const matches = Array.from(value.matchAll(/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/g), (match) => match[0]); return matches.at(-1) || ""; }
export function cachedVideo(payload: Record<string, unknown>): VideoItem { const url = String(payload.view_url || payload.url || ""); const source = String(payload.source || "legacy") as CacheIdentity["source"]; const cacheKey = String(payload.cache_key || payload.name || payload.task_id || crypto.randomUUID()); const id = `${source}:${cacheKey}`; return { id, taskID: String(payload.task_id || id), url, posterURL: String(payload.poster_url || payload.posterURL || ""), prompt: "", progress: 100, status: "completed", postID: String(payload.post_id || videoPostID(url)), originalPostID: String(payload.original_post_id || ""), displayName: String(payload.display_name || payload.name || "视频"), createdAt: Number(payload.mtime_ms || Date.now()), source, cacheKey }; }
