import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";

import { publicEndpoints, publicFetch } from "@/public/api/client";
import { publicRoutePaths } from "@/public/app/public-paths.mjs";
import { clearPublicKey, loadPublicKey, savePublicKey } from "@/public/auth/key-storage";

type AuthState = { key: string; ready: boolean; authenticated: boolean; login: (username: string, password: string) => Promise<void>; logout: () => Promise<void> };
const AuthContext = createContext<AuthState | null>(null);

async function verify(key: string) { await publicFetch(key, publicEndpoints.verify, { cache: "no-store" }); }

export function PublicAuthProvider({ children }: { children: ReactNode }) {
  const [key, setKey] = useState("");
  const [ready, setReady] = useState(false);
  const [authenticated, setAuthenticated] = useState(false);

  useEffect(() => {
    let active = true;
    void loadPublicKey().then(async (stored) => {
      try { await verify(stored); if (active) { setKey(stored); setAuthenticated(true); } }
      catch { clearPublicKey(); }
      finally { if (active) setReady(true); }
    });
    return () => { active = false; };
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    await publicFetch("", publicEndpoints.authLogin, {
      method: "POST",
      body: JSON.stringify({ username: username.trim(), password }),
    });
    await savePublicKey("");
    setKey("");
    setAuthenticated(true);
  }, []);
  const logout = useCallback(async () => {
    try { await publicFetch(key, publicEndpoints.authLogout, { method: "POST" }); } catch { /* local logout still clears the UI */ }
    clearPublicKey(); setKey(""); setAuthenticated(false);
  }, [key]);
  const value = useMemo(() => ({ key, ready, authenticated, login, logout }), [key, ready, authenticated, login, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function usePublicAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("PublicAuthProvider is missing");
  return value;
}

export function PublicAuthBoundary() {
  const auth = usePublicAuth();
  const location = useLocation();
  if (!auth.ready) return <div className="grid min-h-dvh place-items-center text-sm text-muted-foreground">正在验证访问权限...</div>;
  if (!auth.authenticated) return <Navigate to={publicRoutePaths.login} state={{ from: location.pathname }} replace />;
  return <Outlet />;
}
