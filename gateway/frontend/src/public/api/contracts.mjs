export const publicKeyStorage = "grok2api_public_key";

export const publicEndpoints = Object.freeze({
  verify: "/v1/public/verify",
  models: "/v1/public/models",
  chat: "/v1/public/chat/completions",
  imagineConfig: "/v1/public/imagine/config",
  imagineStart: "/v1/public/imagine/start",
  imagineStop: "/v1/public/imagine/stop",
  imagineSSE: "/v1/public/imagine/sse",
  imagineWS: "/v1/public/imagine/ws",
  imagineEdit: "/v1/public/imagine/edit",
  workbenchEdit: "/v1/public/imagine/workbench/edit",
  parentPost: "/v1/public/imagine/parent-post",
  imageCache: "/v1/public/imagine/cache/list",
  videoStart: "/v1/public/video/start",
  videoStop: "/v1/public/video/stop",
  videoSSE: "/v1/public/video/sse",
  videoCache: "/v1/public/video/cache/list",
  videoRename: "/v1/public/video/rename",
  promptEnhance: "/v1/public/prompt/enhance",
  promptEnhanceStop: "/v1/public/prompt/enhance/stop",
  voiceToken: "/v1/public/voice/token",
  voiceSignal: "/v1/public/voice/signal",
});

export function authHeaders(key) {
  const value = String(key || "").trim();
  return value ? { Authorization: `Bearer ${value}` } : {};
}
