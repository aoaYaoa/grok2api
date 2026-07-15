import { apiRequest } from "@/shared/api/client";
import { createObjectDecoder, createPaginatedDecoder, hasShape, isBoolean, isNumber, isOptional, isString } from "@/shared/api/decoder";

export type CacheType = "image" | "video";

export type CacheStats = {
  count: number;
  sizeBytes: number;
  sizeMB: number;
};

export type CacheItem = {
  name: string;
  sizeBytes: number;
  modifiedAtMs: number;
  viewURL: string;
  previewURL?: string;
  postID?: string;
  shareLink?: string;
  originalPostID?: string;
  mediaURL?: string;
  thumbnailURL?: string;
  localURL?: string;
  displayName?: string;
};

export type CacheList = {
  items: CacheItem[];
  page: number;
  pageSize: number;
  total: number;
};

const cacheStatsShape = hasShape({ count: isNumber, sizeBytes: isNumber, sizeMB: isNumber });
const decodeCacheStats = createObjectDecoder<{ image: CacheStats; video: CacheStats }>("cache stats", {
  image: cacheStatsShape,
  video: cacheStatsShape,
});
const cacheItemShape = hasShape({
  name: isString,
  sizeBytes: isNumber,
  modifiedAtMs: isNumber,
  viewURL: isString,
  previewURL: isOptional(isString),
  postID: isOptional(isString),
  shareLink: isOptional(isString),
  originalPostID: isOptional(isString),
  mediaURL: isOptional(isString),
  thumbnailURL: isOptional(isString),
  localURL: isOptional(isString),
  displayName: isOptional(isString),
});
const decodeCacheList = createPaginatedDecoder<CacheItem>(cacheItemShape);
const decodeDeleteResult = createObjectDecoder<{ deleted: boolean }>("cache delete", { deleted: isBoolean });
const decodeCacheItem = createObjectDecoder<CacheItem>("cache item", {
  name: isOptional(isString),
  sizeBytes: isOptional(isNumber),
  modifiedAtMs: isOptional(isNumber),
  viewURL: isOptional(isString),
  postID: isString,
  displayName: isOptional(isString),
  shareLink: isOptional(isString),
});
const decodeClearResult = createObjectDecoder<CacheStats>("cache clear", { count: isNumber, sizeBytes: isNumber, sizeMB: isNumber });

export function getCacheStats(): Promise<{ image: CacheStats; video: CacheStats }> {
  return apiRequest("/api/admin/v1/cache", {}, decodeCacheStats);
}

export function listCacheItems(type: CacheType, page: number, pageSize: number): Promise<CacheList> {
  const query = new URLSearchParams({ type, page: String(page), pageSize: String(pageSize) });
  return apiRequest(`/api/admin/v1/cache/items?${query.toString()}`, {}, decodeCacheList);
}

export function deleteCacheItem(type: CacheType, name: string): Promise<{ deleted: boolean }> {
  const query = new URLSearchParams({ type, name });
  return apiRequest(`/api/admin/v1/cache/items?${query.toString()}`, { method: "DELETE" }, decodeDeleteResult);
}

export function clearCache(type: CacheType): Promise<CacheStats> {
  const query = new URLSearchParams({ type });
  return apiRequest(`/api/admin/v1/cache?${query.toString()}`, { method: "DELETE" }, decodeClearResult);
}

export function renameCachedVideo(postID: string, displayName: string): Promise<CacheItem> {
  return apiRequest(`/api/admin/v1/cache/videos/${encodeURIComponent(postID)}`, { method: "PATCH", body: { displayName } }, decodeCacheItem);
}
