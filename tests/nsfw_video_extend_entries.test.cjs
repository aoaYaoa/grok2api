const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const nsfwHtml = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/pages/nsfw.html'),
  'utf8'
);
const videoHtml = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/pages/video.html'),
  'utf8'
);
const videoJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/js/video.js'),
  'utf8'
);
const videoCss = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/css/video.css'),
  'utf8'
);
const nsfwJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/js/nsfw.js'),
  'utf8'
);
const promptEnhancerJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/common/js/prompt-enhancer.js'),
  'utf8'
);

test('nsfw page exposes a main extend button and mobile proxy entry', () => {
  assert.match(nsfwHtml, /id="extendVideoBtn"/);
  assert.match(nsfwHtml, /data-proxy-click="#extendVideoBtn"/);
});

test('video workstation source contains quick extend selectors', () => {
  assert.match(videoJs, /cache-video-extend/);
  assert.match(videoJs, /video-extend/);
});

test('nsfw workstation source contains extend action selectors', () => {
  assert.match(nsfwJs, /video-extend-btn/);
  assert.match(nsfwJs, /runVideoExtend/);
});

test('nsfw extend prompt no longer persists drafts but still supports enhancer events', () => {
  assert.match(nsfwJs, /video-extend-prompt/);
  assert.match(nsfwJs, /promptOverride/);
  assert.doesNotMatch(nsfwJs, /NSFW_VIDEO_EXTEND_PROMPT_KEY/);
  assert.match(promptEnhancerJs, /prompt-enhance-applied/);
});

test('video and nsfw extend flows expose timeline-based start-time selectors', () => {
  assert.match(videoHtml, /id="editTimeline"/);
  assert.match(videoJs, /video_extension_start_time:\s*extensionStartTime/);
  assert.match(videoJs, /const extensionStartTime = Math\.max\(0, lockedTimestampMs \/ 1000\);/);
  assert.match(nsfwHtml, /id="videoExtendTimeline"/);
  assert.match(nsfwJs, /video_extension_start_time:\s*extendStartTime/);
  assert.match(nsfwJs, /const extendStartTime = Math\.max\(0, state\.videoExtendLockedMs \/ 1000\);/);
});

test('video and nsfw extend panels show the selected start time next to the extend label', () => {
  assert.match(videoHtml, /id="editTimeInline"/);
  assert.match(videoJs, /editTimeInline/);
  assert.match(nsfwHtml, /id="videoExtendTimeInline"/);
  assert.match(nsfwJs, /videoExtendTimeInline/);
});

test('video workstation uses flex title rows and updated player sizes', () => {
  assert.match(videoJs, /header\.appendChild\(actions\);/);
  assert.match(videoJs, /actions\.className = 'video-item-actions';/);
  assert.match(videoCss, /\.history-panel \.video-item-bar\s*\{[\s\S]*display:\s*flex;/);
  assert.match(videoCss, /\.history-panel \.video-item-bar\s*\{[\s\S]*justify-content:\s*space-between;/);
  assert.match(videoCss, /background:\s*#ffffff\s*!important;/);
  assert.match(videoCss, /color:\s*#0f172a\s*!important;/);
  assert.match(videoCss, /width:\s*min\(100%, 630px\);/);
  assert.match(videoCss, /height:\s*450px;/);
  assert.match(videoCss, /max-height:\s*240px;/);
  assert.match(videoCss, /max-height:\s*160px;/);
});

test('video extend placeholders do not stay stuck in generating state after DONE without a parsed link', () => {
  assert.match(videoJs, /if \(raw === '\[DONE\]'\) \{[\s\S]*spliceRun\.failedPlaceholders\.add\(tid\);/);
  assert.match(videoJs, /延长失败: 未返回视频链接/);
});

test('video preview placeholders show percentage progress during generation and extend', () => {
  assert.match(videoJs, /video-item-placeholder">进度 0%<\/div>/);
  assert.match(videoJs, /setPreviewPlaceholderText\(taskState\.previewItem, `进度 \$\{value\}%`\);/);
  assert.match(videoJs, /setPreviewPlaceholderText\(item, `进度 \$\{lastValue\}%`\);/);
});

test('video extend startup failures centralize cleanup so timers and loading state stop', () => {
  assert.match(videoJs, /function finalizeExtendRun\(/);
  assert.match(videoJs, /function finalizeExtendRun\([\s\S]*stopElapsedTimer\(\);/);
  assert.match(videoJs, /function finalizeExtendRun\([\s\S]*setIndeterminate\(false\);/);
  assert.match(videoJs, /catch \(e\) \{[\s\S]*finalizeExtendRun\(spliceRun, \{ statusState: 'error', statusText: '延长失败' \}\);/);
});

test('video extend sse connection errors are recorded as failures before finalizing', () => {
  assert.match(videoJs, /source\.onerror = \(err\) => \{[\s\S]*spliceRun\.failedReasons\.push\('连接异常'\);/);
  assert.match(videoJs, /source\.onerror = \(err\) => \{[\s\S]*spliceRun\.failedPlaceholders\.add\(tid\);/);
  assert.match(videoJs, /source\.onerror = \(err\) => \{[\s\S]*setPreviewTitle\(item, '延长失败: 连接异常'\);/);
});

test('video extend finish_reason stop without a parsed video link is treated as a failed run', () => {
  assert.match(videoJs, /const choice = parsed\.choices && parsed\.choices\[0\];[\s\S]*if \(choice && choice\.finish_reason === 'stop'\) \{/);
  assert.match(videoJs, /if \(choice && choice\.finish_reason === 'stop'\) \{[\s\S]*if \(!taskState\.videoUrl\) \{/);
  assert.match(videoJs, /if \(choice && choice\.finish_reason === 'stop'\) \{[\s\S]*spliceRun\.failedReasons\.push\('未返回视频链接'\);/);
  assert.match(videoJs, /if \(choice && choice\.finish_reason === 'stop'\) \{[\s\S]*source\.close\(\);[\s\S]*checkAllExtendDone\(spliceRun\);/);
});
