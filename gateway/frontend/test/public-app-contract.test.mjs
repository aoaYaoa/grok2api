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

test("NSFW and video workspaces guard duplicate starts and expose timeline extension", async () => {
  const nsfw = await readFile(path.join(root, "src/public/pages/nsfw-page.tsx"), "utf8");
  const video = await readFile(path.join(root, "src/public/pages/video-page.tsx"), "utf8");
  const grid = await readFile(path.join(root, "src/public/components/video-grid.tsx"), "utf8");

  assert.match(nsfw, /imageStartLock/);
  assert.match(nsfw, /videoStartLock/);
  assert.match(nsfw, /video_extension_start_time: extendTime/);
  assert.match(nsfw, /type="number"/);
  assert.match(video, /onExtend=/);
  assert.match(video, /scrollIntoView/);
  assert.match(video, /disabled=\{!prompt\.trim\(\) && !references\.length\}/);
  assert.match(grid, /onExtend\?:/);
});

test("media workspaces keep controls on the left and results on the right", async () => {
  const pages = [
    "imagine-page.tsx",
    "workbench-page.tsx",
    "video-page.tsx",
    "nsfw-page.tsx",
    "voice-page.tsx",
  ];

  for (const page of pages) {
    const source = await readFile(path.join(root, "src/public/pages", page), "utf8");
    const controls = source.indexOf("workspace-controls");
    const results = source.indexOf("workspace-results");

    assert.match(source, /workspace-split/, `${page} must use the shared workspace split`);
    assert.ok(controls >= 0, `${page} must mark its controls column`);
    assert.ok(results > controls, `${page} must render controls before results`);
  }
});

test("mobile chat and more menu use compact touch controls", async () => {
  const shell = await readFile(path.join(root, "src/public/components/public-shell.tsx"), "utf8");
  const chat = await readFile(path.join(root, "src/public/pages/chat-page.tsx"), "utf8");
  const sheet = await readFile(path.join(root, "src/components/ui/sheet.tsx"), "utf8");
  const styles = await readFile(path.join(root, "src/index.css"), "utf8");

  assert.match(shell, /SheetContent side="bottom"/);
  assert.match(shell, /rounded-t-lg/);
  assert.match(shell, /h-14/);
  assert.match(chat, /min-h-12/);
  assert.match(chat, /rounded-2xl/);
  assert.match(sheet, /data-slot="sheet-close"/);
  assert.match(sheet, /focus-visible:ring-2/);
  assert.match(styles, /button:not\(\[role="switch"\]\)/);
});

test("light theme uses ChatGPT-style neutral surface tokens without green", async () => {
  const styles = await readFile(path.join(root, "src/index.css"), "utf8");
  const dashboard = await readFile(path.join(root, "src/features/dashboard/dashboard-page.tsx"), "utf8");

  assert.match(styles, /--background: #ffffff/);
  assert.match(styles, /--foreground: #0d0d0d/);
  assert.match(styles, /--sidebar: #f9f9f9/);
  assert.match(styles, /--secondary: #f4f4f4/);
  assert.match(styles, /--border: #e5e5e5/);
  assert.doesNotMatch(styles, /#10a37f|#19c37d|emerald|teal|green-/i);
  assert.doesNotMatch(`${styles}\n${dashboard}`, /oklch\([^)]* (?:1[2-9]\d|2[0-2]\d)\)/);
});
