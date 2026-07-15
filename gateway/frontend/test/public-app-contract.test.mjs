import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

test("public React entry owns every legacy public page route", async () => {
  const paths = await import("../src/public/app/public-paths.mjs");
  assert.deepEqual(Object.values(paths.publicRoutePaths), [
    "/login",
    "/chat",
    "/imagine",
    "/imagine-workbench",
    "/video",
    "/nsfw",
    "/voice",
  ]);

  const router = await readFile(path.join(root, "src/public/app/router.tsx"), "utf8");
  for (const route of Object.values(paths.publicRoutePaths)) {
    const key = Object.entries(paths.publicRoutePaths).find(([, value]) => value === route)?.[0];
    assert.ok(key && router.includes(`publicRoutePaths.${key}`), `missing React route ${route}`);
  }
});

test("frontend has separate admin and public Vite builds", async () => {
  const packageJSON = JSON.parse(await readFile(path.join(root, "package.json"), "utf8"));
  assert.match(packageJSON.scripts.build, /build:admin/);
  assert.match(packageJSON.scripts.build, /build:public/);

  const publicHTML = await readFile(path.join(root, "public.html"), "utf8");
  assert.match(publicHTML, /src\/public-main\.tsx/);
  assert.doesNotMatch(publicHTML, /static\/public\/js|static\/common\/js/);
});

test("public source does not execute legacy imperative page scripts", async () => {
  const publicMain = await readFile(path.join(root, "src/public-main.tsx"), "utf8");
  assert.doesNotMatch(publicMain, /legacy|static\/public|\.html/);
});
