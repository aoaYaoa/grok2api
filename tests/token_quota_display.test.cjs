const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const projectRoot = path.resolve(__dirname, '..');
const tokenJs = fs.readFileSync(
  path.join(projectRoot, 'app/static/admin/js/token.js'),
  'utf8'
);

function loadTokenAdminContext(elements = {}) {
  const context = {
    window: {},
    document: {
      getElementById(id) {
        return elements[id] || null;
      },
      querySelectorAll() {
        return [];
      },
      addEventListener() {},
      createDocumentFragment() {
        return { appendChild() {} };
      },
      createElement() {
        return {
          dataset: {},
          className: '',
          innerHTML: '',
          innerText: '',
          appendChild() {},
          classList: { add() {}, remove() {}, toggle() {} },
        };
      },
    },
  };
  vm.createContext(context);
  vm.runInContext(tokenJs, context);
  return context;
}

test('token quota object is normalized and rendered without object strings', () => {
  const context = loadTokenAdminContext();

  const quota = context.normalizeQuota({
    auto: { remaining: 85, total: 139 },
    fast: { remaining: 139, total: 139 },
    expert: { remaining: 139, total: 139 },
    grok_4_3: { remaining: 0, total: 0 },
  }, 'ssoSuper');

  assert.equal(context.quotaRemaining(quota, 'auto'), 85);
  assert.equal(context.quotaRemaining(quota, 'fast'), 139);
  assert.equal(context.quotaRemaining(quota, 'expert'), 139);

  const html = context.renderQuotaPills(quota);
  assert.match(html, /Auto:85/);
  assert.match(html, /Fast:139/);
  assert.match(html, /Expert:139/);
  assert.doesNotMatch(html, /\[object Object\]/);
});

test('weekly product quota displays remaining percentages including unused chat', () => {
  const context = loadTokenAdminContext();
  const quota = context.normalizeQuota({
    weekly: {
      remaining: 0,
      total: 10000,
      breakdown: [
        { product_code: 4, usage_percent: 0 },
        { product_code: 5, usage_percent: 100 },
      ],
    },
  }, 'ssoSuper');

  assert.equal(context.quotaProductRemaining(quota, 4), 100);
  assert.equal(context.quotaProductRemaining(quota, 5), 0);
  const html = context.renderQuotaPills(quota);
  assert.match(html, /Chat:100%/);
  assert.match(html, /Imagine:0%/);
});

test('token stats aggregate quota objects without NaN', () => {
  const text = {};
  const elements = new Proxy({}, {
    get(target, id) {
      if (!target[id]) {
        target[id] = {
          set innerText(value) {
            text[id] = value;
          },
          get innerText() {
            return text[id];
          },
        };
      }
      return target[id];
    },
  });
  const context = loadTokenAdminContext(elements);

  context.processTokens({
    ssoSuper: [{
      token: 'token-a',
      status: 'active',
      quota: {
        auto: { remaining: 85, total: 139 },
        fast: { remaining: 139, total: 139 },
        expert: { remaining: 139, total: 139 },
        grok_4_3: { remaining: 0, total: 0 },
      },
      use_count: 7,
    }],
  });
  context.updateStats();

  assert.equal(text['stat-chat-quota'], '85');
  assert.equal(text['stat-image-quota'], '139');
  assert.equal(text['stat-video-quota'], '139');
  assert.equal(text['stat-total-calls'], '7');
  assert.notEqual(text['stat-image-quota'], 'NaN');
});

test('editing only token metadata preserves per-mode quota values', () => {
  const context = loadTokenAdminContext();
  const currentQuota = context.normalizeQuota({
    auto: { remaining: 129, total: 140 },
    fast: { remaining: 140, total: 140 },
    expert: { remaining: 140, total: 140 },
  }, 'ssoSuper');

  const editedQuota = context.quotaForEdit(
    currentQuota,
    'ssoSuper',
    'ssoSuper',
    129,
  );

  assert.equal(context.quotaRemaining(editedQuota, 'auto'), 129);
  assert.equal(context.quotaRemaining(editedQuota, 'fast'), 140);
  assert.equal(context.quotaRemaining(editedQuota, 'expert'), 140);
});

test('missing heavy quota is not replaced with a synthetic allowance', () => {
  const context = loadTokenAdminContext();
  const quota = context.normalizeQuota({
    auto: { remaining: 400, total: 400 },
    fast: { remaining: 400, total: 400 },
    expert: { remaining: 400, total: 400 },
    heavy: null,
  }, 'ssoHeavy');

  assert.equal(context.quotaRemaining(quota, 'heavy'), 0);
  assert.match(context.renderQuotaPills(quota), /Heavy:未返回/);
});

test('empty upstream quota shows not returned instead of tier defaults', () => {
  const context = loadTokenAdminContext();
  const quota = context.normalizeQuota({}, 'ssoHeavy');
  const html = context.renderQuotaPills(quota);

  assert.equal(context.quotaRemaining(quota, 'auto'), 0);
  assert.match(html, /Auto:未返回/);
  assert.match(html, /Fast:未返回/);
  assert.match(html, /Expert:未返回/);
  assert.match(html, /Heavy:未返回/);
  assert.match(html, /G4:未返回/);
  assert.doesNotMatch(html, /Auto:400/);
});

test('quota totals include active tokens only', () => {
  const text = {};
  const elements = new Proxy({}, {
    get(target, id) {
      if (!target[id]) {
        target[id] = {
          set innerText(value) { text[id] = value; },
          get innerText() { return text[id]; },
        };
      }
      return target[id];
    },
  });
  const context = loadTokenAdminContext(elements);

  context.processTokens({
    ssoSuper: [
      { token: 'active', status: 'active', quota: { auto: { remaining: 80 } } },
      { token: 'disabled', status: 'disabled', quota: { auto: { remaining: 140 } } },
      { token: 'cooling', status: 'cooling', quota: { auto: { remaining: 140 } } },
    ],
  });
  context.updateStats();

  assert.equal(text['stat-chat-quota'], '80');
});
