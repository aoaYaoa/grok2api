export function removeStoppedVideoTasks<T extends { taskID: string; status: string }>(
  items: T[],
  taskIDs: string[],
): T[] {
  const stopped = new Set(taskIDs);
  return items.filter((item) => !stopped.has(item.taskID) || item.status === "completed");
}
