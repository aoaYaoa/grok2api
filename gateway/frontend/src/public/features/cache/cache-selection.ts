export function toggleCacheSelection(current: Set<string>, key: string) {
  const next = new Set(current);
  if (next.has(key)) next.delete(key); else next.add(key);
  return next;
}

export function selectCacheKeys(current: Set<string>, keys: string[]) {
  return new Set([...current, ...keys]);
}

export function removeDeletedSelection(current: Set<string>, deletedKeys: string[]) {
  const next = new Set(current);
  deletedKeys.forEach((key) => next.delete(key));
  return next;
}

export function cacheDeletePayload(items: Array<{ source: string; cacheKey: string }>) {
  return {
    items: items.map(({ source, cacheKey }) => ({ source, cache_key: cacheKey })),
  };
}
