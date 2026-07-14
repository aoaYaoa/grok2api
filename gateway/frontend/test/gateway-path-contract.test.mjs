import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const viteConfig = await readFile(new URL("../vite.config.ts", import.meta.url), "utf8");
const routerSource = await readFile(new URL("../src/app/router.tsx", import.meta.url), "utf8");

test("Vite builds frontend assets below /gateway/", () => {
  assert.match(viteConfig, /defineConfig\(\{\s*base:\s*["']\/gateway\/["'],/);
});

test("the browser router resolves routes below /gateway", () => {
  assert.match(
    routerSource,
    /createBrowserRouter\([\s\S]*?,\s*\{\s*basename:\s*["']\/gateway["']\s*\}\s*\);/,
  );
});
