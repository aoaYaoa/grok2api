import assert from "node:assert/strict";
import test from "node:test";

const contracts = await import("../src/public/api/contracts.mjs").catch(() => null);
const stream = await import("../src/public/api/sse-parser.mjs").catch(() => null);

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
    parentPost: "/v1/public/imagine/parent-post",
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
