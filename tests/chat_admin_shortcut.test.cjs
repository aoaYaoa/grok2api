const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const helper = require(path.join(projectRoot, 'app/static/public/js/chat-admin-shortcut.js'));
const headerHtml = fs.readFileSync(
  path.join(projectRoot, 'app/static/common/html/public-header.html'),
  'utf8'
);

test('public header does not expose a visible admin entry', () => {
  assert.equal(headerHtml.includes('管理后台'), false);
  assert.equal(headerHtml.includes('/admin/login'), false);
});

test('hidden admin command matches exact trimmed shortcut', () => {
  assert.equal(helper.isHiddenAdminCommand(' #admin '), true);
  assert.equal(helper.isHiddenAdminCommand('#ADMIN'), false);
  assert.equal(helper.isHiddenAdminCommand('进入后台'), false);
});

test('hidden admin command redirects locally without server request', () => {
  let redirected = '';
  const handled = helper.maybeHandleHiddenAdminCommand('#admin', (url) => {
    redirected = url;
  });
  assert.equal(handled, true);
  assert.equal(redirected, '/admin/login');
});
