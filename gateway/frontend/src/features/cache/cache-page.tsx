import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, HardDrive, Image as ImageIcon, Pencil, RefreshCw, Trash2, Video } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { clearCache, deleteCacheItem, getCacheStats, listCacheItems, renameCachedVideo, type CacheItem, type CacheType } from "@/features/cache/cache-api";
import { ErrorState, LoadingState } from "@/shared/components/data-state";
import { PageHeader } from "@/shared/components/page-header";
import { Pagination } from "@/shared/components/pagination";
import { PersistentVideoPreview } from "@/shared/components/persistent-video-preview";
import { runtimeConfig } from "@/shared/config/runtime-config";

export function CachePage() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const [type, setType] = useState<CacheType>("image");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [deleteItem, setDeleteItem] = useState<CacheItem>();
  const [clearOpen, setClearOpen] = useState(false);
  const [renameItem, setRenameItem] = useState<CacheItem>();
  const [displayName, setDisplayName] = useState("");

  const statsQuery = useQuery({ queryKey: ["legacy-cache", "stats"], queryFn: getCacheStats });
  const itemsQuery = useQuery({ queryKey: ["legacy-cache", "items", type, page, pageSize], queryFn: () => listCacheItems(type, page, pageSize) });
  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["legacy-cache", "stats"] }),
      queryClient.invalidateQueries({ queryKey: ["legacy-cache", "items"] }),
    ]);
  };
  const deleteMutation = useMutation({
    mutationFn: ({ itemType, name }: { itemType: CacheType; name: string }) => deleteCacheItem(itemType, name),
    onSuccess: async () => { setDeleteItem(undefined); await invalidate(); toast.success(t("cache.deleted")); },
    onError: showError,
  });
  const clearMutation = useMutation({
    mutationFn: clearCache,
    onSuccess: async () => { setClearOpen(false); setPage(1); await invalidate(); toast.success(t("cache.cleared")); },
    onError: showError,
  });
  const renameMutation = useMutation({
    mutationFn: ({ postID, name }: { postID: string; name: string }) => renameCachedVideo(postID, name),
    onSuccess: async () => { setRenameItem(undefined); await invalidate(); toast.success(t("cache.renamed")); },
    onError: showError,
  });
  const formatter = useMemo(() => new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }), [i18n.language]);
  const currentStats = statsQuery.data?.[type];
  const items = itemsQuery.data?.items ?? [];

  function showError(error: Error) {
    toast.error(error.message || t("errors.generic"));
  }

  function selectType(value: string) {
    setType(value as CacheType);
    setPage(1);
  }

  function openRename(item: CacheItem) {
    setRenameItem(item);
    setDisplayName(item.displayName ?? "");
  }

  function refresh() {
    void statsQuery.refetch();
    void itemsQuery.refetch();
  }

  return (
    <div className="mx-auto flex w-full max-w-[1440px] flex-col gap-8 px-4 py-8 sm:px-6 lg:px-10 lg:py-10">
      <PageHeader
        title={t("cache.title")}
        description={t("cache.description")}
        actions={<Button variant="secondary" size="sm" onClick={refresh} disabled={statsQuery.isFetching || itemsQuery.isFetching}><RefreshCw className={statsQuery.isFetching || itemsQuery.isFetching ? "animate-spin" : ""} />{t("common.refresh")}</Button>}
      />

      <section className="grid border-y sm:grid-cols-2" aria-label={t("cache.summary")}>
        <CacheSummary icon={ImageIcon} label={t("cache.images")} count={statsQuery.data?.image.count ?? 0} size={formatBytes(statsQuery.data?.image.sizeBytes ?? 0)} />
        <CacheSummary icon={Video} label={t("cache.videos")} count={statsQuery.data?.video.count ?? 0} size={formatBytes(statsQuery.data?.video.sizeBytes ?? 0)} className="border-t sm:border-l sm:border-t-0" />
      </section>

      <section className="min-w-0">
        <div className="flex min-h-12 flex-wrap items-center justify-between gap-3 border-b pb-4">
          <Tabs value={type} onValueChange={selectType}>
            <TabsList>
              <TabsTrigger value="image"><ImageIcon className="mr-1.5 size-3.5" />{t("cache.images")}</TabsTrigger>
              <TabsTrigger value="video"><Video className="mr-1.5 size-3.5" />{t("cache.videos")}</TabsTrigger>
            </TabsList>
          </Tabs>
          <div className="flex items-center gap-3">
            <span className="text-xs text-muted-foreground">{t("cache.currentSummary", { count: currentStats?.count ?? 0, size: formatBytes(currentStats?.sizeBytes ?? 0) })}</span>
            <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" disabled={!currentStats?.count} onClick={() => setClearOpen(true)}><Trash2 />{t("cache.clearType")}</Button>
          </div>
        </div>

        {itemsQuery.isPending ? <LoadingState className="min-h-80" /> : null}
        {itemsQuery.isError ? <ErrorState message={itemsQuery.error.message} onRetry={() => void itemsQuery.refetch()} /> : null}
        {!itemsQuery.isPending && !itemsQuery.isError && items.length === 0 ? (
          <div className="flex min-h-80 flex-col items-center justify-center gap-3 text-muted-foreground"><HardDrive className="size-7" strokeWidth={1.5} /><p className="text-sm">{t("cache.empty")}</p></div>
        ) : null}
        {!itemsQuery.isPending && !itemsQuery.isError && items.length > 0 ? (
          <div className="grid grid-cols-1 gap-x-5 gap-y-7 py-6 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            {items.map((item) => (
              <article key={item.name} className="group min-w-0">
                <div className="relative aspect-video overflow-hidden rounded-md bg-muted">
                  {type === "image" ? <img src={mediaURL(item.previewURL ?? item.viewURL)} alt={item.displayName || item.name} loading="lazy" className="size-full object-cover transition-transform duration-200 group-hover:scale-[1.02]" /> : <PersistentVideoPreview url={mediaURL(item.viewURL)} posterURL={mediaURL(item.thumbnailURL ?? item.previewURL ?? "")} label={item.displayName || item.name} videoClassName="object-cover" />}
                  <Button variant="secondary" size="icon" className="absolute right-2 top-2 size-7 opacity-100 shadow-sm sm:opacity-0 sm:group-hover:opacity-100" asChild>
                    <a href={mediaURL(item.viewURL)} target="_blank" rel="noreferrer" aria-label="打开视频"><ExternalLink className="size-3.5" /></a>
                  </Button>
                </div>
                <div className="mt-3 flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium" title={item.displayName || item.name}>{item.displayName || item.name}</div>
                    <div className="mt-1 flex flex-wrap gap-x-2 text-[11px] text-muted-foreground"><span>{formatBytes(item.sizeBytes)}</span><span>{formatter.format(item.modifiedAtMs)}</span></div>
                  </div>
                  {type === "video" && item.postID ? <Button variant="ghost" size="icon" className="size-8 shrink-0" aria-label={t("cache.rename")} onClick={() => openRename(item)}><Pencil /></Button> : null}
                  <Button variant="ghost" size="icon" className="size-8 shrink-0 text-muted-foreground hover:text-destructive" aria-label={t("common.delete")} onClick={() => setDeleteItem(item)}><Trash2 /></Button>
                </div>
              </article>
            ))}
          </div>
        ) : null}

        {itemsQuery.data && itemsQuery.data.total > 0 ? <Pagination className="border-t pt-4" page={page} pageSize={pageSize} total={itemsQuery.data.total} onPageChange={setPage} onPageSizeChange={(value) => { setPageSize(value); setPage(1); }} /> : null}
      </section>

      <AlertDialog open={Boolean(deleteItem)} onOpenChange={(open) => { if (!open) setDeleteItem(undefined); }}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>{t("cache.deleteTitle")}</AlertDialogTitle><AlertDialogDescription>{t("cache.deleteDescription", { name: deleteItem?.displayName || deleteItem?.name })}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><AlertDialogAction disabled={deleteMutation.isPending} onClick={() => deleteItem && deleteMutation.mutate({ itemType: type, name: deleteItem.name })}>{t("common.delete")}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={clearOpen} onOpenChange={setClearOpen}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>{t("cache.clearTitle", { type: type === "image" ? t("cache.images") : t("cache.videos") })}</AlertDialogTitle><AlertDialogDescription>{t("cache.clearDescription", { count: currentStats?.count ?? 0 })}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><AlertDialogAction disabled={clearMutation.isPending} onClick={() => clearMutation.mutate(type)}>{t("cache.clearType")}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={Boolean(renameItem)} onOpenChange={(open) => { if (!open) setRenameItem(undefined); }}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("cache.renameTitle")}</DialogTitle><DialogDescription>{renameItem?.name}</DialogDescription></DialogHeader>
          <Input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder={t("cache.renamePlaceholder")} autoFocus />
          <DialogFooter><Button variant="secondary" onClick={() => setRenameItem(undefined)}>{t("common.cancel")}</Button><Button disabled={renameMutation.isPending} onClick={() => renameItem?.postID && renameMutation.mutate({ postID: renameItem.postID, name: displayName.trim() })}>{t("common.save")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function CacheSummary({ icon: Icon, label, count, size, className = "" }: { icon: typeof ImageIcon; label: string; count: number; size: string; className?: string }) {
  return <div className={`flex min-h-24 items-center gap-4 px-5 py-4 ${className}`}><span className="flex size-9 items-center justify-center rounded-md bg-secondary text-muted-foreground"><Icon className="size-4" /></span><div><div className="text-xs text-muted-foreground">{label}</div><div className="mt-1 text-xl font-medium">{count.toLocaleString()} <span className="ml-1 text-xs font-normal text-muted-foreground">{size}</span></div></div></div>;
}

function mediaURL(path: string): string {
  if (!path) return "";
  if (/^(?:https?:)?\/\//i.test(path) || path.startsWith("data:") || path.startsWith("blob:")) return path;
  return `${runtimeConfig.apiBaseUrl}${path}`;
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MiB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}
