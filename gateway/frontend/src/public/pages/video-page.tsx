import { ImagePlus, Library, Play, RotateCcw, Scissors, Square, Trash2, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type AnimationEvent } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { usePublicAuth } from "@/public/auth/public-auth";
import { PromptEnhanceButton } from "@/public/components/prompt-enhance-button";
import { ImageGrid } from "@/public/components/image-grid";
import { VideoExtensionResult } from "@/public/components/video-extension-result";
import { VideoGrid, VideoPlayer } from "@/public/components/video-grid";
import { cachedImage, cachedReferenceSource, deleteCachedImages, listCachedImages, type CachedImage } from "@/public/features/image/image-api";
import { toggleCacheSelection } from "@/public/features/cache/cache-selection";
import { useVideoFailureNotice } from "@/public/features/video/video-failure-notice";
import { cachedVideo, deleteCachedVideos, listCachedVideos, renameVideo, startVideo, stopVideos, streamVideo, videoPostID, type VideoItem } from "@/public/features/video/video-api";
import { filesToAssets, isImageUploadFile, type UploadAsset } from "@/public/lib/media";

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

export function VideoPage() {
  const { key } = usePublicAuth();
  const [prompt, setPrompt] = useState("");
  const [references, setReferences] = useState<UploadAsset[]>([]);
  const [ratio, setRatio] = useState("3:2");
  const [length, setLength] = useState("6");
  const [resolution, setResolution] = useState("480p");
  const [preset, setPreset] = useState("normal");
  const [concurrent, setConcurrent] = useState("1");
  const [videos, setVideos] = useState<VideoItem[]>([]);
  const [cachedVideos, setCachedVideos] = useState<VideoItem[]>([]);
  const [cachedImages, setCachedImages] = useState<CachedImage[]>([]);
  const [active, setActive] = useState<VideoItem | null>(null);
  const [extensionResult, setExtensionResult] = useState<VideoItem | null>(null);
  const [starting, setStarting] = useState(false);
  const [running, setRunning] = useState(false);
  const [cacheOpen, setCacheOpen] = useState(false);
  const [imageCacheOpen, setImageCacheOpen] = useState(false);
  const [cacheSelected, setCacheSelected] = useState(new Set<string>());
  const [imageCacheSelected, setImageCacheSelected] = useState(new Set<string>());
  const [renameTarget, setRenameTarget] = useState<VideoItem | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [extendPrompt, setExtendPrompt] = useState("");
  const [extendLength, setExtendLength] = useState("6");
  const [extendTime, setExtendTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const controllers = useRef(new Map<string, AbortController>());
  const taskIDs = useRef<string[]>([]);
  const startLock = useRef(false);
  const startController = useRef<AbortController | null>(null);
  const extensionPanel = useRef<HTMLElement | null>(null);
  const pendingExtensionScroll = useRef(false);
  const { beginVideoGroup, finishVideoTask } = useVideoFailureNotice();

  const appendReferenceFiles = useCallback(async (files: FileList | File[]) => {
    try {
      const items = await filesToAssets(files, Math.max(0, 8 - references.length));
      setReferences((value) => [...value, ...items].slice(0, 8));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取参考图失败");
    }
  }, [references.length]);

  useEffect(() => {
    const paste = (event: ClipboardEvent) => {
      const files = Array.from(event.clipboardData?.files || []).filter(isImageUploadFile);
      if (files.length) void appendReferenceFiles(files);
    };
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  }, [appendReferenceFiles]);

  function watch(taskID: string, label: string, taskPrompt: string, isExtension = false, extensionRootPostID = "") {
    const controller = new AbortController();
    let failureReason = "";
    const initialItem: VideoItem = { id: taskID, taskID, url: "", posterURL: "", prompt: taskPrompt, progress: 0, status: "running", postID: "", displayName: label, createdAt: Date.now(), originalPostID: extensionRootPostID };
    controllers.current.set(taskID, controller);
    setVideos((items) => [initialItem, ...items]);
    if (isExtension) setExtensionResult(initialItem);
    void streamVideo(key, taskID, (update) => {
      if (update.error) {
        failureReason = update.error;
        setVideos((items) => items.filter((item) => item.taskID !== taskID));
        if (isExtension) setExtensionResult((item) => item?.taskID === taskID ? null : item);
        return;
      }
      const applyUpdate = (item: VideoItem) => ({ ...item, progress: update.progress ?? item.progress, url: update.url || item.url, posterURL: update.posterURL || item.posterURL, postID: videoPostID(update.url || item.url), status: update.url || update.done ? "completed" as const : "running" as const });
      setVideos((items) => items.map((item) => item.taskID !== taskID ? item : applyUpdate(item)));
      if (isExtension) setExtensionResult((item) => item?.taskID === taskID ? applyUpdate(item) : item);
    }, controller.signal).catch((error) => {
      if (!controller.signal.aborted) {
        failureReason = error instanceof Error ? error.message : "视频连接失败";
        setVideos((items) => items.filter((item) => item.taskID !== taskID));
        if (isExtension) setExtensionResult((item) => item?.taskID === taskID ? null : item);
      }
    }).finally(() => {
      finishVideoTask(taskID, failureReason);
      controllers.current.delete(taskID);
      if (!controllers.current.size) {
        startLock.current = false;
        setRunning(false);
      }
    });
  }

  async function generate(bodyOverride?: Record<string, unknown>, extensionRootPostID = "") {
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
      beginVideoGroup(ids);
      setStarting(false);
      const isExtension = Boolean(bodyOverride?.is_video_extension);
      ids.forEach((id, index) => watch(id, isExtension ? "延长视频" : `视频 ${videos.length + index + 1}`, taskPrompt, isExtension, extensionRootPostID));
    } catch (error) {
      if (!isAbortError(error)) toast.error(error instanceof Error ? error.message : "创建视频失败");
      if (bodyOverride?.is_video_extension) setExtensionResult(null);
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
    setExtensionResult((item) => item?.status === "completed" ? item : null);
    await stopVideos(key, tasks).catch(() => undefined);
  }

  async function openCache() {
    setCacheOpen(true);
    setCacheSelected(new Set());
    try {
      const payload = await listCachedVideos(key);
      const values = (payload.items || []).map(cachedVideo);
      setCachedVideos(values);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取缓存失败");
    }
  }

  async function openImageCache() {
    setImageCacheOpen(true);
    setImageCacheSelected(new Set());
    try {
      const payload = await listCachedImages(key);
      setCachedImages((payload.items || []).map(cachedImage).filter((item) => Boolean(item.url)));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取图片缓存失败");
    }
  }

  function addCachedReferences() {
    const selected = cachedImages.filter((item) => imageCacheSelected.has(item.id));
    setReferences((current) => [...current, ...selected.map((item) => ({ id: item.id, name: item.name, mime: "image/jpeg", data: cachedReferenceSource(item) }))].slice(0, 8));
    setImageCacheOpen(false);
  }

  async function removeCachedImages() {
    const items = cachedImages.filter((item) => imageCacheSelected.has(item.id));
    if (!items.length || !window.confirm(`永久删除所选 ${items.length} 张缓存图片？`)) return;
    try {
      const result = await deleteCachedImages(key, items.map(({ source, cacheKey }) => ({ source, cacheKey })));
      const deleted = new Set(result.deleted_keys);
      setCachedImages((current) => current.filter((item) => !deleted.has(item.cacheKey)));
      setImageCacheSelected(new Set());
      const message = `已删除 ${result.deleted} 张，跳过 ${result.skipped} 张，失败 ${result.failed} 张`;
      if (result.deleted === 0 && result.failed > 0) toast.error(message); else toast.success(message);
    } catch (error) { toast.error(error instanceof Error ? error.message : "删除图片缓存失败"); }
  }

  async function removeCachedVideos() {
    const items = cachedVideos.filter((item) => cacheSelected.has(item.id) && item.source && item.cacheKey);
    if (!items.length || !window.confirm(`永久删除所选 ${items.length} 个缓存视频？`)) return;
    try {
      const result = await deleteCachedVideos(key, items.map((item) => ({ source: item.source!, cacheKey: item.cacheKey! })));
      const deleted = new Set(result.deleted_keys);
      setCachedVideos((current) => current.filter((item) => !deleted.has(item.cacheKey || "")));
      setCacheSelected(new Set());
      if (active && deleted.has(active.cacheKey || "")) setActive(null);
      const message = `已删除 ${result.deleted} 个，跳过 ${result.skipped} 个，失败 ${result.failed} 个`;
      if (result.deleted === 0 && result.failed > 0) toast.error(message); else toast.success(message);
    } catch (error) { toast.error(error instanceof Error ? error.message : "删除视频缓存失败"); }
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
    const extensionSource = active;
    const extensionRootPostID = extensionSource.originalPostID || extensionSource.postID;
    setExtensionResult(null);
    await generate({ prompt: extendPrompt.trim(), concurrent: 1, video_length: Number(extendLength), is_video_extension: true, source_task_id: extensionSource.taskID, extend_post_id: extensionSource.postID, video_extension_start_time: extendTime, original_post_id: extensionRootPostID, file_attachment_id: extensionRootPostID, stitch_with_extend: true }, extensionRootPostID);
  }

  function scrollToExtensionPanel() {
    requestAnimationFrame(() => extensionPanel.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
  }

  function activate(item: VideoItem, scroll = false, afterCacheClose = false) {
    setActive(item);
    setExtensionResult(null);
    setExtendTime(0);
    setDuration(0);
    if (!scroll) return;
    if (afterCacheClose) {
      pendingExtensionScroll.current = true;
      return;
    }
    scrollToExtensionPanel();
  }

  function finishCacheDialogClose(event: AnimationEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget || event.currentTarget.dataset.state !== "closed" || !pendingExtensionScroll.current) return;
    pendingExtensionScroll.current = false;
    scrollToExtensionPanel();
  }

  function updateExtendTime(value: number) {
    setExtendTime(Math.max(0, Math.min(Number.isFinite(value) ? value : 0, Math.max(duration, 0))));
  }

  return (
    <section className="workspace-page">
      <div className="workspace-heading">
        <div><h1 className="text-xl font-semibold">Video 视频工作台</h1><p className="mt-1 text-sm text-muted-foreground">{starting ? "正在创建任务" : running ? "任务运行中" : `${videos.length} 个视频`}</p></div>
        <div className="workspace-actions flex gap-2"><Button variant="outline" onClick={() => void openCache()}><Library className="size-4" />缓存视频</Button><Button variant="outline" onClick={() => { setVideos([]); setActive(null); setExtensionResult(null); }}><RotateCcw className="size-4" />清空</Button></div>
      </div>

      <div className="workspace-split">
        <aside className="workspace-controls">
          <div className="workspace-panel p-4">
            <div className="workspace-control-group">
              <div className="mb-3 flex flex-wrap gap-2"><label className="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm hover:bg-accent"><ImagePlus className="size-4" />添加参考图<input type="file" accept="image/*,.heic,.heif" multiple className="hidden" onChange={(event) => { const input = event.currentTarget; void appendReferenceFiles(input.files || []).finally(() => { input.value = ""; }); }} /></label><Button variant="outline" onClick={() => void openImageCache()}><Library className="size-4" />从缓存选择</Button><span className="self-center text-xs text-muted-foreground">{references.length}/8</span></div>
            {references.length ? <div className="grid grid-cols-4 gap-2 sm:grid-cols-6">{references.map((item) => <div key={item.id} className="relative aspect-square overflow-hidden rounded-md border"><img src={item.data} alt={item.name} className="size-full object-cover" /><button data-slot="icon-button" className="absolute right-0 top-0 grid size-8 place-items-center bg-background/90" onClick={() => setReferences((values) => values.filter((value) => value.id !== item.id))} aria-label={`移除 ${item.name}`}><X className="size-4" /></button></div>)}</div> : <p className="text-sm text-muted-foreground">支持最多 8 张图片、拖放与粘贴</p>}
            </div>
            <div className="workspace-control-group">
              <label className="workspace-field-label">视频提示词</label><Textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} className="min-h-32" placeholder="描述镜头、动作和风格" />
              <PromptEnhanceButton value={prompt} onEnhanced={setPrompt} disabled={starting || running} />
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
          <div className="aspect-video w-full min-w-0 overflow-hidden rounded-md bg-muted">
            {active?.url ? <VideoPlayer url={active.url} posterURL={active.posterURL} label={active.displayName} className="size-full" onLoadedMetadata={(event) => { const nextDuration = event.currentTarget.duration || 0; setDuration(nextDuration); setExtendTime((value) => Math.min(value, nextDuration)); }} /> : <div className="workspace-empty grid size-full place-items-center p-4 text-center text-sm text-muted-foreground">在视频记录中点击剪刀按钮进入延长区</div>}
          </div>
            <div className="mt-3 grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-3"><label className="text-xs">时间轴<input type="range" min="0" max={Math.max(duration, 0.001)} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} disabled={!active?.url} className="mt-3 w-full" /></label><label className="text-xs">起点（秒）<Input type="number" min="0" max={duration || undefined} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} disabled={!active?.url} className="mt-1 font-mono" /></label></div>
            <p className="mt-1 text-xs text-muted-foreground">当前 {extendTime.toFixed(3)}s / {duration.toFixed(3)}s</p>
            <label className="mt-3 block"><span className="workspace-field-label">延长时长</span><Select value={extendLength} onValueChange={setExtendLength} disabled={!active?.url}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["6", "10", "15"].map((value) => <SelectItem key={value} value={value}>{value} 秒</SelectItem>)}</SelectContent></Select></label>
            <Textarea value={extendPrompt} onChange={(event) => setExtendPrompt(event.target.value)} disabled={!active?.url} className="mt-3 min-h-24" placeholder="留空使用 spicy，或描述接下来的画面" />
            <PromptEnhanceButton value={extendPrompt} onEnhanced={setExtendPrompt} disabled={starting || running} />
            <Button className="mt-3 w-full" onClick={() => void extend()} disabled={starting || running || !active?.postID}><Scissors className="size-4" />从 {extendTime.toFixed(3)}s 延长</Button>
            {active?.url && !active.postID && <p className="mt-2 text-xs text-warning">从缓存选择带 postId 的视频后可延长</p>}
            <VideoExtensionResult result={extensionResult} onContinue={(item) => activate(item, true)} />
          </section>
        </aside>

        <main className="workspace-results">
          <div><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">视频历史</h2><span className="text-xs text-muted-foreground">{videos.length} 个</span></div><VideoGrid videos={videos} activeID={active?.id} onActivate={(item) => activate(item)} onExtend={(item) => activate(item, true)} onRename={(item) => { setRenameTarget(item); setRenameValue(item.displayName); }} /></div>
        </main>
      </div>

      <Dialog open={Boolean(renameTarget)} onOpenChange={(open) => !open && setRenameTarget(null)}><DialogContent><DialogHeader><DialogTitle>重命名视频</DialogTitle></DialogHeader><Input value={renameValue} onChange={(event) => setRenameValue(event.target.value)} /><Button onClick={() => void saveRename()}>保存</Button></DialogContent></Dialog>
      <Dialog open={imageCacheOpen} onOpenChange={setImageCacheOpen}><DialogContent className="max-w-5xl"><DialogHeader><DialogTitle>选择缓存图片</DialogTitle></DialogHeader><div className="max-h-[70dvh] overflow-auto"><div className="sticky top-0 z-10 mb-3 flex items-center justify-between gap-2 border-b bg-background py-2"><span className="text-sm text-muted-foreground">已选 {imageCacheSelected.size} 张，最多添加至 8 张</span><div className="flex gap-2"><Button variant="outline" disabled={!imageCacheSelected.size} onClick={() => void removeCachedImages()}><Trash2 className="size-4" />删除所选</Button><Button disabled={!imageCacheSelected.size || references.length >= 8} onClick={addCachedReferences}><ImagePlus className="size-4" />添加所选</Button></div></div><ImageGrid images={cachedImages} selected={imageCacheSelected} onSelect={(id) => setImageCacheSelected((current) => toggleCacheSelection(current, id))} onOpen={(image) => setImageCacheSelected((current) => toggleCacheSelection(current, image.id))} /></div></DialogContent></Dialog>
      <Dialog open={cacheOpen} onOpenChange={setCacheOpen}><DialogContent className="max-w-5xl" onAnimationEnd={finishCacheDialogClose}><DialogHeader><DialogTitle>缓存视频</DialogTitle></DialogHeader><div className="max-h-[70dvh] overflow-auto"><div className="sticky top-0 z-10 mb-3 flex items-center justify-between gap-2 border-b bg-background py-2"><span className="text-sm text-muted-foreground">已选 {cacheSelected.size} 个</span><Button variant="outline" disabled={!cacheSelected.size} onClick={() => void removeCachedVideos()}><Trash2 className="size-4" />删除所选</Button></div><VideoGrid videos={cachedVideos} activeID={active?.id} selected={cacheSelected} onSelect={(id) => setCacheSelected((current) => toggleCacheSelection(current, id))} onActivate={(item) => { setCacheOpen(false); activate(item, true, true); }} onExtend={(item) => { setCacheOpen(false); activate(item, true, true); }} /></div></DialogContent></Dialog>
    </section>
  );
}
