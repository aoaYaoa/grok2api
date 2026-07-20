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

	const creativeConsoleMatches = matchRoutes(
	  routerModule.gatewayRouterRoutes,
	  "/gateway/creative-console",
	  routerModule.gatewayRouterOptions.basename,
	);
	assert.ok(creativeConsoleMatches, "the Creative Console URL must match the actual router configuration");
	assert.equal(creativeConsoleMatches.at(-1)?.route.path, gatewayPaths.gatewayRoutePaths.creativeConsole);
  } finally {
    await server.close();
    await rm(cacheDir, { recursive: true, force: true });
  }
});

test("v3.0.5 Creative Console is wired into the customized admin shell", async () => {
  const packageJSON = JSON.parse(await readFile(path.join(frontendRoot, "package.json"), "utf8"));
  const paths = await readFile(path.join(frontendRoot, "src/app/gateway-paths.mjs"), "utf8");
  const router = await readFile(path.join(frontendRoot, "src/app/router.tsx"), "utf8");
  const deferred = await readFile(path.join(frontendRoot, "src/app/deferred-pages.tsx"), "utf8");
  const shell = await readFile(path.join(frontendRoot, "src/app/app-shell.tsx"), "utf8");
  const api = await readFile(path.join(frontendRoot, "src/features/creative-console/creative-console-api.ts"), "utf8").catch(() => "");
  const page = await readFile(path.join(frontendRoot, "src/features/creative-console/creative-console-page.tsx"), "utf8").catch(() => "");

  assert.equal(packageJSON.dependencies.marked, "^18.0.6");
  assert.equal(packageJSON.dependencies["@shadcn/react"], "^0.2.1");
  assert.match(paths, /creativeConsole:\s*"\/creative-console"/);
  assert.match(router, /gatewayRoutePaths\.creativeConsole/);
  assert.match(deferred, /DeferredCreativeConsolePage/);
  assert.match(shell, /nav\.creativeConsole/);
  assert.match(api, /fetch\("\/v1\/responses"/);
  assert.match(api, /response\.output_text\.delta/);
  assert.match(page, /import \{ marked \} from "marked"/);
  assert.match(page, /reasoningEffort/);
  assert.match(page, /webSearch/);
  assert.match(page, /xSearch/);
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

test("account batch workflows expose quota and conversion strategy controls", async () => {
  const api = await readFile(path.join(frontendRoot, "src/features/accounts/accounts-api.ts"), "utf8");
  const page = await readFile(path.join(frontendRoot, "src/features/accounts/accounts-page.tsx"), "utf8");
  const translations = await readFile(path.join(frontendRoot, "src/shared/i18n/index.ts"), "utf8");

  assert.match(api, /export function refreshAccountsQuota/);
  assert.match(api, /\/accounts\/batch\/refresh-quotas/);
  assert.match(page, /refreshAccountsQuota\(\[\.\.\.selected\], provider\)/);
  assert.match(page, /const \[conversionStrategy, setConversionStrategy\]/);
  assert.match(page, /const \[webConsoleSyncStrategy, setWebConsoleSyncStrategy\]/);
  assert.match(page, /strategy: conversionStrategy/);
  assert.match(page, /strategy: webConsoleSyncStrategy/);
  assert.match(translations, /accountTaskBatchSize: "单次全量任务账号数"/);
  assert.match(translations, /accountTaskBatchSize: "Accounts per all-task batch"/);
});

test("web quota cells show official remaining capacity and disclose missing media quota", async () => {
  const quota = await readFile(path.join(frontendRoot, "src/features/accounts/account-quota.tsx"), "utf8");
  const translations = await readFile(path.join(frontendRoot, "src/shared/i18n/index.ts"), "utf8");

  assert.match(quota, /formatNumber\(window\.remaining, locale, 0\).*formatNumber\(window\.total, locale, 0\)/s);
  assert.match(quota, /window\.remaining \/ window\.total \* 100/);
  assert.doesNotMatch(quota, /window\.total - window\.remaining/);
  assert.match(quota, /const remainingPercent = Math\.max\(0, 100 - usedPercent\)/);
  assert.match(quota, /formatNumber\(remainingPercent, locale, 1\)/);
  assert.match(quota, /width: `\$\{remainingPercent\}%`/);
  assert.match(quota, /officialChatQuotaWindow/);
  assert.match(quota, /mediaWeeklyQuotaUnavailable/);
  assert.match(quota, /window\.syncedAt/);
  assert.match(translations, /officialChatQuotaWindow: "官方 2 小时聊天额度"/);
  assert.match(translations, /mediaWeeklyQuotaUnavailable: "媒体周额度未同步"/);
  assert.match(translations, /officialChatQuotaWindow: "Official 2-hour chat quota"/);
  assert.match(translations, /mediaWeeklyQuotaUnavailable: "Media weekly quota unavailable"/);
});
