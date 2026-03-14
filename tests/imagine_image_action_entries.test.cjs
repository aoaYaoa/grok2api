const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const imagineJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/js/imagine.js'),
  'utf8'
);
const imagineCss = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/css/imagine.css'),
  'utf8'
);
const workbenchJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/js/imagine_workbench.js'),
  'utf8'
);
const workbenchHtml = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/pages/imagine_workbench.html'),
  'utf8'
);
const workbenchCss = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/css/imagine_workbench.css'),
  'utf8'
);
const promptEnhancerJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/common/js/prompt-enhancer.js'),
  'utf8'
);

test('imagine waterfall cards expose continue and outpaint prompt actions', () => {
  assert.match(imagineJs, /image-action-prompt/);
  assert.match(imagineJs, /image-continue-btn/);
  assert.match(imagineJs, /image-outpaint-btn/);
  assert.match(imagineJs, /IMAGINE_IMAGE_ACTION_PROMPT_KEY/);
  assert.match(imagineCss, /\.image-action-row\s*\{/);
});

test('imagine workbench history cards expose continue and outpaint prompt actions', () => {
  assert.match(workbenchJs, /history-image-action-prompt/);
  assert.match(workbenchJs, /history-image-continue-btn/);
  assert.match(workbenchJs, /history-image-outpaint-btn/);
  assert.match(workbenchJs, /WORKBENCH_IMAGE_ACTION_PROMPT_KEY/);
  assert.match(workbenchCss, /\.history-image-action-row\s*\{/);
});

test('image action prompt fields persist enhanced values too', () => {
  assert.match(imagineJs, /prompt-enhance-applied/);
  assert.match(workbenchJs, /prompt-enhance-applied/);
  assert.match(promptEnhancerJs, /prompt-enhance-applied/);
});

test('workbench image actions normalize source image URLs before submit and history reuse', () => {
  assert.match(workbenchJs, /const sourceImageUrl = pickSourceImageUrl\(/);
  assert.match(workbenchJs, /sourceImageUrl:\s*pickSourceImageUrl\(/);
});

test('imagine waterfall image cards wire continue and outpaint actions through card state', () => {
  assert.match(imagineJs, /if \(e\.target\.closest\('\.image-continue-btn'\)\)/);
  assert.match(imagineJs, /runWaterfallImageAction\(item, 'continue'\)/);
  assert.match(imagineJs, /const sourceImageUrl = resolveSourceImageByParentPostId\(/);
  assert.match(imagineJs, /item\.dataset\.sourceImageUrl = nextSourceImageUrl/);
});

test('imagine waterfall action buttons bind direct click listeners so row-level propagation does not swallow them', () => {
  assert.match(imagineJs, /continueBtn\.addEventListener\('click'/);
  assert.match(imagineJs, /outpaintBtn\.addEventListener\('click'/);
});

test('workbench source picking prefers local preview urls before legacy source urls', () => {
  assert.match(workbenchJs, /hit && hit\.image_url,[\s\S]*hit && hit\.source_image_url,/);
});

test('workbench merge mode suppresses parent chain when 2 or more references are present', () => {
  assert.match(workbenchJs, /const useReferenceMergeMode = references\.length >= 2;/);
  assert.match(workbenchJs, /if \(parentPostId && !useReferenceMergeMode\)/);
  assert.match(workbenchJs, /mode:\s*useReferenceMergeMode \? 'upload' :/);
});

test('imagine workbench exposes stepwise merge entry point', () => {
  assert.match(workbenchHtml, /stepwise-merge/);
  assert.match(workbenchJs, /startStepwiseMerge/);
});

test('imagine workbench exposes stepwise merge modal state', () => {
  assert.match(workbenchJs, /stepwiseMergeState/);
  assert.match(workbenchJs, /openStepwiseMergeModal/);
});


test('workbench preview sizing stays fixed and history enhancer controls are visible', () => {
  assert.match(workbenchCss, /--workbench-preview-height/);
  assert.match(workbenchCss, /\.preview-shell\s*\{[^}]*height:\s*var\(--workbench-preview-height\)/s);
  assert.match(workbenchCss, /\.history-prompt-enhance-wrap\s*>\s*\.prompt-enhance-actions/);
  assert.match(workbenchJs, /history-prompt-enhance-wrap/);
});

test('imagine waterfall actions include a local parentPostId extractor helper', () => {
  assert.match(imagineJs, /function extractParentPostIdFromText\(text\) \{/);
  assert.match(imagineJs, /const direct = extractParentPostIdFromText\(String\(item\.dataset\.parentPostId/);
});

test('imagine workbench current preview exposes a larger shell and click-to-open preview modal', () => {
  assert.match(workbenchHtml, /id="previewLightbox"/);
  assert.match(workbenchHtml, /id="previewLightboxImg"/);
  assert.match(workbenchJs, /const previewLightbox = document.getElementById\('previewLightbox'\);/);
  assert.match(workbenchJs, /function openPreviewLightbox\(url\)/);
  assert.match(workbenchJs, /item\.addEventListener\('click', \(\) => \{/);
  assert.match(workbenchCss, /--workbench-preview-height:\s*420px;/);
  assert.match(workbenchCss, /\.preview-lightbox\.active\s*\{/);
});

test('imagine workbench preview lightbox exposes a download action wired from the opened image', () => {
  assert.match(workbenchHtml, /id="previewLightboxDownload"/);
  assert.match(workbenchJs, /const previewLightboxDownload = document.getElementById\('previewLightboxDownload'\);/);
  assert.match(workbenchJs, /previewLightboxDownload\.href = safeUrl;/);
  assert.match(workbenchJs, /previewLightboxDownload\.setAttribute\('download',/);
  assert.match(workbenchCss, /\.preview-lightbox-actions\s*\{/);
});
