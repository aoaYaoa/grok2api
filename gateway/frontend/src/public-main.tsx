import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";

import { publicRouter } from "@/public/app/router";
import { PublicProviders } from "@/public/app/providers";
import { PublicAuthProvider } from "@/public/auth/public-auth";
import "@/shared/i18n";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <PublicProviders>
      <PublicAuthProvider><RouterProvider router={publicRouter} /></PublicAuthProvider>
    </PublicProviders>
  </StrictMode>,
);
