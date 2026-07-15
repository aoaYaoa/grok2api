import { ImagePlus, Play, Scissors, Sparkles, Square, Trash2, X } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { usePublicAuth } from "@/public/auth/public-auth";
import { ImageGrid } from "@/public/components/image-grid";
import { VideoGrid } from "@/public/components/video-grid";
import { editImage, generatedImage, imageFromEdit, startImage, stopImages, streamImage, type GeneratedImage } from "@/public/features/image/image-api";
import { startVideo, stopVideos, streamVideo, videoPostID, type VideoItem } from "@/public/features/video/video-api";
import { filesToAssets, type UploadAsset } from "@/public/lib/media";

const supportedLocalImageTypes = new Set(["image/jpeg", "image/png", "image/webp", "image/gif"]);
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
  const [selected, setSelected] = useState<GeneratedImage | null>(null);
  const [videos, setVideos] = useState<VideoItem[]>([]);
  const [activeVideo, setActiveVideo] = useState<VideoItem | null>(null);
  const [localImage, setLocalImage] = useState<UploadAsset | null>(null);
  const [imageStarting, setImageStarting] = useState(false);
  const [imageRunning, setImageRunning] = useState(false);
  const [videoStarting, setVideoStarting] = useState(false);
  const [videoRunning, setVideoRunning] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editPrompt, setEditPrompt] = useState("");
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
    if (!supportedLocalImageTypes.has(file.type.toLowerCase())) {
      toast.error("仅支持 JPEG、PNG、WebP 或 GIF 图片");
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

  function watchVideo(taskID: string, label: string, prompt: string) {
    const controller = new AbortController();
    videoControllers.current.set(taskID, controller);
    setVideos((items) => [{ id: taskID, taskID, url: "", prompt, progress: 0, status: "running", postID: "", displayName: label, createdAt: Date.now() }, ...items]);
    void streamVideo(key, taskID, (update) => setVideos((items) => items.map((item) => item.taskID !== taskID ? item : { ...item, progress: update.progress ?? item.progress, url: update.url || item.url, postID: videoPostID(update.url || item.url), status: update.error ? "failed" : update.url || update.done ? "completed" : "running", error: update.error })), controller.signal).catch((error) => {
      if (!controller.signal.aborted) toast.error(error instanceof Error ? error.message : "视频连接失败");
    }).finally(() => {
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
    try {
      const result = await startVideo(key, {
        prompt,
        aspect_ratio: ratio,
        video_length: Number(length),
        resolution_name: resolution,
        preset: prompt ? "custom" : "spicy",
        concurrent: extension ? 1 : Number(parallel),
        image_url: extension ? undefined : source,
        source_image_url: extension ? undefined : source,
        is_video_extension: Boolean(extension),
        extend_post_id: extension?.postID,
        original_post_id: extension?.originalPostID || extension?.postID,
        file_attachment_id: extension?.originalPostID || extension?.postID,
        video_extension_start_time: extendTime,
        stitch_with_extend: Boolean(extension),
      }, startController.signal);
      if (startController.signal.aborted) return;
      const ids = result.task_ids?.length ? result.task_ids : [result.task_id];
      videoTasks.current = ids;
      setVideoStarting(false);
      ids.forEach((id, index) => watchVideo(id, extension ? "延长视频" : `NSFW 视频 ${index + 1}`, prompt));
    } catch (error) {
      if (!isAbortError(error)) toast.error(error instanceof Error ? error.message : "视频任务创建失败");
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
    await stopVideos(key, tasks).catch(() => undefined);
  }

  function selectVideo(item: VideoItem, scroll = false) {
    setActiveVideo(item);
    setExtendTime(0);
    if (scroll) requestAnimationFrame(() => extensionPanel.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
  }

  function updateExtendTime(value: number) {
    setExtendTime(Math.max(0, Math.min(Number.isFinite(value) ? value : 0, Math.max(duration, 0))));
  }

  return (
    <section className="workspace-page">
      <div className="workspace-heading">
        <div><h1 className="text-xl font-semibold">NSFW 工作台</h1><p className="mt-1 text-sm text-muted-foreground">候选图、图生视频与时间轴延长集中处理</p></div>
        <div className="flex gap-2"><Button variant="outline" onClick={() => { setImages([]); setSelected(null); setLocalImage(null); }}><Trash2 className="size-4" />清空图片</Button><Button variant="outline" onClick={() => { setVideos([]); setActiveVideo(null); }}><Trash2 className="size-4" />清空视频</Button></div>
      </div>

      <div className="workspace-split">
        <aside className="workspace-controls">
          <div className="workspace-panel p-4">
            <div className="workspace-control-group">
              <h2 className="mb-3 text-sm font-semibold">1. 生成候选图</h2>
              <label className="workspace-field-label">候选图提示词</label>
              <Textarea value={imagePrompt} onChange={(event) => setImagePrompt(event.target.value)} className="min-h-32" placeholder="描述要生成的 NSFW 候选图" />
              <label className="mt-3 block"><span className="workspace-field-label">画面比例</span><Select value={ratio} onValueChange={setRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["16:9", "9:16", "3:2", "2:3", "1:1"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label>
              {imageStarting ? <Button className="mt-3 w-full" disabled><Sparkles className="size-4" />创建任务中...</Button> : imageRunning ? <Button variant="destructive" className="mt-3 w-full" onClick={() => void stopCandidates()}><Square className="size-4" />停止候选</Button> : <Button className="mt-3 w-full" onClick={() => void generateCandidates()} disabled={!imagePrompt.trim()}><Sparkles className="size-4" />生成 6 张</Button>}
            </div>

            <div className="workspace-control-group">
              <h2 className="mb-3 text-sm font-semibold">2. 编辑或选择参考图</h2>
              {localImage ? <div className="relative overflow-hidden rounded-md border bg-muted"><img src={localImage.data} alt={localImage.name} className="aspect-video w-full object-contain" /><Button type="button" variant="secondary" size="icon" className="absolute right-2 top-2 size-8 shadow-sm" onClick={() => setLocalImage(null)} aria-label="移除本地参考图"><X className="size-4" /></Button><div className="flex items-center justify-between gap-3 border-t bg-card px-3 py-2 text-xs"><span className="min-w-0 truncate font-medium">{localImage.name}</span><span className="shrink-0 text-muted-foreground">{localImage.mime.replace("image/", "").toUpperCase()}</span></div></div> : selected ? <img src={selected.url} alt={selected.prompt} className="aspect-video w-full rounded-md border bg-muted object-contain" /> : <div className="workspace-empty grid min-h-28 place-items-center text-sm text-muted-foreground">从右侧选择一张候选图</div>}
              <Textarea value={editPrompt} onChange={(event) => setEditPrompt(event.target.value)} className="mt-3 min-h-24" placeholder="输入图片编辑要求" />
              <Button variant="outline" className="mt-2 w-full" onClick={() => void editSelected()} disabled={editing || !selected || !editPrompt.trim()}><Sparkles className="size-4" />{editing ? "编辑中..." : "编辑选中图"}</Button>
              <label className="mt-3 flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-md border text-sm hover:bg-accent"><ImagePlus className="size-4" />{localImage ? "更换本地参考图" : "选择本地参考图"}<input type="file" accept="image/jpeg,image/png,image/webp,image/gif" className="hidden" onChange={(event) => { const input = event.currentTarget; void selectLocalReference(input.files || []).finally(() => { input.value = ""; }); }} /></label>
              {localImage && <p className="mt-2 text-xs text-muted-foreground">{formatFileSize(Math.round((localImage.data.length * 3) / 4))} · 待任务上传</p>}
            </div>

            <div className="workspace-control-group">
              <h2 className="mb-3 text-sm font-semibold">3. 生成视频</h2>
              <label className="workspace-field-label">视频提示词</label>
              <Textarea value={videoPrompt} onChange={(event) => setVideoPrompt(event.target.value)} className="min-h-28" placeholder="留空使用 spicy，填写则使用 custom" />
              <div className="mt-3 grid grid-cols-2 gap-3"><label><span className="workspace-field-label">并发任务</span><Select value={parallel} onValueChange={setParallel}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["1", "2", "3", "4"].map((value) => <SelectItem key={value} value={value}>{value} 路</SelectItem>)}</SelectContent></Select></label><label><span className="workspace-field-label">分辨率</span><Select value={resolution} onValueChange={setResolution}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["480p", "720p"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select></label><label className="col-span-2"><span className="workspace-field-label">视频时长</span><Select value={length} onValueChange={setLength}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["6", "10", "15"].map((value) => <SelectItem key={value} value={value}>{value} 秒</SelectItem>)}</SelectContent></Select></label></div>
              {videoStarting ? <Button className="mt-3 w-full" disabled><Play className="size-4" />上传参考图并创建任务...</Button> : videoRunning ? <Button variant="destructive" className="mt-3 w-full" onClick={() => void stopVideoRun()}><Square className="size-4" />中断视频</Button> : <Button className="mt-3 w-full" onClick={() => void generateVideos()} disabled={!selected && !localImage}><Play className="size-4" />生成视频</Button>}
            </div>
          </div>

          <section ref={extensionPanel} className="workspace-panel scroll-mt-20 p-4">
            <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold"><Scissors className="size-4 text-info" />视频时间轴延长</h2>
            {activeVideo?.url ? <>
              <video src={activeVideo.url} controls playsInline className="aspect-video w-full rounded-md bg-black" onLoadedMetadata={(event) => { const nextDuration = event.currentTarget.duration || 0; setDuration(nextDuration); setExtendTime((value) => Math.min(value, nextDuration)); }} />
              <div className="mt-3 grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-3">
                <label className="text-xs">时间轴<input type="range" min="0" max={Math.max(duration, 0.001)} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} className="mt-3 w-full" /></label>
                <label className="text-xs">起点（秒）<Input type="number" min="0" max={duration || undefined} step="0.001" value={extendTime} onChange={(event) => updateExtendTime(Number(event.target.value))} className="mt-1 font-mono" /></label>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">当前 {extendTime.toFixed(3)}s / {duration.toFixed(3)}s</p>
              <Textarea value={extendPrompt} onChange={(event) => setExtendPrompt(event.target.value)} className="mt-3 min-h-24" placeholder="留空使用 spicy，或描述接下来的画面" />
              <Button className="mt-3 w-full" onClick={() => void generateVideos(activeVideo)} disabled={videoStarting || videoRunning || !activeVideo.postID}><Scissors className="size-4" />从 {extendTime.toFixed(3)}s 延长</Button>
              {!activeVideo.postID && <p className="mt-2 text-xs text-warning">当前结果缺少 postId，无法延长</p>}
            </> : <div className="workspace-empty grid min-h-48 place-items-center p-4 text-center text-sm text-muted-foreground">在视频记录中点击剪刀按钮进入延长区</div>}
          </section>
        </aside>

        <main className="workspace-results">
          <div><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">候选图片</h2><span className="text-xs text-muted-foreground">{selected ? "已选择 1 张" : `${images.length} 张`}</span></div><ImageGrid images={images} selected={new Set(selected ? [selected.id] : [])} onSelect={(id) => setSelected(images.find((item) => item.id === id) || null)} onOpen={setSelected} onEdit={setSelected} /></div>
          <div><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">视频结果</h2><span className="text-xs text-muted-foreground">{videos.length} 个</span></div><VideoGrid videos={videos} activeID={activeVideo?.id} onActivate={(item) => selectVideo(item)} onExtend={(item) => selectVideo(item, true)} /></div>
        </main>
      </div>
    </section>
  );
}
