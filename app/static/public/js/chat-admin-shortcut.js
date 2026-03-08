(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
  if (root) {
    root.ChatAdminShortcut = api;
  }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  const HIDDEN_ADMIN_COMMAND = '#admin';
  const ADMIN_LOGIN_PATH = '/admin/login';

  function normalizePrompt(value) {
    return String(value || '').trim();
  }

  function isHiddenAdminCommand(value) {
    return normalizePrompt(value) === HIDDEN_ADMIN_COMMAND;
  }

  function maybeHandleHiddenAdminCommand(value, navigate) {
    if (!isHiddenAdminCommand(value)) return false;
    if (typeof navigate === 'function') {
      navigate(ADMIN_LOGIN_PATH);
    }
    return true;
  }

  return {
    HIDDEN_ADMIN_COMMAND,
    ADMIN_LOGIN_PATH,
    isHiddenAdminCommand,
    maybeHandleHiddenAdminCommand,
  };
});
