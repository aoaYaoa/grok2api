export const gatewayBasename = "/gateway";
export const gatewayViteBase = `${gatewayBasename}/`;

export const gatewayRoutePaths = Object.freeze({
  login: "/login",
  dashboard: "/dashboard",
  accounts: "/accounts",
  models: "/models",
  clientKeys: "/client-keys",
  requestAudits: "/request-audits",
  docs: "/docs",
  docsDefault: "/docs/chat/completions",
  docsEndpoint: "/docs/:category/:endpoint",
  settings: "/settings",
});
