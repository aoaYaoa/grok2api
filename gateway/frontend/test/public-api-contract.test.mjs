import assert from "node:assert/strict";
import test from "node:test";

const contracts = await import("../src/public/api/contracts.mjs").catch(() => null);
const stream = await import("../src/public/api/sse-parser.mjs").catch(() => null);
const publicErrors = await import("../src/public/api/public-error.mjs").catch(() => null);
const imageCacheURL = await import("../src/public/features/image/cache-url.ts").catch(() => null);

test("public API contract preserves every active workspace endpoint", () => {
  assert.ok(contracts, "missing public API contract module");
  assert.deepEqual(contracts.publicEndpoints, {
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
    imageCache: "/v1/public/imagine/cache/list",
    imageCacheDelete: "/v1/public/imagine/cache/delete",
    videoStart: "/v1/public/video/start",
    videoStop: "/v1/public/video/stop",
    videoSSE: "/v1/public/video/sse",
    videoCache: "/v1/public/video/cache/list",
    videoCacheDelete: "/v1/public/video/cache/delete",
    videoRename: "/v1/public/video/rename",
    promptEnhance: "/v1/public/prompt/enhance",
    promptEnhanceStop: "/v1/public/prompt/enhance/stop",
    voiceToken: "/v1/public/voice/token",
    voiceSignal: "/v1/public/voice/signal",
  });
});

test("SSE parser preserves named events and chunk boundaries", () => {
  assert.ok(stream, "missing SSE parser module");
  const parser = stream.createSSEParser();
  assert.deepEqual(parser.push("event: progress\ndata: {\"progress\":4}"), []);
  assert.deepEqual(parser.push("\n\ndata: {\"type\":\"image\",\"b64_json\":\"abc\"}\n\n"), [
    { event: "progress", data: { progress: 4 } },
    { event: "message", data: { type: "image", b64_json: "abc" } },
  ]);
});

test("public key helpers keep compatibility with the legacy storage name", () => {
  assert.equal(contracts.publicKeyStorage, "grok2api_public_key");
  assert.deepEqual(contracts.authHeaders(" key "), { Authorization: "Bearer key" });
  assert.deepEqual(contracts.authHeaders(""), {});
});

test("public errors hide database details and upstream challenge HTML", () => {
  assert.ok(publicErrors, "missing public error sanitizer");
  assert.equal(
    publicErrors.sanitizePublicError("constraint failed: CHECK constraint failed: chk_media_jobs_input_json (275)"),
    "服务暂时无法处理该任务，请稍后重试",
  );
  assert.equal(
    publicErrors.sanitizePublicError("上传图片失败，上游返回 403: <!DOCTYPE html><title>Just a moment...</title>"),
    "上游安全验证暂时未通过，请稍后重试",
  );
});

test("cached image paths become absolute URLs before reuse", () => {
  assert.ok(imageCacheURL, "missing image cache URL normalizer");
  assert.equal(
    imageCacheURL.absoluteImageCacheURL("/v1/files/image/cached.jpg", "https://grok.uonoe.com"),
    "https://grok.uonoe.com/v1/files/image/cached.jpg",
  );
  assert.equal(
    imageCacheURL.absoluteImageCacheURL("https://cdn.example/image.png", "https://grok.uonoe.com"),
    "https://cdn.example/image.png",
  );
  assert.equal(
    imageCacheURL.absoluteImageCacheURL("/v1/files/image/cached.jpg", "http://localhost:18001"),
    "http://localhost:18001/v1/files/image/cached.jpg",
  );
});
