import { LoaderCircle, WandSparkles } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { publicEndpoints, publicFetch } from "@/public/api/client";
import { usePublicAuth } from "@/public/auth/public-auth";
import { cn } from "@/shared/lib/cn";

type PromptEnhanceButtonProps = {
  value: string;
  onEnhanced: (value: string) => void;
  disabled?: boolean;
  className?: string;
};

export function PromptEnhanceButton({ value, onEnhanced, disabled = false, className }: PromptEnhanceButtonProps) {
  const { key } = usePublicAuth();
  const [loading, setLoading] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const valueRef = useRef(value);

  useEffect(() => { valueRef.current = value; }, [value]);
  useEffect(() => () => controller.current?.abort(), []);

  async function enhance() {
    const prompt = value.trim();
    if (!prompt || loading) return;
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    setLoading(true);
    try {
      const payload = await publicFetch<{ prompt?: string; enhanced_prompt?: string; content?: string }>(key, publicEndpoints.promptEnhance, {
        method: "POST",
        body: JSON.stringify({ prompt, temperature: 0.7, request_id: crypto.randomUUID() }),
        signal: request.signal,
      });
      const enhanced = String(payload.enhanced_prompt || payload.prompt || payload.content || "").trim();
      if (!enhanced) throw new Error("服务端未返回优化结果");
      if (valueRef.current.trim() !== prompt) {
        toast.info("输入内容已变化，未覆盖当前提示词");
        return;
      }
      onEnhanced(enhanced);
      toast.success("提示词已优化");
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        toast.error(error instanceof Error ? error.message : "提示词优化失败");
      }
    } finally {
      if (controller.current === request) {
        controller.current = null;
        setLoading(false);
      }
    }
  }

  return (
    <Button variant="ghost" size="sm" className={cn("mt-2", className)} onClick={() => void enhance()} disabled={disabled || loading || !value.trim()}>
      {loading ? <LoaderCircle className="size-4 animate-spin" /> : <WandSparkles className="size-4" />}
      {loading ? "优化中..." : "优化提示词"}
    </Button>
  );
}
