export const gatewayBasename: "/gateway";
export const gatewayViteBase: "/gateway/";
export const gatewayRoutePaths: Readonly<{
  login: "/login";
  dashboard: "/dashboard";
  accounts: "/accounts";
  models: "/models";
  clientKeys: "/client-keys";
  requestAudits: "/request-audits";
  cache: "/cache";
  docs: "/docs";
  docsDefault: "/docs/chat/completions";
  docsEndpoint: "/docs/:category/:endpoint";
  settings: "/settings";
}>;
