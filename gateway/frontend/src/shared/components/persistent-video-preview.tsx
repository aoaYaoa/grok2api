import { PlayCircle } from "lucide-react";
import { useEffect, useRef, useState, type ReactEventHandler } from "react";

import { cn } from "@/shared/lib/cn";
import { loadedVideoURLs } from "@/shared/lib/video-preview-cache";

type PersistentVideoPreviewProps = {
  url: string;
  posterURL?: string;
  label: string;
  className?: string;
  videoClassName?: string;
  eager?: boolean;
  controls?: boolean;
  onLoadedMetadata?: ReactEventHandler<HTMLVideoElement>;
};

export function PersistentVideoPreview({
  url,
  posterURL = "",
  label,
  className,
  videoClassName,
  eager = false,
  controls = true,
  onLoadedMetadata,
}: PersistentVideoPreviewProps) {
  const frame = useRef<HTMLDivElement>(null);
  const video = useRef<HTMLVideoElement>(null);
  const previouslyLoaded = loadedVideoURLs.has(url);
  const supportsIntersectionObserver = typeof window !== "undefined" && "IntersectionObserver" in window;
  const [requestedURL, setRequestedURL] = useState(() => eager || previouslyLoaded || !supportsIntersectionObserver ? url : "");
  const [readyURL, setReadyURL] = useState(() => previouslyLoaded ? url : "");
  const shouldLoad = eager || loadedVideoURLs.has(url) || requestedURL === url || !supportsIntersectionObserver;
  const frameReady = Boolean(posterURL) || loadedVideoURLs.has(url) || readyURL === url;

  useEffect(() => {
    const node = frame.current;
    if (!node || eager || !("IntersectionObserver" in window)) return;
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) {
        setRequestedURL(url);
      } else {
        video.current?.pause();
      }
    }, { rootMargin: "240px 0px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [eager, url]);

  function markReady() {
    loadedVideoURLs.add(url);
    setReadyURL(url);
  }

  function startPlayback() {
    setRequestedURL(url);
    requestAnimationFrame(() => void video.current?.play().catch(() => undefined));
  }

  return (
    <div ref={frame} className={cn("relative size-full overflow-hidden bg-muted", className)}>
      <video
        ref={video}
        src={shouldLoad ? videoPreviewSource(url) : undefined}
        poster={posterURL || undefined}
        controls={controls}
        muted={!controls}
        playsInline
        preload={shouldLoad ? "auto" : "none"}
        className={cn("size-full object-contain transition-opacity", frameReady ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0", videoClassName)}
        onLoadedMetadata={(event) => {
          onLoadedMetadata?.(event);
          seekPreviewFrame(event.currentTarget);
        }}
        onLoadedData={markReady}
        onCanPlay={markReady}
        onSeeked={markReady}
      />
      {!frameReady ? (
        <button type="button" className="video-preview-placeholder absolute inset-0 grid size-full place-items-center bg-muted text-muted-foreground" onClick={startPlayback} aria-label={`播放 ${label}`}>
          <span className="grid justify-items-center gap-2 px-4 text-center text-xs">
            <PlayCircle className="size-10" strokeWidth={1.5} />
            <span className="line-clamp-2">{shouldLoad ? "正在加载封面" : label}</span>
          </span>
        </button>
      ) : null}
    </div>
  );
}

function videoPreviewSource(url: string) {
  const hashIndex = url.indexOf("#");
  return `${hashIndex >= 0 ? url.slice(0, hashIndex) : url}#t=0.001`;
}

function seekPreviewFrame(element: HTMLVideoElement) {
  if (Number.isFinite(element.duration) && element.duration > 0 && element.currentTime < 0.001) {
    element.currentTime = Math.min(0.001, element.duration / 2);
  }
}
