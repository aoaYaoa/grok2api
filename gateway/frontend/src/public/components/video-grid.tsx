import { Download, ExternalLink, Pencil, PlayCircle, Scissors } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import type { VideoItem } from "@/public/features/video/video-api";
import { downloadURL } from "@/public/lib/media";
import { cn } from "@/shared/lib/cn";

type VideoGridProps = {
  videos: VideoItem[];
  activeID?: string;
  onActivate: (item: VideoItem) => void;
  onRename?: (item: VideoItem) => void;
  onExtend?: (item: VideoItem) => void;
};

export function VideoGrid({ videos, activeID, onActivate, onRename, onExtend }: VideoGridProps) {
  if (!videos.length) return <div className="workspace-empty grid min-h-44 place-items-center text-sm text-muted-foreground">生成视频与缓存视频会显示在这里</div>;
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {videos.map((item) => (
        <article key={`${item.id}-${item.createdAt}`} className={cn("min-w-0 overflow-hidden rounded-md border bg-card shadow-sm", activeID === item.id && "ring-2 ring-inset ring-primary")}>
          <div className="relative aspect-video w-full bg-muted">
            {item.url
              ? <LazyVideoPreview url={item.url} label={item.displayName} />
              : <button className="grid size-full place-items-center px-4 text-center text-sm text-muted-foreground" onClick={() => onActivate(item)}><span className="line-clamp-4 break-words">{item.error || `进度 ${item.progress}%`}</span></button>}
            <span className="pointer-events-none absolute left-2 top-2 flex items-center gap-1 rounded bg-background/90 px-2 py-1 text-xs"><PlayCircle className="size-3" />{item.status === "failed" ? "失败" : item.status === "completed" ? "完成" : `${item.progress}%`}</span>
          </div>
          <div className="flex h-12 items-center gap-1.5 px-3">
            <span className="min-w-0 flex-1 truncate text-sm">{item.displayName}</span>
            {item.url && <>
              {onExtend && <Button variant="ghost" size="icon" className="size-8 text-info" onClick={() => onExtend(item)} disabled={!item.postID} aria-label="选择此视频进行延长"><Scissors className="size-4" /></Button>}
              <Button variant="ghost" size="icon" className="size-8" asChild><a href={item.url} target="_blank" rel="noreferrer" aria-label="打开视频"><ExternalLink className="size-4" /></a></Button>
              <Button variant="ghost" size="icon" className="size-8" onClick={() => downloadURL(item.url, `${item.displayName || "video"}.mp4`)} aria-label="下载视频"><Download className="size-4" /></Button>
              {onRename && <Button variant="ghost" size="icon" className="size-8" onClick={() => onRename(item)} aria-label="重命名视频"><Pencil className="size-4" /></Button>}
            </>}
          </div>
        </article>
      ))}
    </div>
  );
}

function LazyVideoPreview({ url, label }: { url: string; label: string }) {
  const frame = useRef<HTMLDivElement>(null);
  const video = useRef<HTMLVideoElement>(null);
  const [nearViewport, setNearViewport] = useState(() => typeof window === "undefined" || !("IntersectionObserver" in window));
  const [readyURL, setReadyURL] = useState("");
  const frameReady = readyURL === url;

  useEffect(() => {
    const node = frame.current;
    if (!node) return;
    if (!("IntersectionObserver" in window)) return;
    const observer = new IntersectionObserver(([entry]) => {
      setNearViewport(entry.isIntersecting);
      if (!entry.isIntersecting) setReadyURL("");
    }, { rootMargin: "240px 0px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const element = video.current;
    if (!nearViewport || !element) return;
    element.preload = "metadata";
    element.src = url;
    element.load();
    return () => {
      element.pause();
      element.removeAttribute("src");
      element.load();
    };
  }, [nearViewport, url]);

  return (
    <div ref={frame} className="relative size-full overflow-hidden bg-muted">
      <video
        ref={video}
        controls
        playsInline
        preload="none"
        className={cn("size-full object-contain transition-opacity", frameReady ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0")}
        onLoadedMetadata={(event) => {
          const element = event.currentTarget;
          if (Number.isFinite(element.duration) && element.duration > 0) {
            element.currentTime = Math.min(0.05, element.duration / 2);
          }
        }}
        onLoadedData={() => setReadyURL(url)}
        onSeeked={() => setReadyURL(url)}
      />
      {!frameReady ? <button type="button" className="video-preview-placeholder absolute inset-0 grid size-full place-items-center bg-muted text-muted-foreground" onClick={() => { setNearViewport(true); void video.current?.play().catch(() => undefined); }} aria-label={`播放 ${label}`}><span className="grid justify-items-center gap-2 px-4 text-center text-xs"><PlayCircle className="size-10" strokeWidth={1.5} /><span className="line-clamp-2">{nearViewport ? "正在加载封面" : label}</span></span></button> : null}
    </div>
  );
}
