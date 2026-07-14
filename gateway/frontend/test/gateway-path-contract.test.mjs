import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import { matchRoutes } from "react-router-dom";
import { createServer, loadConfigFromFile } from "vite";

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

test("the actual router configuration resolves nested gateway docs URLs", async () => {
  const loadedConfig = await loadConfigFromFile(
    { command: "serve", mode: "test", isSsrBuild: true, isPreview: false },
    path.join(frontendRoot, "vite.config.ts"),
    frontendRoot,
    undefined,
    undefined,
    "runner",
  );
  assert.ok(loadedConfig, "vite.config.ts must load for router tests");

  const server = await createServer({
    ...loadedConfig.config,
    configFile: false,
    root: frontendRoot,
    cacheDir: "/private/tmp/grok2api-gateway-router-test",
    server: { middlewareMode: true, hmr: false },
    appType: "custom",
  });

  try {
    const routerModule = await server.ssrLoadModule("/src/app/router.tsx");
    const matches = matchRoutes(
      routerModule.gatewayRouterRoutes,
      "/gateway/docs/chat/completions",
      routerModule.gatewayRouterOptions.basename,
    );

    assert.ok(matches, "the nested docs URL must match the actual router configuration");
    assert.equal(matches.at(-1)?.route.path, gatewayPaths.gatewayRoutePaths.docsEndpoint);
    assert.deepEqual(matches.at(-1)?.params, { category: "chat", endpoint: "completions" });
  } finally {
    await server.close();
  }
});
