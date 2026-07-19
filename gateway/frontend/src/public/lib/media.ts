export type UploadAsset = { id: string; name: string; mime: string; data: string; requestData?: string };

const heicMIMETypes = new Set(["image/heic", "image/heif"]);
const imageExtensions = /\.(?:avif|gif|heic|heif|jpe?g|png|webp)$/i;

export function isHEICFile(file: File) {
  return heicMIMETypes.has(file.type.toLowerCase()) || /\.(?:heic|heif)$/i.test(file.name);
}

export function isImageUploadFile(file: File) {
  return file.type.toLowerCase().startsWith("image/") || imageExtensions.test(file.name);
}

export function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result || "")); reader.onerror = () => reject(reader.error || new Error("读取文件失败")); reader.readAsDataURL(file); });
}

async function normalizeUploadFile(file: File) {
  if (!isHEICFile(file)) return file;
  try {
    const { default: heic2any } = await import("heic2any");
    const converted = await heic2any({ blob: file, toType: "image/jpeg", quality: 0.92 });
    const blob = Array.isArray(converted) ? converted[0] : converted;
    if (!(blob instanceof Blob) || blob.size === 0) throw new Error("转换结果为空");
    const name = /\.(?:heic|heif)$/i.test(file.name) ? file.name.replace(/\.(?:heic|heif)$/i, ".jpg") : `${file.name}.jpg`;
    return new File([blob], name, { type: "image/jpeg", lastModified: file.lastModified });
  } catch {
    throw new Error("HEIC 图片转换失败，请改用 JPEG 或 PNG 后重试");
  }
}

export async function filesToAssets(files: FileList | File[], limit = 8) {
  return Promise.all(Array.from(files).slice(0, limit).map(async (file) => {
    const normalized = await normalizeUploadFile(file);
    return { id: crypto.randomUUID(), name: normalized.name, mime: normalized.type, data: await fileToDataURL(normalized) };
  }));
}

export function downloadURL(url: string, name: string) { const anchor = document.createElement("a"); anchor.href = url; anchor.download = name; anchor.rel = "noreferrer"; document.body.appendChild(anchor); anchor.click(); anchor.remove(); }
export function extractParentPostID(value: string) { return value.trim().match(/[0-9a-fA-F]{8}-[0-9a-fA-F-]{24,28}/)?.[0] || ""; }
export function imageSource(payload: Record<string, unknown>) { const raw = String(payload.current_source_image_url || payload.source_image_url || payload.image_url || payload.url || payload.image || ""); if (raw) return raw; const base64 = String(payload.b64_json || ""); return base64 ? `data:${String(payload.mime_type || "image/jpeg")};base64,${base64}` : ""; }
export function videoURLFromText(value: string) { return value.match(/\[video\]\(([^)]+)\)/)?.[1] || value.match(/https?:\/\/[^\s)]+\.mp4(?:\?[^\s)]*)?/i)?.[0] || ""; }
