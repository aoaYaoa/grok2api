const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const sw = fs.readFileSync(path.join(__dirname, '..', 'app/static/public/sw.js'), 'utf8');

test('service worker cache version is bumped after public header/admin entry changes', () => {
  assert.match(sw, /const CACHE_NAME = `\$\{CACHE_PREFIX\}v3`;/);
});
