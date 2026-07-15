import { Download, ExternalLink, Pencil, PlayCircle, Scissors } from "lucide-react";

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
        <article key={`${item.id}-${item.createdAt}`} className={cn("overflow-hidden rounded-md border bg-card shadow-sm", activeID === item.id && "ring-2 ring-primary")}>
          <div className="relative aspect-video w-full bg-muted">
            {item.url
              ? <video src={item.url} controls playsInline preload="metadata" className="size-full object-contain" />
              : <button className="grid size-full place-items-center text-sm text-muted-foreground" onClick={() => onActivate(item)}>{item.error || `进度 ${item.progress}%`}</button>}
            <span className="pointer-events-none absolute left-2 top-2 flex items-center gap-1 rounded bg-background/90 px-2 py-1 text-xs"><PlayCircle className="size-3" />{item.status === "failed" ? "失败" : item.status === "completed" ? "完成" : `${item.progress}%`}</span>
          </div>
          <div className="flex h-12 items-center gap-1 px-2">
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
