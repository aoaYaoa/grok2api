import { ClipboardPaste, Download, ImagePlus, RotateCcw, Sparkles, Trash2, X } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { usePublicAuth } from "@/public/auth/public-auth";
import { PromptEnhanceButton } from "@/public/components/prompt-enhance-button";
import { editImage, imageFromEdit, resolveParentPost, type GeneratedImage } from "@/public/features/image/image-api";
import { downloadURL, extractParentPostID, filesToAssets, imageSource, type UploadAsset } from "@/public/lib/media";

type Reference = UploadAsset & { parentPostID?: string };
export function WorkbenchPage() {
  const { key } = usePublicAuth(); const [references, setReferences] = useState<Reference[]>([]); const [parentInput, setParentInput] = useState(""); const [prompt, setPrompt] = useState(""); const [history, setHistory] = useState<GeneratedImage[]>([]); const [current, setCurrent] = useState<GeneratedImage | null>(null); const [progress, setProgress] = useState(0); const [status, setStatus] = useState("未开始"); const [running, setRunning] = useState(false);
  useEffect(() => { const paste = (event: ClipboardEvent) => { const files = Array.from(event.clipboardData?.files || []).filter((file) => file.type.startsWith("image/")); if (files.length) void filesToAssets(files, Math.max(0, 8 - references.length)).then((items) => setReferences((value) => [...value, ...items].slice(0, 8))); }; window.addEventListener("paste", paste); return () => window.removeEventListener("paste", paste); }, [references.length]);
  async function addParent() { const id = extractParentPostID(parentInput); if (!id) return toast.error("未识别到 parentPostId"); try { const payload = await resolveParentPost(key, id); const url = imageSource(payload); if (!url) throw new Error("未找到图片地址"); setReferences((items) => [...items, { id: crypto.randomUUID(), name: id, mime: "image/jpeg", data: url, parentPostID: id }].slice(0, 8)); setParentInput(""); } catch (error) { toast.error(error instanceof Error ? error.message : "加载 parentPostId 失败"); } }
  async function submit() { if (!prompt.trim() || !references.length || running) return; setRunning(true); setStatus("编辑中"); setProgress(2); try { const payload = await editImage(key, { workbench: true, prompt: prompt.trim(), image_references: references.map((item) => item.data), reference_items: references.map((item) => ({ image_url: item.data, source_image_url: item.data, parent_post_id: item.parentPostID || "" })) }, (value, message) => { setProgress(value); setStatus(message); }); const image = imageFromEdit(payload, prompt.trim()); if (!image) throw new Error("编辑结果为空"); setCurrent(image); setHistory((items) => [image, ...items]); setReferences([{ id: image.id, name: image.parentPostID || "编辑结果", mime: "image/jpeg", data: image.sourceURL, parentPostID: image.parentPostID }]); setProgress(100); setStatus("编辑完成"); toast.success("编辑完成"); } catch (error) { setStatus("编辑失败"); toast.error(error instanceof Error ? error.message : "编辑失败"); } finally { setRunning(false); } }
  function reset() { setReferences([]); setCurrent(null); setPrompt(""); setProgress(0); setStatus("未开始"); }
  return <section className="workspace-page">
    <div className="workspace-heading"><div><h1 className="text-xl font-semibold">图片编辑工作台</h1><p className="mt-1 text-sm text-muted-foreground">{status}</p></div><div className="workspace-actions flex gap-2"><Button variant="outline" onClick={reset}><RotateCcw className="size-4" />重置链路</Button><Button variant="outline" onClick={() => setHistory([])}><Trash2 className="size-4" />清空历史</Button></div></div>

    <div className="workspace-split">
      <aside className="workspace-controls">
        <div className="workspace-panel p-4">
          <div className="workspace-control-group">
            <div className="mb-3 flex flex-wrap items-center gap-2"><label className="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm hover:bg-accent"><ImagePlus className="size-4" />添加参考图<input type="file" accept="image/*" multiple className="hidden" onChange={(event) => void filesToAssets(event.target.files || [], Math.max(0, 8 - references.length)).then((items) => setReferences((value) => [...value, ...items].slice(0, 8)))} /></label><span className="text-xs text-muted-foreground">{references.length}/8</span></div>
            <div className="flex gap-2"><Input value={parentInput} onChange={(event) => setParentInput(event.target.value)} placeholder="parentPostId 或 URL" /><Button variant="outline" size="icon" onClick={() => void addParent()} aria-label="使用 parentPostId"><ClipboardPaste className="size-4" /></Button></div>
            <p className="mt-2 text-xs text-muted-foreground">支持拖入、粘贴和最多 8 张参考图</p>
            {references.length > 0 && <div className="mt-3 grid grid-cols-3 gap-2">{references.map((item) => <div key={item.id} className="relative aspect-square overflow-hidden rounded-md border bg-background"><img src={item.data} alt={item.name} className="size-full object-cover" /><button onClick={() => setReferences((values) => values.filter((value) => value.id !== item.id))} className="absolute right-1 top-1 grid size-8 place-items-center rounded-md bg-background/90 shadow-sm" aria-label={`移除 ${item.name}`}><X className="size-4" /></button></div>)}</div>}
          </div>
          <div className="workspace-control-group">
            <label className="workspace-field-label">编辑提示词</label>
            <Textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} className="min-h-40" placeholder="例如：@Image 1 在左侧，@Image 2 在右侧，两人合照" />
            <PromptEnhanceButton value={prompt} onEnhanced={setPrompt} disabled={running} />
            <Button className="mt-3 w-full" onClick={() => void submit()} disabled={running || !prompt.trim() || !references.length}><Sparkles className="size-4" />{running ? "编辑中..." : "执行编辑"}</Button>
            {progress > 0 && <div className="mt-3"><div className="h-2 overflow-hidden rounded bg-muted"><div className="h-full bg-primary transition-[width]" style={{ width: `${progress}%` }} /></div><p className="mt-1 text-xs text-muted-foreground">{status} {progress}%</p></div>}
          </div>
        </div>
      </aside>

      <main className="workspace-results">
        <section>
          <div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">当前结果</h2>{current && <Button variant="outline" size="sm" onClick={() => downloadURL(current.url, `workbench-${Date.now()}.jpg`)}><Download className="size-4" />下载</Button>}</div>
          <div className="workspace-panel-muted grid min-h-96 place-items-center p-3">{current ? <button onClick={() => downloadURL(current.url, `workbench-${Date.now()}.jpg`)}><img src={current.url} alt={current.prompt} className="max-h-[62dvh] object-contain" /></button> : <p className="text-sm text-muted-foreground">编辑结果会显示在这里</p>}</div>
        </section>
        <section><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">编辑历史</h2><span className="text-xs text-muted-foreground">{history.length} 条</span></div>{history.length ? <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4">{history.map((item) => <button key={`${item.id}-${item.createdAt}`} className="aspect-square overflow-hidden rounded-md border bg-card" onClick={() => setCurrent(item)}><img src={item.url} alt={item.prompt} className="size-full object-cover" loading="lazy" /></button>)}</div> : <div className="workspace-empty grid min-h-28 place-items-center text-sm text-muted-foreground">暂无历史</div>}</section>
      </main>
    </div>
  </section>;
}
