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
  const videoAPI = await readFile(path.join(root, "src/public/features/video/video-api.ts"), "utf8");
  const grid = await readFile(path.join(root, "src/public/components/video-grid.tsx"), "utf8");
  const persistentVideo = await readFile(path.join(root, "src/shared/components/persistent-video-preview.tsx"), "utf8");

  assert.match(nsfw, /imageStartLock/);
  assert.match(nsfw, /videoStartLock/);
  assert.match(nsfw, /video_extension_start_time: extendTime/);
  assert.match(nsfw, /source_task_id: extension\?\.taskID/);
  assert.match(nsfw, /extendLength/);
  assert.match(nsfw, /listCachedVideos/);
  assert.match(nsfw, /const \[cacheOpen, setCacheOpen\]/);
  assert.match(nsfw, /<VideoGrid videos=\{cachedVideos\}/);
  assert.match(nsfw, /type="number"/);
  assert.match(video, /onExtend=/);
  assert.match(video, /scrollIntoView/);
  assert.match(video, /source_task_id: active\.taskID/);
  assert.match(video, /extendLength/);
  assert.match(video, /disabled=\{!prompt\.trim\(\) && !references\.length\}/);
  assert.match(videoAPI, /matchAll/);
  assert.match(videoAPI, /matches\.at\(-1\)/);
  assert.match(videoAPI, /视频任务结束但未返回结果/);
  assert.match(grid, /onExtend\?:/);
  assert.match(persistentVideo, /<video[^>]*controls/);
  assert.match(grid, /item\.status === "failed" \? "失败"/);
  assert.doesNotMatch(grid, /onPlay=\{\(\) => onActivate/);
});

test("NSFW local references show a validated preview before task upload", async () => {
  const nsfw = await readFile(path.join(root, "src/public/pages/nsfw-page.tsx"), "utf8");

  assert.match(nsfw, /const \[localImage, setLocalImage\] = useState<UploadAsset \| null>\(null\)/);
  assert.match(nsfw, /accept="image\/jpeg,image\/png,image\/webp,image\/gif"/);
  assert.match(nsfw, /localImage\.data/);
  assert.match(nsfw, /alt=\{localImage\.name\}/);
  assert.match(nsfw, /setLocalImage\(null\)/);
  assert.match(nsfw, /上传参考图并创建任务/);
  assert.match(nsfw, /source_image_url: extension \? undefined : source/);
  assert.doesNotMatch(nsfw, /\n\s+image_url: extension \? undefined : source/);
  assert.doesNotMatch(nsfw, /已加载本地参考图/);
});

test("public app retires legacy service workers and PWA caches", async () => {
  const publicMain = await readFile(path.join(root, "src/public-main.tsx"), "utf8");
  const worker = await readFile(path.join(root, "public/sw.js"), "utf8");

  assert.match(publicMain, /navigator\.serviceWorker\.getRegistrations/);
  assert.match(publicMain, /registration\.unregister/);
  assert.match(publicMain, /scriptURL/);
  assert.match(publicMain, /\/sw\.js/);
  assert.match(publicMain, /caches\.keys/);
  assert.match(publicMain, /grok2api-pwa-/);
  assert.match(publicMain, /\.catch\(/);
  assert.doesNotMatch(worker, /client\.navigate/);
});

test("video cache dialog does not merge the whole cache into session history", async () => {
  const video = await readFile(path.join(root, "src/public/pages/video-page.tsx"), "utf8");
  const openCache = video.slice(video.indexOf("async function openCache"), video.indexOf("async function saveRename"));

  assert.match(video, /const \[cachedVideos, setCachedVideos\]/);
  assert.match(openCache, /setCachedVideos/);
  assert.doesNotMatch(openCache, /setVideos/);
  assert.match(video, /<VideoGrid videos=\{cachedVideos\}/);
});

test("video cards lazy-load visible media and keep selected rings inside cards", async () => {
  const grid = await readFile(path.join(root, "src/public/components/video-grid.tsx"), "utf8");
  const persistentVideo = await readFile(path.join(root, "src/shared/components/persistent-video-preview.tsx"), "utf8");
  const videoAPI = await readFile(path.join(root, "src/public/features/video/video-api.ts"), "utf8");
  const imageGrid = await readFile(path.join(root, "src/public/components/image-grid.tsx"), "utf8");

  assert.match(persistentVideo, /IntersectionObserver/);
  assert.match(persistentVideo, /#t=0\.001/);
  assert.match(persistentVideo, /preload=\{shouldLoad \? "auto" : "none"\}/);
  assert.match(persistentVideo, /poster=\{posterURL \|\| undefined\}/);
  assert.match(persistentVideo, /video-preview-placeholder/);
  assert.match(grid, /export function VideoPlayer/);
  assert.match(videoAPI, /posterURL/);
  assert.match(videoAPI, /data\.poster_url/);
  assert.match(grid, /ring-inset/);
  assert.match(imageGrid, /ring-inset/);
  assert.match(persistentVideo, /loadedVideoURLs/);
  assert.doesNotMatch(persistentVideo, /removeAttribute\("src"\)/);
  assert.doesNotMatch(persistentVideo, /setFrameReady\(false\)/);
});

test("admin shell and cache keep mobile controls and video previews stable", async () => {
  const shell = await readFile(path.join(root, "src/app/app-shell.tsx"), "utf8");
  const tabs = await readFile(path.join(root, "src/components/ui/tabs.tsx"), "utf8");
  const period = await readFile(path.join(root, "src/shared/components/period-selector.tsx"), "utf8");
  const segmented = await readFile(path.join(root, "src/shared/components/segmented-control.tsx"), "utf8");
  const dashboard = await readFile(path.join(root, "src/features/dashboard/dashboard-page.tsx"), "utf8");
  const cache = await readFile(path.join(root, "src/features/cache/cache-page.tsx"), "utf8");

  assert.match(shell, /sticky top-0/);
  assert.match(shell, /env\(safe-area-inset-top\)/);
  assert.match(tabs, /overflow-y-hidden/);
  assert.match(tabs, /items-stretch/);
  assert.match(tabs, /h-full/);
  assert.match(tabs, /border-transparent/);
  assert.match(tabs, /data-\[state=active\]:border-border/);
  assert.match(period, /SegmentedControl/);
  assert.match(segmented, /h-9/);
  assert.match(segmented, /items-stretch/);
  assert.match(segmented, /leading-none/);
  assert.match(segmented, /border-transparent/);
  assert.match(segmented, /border-border bg-background/);
  assert.match(segmented, /role="radiogroup"/);
  assert.match(segmented, /role="radio"/);
  assert.match(segmented, /aria-checked=\{selected\}/);
  assert.match(dashboard, /<SegmentedControl/);
  assert.doesNotMatch(dashboard, /inline-flex h-8 items-center rounded-md bg-muted/);
  assert.match(cache, /PersistentVideoPreview/);
  assert.match(cache, /thumbnailURL/);
});

test("failed video tasks are removed and reported as one task-group notice", async () => {
  const video = await readFile(path.join(root, "src/public/pages/video-page.tsx"), "utf8");
  const nsfw = await readFile(path.join(root, "src/public/pages/nsfw-page.tsx"), "utf8");
  const failures = await readFile(path.join(root, "src/public/features/video/video-failure-notice.ts"), "utf8");

  for (const source of [video, nsfw]) {
    assert.match(source, /useVideoFailureNotice/);
    assert.match(source, /beginVideoGroup\(ids\)/);
    assert.match(source, /finishVideoTask\(taskID/);
    assert.match(source, /items\.filter\(\(item\) => item\.taskID !== taskID\)/);
    assert.doesNotMatch(source, /status: update\.error \? "failed"/);
  }
  assert.match(failures, /pending: new Set\(taskIDs\)/);
  assert.match(failures, /个视频任务失败，已从列表移除/);
});

test("every visual prompt editor exposes the shared prompt enhancement action", async () => {
  const pages = {
    "imagine-page.tsx": 2,
    "workbench-page.tsx": 1,
    "video-page.tsx": 2,
    "nsfw-page.tsx": 4,
  };

  for (const [page, minimum] of Object.entries(pages)) {
    const source = await readFile(path.join(root, "src/public/pages", page), "utf8");
    const count = source.match(/<PromptEnhanceButton/g)?.length || 0;
    assert.ok(count >= minimum, `${page} must expose prompt enhancement for each visual prompt field`);
  }

  const enhancer = await readFile(path.join(root, "src/public/components/prompt-enhance-button.tsx"), "utf8");
  assert.match(enhancer, /publicEndpoints\.promptEnhance/);
  assert.match(enhancer, /优化中/);
  assert.match(enhancer, /valueRef\.current\.trim\(\) !== prompt/);
  assert.match(enhancer, /disabled=\{disabled \|\| loading \|\| !value\.trim\(\)\}/);
});

test("mobile dialogs, toasts, heading actions, and tabs keep controls inside their surfaces", async () => {
  const dialog = await readFile(path.join(root, "src/components/ui/dialog.tsx"), "utf8");
  const tabs = await readFile(path.join(root, "src/components/ui/tabs.tsx"), "utf8");
  const button = await readFile(path.join(root, "src/components/ui/button.tsx"), "utf8");
  const styles = await readFile(path.join(root, "src/index.css"), "utf8");

  assert.match(dialog, /data-slot="dialog-close"/);
  assert.match(dialog, /size-9/);
  assert.match(tabs, /overflow-x-auto/);
  assert.match(tabs, /shrink-0/);
  assert.match(button, /data-slot/);
  assert.match(styles, /\[data-sonner-toast\] \[data-close-button\]/);
  assert.match(styles, /:not\(\[role="tab"\]\)/);
  assert.match(styles, /:not\(\[role="radio"\]\)/);
  assert.match(styles, /:not\(\[data-slot="icon-button"\]\)/);
  assert.match(styles, /\.workspace-actions/);
  assert.doesNotMatch(styles, /workspace-heading > :last-child/);
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
