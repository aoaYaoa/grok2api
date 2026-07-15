import { Download, Play, Sparkles, Square, Trash2, WandSparkles } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { publicEndpoints, publicFetch } from "@/public/api/client";
import { usePublicAuth } from "@/public/auth/public-auth";
import { ImageGrid } from "@/public/components/image-grid";
import { editImage, generatedImage, imageFromEdit, startImage, stopImages, streamImage, type GeneratedImage } from "@/public/features/image/image-api";
import { downloadURL } from "@/public/lib/media";

export function ImaginePage() {
  const { key } = usePublicAuth(); const [prompt, setPrompt] = useState(""); const [ratio, setRatio] = useState("2:3"); const [concurrent, setConcurrent] = useState("1"); const [nsfw, setNsfw] = useState(true); const [pro, setPro] = useState(false); const [autoDownload, setAutoDownload] = useState(false); const [images, setImages] = useState<GeneratedImage[]>([]); const [selected, setSelected] = useState(new Set<string>()); const [active, setActive] = useState<GeneratedImage | null>(null); const [editPrompt, setEditPrompt] = useState(""); const [editing, setEditing] = useState(false); const [running, setRunning] = useState(false); const [activeCount, setActiveCount] = useState(0); const taskIDs = useRef<string[]>([]); const controllers = useRef<AbortController[]>([]);
  async function enhance() { if (!prompt.trim()) return; try { const payload = await publicFetch<{ prompt?: string; enhanced_prompt?: string; content?: string }>(key, publicEndpoints.promptEnhance, { method: "POST", body: JSON.stringify({ prompt, temperature: 0.7, request_id: crypto.randomUUID() }) }); setPrompt(payload.enhanced_prompt || payload.prompt || payload.content || prompt); } catch (error) { toast.error(error instanceof Error ? error.message : "提示词优化失败"); } }
  async function start() { if (!prompt.trim() || running) return; setRunning(true); try { const starts = await Promise.all(Array.from({ length: Number(concurrent) }, () => startImage(key, { prompt: prompt.trim(), aspect_ratio: ratio, nsfw, pro }))); taskIDs.current = starts.map((item) => item.task_id); controllers.current = starts.map(() => new AbortController()); setActiveCount(starts.length); starts.forEach((item, index) => { void streamImage(key, item.task_id, (event) => { if (event.type === "error" || event.error) { toast.error(String(event.message || "图片生成失败")); return; } const image = generatedImage(event, prompt); if (image) { setImages((current) => [image, ...current]); if (autoDownload) downloadURL(image.url, `imagine-${Date.now()}.jpg`); } }, controllers.current[index].signal).catch((error) => { if (!controllers.current[index].signal.aborted) toast.error(error.message); }).finally(() => { setActiveCount((value) => Math.max(0, value - 1)); if (index === starts.length - 1) setRunning(false); }); }); toast.success(`已启动 ${starts.length} 个任务`); } catch (error) { setRunning(false); setActiveCount(0); toast.error(error instanceof Error ? error.message : "创建图片任务失败"); } }
  async function stop() { controllers.current.forEach((item) => item.abort()); await stopImages(key, taskIDs.current).catch(() => undefined); taskIDs.current = []; controllers.current = []; setActiveCount(0); setRunning(false); }
  async function submitEdit() { if (!active || !editPrompt.trim()) return; setEditing(true); try { const payload = await editImage(key, { prompt: editPrompt.trim(), parent_post_id: active.parentPostID, source_image_url: active.sourceURL }); const image = imageFromEdit(payload, editPrompt.trim()); if (image) { setImages((current) => [image, ...current]); setActive(image); setEditPrompt(""); toast.success("编辑完成"); } } catch (error) { toast.error(error instanceof Error ? error.message : "编辑失败"); } finally { setEditing(false); } }
  function toggle(id: string) { setSelected((current) => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next; }); }
  function downloadSelected() { images.filter((item) => selected.has(item.id)).forEach((item, index) => setTimeout(() => downloadURL(item.url, `imagine-${Date.now()}-${index + 1}.jpg`), index * 120)); }
  return <section className="workspace-page">
    <div className="workspace-heading">
      <div><h1 className="text-xl font-semibold">Imagine 瀑布流</h1><p className="mt-1 text-sm text-muted-foreground">{running ? `${activeCount} 路持续生成中` : `${images.length} 张图片`}</p></div>
      <div className="flex gap-2">{selected.size > 0 && <Button variant="outline" onClick={downloadSelected}><Download className="size-4" />下载 {selected.size}</Button>}<Button variant="outline" onClick={() => { setImages([]); setSelected(new Set()); }}><Trash2 className="size-4" />清空</Button></div>
    </div>

    <div className="workspace-split">
      <aside className="workspace-controls">
        <div className="workspace-panel p-4">
          <div className="workspace-control-group">
            <label className="workspace-field-label">生成提示词</label>
            <Textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} className="min-h-36" placeholder="描述想要持续生成的画面" />
            <Button variant="ghost" size="sm" className="mt-2" onClick={() => void enhance()}><WandSparkles className="size-4" />优化提示词</Button>
          </div>
          <div className="workspace-control-group grid grid-cols-2 gap-3">
            <label><span className="workspace-field-label">画面比例</span><Select value={ratio} onValueChange={setRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["2:3","1:1","3:2","16:9","9:16"].map((value) => <SelectItem value={value} key={value}>{value}</SelectItem>)}</SelectContent></Select></label>
            <label><span className="workspace-field-label">并发任务</span><Select value={concurrent} onValueChange={setConcurrent}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["1","2","3"].map((value) => <SelectItem value={value} key={value}>{value} 路</SelectItem>)}</SelectContent></Select></label>
          </div>
          <div className="workspace-control-group grid gap-2">
            <label className="flex min-h-11 items-center justify-between rounded-md border px-3 text-sm">NSFW<Switch checked={nsfw} onCheckedChange={setNsfw} /></label>
            <label className="flex min-h-11 items-center justify-between rounded-md border px-3 text-sm">2K Pro<Switch checked={pro} onCheckedChange={setPro} /></label>
            <label className="flex min-h-11 items-center justify-between rounded-md border px-3 text-sm">生成后自动下载<Switch checked={autoDownload} onCheckedChange={setAutoDownload} /></label>
          </div>
          <div className="workspace-control-group">
            {running ? <Button variant="destructive" className="w-full" onClick={() => void stop()}><Square className="size-4" />停止生成</Button> : <Button className="w-full" onClick={() => void start()} disabled={!prompt.trim()}><Play className="size-4" />开始生成</Button>}
          </div>
        </div>
      </aside>

      <main className="workspace-results">
        <div className="flex items-center justify-between"><h2 className="text-sm font-semibold">生成结果</h2><span className="text-xs text-muted-foreground">{images.length} 张</span></div>
        <ImageGrid images={images} selected={selected} onSelect={toggle} onOpen={setActive} onEdit={(image) => { setActive(image); setEditPrompt(""); }} />
      </main>
    </div>

    <Dialog open={Boolean(active)} onOpenChange={(open) => !open && setActive(null)}><DialogContent className="max-w-4xl"><DialogHeader><DialogTitle>图片预览与编辑</DialogTitle></DialogHeader>{active && <div className="space-y-4"><div className="max-h-[58dvh] overflow-auto rounded-md bg-muted"><img src={active.url} alt={active.prompt} className="mx-auto max-h-[58dvh] object-contain" /></div><div><p className="mb-2 text-sm text-muted-foreground">{active.parentPostID || "本地图片"}</p><Textarea value={editPrompt} onChange={(event) => setEditPrompt(event.target.value)} className="min-h-28" placeholder="输入修改要求" /><div className="mt-3 flex flex-wrap gap-2"><Button disabled={editing || !editPrompt.trim()} onClick={() => void submitEdit()}><Sparkles className="size-4" />{editing ? "编辑中..." : "发送编辑"}</Button><Button variant="outline" onClick={() => downloadURL(active.url, `imagine-${Date.now()}.jpg`)}><Download className="size-4" />下载</Button></div></div></div>}</DialogContent></Dialog>
  </section>;
}
