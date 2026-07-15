import { Image, Images, LogOut, Menu, MessageSquare, Mic2, Moon, ShieldAlert, Sun, Video } from "lucide-react";
import { useTheme } from "next-themes";
import { useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { publicRoutePaths } from "@/public/app/public-paths.mjs";
import { usePublicAuth } from "@/public/auth/public-auth";
import { cn } from "@/shared/lib/cn";

const navigation = [
  { to: publicRoutePaths.chat, label: "Chat", icon: MessageSquare },
  { to: publicRoutePaths.imagine, label: "Imagine", icon: Images },
  { to: publicRoutePaths.workbench, label: "图片编辑", short: "编辑", icon: Image },
  { to: publicRoutePaths.video, label: "Video", icon: Video },
  { to: publicRoutePaths.nsfw, label: "NSFW", icon: ShieldAlert },
  { to: publicRoutePaths.voice, label: "Voice", icon: Mic2 },
];

export function PublicShell() {
  const auth = usePublicAuth();
  const navigate = useNavigate();
  const { resolvedTheme, setTheme } = useTheme();
  const [moreOpen, setMoreOpen] = useState(false);
  const logout = () => { auth.logout(); navigate(publicRoutePaths.login, { replace: true }); };
  return (
    <div className="min-h-dvh bg-background text-foreground">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-[1600px] items-center gap-4 px-4">
          <NavLink to={publicRoutePaths.chat} className="shrink-0 font-semibold">Grok2API</NavLink>
          <nav className="hidden min-w-0 flex-1 items-center gap-1 md:flex" aria-label="公共工作台">
            {navigation.map(({ to, label, icon: Icon }) => <NavLink key={to} to={to} className={({ isActive }) => cn("flex h-9 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground hover:bg-accent hover:text-foreground", isActive && "bg-accent text-foreground")}><Icon className="size-4" />{label}</NavLink>)}
          </nav>
          <div className="ml-auto flex items-center gap-1">
            <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")} aria-label="切换主题">{resolvedTheme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}</Button></TooltipTrigger><TooltipContent>切换主题</TooltipContent></Tooltip>
            <a href="/gateway/login" className="hidden h-9 items-center px-2 text-sm text-muted-foreground hover:text-foreground sm:flex">管理端</a>
            <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={logout} aria-label="退出"><LogOut className="size-4" /></Button></TooltipTrigger><TooltipContent>退出</TooltipContent></Tooltip>
          </div>
        </div>
      </header>
      <main id="public-main" className="mx-auto max-w-[1600px] px-4 py-5 pb-24 md:pb-6"><Outlet /></main>
      <nav className="fixed inset-x-0 bottom-0 z-40 grid grid-cols-5 border-t bg-background md:hidden" aria-label="手机工作台">
        {navigation.slice(0, 4).map(({ to, label, short, icon: Icon }) => <NavLink key={to} to={to} className={({ isActive }) => cn("flex min-h-16 flex-col items-center justify-center gap-1 text-[11px] text-muted-foreground", isActive && "text-foreground")}><Icon className="size-5" /><span>{short || label}</span></NavLink>)}
        <Sheet open={moreOpen} onOpenChange={setMoreOpen}><SheetTrigger asChild><button className="flex min-h-16 flex-col items-center justify-center gap-1 text-[11px] text-muted-foreground"><Menu className="size-5" /><span>更多</span></button></SheetTrigger><SheetContent side="bottom" className="rounded-none"><SheetHeader><SheetTitle>更多工作台</SheetTitle></SheetHeader><div className="grid gap-2 p-4">{navigation.slice(4).map(({ to, label, icon: Icon }) => <NavLink key={to} to={to} onClick={() => setMoreOpen(false)} className="flex min-h-12 items-center gap-3 rounded-md border px-4"><Icon className="size-5" />{label}</NavLink>)}<a href="/gateway/login" className="flex min-h-12 items-center gap-3 rounded-md border px-4">管理后台</a><button onClick={logout} className="flex min-h-12 items-center gap-3 rounded-md border px-4 text-destructive"><LogOut className="size-5" />退出</button></div></SheetContent></Sheet>
      </nav>
    </div>
  );
}
