import { Check, Download, Maximize2, Pencil, Square } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { GeneratedImage } from "@/public/features/image/image-api";
import { downloadURL } from "@/public/lib/media";
import { cn } from "@/shared/lib/cn";

export function ImageGrid({ images, selected, onSelect, onOpen, onEdit }: { images: GeneratedImage[]; selected?: Set<string>; onSelect?: (id: string) => void; onOpen: (image: GeneratedImage) => void; onEdit?: (image: GeneratedImage) => void }) {
  if (!images.length) return <div className="grid min-h-56 place-items-center border-y text-sm text-muted-foreground">生成结果会显示在这里</div>;
  return <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">{images.map((image, index) => <article key={`${image.id}-${image.createdAt}`} className={cn("overflow-hidden border bg-card", selected?.has(image.id) && "ring-2 ring-primary")}>
    <button className="relative block aspect-[2/3] w-full overflow-hidden bg-muted" onClick={() => onOpen(image)} aria-label={`预览图片 ${index + 1}`}><img src={image.url} alt={image.prompt || `生成图片 ${index + 1}`} className="size-full object-cover" loading="lazy" /><span className="absolute right-2 top-2 grid size-8 place-items-center rounded-md bg-background/85"><Maximize2 className="size-4" /></span></button>
    <div className="flex h-11 items-center justify-between gap-1 px-2"><span className="min-w-0 truncate text-xs text-muted-foreground">#{index + 1}{image.elapsedMS ? ` · ${image.elapsedMS}ms` : ""}</span><div className="flex">{onSelect && <Button size="icon" variant="ghost" className="size-8" onClick={() => onSelect(image.id)} aria-label="选择图片">{selected?.has(image.id) ? <Check className="size-4" /> : <Square className="size-4" />}</Button>}{onEdit && <Button size="icon" variant="ghost" className="size-8" onClick={() => onEdit(image)} aria-label="编辑图片"><Pencil className="size-4" /></Button>}<Button size="icon" variant="ghost" className="size-8" onClick={() => downloadURL(image.url, `imagine-${Date.now()}.jpg`)} aria-label="下载图片"><Download className="size-4" /></Button></div></div>
  </article>)}</div>;
}
