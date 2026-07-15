export const gatewayBasename = "/gateway";
export const gatewayViteBase = `${gatewayBasename}/`;

export const gatewayRoutePaths = Object.freeze({
  login: "/login",
  dashboard: "/dashboard",
  accounts: "/accounts",
  models: "/models",
  clientKeys: "/client-keys",
  gallery: "/gallery",
  videoGallery: "/video-gallery",
  requestAudits: "/request-audits",
  cache: "/cache",
  docs: "/docs",
  docsDefault: "/docs/chat/completions",
  docsEndpoint: "/docs/:category/:endpoint",
  settings: "/settings",
});
