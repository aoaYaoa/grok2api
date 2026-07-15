import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
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

  const cacheDir = await mkdtemp(path.join(os.tmpdir(), "grok2api-gateway-router-"));
  const server = await createServer({
    ...loadedConfig.config,
    configFile: false,
    root: frontendRoot,
    cacheDir,
    server: { middlewareMode: true, hmr: false },
    appType: "custom",
  });

  try {
    assert.ok(server.config.cacheDir.startsWith(os.tmpdir()), "router test cache must use the platform temp directory");
    const routerModule = await server.ssrLoadModule("/src/app/router.tsx");
    const matches = matchRoutes(
      routerModule.gatewayRouterRoutes,
      "/gateway/docs/chat/completions",
      routerModule.gatewayRouterOptions.basename,
    );

    assert.ok(matches, "the nested docs URL must match the actual router configuration");
    assert.equal(matches.at(-1)?.route.path, gatewayPaths.gatewayRoutePaths.docsEndpoint);
    assert.deepEqual(matches.at(-1)?.params, { category: "chat", endpoint: "completions" });

	const cacheMatches = matchRoutes(
	  routerModule.gatewayRouterRoutes,
	  "/gateway/cache",
	  routerModule.gatewayRouterOptions.basename,
	);
	assert.ok(cacheMatches, "the React cache URL must match the actual router configuration");
	assert.equal(cacheMatches.at(-1)?.route.path, gatewayPaths.gatewayRoutePaths.cache);
  } finally {
    await server.close();
    await rm(cacheDir, { recursive: true, force: true });
  }
});

test("admin action menu links back to the public workspace", async () => {
  const shell = await readFile(path.join(frontendRoot, "src/app/app-shell.tsx"), "utf8");
  const translations = await readFile(path.join(frontendRoot, "src/shared/i18n/index.ts"), "utf8");

  assert.match(shell, /publicRoutePaths\.chat/);
  assert.match(shell, /href=\{publicRoutePaths\.chat\}/);
  assert.match(shell, /shell\.backToWorkspace/);
  assert.match(translations, /backToWorkspace: "返回工作台"/);
  assert.match(translations, /backToWorkspace: "Back to workspace"/);
});
