import { ImagePlus, Library, Play, Plus, RotateCcw, Scissors, Square, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { usePublicAuth } from "@/public/auth/public-auth";
import { VideoGrid } from "@/public/components/video-grid";
import { resolveParentPost } from "@/public/features/image/image-api";
import { cachedVideo, listCachedVideos, renameVideo, startVideo, stopVideos, streamVideo, videoPostID, type VideoItem } from "@/public/features/video/video-api";
import { extractParentPostID, filesToAssets, imageSource, type UploadAsset } from "@/public/lib/media";

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

export function VideoPage() {
  const { key } = usePublicAuth();
  const [prompt, setPrompt] = useState("");
  const [references, setReferences] = useState<UploadAsset[]>([]);
  const [parentInput, setParentInput] = useState("");
  const [ratio, setRatio] = useState("3:2");
  const [length, setLength] = useState("6");
  const [resolution, setResolution] = useState("480p");
  const [preset, setPreset] = useState("normal");
  const [concurrent, setConcurrent] = useState("1");
  const [videos, setVideos] = useState<VideoItem[]>([]);
  const [cachedVideos, setCachedVideos] = useState<VideoItem[]>([]);
  const [active, setActive] = useState<VideoItem | null>(null);
  const [starting, setStarting] = useState(false);
  const [running, setRunning] = useState(false);
  const [cacheOpen, setCacheOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState<VideoItem | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [extendPrompt, setExtendPrompt] = useState("");
  const [extendTime, setExtendTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const controllers = useRef(new Map<string, AbortController>());
  const taskIDs = useRef<string[]>([]);
  const startLock = useRef(false);
  const startController = useRef<AbortController | null>(null);
  const extensionPanel = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const paste = (event: ClipboardEvent) => {
      const files = Array.from(event.clipboardData?.files || []).filter((file) => file.type.startsWith("image/"));
      if (files.length) void filesToAssets(files, Math.max(0, 8 - references.length)).then((items) => setReferences((value) => [...value, ...items].slice(0, 8)));
    };
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  }, [references.length]);

  async function addParent() {
    const id = extractParentPostID(parentInput);
    if (!id) return toast.error("未识别到 parentPostId");
    try {
      const payload = await resolveParentPost(key, id);
      const url = imageSource(payload);
      if (!url) throw new Error("未找到图片地址");
      setReferences((items) => [...items, { id, name: id, mime: "image/jpeg", data: url }].slice(0, 8));
      setParentInput("");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载失败");
    }
  }

  function watch(taskID: string, label: string, taskPrompt: string) {
    const controller = new AbortController();
    controllers.current.set(taskID, controller);
    setVideos((items) => [{ id: taskID, taskID, url: "", prompt: taskPrompt, progress: 0, status: "running", postID: "", displayName: label, createdAt: Date.now() }, ...items]);
    void streamVideo(key, taskID, (update) => setVideos((items) => items.map((item) => item.taskID !== taskID ? item : { ...item, progress: update.progress ?? item.progress, url: update.url || item.url, postID: videoPostID(update.url || item.url), status: update.error ? "failed" : update.url || update.done ? "completed" : "running", error: update.error })), controller.signal).catch((error) => {
      if (!controller.signal.aborted) toast.error(error instanceof Error ? error.message : "视频连接失败");
    }).finally(() => {
      controllers.current.delete(taskID);
      if (!controllers.current.size) {
        startLock.current = false;
        setRunning(false);
      }
    });
  }

  async function generate(bodyOverride?: Record<string, unknown>) {
    if (startLock.current) return;
    if (!prompt.trim() && !references.length && !bodyOverride) return toast.error("请输入提示词或添加参考图");
    startLock.current = true;
    setStarting(true);
    setRunning(true);
    const controller = new AbortController();
    startController.current = controller;
    const taskPrompt = bodyOverride ? extendPrompt.trim() : prompt.trim();
    try {
      const result = await startVideo(key, { prompt: taskPrompt, aspect_ratio: ratio, video_length: Number(length), resolution_name: resolution, preset, concurrent: Number(concurrent), image_references: references.map((item) => item.data), source_image_urls: references.map((item) => item.data), ...(bodyOverride || {}) }, controller.signal);
      if (controller.signal.aborted) return;
      const ids = result.task_ids?.length ? result.task_ids : [result.task_id];
      taskIDs.current = ids;
      setStarting(false);
      ids.forEach((id, index) => watch(id, bodyOverride ? "延长视频" : `视频 ${videos.length + index + 1}`, taskPrompt));
    } catch (error) {
      if (!isAbortError(error)) toast.error(error instanceof Error ? error.message : "创建视频失败");
      startLock.current = false;
      setRunning(false);
    } finally {
      startController.current = null;
      setStarting(false);
    }
  }

  async function stop() {
    startController.current?.abort();
    startController.current = null;
    controllers.current.forEach((value) => value.abort());
    controllers.current.clear();
    const tasks = taskIDs.current;
    taskIDs.current = [];
    startLock.current = false;
    setStarting(false);
    setRunning(false);
    await stopVideos(key, tasks).catch(() => undefined);
  }

  async function openCache() {
    setCacheOpen(true);
    try {
      const payload = await listCachedVideos(key);
      const values = (payload.items || []).map(cachedVideo);
      setCachedVideos(values);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取缓存失败");
    }
  }

  async function saveRename() {
    if (!renameTarget || !renameValue.trim()) return;
    try {
      await renameVideo(key, renameTarget, renameValue.trim());
      setVideos((items) => items.map((item) => item.id === renameTarget.id ? { ...item, displayName: renameValue.trim() } : item));
      setCachedVideos((items) => items.map((item) => item.id === renameTarget.id ? { ...item, displayName: renameValue.trim() } : item));
      setRenameTarget(null);
      toast.success("名称已更新");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "重命名失败");
    }
  }

  async function extend() {
    if (!active?.postID) return toast.error("当前视频没有可用 postId，请从缓存中选择");
    await generate({ prompt: extendPrompt.trim(), concurrent: 1, video_length: Number(length), is_video_extension: true, extend_post_id: active.postID, video_extension_start_time: extendTime, original_post_id: active.originalPostID || active.postID, file_attachment_id: active.originalPostID || active.postID, stitch_with_extend: true });
  }

  function activate(item: VideoItem, scroll = false) {
    setActive(item);
    setExtendTime(0);
    if (scroll) requestAnimationFrame(() => extensionPanel.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
  }

  function updateExtendTime(value: number) {
    setExtendTime(Math.max(0, Math.min(Number.isFinite(value) ? value : 0, Math.max(duration, 0))));
  }

  return (
    <section className="workspace-page">
      <div className="workspace-heading">
        <div><h1 className="text-xl font-semibold">Video 视频工作台</h1><p className="mt-1 text-sm text-muted-foreground">{starting ? "正在创建任务" : running ? "任务运行中" : `${videos.length} 个视频`}</p></div>
        <div className="flex gap-2"><Button variant="outline" onClick={() => void openCache()}><Library className="size-4" />缓存视频</Button><Button variant="outline" onClick={() => { setVideos([]); setActive(null); }}><RotateCcw className="size-4" />清空</Button></div>
      </div>

      <div className="workspace-split">
        <aside className="workspace-controls">
          <div className="workspace-panel p-4">
            <div className="workspace-control-group">
              <div className="mb-3 flex flex-wrap gap-2"><label className="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm hover:bg-accent"><ImagePlus className="size-4" />添加参考图<input type="file" accept="image/*" multiple className="hidden" onChange={(event) => void filesToAssets(event.target.files || [], Math.max(0, 8 - references.length)).then((items) => setReferences((value) => [...value, ...items].slice(0, 8)))} /></label><div className="flex min-w-0 flex-1 gap-2"><Input value={parentInput} onChange={(event) => setParentInput(event.target.value)} placeholder="parentPostId" /><Button variant="outline" size="icon" onClick={() => void addParent()} aria-label="添加 parentPostId"><Plus className="size-4" /></Button></div></div>
            {references.length ? <div className="grid grid-cols-4 gap-2 sm:grid-cols-6">{references.map((item) => <div key={item.id} className="relative aspect-square overflow-hidden rounded-md border"><img src={item.data} alt={item.name} className="size-full object-cover" /><button className="absolute right-0 top-0 grid size-8 place-items-center bg-background/90" onClick={() => setReferences((values) => values.filter((value) => value.id !== item.id))} aria-label={`移除 ${item.name}`}><X className="size-4" /></button></div>)}</div> : <p className="text-sm text-muted-foreground">支持最多 8 张图片、拖放与粘贴</p>}
            </div>
            <div className="workspace-control-group">
              <label className="workspace-field-label">视频提示词</label><Textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} className="min-h-32" placeholder="描述镜头、动作和风格" />
              <div className="mt-3 grid grid-cols-2 gap-3">
                <label><span className="workspace-field-label">画面比例</span><Select value={ratio} onValueChange={setRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["3:2", "2:3", "16:9", "9:16", "1:1"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label>
                <label><span className="workspace-field-label">视频时长</span><Select value={length} onValueChange={setLength}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["6", "10", "15"].map((value) => <SelectItem key={value} value={value}>{value} 秒</SelectItem>)}</SelectContent></Select></label>
                <label><span className="workspace-field-label">分辨率</span><Select value={resolution} onValueChange={setResolution}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["480p", "720p"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label>
                <label><span className="workspace-field-label">生成模式</span><Select value={preset} onValueChange={setPreset}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["normal", "fun", "spicy", "custom"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label>
                <label className="col-span-2"><span className="workspace-field-label">并发任务</span><Select value={concurrent} onValueChange={setConcurrent}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["1", "2", "3", "4"].map((value) => <SelectItem key={value} value={value}>{value} 路</SelectItem>)}</SelectContent></Select></label>
              </div>
              {starting ? <Button className="mt-4 w-full" disabled><Play className="size-4" />创建中...</Button> : running ? <Button variant="destructive" className="mt-4 w-full" onClick={() => void stop()}><Square className="size-4" />停止任务</Button> : <Button className="mt-4 w-full" onClick={() => void generate()} disabled={!prompt.trim() && !references.length}><Play className="size-4" />生成视频</Button>}
            </div>
          </div>

          <section ref={extensionPanel} className="workspace-panel scroll-mt-20 p-4">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold"><Scissors className="size-4 text-info" />当前视频与延长</h2>
          {active?.url ? <>
            <video src={active.url} controls playsInline className="aspect-video w-full rounded-md bg-black" onLoadedMetadata={(event) => { const nextDuration = event.currentTarget.duration || 0; setDuration(nextDuration); setExtendTime((value) => Math.min(value, nextDuration)); }} />
            <div className="mt-3 grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-3"><label className="text-xs">时间轴<input type="range" min="0" max={Math.max(duration, 0.001)} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} className="mt-3 w-full" /></label><label className="text-xs">起点（秒）<Input type="number" min="0" max={duration || undefined} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} className="mt-1 font-mono" /></label></div>
            <p className="mt-1 text-xs text-muted-foreground">当前 {extendTime.toFixed(3)}s / {duration.toFixed(3)}s</p>
            <Textarea value={extendPrompt} onChange={(event) => setExtendPrompt(event.target.value)} className="mt-3 min-h-24" placeholder="留空使用 spicy，或描述接下来的画面" />
            <Button className="mt-3 w-full" onClick={() => void extend()} disabled={starting || running || !active.postID}><Scissors className="size-4" />从 {extendTime.toFixed(3)}s 延长</Button>
            {!active.postID && <p className="mt-2 text-xs text-warning">从缓存选择带 postId 的视频后可延长</p>}
          </> : <div className="workspace-empty grid min-h-64 place-items-center p-4 text-center text-sm text-muted-foreground">在视频记录中点击剪刀按钮进入延长区</div>}
          </section>
        </aside>

        <main className="workspace-results">
          <div><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">视频历史</h2><span className="text-xs text-muted-foreground">{videos.length} 个</span></div><VideoGrid videos={videos} activeID={active?.id} onActivate={(item) => activate(item)} onExtend={(item) => activate(item, true)} onRename={(item) => { setRenameTarget(item); setRenameValue(item.displayName); }} /></div>
        </main>
      </div>

      <Dialog open={Boolean(renameTarget)} onOpenChange={(open) => !open && setRenameTarget(null)}><DialogContent><DialogHeader><DialogTitle>重命名视频</DialogTitle></DialogHeader><Input value={renameValue} onChange={(event) => setRenameValue(event.target.value)} /><Button onClick={() => void saveRename()}>保存</Button></DialogContent></Dialog>
      <Dialog open={cacheOpen} onOpenChange={setCacheOpen}><DialogContent className="max-w-5xl"><DialogHeader><DialogTitle>缓存视频</DialogTitle></DialogHeader><div className="max-h-[70dvh] overflow-auto"><VideoGrid videos={cachedVideos} activeID={active?.id} onActivate={(item) => { setCacheOpen(false); activate(item, true); }} onExtend={(item) => { setCacheOpen(false); activate(item, true); }} /></div></DialogContent></Dialog>
    </section>
  );
}
