import { Check, Download, ExternalLink, Pencil, PlayCircle, Scissors, Square } from "lucide-react";
import type { ReactEventHandler } from "react";

import { Button } from "@/components/ui/button";
import type { VideoItem } from "@/public/features/video/video-api";
import { downloadURL } from "@/public/lib/media";
import { PersistentVideoPreview } from "@/shared/components/persistent-video-preview";
import { cn } from "@/shared/lib/cn";

type VideoGridProps = {
  videos: VideoItem[];
  activeID?: string;
  onActivate: (item: VideoItem) => void;
  onRename?: (item: VideoItem) => void;
  onExtend?: (item: VideoItem) => void;
  selected?: Set<string>;
  onSelect?: (id: string) => void;
};

export function VideoGrid({ videos, activeID, onActivate, onRename, onExtend, selected, onSelect }: VideoGridProps) {
  if (!videos.length) return <div className="workspace-empty grid min-h-44 place-items-center text-sm text-muted-foreground">生成视频与缓存视频会显示在这里</div>;
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {videos.map((item) => (
        <article key={item.taskID || item.id} className={cn("min-w-0 overflow-hidden rounded-md border bg-card shadow-sm", activeID === item.id && "ring-2 ring-inset ring-primary")}>
          <div className="relative aspect-video w-full bg-muted">
            {item.url
              ? <LazyVideoPreview url={item.url} posterURL={item.posterURL} label={item.displayName} />
              : <button className="grid size-full place-items-center px-4 text-center text-sm text-muted-foreground" onClick={() => onActivate(item)}><span className="line-clamp-4 break-words">{item.error || `进度 ${item.progress}%`}</span></button>}
            <span className="pointer-events-none absolute left-2 top-2 flex items-center gap-1 rounded bg-background/90 px-2 py-1 text-xs"><PlayCircle className="size-3" />{item.status === "failed" ? "失败" : item.status === "completed" ? "完成" : `${item.progress}%`}</span>
            {onSelect && <Button variant="secondary" size="icon" className="absolute right-2 top-2 size-8 shadow-sm" onClick={() => onSelect(item.id)} aria-label="选择缓存视频">{selected?.has(item.id) ? <Check className="size-4" /> : <Square className="size-4" />}</Button>}
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

type VideoPlayerProps = {
  url: string;
  posterURL?: string;
  label: string;
  className?: string;
  onLoadedMetadata?: ReactEventHandler<HTMLVideoElement>;
};

export function VideoPlayer({ url, posterURL = "", label, className, onLoadedMetadata }: VideoPlayerProps) {
  return <PersistentVideoPreview url={url} posterURL={posterURL} label={label} className={cn("bg-black", className)} eager onLoadedMetadata={onLoadedMetadata} />;
}

function LazyVideoPreview({ url, posterURL, label }: { url: string; posterURL: string; label: string }) {
  return <PersistentVideoPreview url={url} posterURL={posterURL} label={label} />;
}
