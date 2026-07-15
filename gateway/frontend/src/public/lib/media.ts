export type UploadAsset = { id: string; name: string; mime: string; data: string };

export function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result || "")); reader.onerror = () => reject(reader.error || new Error("读取文件失败")); reader.readAsDataURL(file); });
}

export async function filesToAssets(files: FileList | File[], limit = 8) {
  return Promise.all(Array.from(files).slice(0, limit).map(async (file) => ({ id: crypto.randomUUID(), name: file.name, mime: file.type, data: await fileToDataURL(file) })));
}

export function downloadURL(url: string, name: string) { const anchor = document.createElement("a"); anchor.href = url; anchor.download = name; anchor.rel = "noreferrer"; document.body.appendChild(anchor); anchor.click(); anchor.remove(); }
export function extractParentPostID(value: string) { return value.trim().match(/[0-9a-fA-F]{8}-[0-9a-fA-F-]{24,28}/)?.[0] || ""; }
export function imageSource(payload: Record<string, unknown>) { const raw = String(payload.current_source_image_url || payload.source_image_url || payload.image_url || payload.url || payload.image || ""); if (raw) return raw; const base64 = String(payload.b64_json || ""); return base64 ? `data:${String(payload.mime_type || "image/jpeg")};base64,${base64}` : ""; }
export function videoURLFromText(value: string) { return value.match(/\[video\]\(([^)]+)\)/)?.[1] || value.match(/https?:\/\/[^\s)]+\.mp4(?:\?[^\s)]*)?/i)?.[0] || ""; }
