import { publicEndpoints } from "@/public/api/contracts.mjs";
import { sanitizePublicError } from "@/public/api/public-error.mjs";
import { createSSEParser, type SSEFrame } from "@/public/api/sse-parser.mjs";

export class PublicAPIError extends Error {
  constructor(public readonly status: number, message: string) { super(message); }
}

function errorMessage(value: unknown, fallback: string) {
  if (!value || typeof value !== "object") return sanitizePublicError(typeof value === "string" ? value : "", fallback);
  const payload = value as Record<string, unknown>;
  const error = payload.error;
  if (typeof payload.detail === "string") return sanitizePublicError(payload.detail, fallback);
  if (typeof error === "string") return sanitizePublicError(error, fallback);
  if (error && typeof error === "object" && typeof (error as Record<string, unknown>).message === "string") return sanitizePublicError(String((error as Record<string, unknown>).message), fallback);
  return sanitizePublicError("", fallback);
}

export async function publicFetch<T>(key: string, input: RequestInfo | URL, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (key) headers.set("Authorization", `Bearer ${key}`);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const response = await fetch(input, { ...init, headers });
  const text = await response.text();
  let payload: unknown = null;
  if (text) {
    try { payload = JSON.parse(text); } catch { payload = text; }
  }
  if (!response.ok) throw new PublicAPIError(response.status, errorMessage(payload, `${response.status} ${response.statusText}`));
  return payload as T;
}

export async function publicSSE<T>(key: string, url: string, onFrame: (frame: SSEFrame<T>) => void, signal?: AbortSignal) {
  const headers = new Headers({ Accept: "text/event-stream" });
  if (key) headers.set("Authorization", `Bearer ${key}`);
  const response = await fetch(url, { headers, signal });
  if (!response.ok || !response.body) {
    const text = await response.text();
    throw new PublicAPIError(response.status, sanitizePublicError(text, `${response.status} ${response.statusText}`));
  }
  const parser = createSSEParser();
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  while (true) {
    const { done, value } = await reader.read();
    for (const frame of parser.push(decoder.decode(value || new Uint8Array(), { stream: !done }))) onFrame(frame as SSEFrame<T>);
    if (done) break;
  }
}

export async function publicSSERequest<T>(key: string, url: string, init: RequestInit, onFrame: (frame: SSEFrame<T>) => void, signal?: AbortSignal) {
  const headers = new Headers(init.headers);
  headers.set("Accept", "text/event-stream");
  if (key) headers.set("Authorization", `Bearer ${key}`);
  if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const response = await fetch(url, { ...init, headers, signal });
  if (!response.ok || !response.body) throw new PublicAPIError(response.status, sanitizePublicError(await response.text(), `${response.status} ${response.statusText}`));
  const parser = createSSEParser(); const reader = response.body.getReader(); const decoder = new TextDecoder();
  while (true) { const { done, value } = await reader.read(); for (const frame of parser.push(decoder.decode(value || new Uint8Array(), { stream: !done }))) onFrame(frame as SSEFrame<T>); if (done) break; }
}

export { publicEndpoints };
