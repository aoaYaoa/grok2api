import { Eye, EyeOff, LogIn } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { publicRoutePaths } from "@/public/app/public-paths.mjs";
import { usePublicAuth } from "@/public/auth/public-auth";
import { GitHubMark } from "@/shared/components/github-mark";

export function LoginPage() {
  const auth = usePublicAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [key, setKey] = useState("");
  const [show, setShow] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (auth.ready && auth.authenticated) navigate(publicRoutePaths.chat, { replace: true });
  }, [auth.ready, auth.authenticated, navigate]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    try {
      await auth.login(key);
      const from = (location.state as { from?: string } | null)?.from || publicRoutePaths.chat;
      navigate(from, { replace: true });
      toast.success("验证成功");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Public Key 验证失败");
    } finally { setSubmitting(false); }
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-background p-4 sm:p-6">
      <section className="workspace-panel w-full max-w-sm border-t-4 border-t-primary p-6 shadow-md">
        <div className="mb-7 flex items-center justify-between">
          <div><h1 className="text-xl font-semibold">Grok2API</h1><p className="mt-1 text-sm text-muted-foreground">公共创作工作台</p></div>
          <a href="https://github.com/chenyme/grok2api" target="_blank" rel="noreferrer" aria-label="GitHub" className="grid size-11 place-items-center rounded-md hover:bg-accent"><GitHubMark className="size-5" /></a>
        </div>
        <form onSubmit={submit} className="space-y-4">
          <label className="block text-sm font-medium" htmlFor="public-key">Public Key</label>
          <div className="relative">
            <Input id="public-key" type={show ? "text" : "password"} value={key} onChange={(event) => setKey(event.target.value)} autoComplete="current-password" className="h-11 pr-12" placeholder="未设置密钥时可留空" />
            <button type="button" onClick={() => setShow((value) => !value)} className="absolute right-0 top-0 grid size-11 place-items-center text-muted-foreground" aria-label={show ? "隐藏 Public Key" : "显示 Public Key"}>{show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}</button>
          </div>
          <Button type="submit" className="h-11 w-full" disabled={submitting}><LogIn className="size-4" />{submitting ? "验证中..." : "进入工作台"}</Button>
        </form>
        <a href="/gateway/login" className="mt-5 block text-center text-sm text-muted-foreground hover:text-foreground">进入管理后台</a>
      </section>
    </main>
  );
}
