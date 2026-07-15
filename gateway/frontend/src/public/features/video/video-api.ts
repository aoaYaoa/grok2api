import { publicEndpoints, publicFetch, publicSSE } from "@/public/api/client";
import { videoURLFromText } from "@/public/lib/media";

export type VideoItem = { id: string; taskID: string; url: string; prompt: string; progress: number; status: "queued" | "running" | "completed" | "failed"; postID: string; displayName: string; error?: string; createdAt: number; originalPostID?: string };
export type VideoStartBody = { prompt: string; aspect_ratio: string; video_length: number; resolution_name: string; concurrent?: number; image_url?: string; source_image_url?: string; image_references?: string[]; source_image_urls?: string[]; reference_items?: Array<Record<string, string>>; preset?: string; is_video_extension?: boolean; extend_post_id?: string; video_extension_start_time?: number; original_post_id?: string; file_attachment_id?: string; stitch_with_extend?: boolean };

export async function startVideo(key: string, body: VideoStartBody) { return publicFetch<{ task_id: string; task_ids?: string[] }>(key, publicEndpoints.videoStart, { method: "POST", body: JSON.stringify(body) }); }
export async function stopVideos(key: string, taskIDs: string[]) { if (taskIDs.length) await publicFetch(key, publicEndpoints.videoStop, { method: "POST", body: JSON.stringify({ task_ids: taskIDs }) }); }
export function streamVideo(key: string, taskID: string, onUpdate: (update: { progress?: number; url?: string; done?: boolean; error?: string }) => void, signal: AbortSignal) {
  return publicSSE<Record<string, unknown> | string>(key, `${publicEndpoints.videoSSE}?task_id=${encodeURIComponent(taskID)}`, ({ data }) => {
    if (data === "[DONE]") { onUpdate({ done: true }); return; }
    if (!data || typeof data !== "object") return;
    if (data.error) { onUpdate({ error: String(data.error), done: true }); return; }
    const choice = (data.choices as Array<{ delta?: { content?: string }; finish_reason?: string }> | undefined)?.[0]; const content = choice?.delta?.content || ""; const url = videoURLFromText(content); const progress = Number(content.match(/(\d{1,3})%/)?.[1] || 0);
    onUpdate({ ...(progress ? { progress } : {}), ...(url ? { url, progress: 100 } : {}), ...(choice?.finish_reason ? { done: true } : {}) });
  }, signal);
}
export async function listCachedVideos(key: string) { return publicFetch<{ items?: Array<Record<string, unknown>> }>(key, `${publicEndpoints.videoCache}?page=1&page_size=200`); }
export async function renameVideo(key: string, item: VideoItem, displayName: string) { return publicFetch(key, publicEndpoints.videoRename, { method: "POST", body: JSON.stringify({ post_id: item.postID, name: item.id, display_name: displayName }) }); }
export function videoPostID(value: string) { return value.match(/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/)?.[0] || ""; }
export function cachedVideo(payload: Record<string, unknown>): VideoItem { const url = String(payload.view_url || payload.url || ""); const id = String(payload.name || payload.task_id || crypto.randomUUID()); return { id, taskID: String(payload.task_id || id), url, prompt: "", progress: 100, status: "completed", postID: String(payload.post_id || videoPostID(url)), originalPostID: String(payload.original_post_id || ""), displayName: String(payload.display_name || payload.name || "视频"), createdAt: Number(payload.mtime_ms || Date.now()) }; }
