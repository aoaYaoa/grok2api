import { ExternalLink, LoaderCircle, Scissors } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { VideoItem } from "@/public/features/video/video-api";
import { PersistentVideoPreview } from "@/shared/components/persistent-video-preview";

type VideoExtensionResultProps = {
  result: VideoItem | null;
  onContinue: (result: VideoItem) => void;
};

export function VideoExtensionResult({ result, onContinue }: VideoExtensionResultProps) {
  if (!result) {
    return (
      <div className="mt-4 flex min-h-24 items-center border-t pt-4 text-sm text-muted-foreground">
        延长结果会显示在这里，当前源视频不会被替换。
      </div>
    );
  }

  if (!result.url) {
    const progress = Math.max(0, Math.min(result.progress || 0, 100));
    return (
      <div className="mt-4 min-h-24 border-t pt-4" aria-live="polite">
        <div className="flex items-center gap-3">
          <LoaderCircle className="size-5 shrink-0 animate-spin text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="font-medium">延长生成中</span>
              <span className="font-mono text-xs text-muted-foreground">{progress}%</span>
            </div>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-secondary">
              <div className="h-full rounded-full bg-primary transition-[width] duration-300" style={{ width: `${progress}%` }} />
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mt-4 min-h-24 border-t pt-4" aria-live="polite">
      <div className="flex min-w-0 items-center gap-3">
        <div className="aspect-video w-28 shrink-0 overflow-hidden rounded-md bg-muted">
          <PersistentVideoPreview url={result.url} posterURL={result.posterURL} label={result.displayName} controls={false} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">延长结果</p>
          <p className="mt-1 truncate text-xs text-muted-foreground">{result.displayName}</p>
        </div>
        <Button variant="ghost" size="icon" asChild>
          <a href={result.url} target="_blank" rel="noreferrer" aria-label="打开延长结果">
            <ExternalLink className="size-4" />
          </a>
        </Button>
      </div>
      <Button variant="outline" className="mt-3 w-full" onClick={() => onContinue(result)} disabled={!result.postID}>
        <Scissors className="size-4" />以此结果继续延长
      </Button>
      {!result.postID && <p className="mt-2 text-xs text-warning">结果缺少 postId，暂时不能继续延长</p>}
    </div>
  );
}
