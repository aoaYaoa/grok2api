import { lazy, Suspense, type ReactNode } from "react";
import { Navigate, createBrowserRouter } from "react-router-dom";

import { publicRoutePaths } from "@/public/app/public-paths.mjs";
import { PublicAuthBoundary } from "@/public/auth/public-auth";
import { PublicShell } from "@/public/components/public-shell";
import { LoginPage } from "@/public/pages/login-page";

const ChatPage = lazy(() => import("@/public/pages/chat-page").then((module) => ({ default: module.ChatPage })));
const ImaginePage = lazy(() => import("@/public/pages/imagine-page").then((module) => ({ default: module.ImaginePage })));
const WorkbenchPage = lazy(() => import("@/public/pages/workbench-page").then((module) => ({ default: module.WorkbenchPage })));
const VideoPage = lazy(() => import("@/public/pages/video-page").then((module) => ({ default: module.VideoPage })));
const NsfwPage = lazy(() => import("@/public/pages/nsfw-page").then((module) => ({ default: module.NsfwPage })));
const VoicePage = lazy(() => import("@/public/pages/voice-page").then((module) => ({ default: module.VoicePage })));

function deferred(page: ReactNode) { return <Suspense fallback={<div className="grid min-h-64 place-items-center text-sm text-muted-foreground">正在加载工作台...</div>}>{page}</Suspense>; }

export const publicRouter = createBrowserRouter([
  { path: publicRoutePaths.login, element: <LoginPage /> },
  {
    element: <PublicAuthBoundary />,
    children: [
      { element: <PublicShell />, children: [
        { path: publicRoutePaths.chat, element: deferred(<ChatPage />) },
        { path: publicRoutePaths.imagine, element: deferred(<ImaginePage />) },
        { path: publicRoutePaths.workbench, element: deferred(<WorkbenchPage />) },
        { path: publicRoutePaths.video, element: deferred(<VideoPage />) },
        { path: publicRoutePaths.nsfw, element: deferred(<NsfwPage />) },
        { path: publicRoutePaths.voice, element: deferred(<VoicePage />) },
      ] },
    ],
  },
  { path: "/", element: <Navigate to={publicRoutePaths.chat} replace /> },
  { path: "*", element: <Navigate to={publicRoutePaths.chat} replace /> },
]);
