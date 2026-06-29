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

test('token admin page can refresh page-size labels without global i18n', () => {
  const option = { value: '20', textContent: '' };
  const context = {
    window: {},
    document: {
      getElementById(id) {
        if (id === 'page-size') return { options: [option] };
        return null;
      },
      querySelectorAll() {
        return [];
      },
    },
  };

  vm.createContext(context);
  vm.runInContext(tokenJs, context);

  assert.doesNotThrow(() => context.refreshPageSizeOptionsI18n());
  assert.equal(option.textContent, '20 / 页');
});

test('token admin init still loads tokens without global i18n', async () => {
  const option = { value: '50', textContent: '' };
  const fetchUrls = [];
  const context = {
    window: {},
    ensureAdminKey: async () => 'admin-key',
    buildAuthHeaders: () => ({ Authorization: 'Bearer admin-key' }),
    fetch: async (url) => {
      fetchUrls.push(url);
      return {
        ok: true,
        status: 200,
        json: async () => ({ ssoSuper: [] }),
      };
    },
    showToast: () => {},
    requestAnimationFrame: (callback) => callback(),
    setTimeout: (callback) => callback(),
    document: {
      getElementById(id) {
        if (id === 'page-size') return { value: '50', options: [option] };
        if (id === 'token-table-body') {
          return {
            innerHTML: '',
            replaceChildren() {},
          };
        }
        if (id === 'loading' || id === 'empty-state') {
          return {
            classList: {
              add() {},
              remove() {},
            },
          };
        }
        return null;
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
          appendChild() {},
          classList: { add() {}, remove() {}, toggle() {} },
          setAttribute() {},
        };
      },
    },
  };

  vm.createContext(context);
  vm.runInContext(tokenJs, context);
  await context.init();

  assert.deepEqual(fetchUrls, ['/v1/admin/tokens']);
  assert.equal(option.textContent, '50 / 页');
});
