import { ImagePlus, Library, Play, Scissors, Sparkles, Square, Trash2, X } from "lucide-react";
import { useRef, useState, type AnimationEvent } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { usePublicAuth } from "@/public/auth/public-auth";
import { ImageGrid } from "@/public/components/image-grid";
import { PromptEnhanceButton } from "@/public/components/prompt-enhance-button";
import { VideoExtensionResult } from "@/public/components/video-extension-result";
import { VideoGrid, VideoPlayer } from "@/public/components/video-grid";
import { cachedImage, cachedReferenceSource, deleteCachedImages, editImage, generatedImage, imageFromEdit, listCachedImages, startImage, stopImages, streamImage, type CachedImage, type GeneratedImage } from "@/public/features/image/image-api";
import { toggleCacheSelection } from "@/public/features/cache/cache-selection";
import { useVideoFailureNotice } from "@/public/features/video/video-failure-notice";
import { cachedVideo, deleteCachedVideos, listCachedVideos, startVideo, stopVideos, streamVideo, videoPostID, type VideoItem } from "@/public/features/video/video-api";
import { filesToAssets, isHEICFile, type UploadAsset } from "@/public/lib/media";

const supportedLocalImageTypes = new Set(["image/jpeg", "image/png", "image/webp", "image/gif", "image/heic", "image/heif"]);
const maxLocalImageBytes = 20 * 1024 * 1024;

function formatFileSize(bytes: number) {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

export function NsfwPage() {
  const { key } = usePublicAuth();
  const [imagePrompt, setImagePrompt] = useState("");
  const [videoPrompt, setVideoPrompt] = useState("");
  const [extendPrompt, setExtendPrompt] = useState("");
  const [ratio, setRatio] = useState("16:9");
  const [parallel, setParallel] = useState("4");
  const [resolution, setResolution] = useState("480p");
  const [length, setLength] = useState("6");
  const [images, setImages] = useState<GeneratedImage[]>([]);
  const [cachedImages, setCachedImages] = useState<CachedImage[]>([]);
  const [imageCacheOpen, setImageCacheOpen] = useState(false);
  const [imageCacheLoading, setImageCacheLoading] = useState(false);
  const [imageCacheSelected, setImageCacheSelected] = useState(new Set<string>());
  const [selected, setSelected] = useState<GeneratedImage | null>(null);
  const [videos, setVideos] = useState<VideoItem[]>([]);
  const [cachedVideos, setCachedVideos] = useState<VideoItem[]>([]);
  const [activeVideo, setActiveVideo] = useState<VideoItem | null>(null);
  const [extensionResult, setExtensionResult] = useState<VideoItem | null>(null);
  const [cacheOpen, setCacheOpen] = useState(false);
  const [cacheSelected, setCacheSelected] = useState(new Set<string>());
  const [localImage, setLocalImage] = useState<UploadAsset | null>(null);
  const [imageStarting, setImageStarting] = useState(false);
  const [imageRunning, setImageRunning] = useState(false);
  const [videoStarting, setVideoStarting] = useState(false);
  const [videoRunning, setVideoRunning] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editPrompt, setEditPrompt] = useState("");
  const [extendLength, setExtendLength] = useState("6");
  const [extendTime, setExtendTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const imageTasks = useRef<string[]>([]);
  const videoTasks = useRef<string[]>([]);
  const imageControllers = useRef<AbortController[]>([]);
  const videoControllers = useRef(new Map<string, AbortController>());
  const imageStartController = useRef<AbortController | null>(null);
  const videoStartController = useRef<AbortController | null>(null);
  const imageStartLock = useRef(false);
  const videoStartLock = useRef(false);
  const activeImageStreams = useRef(0);
  const extensionPanel = useRef<HTMLElement | null>(null);
  const pendingExtensionScroll = useRef(false);
  const { beginVideoGroup, finishVideoTask } = useVideoFailureNotice();

  async function stopCandidates() {
    imageStartController.current?.abort();
    imageStartController.current = null;
    imageControllers.current.forEach((item) => item.abort());
    const tasks = imageTasks.current;
    imageControllers.current = [];
    imageTasks.current = [];
    activeImageStreams.current = 0;
    imageStartLock.current = false;
    setImageStarting(false);
    setImageRunning(false);
    await stopImages(key, tasks).catch(() => undefined);
  }

  async function generateCandidates() {
    const prompt = imagePrompt.trim();
    if (!prompt || imageStartLock.current) return;
    imageStartLock.current = true;
    setImageStarting(true);
    setImageRunning(true);
    setImages([]);
    setSelected(null);
    const startController = new AbortController();
    imageStartController.current = startController;
    let collected = 0;
    try {
      const settled = await Promise.allSettled(Array.from({ length: 3 }, () => startImage(key, { prompt, aspect_ratio: ratio, nsfw: true, pro: false }, startController.signal)));
      if (startController.signal.aborted) return;
      const starts = settled.flatMap((item) => item.status === "fulfilled" ? [item.value] : []);
      if (!starts.length) {
        const failure = settled.find((item) => item.status === "rejected");
        throw failure?.status === "rejected" ? failure.reason : new Error("候选图任务创建失败");
      }
      if (starts.length < settled.length) toast.warning(`已启动 ${starts.length}/3 路候选任务`);
      imageTasks.current = starts.map((item) => item.task_id);
      imageControllers.current = starts.map(() => new AbortController());
      activeImageStreams.current = starts.length;
      setImageStarting(false);
      starts.forEach((item, index) => {
        const controller = imageControllers.current[index];
        void streamImage(key, item.task_id, (event) => {
          const image = generatedImage(event, prompt);
          if (!image || collected >= 6) return;
          collected += 1;
          setImages((items) => [...items, image]);
          if (collected >= 6) void stopCandidates();
        }, controller.signal).catch((error) => {
          if (!controller.signal.aborted) toast.error(error instanceof Error ? error.message : "候选图连接失败");
        }).finally(() => {
          activeImageStreams.current = Math.max(0, activeImageStreams.current - 1);
          if (!activeImageStreams.current) {
            imageStartLock.current = false;
            setImageRunning(false);
          }
        });
      });
    } catch (error) {
      if (!isAbortError(error)) toast.error(error instanceof Error ? error.message : "候选图创建失败");
      imageStartLock.current = false;
      setImageRunning(false);
    } finally {
      imageStartController.current = null;
      setImageStarting(false);
    }
  }

  async function editSelected() {
    if (!selected || !editPrompt.trim() || editing) return;
    setEditing(true);
    try {
      const payload = await editImage(key, { prompt: editPrompt.trim(), parent_post_id: selected.parentPostID, source_image_url: selected.sourceURL });
      const image = imageFromEdit(payload, editPrompt.trim());
      if (image) {
        setImages((items) => [image, ...items]);
        setSelected(image);
        setEditPrompt("");
        toast.success("候选图编辑完成");
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "编辑失败");
    } finally {
      setEditing(false);
    }
  }

  async function selectLocalReference(files: FileList | File[]) {
    const file = Array.from(files)[0];
    if (!file) return;
    if (!supportedLocalImageTypes.has(file.type.toLowerCase()) && !isHEICFile(file)) {
      toast.error("仅支持 JPEG、PNG、WebP、GIF 或 HEIC 图片");
      return;
    }
    if (file.size > maxLocalImageBytes) {
      toast.error("参考图不能超过 20 MB");
      return;
    }
    try {
      const [asset] = await filesToAssets([file], 1);
      if (!asset?.data) throw new Error("图片内容为空");
      setLocalImage(asset);
      toast.success("本地参考图已就绪");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取本地参考图失败");
    }
  }

  function watchVideo(taskID: string, label: string, prompt: string, isExtension = false, extensionRootPostID = "") {
    const controller = new AbortController();
    let failureReason = "";
    const initialItem: VideoItem = { id: taskID, taskID, url: "", posterURL: "", prompt, progress: 0, status: "running", postID: "", displayName: label, createdAt: Date.now(), originalPostID: extensionRootPostID };
    videoControllers.current.set(taskID, controller);
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
      videoControllers.current.delete(taskID);
      if (!videoControllers.current.size) {
        videoStartLock.current = false;
        setVideoRunning(false);
      }
    });
  }

  async function generateVideos(extension?: VideoItem) {
    const source = localImage?.data || selected?.sourceURL || selected?.url;
    if (!source && !extension) return toast.error("请先选择候选图或上传本地参考图");
    if (extension && !extension.postID) return toast.error("当前视频缺少 postId");
    if (videoStartLock.current) return;
    videoStartLock.current = true;
    setVideoStarting(true);
    setVideoRunning(true);
    const startController = new AbortController();
    videoStartController.current = startController;
    const prompt = extension ? extendPrompt.trim() : videoPrompt.trim();
    if (extension) setExtensionResult(null);
    try {
      const result = await startVideo(key, {
        prompt,
        aspect_ratio: ratio,
        video_length: extension ? Number(extendLength) : Number(length),
        resolution_name: resolution,
        preset: prompt ? "custom" : "spicy",
        concurrent: extension ? 1 : Number(parallel),
        source_image_url: extension ? undefined : source,
        is_video_extension: Boolean(extension),
        source_task_id: extension?.taskID,
        extend_post_id: extension?.postID,
        original_post_id: extension?.originalPostID || extension?.postID,
        file_attachment_id: extension?.originalPostID || extension?.postID,
        video_extension_start_time: extendTime,
        stitch_with_extend: Boolean(extension),
      }, startController.signal);
      if (startController.signal.aborted) return;
      const ids = result.task_ids?.length ? result.task_ids : [result.task_id];
      videoTasks.current = ids;
      beginVideoGroup(ids);
      setVideoStarting(false);
      const extensionRootPostID = extension?.originalPostID || extension?.postID || "";
      ids.forEach((id, index) => watchVideo(id, extension ? "延长视频" : `NSFW 视频 ${index + 1}`, prompt, Boolean(extension), extensionRootPostID));
    } catch (error) {
      if (!isAbortError(error)) toast.error(error instanceof Error ? error.message : "视频任务创建失败");
      if (extension) setExtensionResult(null);
      videoStartLock.current = false;
      setVideoRunning(false);
    } finally {
      videoStartController.current = null;
      setVideoStarting(false);
    }
  }

  async function stopVideoRun() {
    videoStartController.current?.abort();
    videoStartController.current = null;
    videoControllers.current.forEach((item) => item.abort());
    videoControllers.current.clear();
    const tasks = videoTasks.current;
    videoTasks.current = [];
    videoStartLock.current = false;
    setVideoStarting(false);
    setVideoRunning(false);
    setExtensionResult((item) => item?.status === "completed" ? item : null);
    await stopVideos(key, tasks).catch(() => undefined);
  }

  async function openImageCache() {
    setImageCacheOpen(true);
    setImageCacheSelected(new Set());
    setImageCacheLoading(true);
    try {
      const payload = await listCachedImages(key);
      setCachedImages((payload.items || []).map(cachedImage).filter((item) => Boolean(item.url)));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取图片缓存失败");
    } finally {
      setImageCacheLoading(false);
    }
  }

  async function openCache() {
    setCacheOpen(true);
    setCacheSelected(new Set());
    try {
      const payload = await listCachedVideos(key);
      setCachedVideos((payload.items || []).map(cachedVideo));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取缓存失败");
    }
  }

  function chooseCachedImage(image: GeneratedImage) {
    const cached = cachedImages.find((item) => item.id === image.id);
    if (!cached) return;
    setSelected({ ...cached, sourceURL: cachedReferenceSource(cached) });
    setLocalImage(null);
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
      toast.success(`已删除 ${result.deleted} 张`);
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
      if (activeVideo && deleted.has(activeVideo.cacheKey || "")) setActiveVideo(null);
      toast.success(`已删除 ${result.deleted} 个`);
    } catch (error) { toast.error(error instanceof Error ? error.message : "删除视频缓存失败"); }
  }

  function scrollToExtensionPanel() {
    requestAnimationFrame(() => extensionPanel.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
  }

  function selectVideo(item: VideoItem, scroll = false, afterCacheClose = false) {
    setActiveVideo(item);
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
        <div><h1 className="text-xl font-semibold">NSFW 工作台</h1><p className="mt-1 text-sm text-muted-foreground">候选图、图生视频与时间轴延长集中处理</p></div>
        <div className="workspace-actions flex flex-wrap justify-end gap-2"><Button variant="outline" onClick={() => void openImageCache()}><Library className="size-4" />缓存图片</Button><Button variant="outline" onClick={() => void openCache()}><Library className="size-4" />缓存视频</Button><Button variant="outline" onClick={() => { setImages([]); setSelected(null); setLocalImage(null); }}><Trash2 className="size-4" />清空图片</Button><Button variant="outline" onClick={() => { setVideos([]); setActiveVideo(null); setExtensionResult(null); }}><Trash2 className="size-4" />清空视频</Button></div>
      </div>

      <div className="workspace-split">
        <aside className="workspace-controls">
          <div className="workspace-panel p-4">
            <div className="workspace-control-group">
              <h2 className="mb-3 text-sm font-semibold">1. 生成候选图</h2>
              <label className="workspace-field-label">候选图提示词</label>
              <Textarea value={imagePrompt} onChange={(event) => setImagePrompt(event.target.value)} className="min-h-32" placeholder="描述要生成的 NSFW 候选图" />
              <PromptEnhanceButton value={imagePrompt} onEnhanced={setImagePrompt} disabled={imageStarting || imageRunning} />
              <label className="mt-3 block"><span className="workspace-field-label">画面比例</span><Select value={ratio} onValueChange={setRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["16:9", "9:16", "3:2", "2:3", "1:1"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label>
              {imageStarting ? <Button className="mt-3 w-full" disabled><Sparkles className="size-4" />创建任务中...</Button> : imageRunning ? <Button variant="destructive" className="mt-3 w-full" onClick={() => void stopCandidates()}><Square className="size-4" />停止候选</Button> : <Button className="mt-3 w-full" onClick={() => void generateCandidates()} disabled={!imagePrompt.trim()}><Sparkles className="size-4" />生成 6 张</Button>}
            </div>

            <div className="workspace-control-group">
              <h2 className="mb-3 text-sm font-semibold">2. 编辑或选择参考图</h2>
              {localImage ? <div className="relative overflow-hidden rounded-md border bg-muted"><img src={localImage.data} alt={localImage.name} className="aspect-video w-full object-contain" /><Button type="button" variant="secondary" size="icon" className="absolute right-2 top-2 size-8 shadow-sm" onClick={() => setLocalImage(null)} aria-label="移除本地参考图"><X className="size-4" /></Button><div className="flex items-center justify-between gap-3 border-t bg-card px-3 py-2 text-xs"><span className="min-w-0 truncate font-medium">{localImage.name}</span><span className="shrink-0 text-muted-foreground">{localImage.mime.replace("image/", "").toUpperCase()}</span></div></div> : selected ? <img src={selected.url} alt={selected.prompt} className="aspect-video w-full rounded-md border bg-muted object-contain" /> : <div className="workspace-empty grid min-h-28 place-items-center text-sm text-muted-foreground">从右侧选择一张候选图</div>}
              <Textarea value={editPrompt} onChange={(event) => setEditPrompt(event.target.value)} className="mt-3 min-h-24" placeholder="输入图片编辑要求" />
              <PromptEnhanceButton value={editPrompt} onEnhanced={setEditPrompt} disabled={editing} />
              <Button variant="outline" className="mt-2 w-full" onClick={() => void editSelected()} disabled={editing || !selected || !editPrompt.trim()}><Sparkles className="size-4" />{editing ? "编辑中..." : "编辑选中图"}</Button>
              <label className="mt-3 flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-md border text-sm hover:bg-accent"><ImagePlus className="size-4" />{localImage ? "更换本地参考图" : "选择本地参考图"}<input type="file" accept="image/jpeg,image/png,image/webp,image/gif,image/heic,image/heif,.heic,.heif" className="hidden" onChange={(event) => { const input = event.currentTarget; void selectLocalReference(input.files || []).finally(() => { input.value = ""; }); }} /></label>
              {localImage && <p className="mt-2 text-xs text-muted-foreground">{formatFileSize(Math.round((localImage.data.length * 3) / 4))} · 待任务上传</p>}
            </div>

            <div className="workspace-control-group">
              <h2 className="mb-3 text-sm font-semibold">3. 生成视频</h2>
              <label className="workspace-field-label">视频提示词</label>
              <Textarea value={videoPrompt} onChange={(event) => setVideoPrompt(event.target.value)} className="min-h-28" placeholder="留空使用 spicy，填写则使用 custom" />
              <PromptEnhanceButton value={videoPrompt} onEnhanced={setVideoPrompt} disabled={videoStarting || videoRunning} />
              <div className="mt-3 grid grid-cols-2 gap-3"><label><span className="workspace-field-label">并发任务</span><Select value={parallel} onValueChange={setParallel}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["1", "2", "3", "4"].map((value) => <SelectItem key={value} value={value}>{value} 路</SelectItem>)}</SelectContent></Select></label><label><span className="workspace-field-label">分辨率</span><Select value={resolution} onValueChange={setResolution}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["480p", "720p"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label><label className="col-span-2"><span className="workspace-field-label">视频时长</span><Select value={length} onValueChange={setLength}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["6", "10", "15"].map((value) => <SelectItem key={value} value={value}>{value} 秒</SelectItem>)}</SelectContent></Select></label></div>
              {videoStarting ? <Button className="mt-3 w-full" disabled><Play className="size-4" />上传参考图并创建任务...</Button> : videoRunning ? <Button variant="destructive" className="mt-3 w-full" onClick={() => void stopVideoRun()}><Square className="size-4" />中断视频</Button> : <Button className="mt-3 w-full" onClick={() => void generateVideos()} disabled={!selected && !localImage}><Play className="size-4" />生成视频</Button>}
            </div>
          </div>

          <section ref={extensionPanel} className="workspace-panel scroll-mt-20 p-4">
            <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold"><Scissors className="size-4 text-info" />视频时间轴延长</h2>
            <div className="aspect-video w-full min-w-0 overflow-hidden rounded-md bg-muted">
              {activeVideo?.url ? <VideoPlayer url={activeVideo.url} posterURL={activeVideo.posterURL} label={activeVideo.displayName} className="size-full" onLoadedMetadata={(event) => { const nextDuration = event.currentTarget.duration || 0; setDuration(nextDuration); setExtendTime((value) => Math.min(value, nextDuration)); }} /> : <div className="workspace-empty grid size-full place-items-center p-4 text-center text-sm text-muted-foreground">在视频记录中点击剪刀按钮进入延长区</div>}
            </div>
              <div className="mt-3 grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-3">
                <label className="text-xs">时间轴<input type="range" min="0" max={Math.max(duration, 0.001)} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} disabled={!activeVideo?.url} className="mt-3 w-full" /></label>
                <label className="text-xs">起点（秒）<Input type="number" min="0" max={duration || undefined} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} disabled={!activeVideo?.url} className="mt-1 font-mono" /></label>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">当前 {extendTime.toFixed(3)}s / {duration.toFixed(3)}s</p>
              <label className="mt-3 block"><span className="workspace-field-label">延长时长</span><Select value={extendLength} onValueChange={setExtendLength} disabled={!activeVideo?.url}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["6", "10", "15"].map((value) => <SelectItem key={value} value={value}>{value} 秒</SelectItem>)}</SelectContent></Select></label>
              <Textarea value={extendPrompt} onChange={(event) => setExtendPrompt(event.target.value)} disabled={!activeVideo?.url} className="mt-3 min-h-24" placeholder="留空使用 spicy，或描述接下来的画面" />
              <PromptEnhanceButton value={extendPrompt} onEnhanced={setExtendPrompt} disabled={videoStarting || videoRunning} />
              <Button className="mt-3 w-full" onClick={() => activeVideo && void generateVideos(activeVideo)} disabled={videoStarting || videoRunning || !activeVideo?.postID}><Scissors className="size-4" />从 {extendTime.toFixed(3)}s 延长</Button>
              {activeVideo?.url && !activeVideo.postID && <p className="mt-2 text-xs text-warning">当前结果缺少 postId，无法延长</p>}
              <VideoExtensionResult result={extensionResult} onContinue={(item) => selectVideo(item, true)} />
          </section>
        </aside>

        <main className="workspace-results">
          <div><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">候选图片</h2><span className="text-xs text-muted-foreground">{selected ? "已选择 1 张" : `${images.length} 张`}</span></div><ImageGrid images={images} selected={new Set(selected ? [selected.id] : [])} onSelect={(id) => setSelected(images.find((item) => item.id === id) || null)} onOpen={setSelected} onEdit={setSelected} /></div>
          <div><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">视频结果</h2><span className="text-xs text-muted-foreground">{videos.length} 个</span></div><VideoGrid videos={videos} activeID={activeVideo?.id} onActivate={(item) => selectVideo(item)} onExtend={(item) => selectVideo(item, true)} /></div>
        </main>
      </div>
      <Dialog open={imageCacheOpen} onOpenChange={setImageCacheOpen}><DialogContent className="max-w-5xl"><DialogHeader><DialogTitle>缓存图片</DialogTitle></DialogHeader><div className="max-h-[70dvh] overflow-auto"><div className="sticky top-0 z-10 mb-3 flex items-center justify-between gap-2 border-b bg-background py-2"><span className="text-sm text-muted-foreground">点击图片用于视频，或多选删除</span><Button variant="outline" disabled={!imageCacheSelected.size} onClick={() => void removeCachedImages()}><Trash2 className="size-4" />删除所选</Button></div>{imageCacheLoading ? <div className="workspace-empty grid min-h-44 place-items-center text-sm text-muted-foreground">正在读取缓存...</div> : cachedImages.length ? <ImageGrid images={cachedImages} selected={imageCacheSelected} onSelect={(id) => setImageCacheSelected((current) => toggleCacheSelection(current, id))} onOpen={chooseCachedImage} onEdit={chooseCachedImage} /> : <div className="workspace-empty grid min-h-44 place-items-center text-sm text-muted-foreground">暂无缓存图片</div>}</div></DialogContent></Dialog>
      <Dialog open={cacheOpen} onOpenChange={setCacheOpen}><DialogContent className="max-w-5xl" onAnimationEnd={finishCacheDialogClose}><DialogHeader><DialogTitle>缓存视频</DialogTitle></DialogHeader><div className="max-h-[70dvh] overflow-auto"><div className="sticky top-0 z-10 mb-3 flex items-center justify-between gap-2 border-b bg-background py-2"><span className="text-sm text-muted-foreground">已选 {cacheSelected.size} 个</span><Button variant="outline" disabled={!cacheSelected.size} onClick={() => void removeCachedVideos()}><Trash2 className="size-4" />删除所选</Button></div><VideoGrid videos={cachedVideos} activeID={activeVideo?.id} selected={cacheSelected} onSelect={(id) => setCacheSelected((current) => toggleCacheSelection(current, id))} onActivate={(item) => { setCacheOpen(false); selectVideo(item, true, true); }} onExtend={(item) => { setCacheOpen(false); selectVideo(item, true, true); }} /></div></DialogContent></Dialog>
    </section>
  );
}
