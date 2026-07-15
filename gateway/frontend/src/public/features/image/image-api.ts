import { publicEndpoints, publicFetch, publicSSE, publicSSERequest } from "@/public/api/client";
import { imageSource } from "@/public/lib/media";

export type GeneratedImage = { id: string; url: string; prompt: string; parentPostID: string; sourceURL: string; elapsedMS?: number; createdAt: number };
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

export async function resolveParentPost(key: string, value: string) { return publicFetch<Record<string, unknown>>(key, `${publicEndpoints.parentPost}?parent_post_id=${encodeURIComponent(value)}`); }
