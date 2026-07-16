import { useCallback, useEffect, useRef } from "react";
import { toast } from "sonner";

type VideoTaskGroup = {
  pending: Set<string>;
  failed: number;
  reason: string;
};

export function useVideoFailureNotice() {
  const taskGroups = useRef(new Map<string, VideoTaskGroup>());

  useEffect(() => () => taskGroups.current.clear(), []);

  const beginVideoGroup = useCallback((taskIDs: string[]) => {
    const group: VideoTaskGroup = { pending: new Set(taskIDs), failed: 0, reason: "" };
    taskIDs.forEach((taskID) => taskGroups.current.set(taskID, group));
  }, []);

  const finishVideoTask = useCallback((taskID: string, reason = "") => {
    const group = taskGroups.current.get(taskID);
    if (!group) return;
    taskGroups.current.delete(taskID);
    group.pending.delete(taskID);
    if (reason.trim()) {
      group.failed += 1;
      if (!group.reason) group.reason = reason.trim();
    }
    if (group.pending.size || !group.failed) return;
    toast.error(`${group.failed} 个视频任务失败，已从列表移除`, group.reason ? { description: group.reason } : undefined);
  }, []);

  return { beginVideoGroup, finishVideoTask };
}
