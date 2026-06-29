const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const nsfwHtml = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/pages/nsfw.html'),
  'utf8'
);
const nsfwCss = fs.readFileSync(
  path.join(projectRoot, 'app/static/public/css/nsfw.css'),
  'utf8'
);

test('nsfw page keeps local upload controls in the main parameter form', () => {
  assert.match(nsfwHtml, /id="nsfwLocalUploadField"/);
  assert.match(
    nsfwHtml,
    /id="nsfwLocalUploadField"[\s\S]*id="selectVideoImageBtn"[\s\S]*id="clearVideoImageBtn"[\s\S]*id="videoImageFileInput"[\s\S]*id="ratioSelect"/
  );
  assert.match(nsfwCss, /\.nsfw-local-upload-field\b/);
});
