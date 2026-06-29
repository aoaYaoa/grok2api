const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const pagePath = path.join(
  projectRoot,
  'app/static/public/pages/pure_html_generator.html'
);

test('pure html generator page exists and targets compatibility endpoints', () => {
  const html = fs.readFileSync(pagePath, 'utf8');

  assert.match(html, /\/v1\/images\/generations/);
  assert.match(html, /\/v1\/videos/);
  assert.match(html, /pure_html_gen_base_url/);
  assert.match(html, /pure_html_gen_history/);
});

test('pure html generator page only exposes image sizes supported by local api', () => {
  const html = fs.readFileSync(pagePath, 'utf8');

  assert.match(html, /1280x720/);
  assert.match(html, /1792x1024/);
  assert.match(html, /1024x1792/);
  assert.match(html, /1024x1024/);
  assert.doesNotMatch(html, /1536x1024/);
  assert.doesNotMatch(html, /1024x1536/);
});
