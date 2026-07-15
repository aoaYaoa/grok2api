import path from "node:path";
import { fileURLToPath } from "node:url";
import { existsSync, renameSync } from "node:fs";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

import { gatewayViteBase } from "./src/app/gateway-paths.mjs";

export default defineConfig(({ mode }) => {
  const publicBuild = mode === "public";
  return {
    base: publicBuild ? "/" : gatewayViteBase,
    plugins: [
      react(),
      tailwindcss(),
      {
        name: "public-html-output",
        closeBundle() {
          if (!publicBuild) return;
          const outputRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "dist/public");
          const source = path.join(outputRoot, "public.html");
          if (existsSync(source)) renameSync(source, path.join(outputRoot, "index.html"));
        },
      },
    ],
    define: {
      __GROK2API_DEV_API_TARGET__: JSON.stringify(process.env.VITE_DEV_API_TARGET ?? ""),
    },
    resolve: {
      alias: {
        "@": path.resolve(path.dirname(fileURLToPath(import.meta.url)), "./src"),
      },
    },
    server: {
      port: 5173,
      proxy: {
        "/api": process.env.VITE_DEV_API_TARGET ?? "http://127.0.0.1:8000",
        "/v1": process.env.VITE_DEV_API_TARGET ?? "http://127.0.0.1:8000",
        "/healthz": process.env.VITE_DEV_API_TARGET ?? "http://127.0.0.1:8000",
        "/readyz": process.env.VITE_DEV_API_TARGET ?? "http://127.0.0.1:8000",
      },
    },
    build: {
      outDir: publicBuild ? "dist/public" : "dist/admin",
      emptyOutDir: true,
      sourcemap: false,
      rollupOptions: publicBuild
        ? { input: path.resolve(path.dirname(fileURLToPath(import.meta.url)), "public.html") }
        : undefined,
    },
  };
});
