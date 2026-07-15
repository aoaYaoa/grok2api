import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";

import { publicRouter } from "@/public/app/router";
import { PublicProviders } from "@/public/app/providers";
import { PublicAuthProvider } from "@/public/auth/public-auth";
import "@/shared/i18n";
import "./index.css";

const retiredCachePrefix = "grok2api-pwa-";

async function retirePWA() {
  if ("serviceWorker" in navigator) {
    const registrations = await navigator.serviceWorker.getRegistrations();
    const retiredRegistrations = registrations.filter((registration) => {
      const scriptURL = registration.active?.scriptURL || registration.waiting?.scriptURL || registration.installing?.scriptURL;
      if (!scriptURL) return false;
      try {
        return new URL(scriptURL).pathname === "/sw.js";
      } catch {
        return false;
      }
    });
    await Promise.all(retiredRegistrations.map((registration) => registration.unregister()));
  }
  if ("caches" in window) {
    const keys = await caches.keys();
    await Promise.all(keys.filter((key) => key.startsWith(retiredCachePrefix)).map((key) => caches.delete(key)));
  }
}

void retirePWA().catch(() => undefined);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <PublicProviders>
      <PublicAuthProvider><RouterProvider router={publicRouter} /></PublicAuthProvider>
    </PublicProviders>
  </StrictMode>,
);
