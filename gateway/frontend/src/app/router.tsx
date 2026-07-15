import { Navigate, createBrowserRouter, type RouteObject } from "react-router-dom";

import { AnonymousBoundary, AuthBoundary } from "@/app/auth-boundary";
import { DeferredAccountsPage, DeferredApiDocsPage, DeferredAppShell, DeferredCachePage, DeferredClientKeysPage, DeferredDashboardPage, DeferredModelsPage, DeferredRequestAuditsPage, DeferredSettingsPage } from "@/app/deferred-pages";
import { gatewayBasename, gatewayRoutePaths } from "@/app/gateway-paths.mjs";
import { LoginPage } from "@/features/auth/login-page";

export const gatewayRouterRoutes: RouteObject[] = [
  {
    element: <AnonymousBoundary />,
    children: [{ path: gatewayRoutePaths.login, element: <LoginPage /> }],
  },
  {
    element: <AuthBoundary />,
    children: [
      {
        element: <DeferredAppShell />,
        children: [
          { index: true, element: <Navigate to={gatewayRoutePaths.dashboard} replace /> },
          { path: gatewayRoutePaths.dashboard, element: <DeferredDashboardPage /> },
          { path: gatewayRoutePaths.accounts, element: <DeferredAccountsPage /> },
          { path: gatewayRoutePaths.models, element: <DeferredModelsPage /> },
          { path: gatewayRoutePaths.clientKeys, element: <DeferredClientKeysPage /> },
          { path: gatewayRoutePaths.requestAudits, element: <DeferredRequestAuditsPage /> },
          { path: gatewayRoutePaths.cache, element: <DeferredCachePage /> },
          { path: gatewayRoutePaths.docs, element: <Navigate to={gatewayRoutePaths.docsDefault} replace /> },
          { path: gatewayRoutePaths.docsEndpoint, element: <DeferredApiDocsPage /> },
          { path: gatewayRoutePaths.settings, element: <DeferredSettingsPage /> },
        ],
      },
    ],
  },
  { path: "*", element: <Navigate to={gatewayRoutePaths.dashboard} replace /> },
];

export const gatewayRouterOptions = { basename: gatewayBasename } as const;

export function createGatewayRouter() {
  return createBrowserRouter(gatewayRouterRoutes, gatewayRouterOptions);
}
