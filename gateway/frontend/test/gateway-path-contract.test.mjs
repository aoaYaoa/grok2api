import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import { matchRoutes } from "react-router-dom";
import { loadConfigFromFile } from "vite";

const frontendRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const gatewayPaths = await import("../src/app/gateway-paths.mjs").catch(() => null);

test("Vite exports the shared /gateway/ production base", async () => {
  const loadedConfig = await loadConfigFromFile(
    { command: "build", mode: "production", isSsrBuild: false, isPreview: false },
    path.join(frontendRoot, "vite.config.ts"),
    frontendRoot,
    undefined,
    undefined,
    "runner",
  ).catch(() => null);

  assert.ok(loadedConfig, "vite.config.ts must be loadable at runtime");
  assert.ok(gatewayPaths, "gateway paths must be exported as a runtime manifest");
  assert.equal(loadedConfig.config.base, gatewayPaths.gatewayViteBase);
});

test("nested gateway docs URLs resolve through the shared route manifest", () => {
  assert.ok(gatewayPaths, "gateway paths must be exported as a runtime manifest");

  const routes = [{ path: gatewayPaths.gatewayRoutePaths.docsEndpoint }];
  const matches = matchRoutes(
    routes,
    "/gateway/docs/chat/completions",
    gatewayPaths.gatewayBasename,
  );

  assert.ok(matches, "the nested docs URL must match a gateway route");
  assert.equal(matches.at(-1)?.route.path, gatewayPaths.gatewayRoutePaths.docsEndpoint);
  assert.deepEqual(matches.at(-1)?.params, { category: "chat", endpoint: "completions" });
});
