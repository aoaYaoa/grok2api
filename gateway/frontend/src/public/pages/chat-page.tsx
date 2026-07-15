import { Copy, FilePlus2, RotateCcw, Send, Settings2, Square, Trash2, X } from "lucide-react";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { publicEndpoints, publicFetch } from "@/public/api/client";
import { usePublicAuth } from "@/public/auth/public-auth";
import { filesToAssets, type UploadAsset } from "@/public/lib/media";
import { cn } from "@/shared/lib/cn";

type MessageContent = string | Array<{ type: "text"; text: string } | { type: "file"; file: { file_data: string } }>;
type Message = { id: string; role: "user" | "assistant"; content: MessageContent; display: string; files?: UploadAsset[] };

function textDelta(payload: unknown) {
  if (!payload || typeof payload !== "object") return "";
  const choice = (payload as { choices?: Array<{ delta?: { content?: string; reasoning_content?: string } }> }).choices?.[0];
  return choice?.delta?.content || choice?.delta?.reasoning_content || "";
}

export function ChatPage() {
  const { key } = usePublicAuth();
  const [models, setModels] = useState<string[]>([]);
  const [model, setModel] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [prompt, setPrompt] = useState("");
  const [files, setFiles] = useState<UploadAsset[]>([]);
  const [system, setSystem] = useState("");
  const [reasoning, setReasoning] = useState("default");
  const [temperature, setTemperature] = useState(0.8);
  const [topP, setTopP] = useState(0.95);
  const [settings, setSettings] = useState(false);
  const [running, setRunning] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    void publicFetch<{ data?: Array<{ id?: string }> }>(key, publicEndpoints.models).then((payload) => {
      const values = (payload.data || []).map((item) => item.id || "").filter(Boolean);
      setModels(values);
      setModel((current) => current || values[0] || "grok-4.1-fast");
    }).catch((error) => toast.error(error.message));
  }, [key]);
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [messages]);

  async function run(history: Message[]) {
    const assistantID = crypto.randomUUID();
    setMessages([...history, { id: assistantID, role: "assistant", content: "", display: "" }]);
    setRunning(true);
    const controller = new AbortController();
    abortRef.current = controller;
    const apiMessages: Array<{ role: "system" | "user" | "assistant"; content: MessageContent }> = history.map(({ role, content }) => ({ role, content }));
    if (system.trim()) apiMessages.unshift({ role: "system", content: system.trim() });
    const body: Record<string, unknown> = { model, messages: apiMessages, stream: true, temperature, top_p: topP };
    if (reasoning !== "default") body.reasoning_effort = reasoning;
    try {
      const response = await fetch(publicEndpoints.chat, { method: "POST", headers: { "Content-Type": "application/json", ...(key ? { Authorization: `Bearer ${key}` } : {}) }, body: JSON.stringify(body), signal: controller.signal });
      if (!response.ok || !response.body) throw new Error((await response.text()) || `请求失败: ${response.status}`);
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      let output = "";
      while (true) {
        const { done, value } = await reader.read();
        buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
        const frames = buffer.split("\n\n");
        buffer = frames.pop() || "";
        for (const frame of frames) for (const line of frame.split("\n")) if (line.startsWith("data:")) {
          const raw = line.slice(5).trim();
          if (!raw || raw === "[DONE]") continue;
          try {
            output += textDelta(JSON.parse(raw));
            setMessages((current) => current.map((item) => item.id === assistantID ? { ...item, content: output, display: output } : item));
          } catch {
            // Ignore keepalive frames.
          }
        }
        if (done) break;
      }
    } catch (error) {
      if (!controller.signal.aborted) toast.error(error instanceof Error ? error.message : "聊天请求失败");
    } finally {
      setRunning(false);
      abortRef.current = null;
    }
  }

  async function send() {
    const text = prompt.trim();
    if ((!text && files.length === 0) || running) return;
    const blocks: MessageContent = files.length ? [...(text ? [{ type: "text" as const, text }] : []), ...files.map((file) => ({ type: "file" as const, file: { file_data: file.data } }))] : text;
    const next = [...messages, { id: crypto.randomUUID(), role: "user" as const, content: blocks, display: text, files }];
    setMessages(next);
    setPrompt("");
    setFiles([]);
    await run(next);
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void send();
    }
  }

  const retry = () => {
    const index = messages.map((item) => item.role).lastIndexOf("user");
    if (index >= 0 && !running) void run(messages.slice(0, index + 1));
  };

  return (
    <section className="mx-auto flex min-h-[calc(100dvh-7rem)] max-w-4xl flex-col">
      <div className="mb-2 flex items-center justify-between gap-3 px-1">
        <div><h1 className="text-lg font-semibold">Chat</h1><p className="text-xs text-muted-foreground">{running ? "正在生成" : model || "正在加载模型"}</p></div>
        <div className="flex gap-1"><Button variant="ghost" size="icon" onClick={retry} disabled={running || !messages.length} aria-label="重试"><RotateCcw className="size-4" /></Button><Button variant="ghost" size="icon" onClick={() => setMessages([])} disabled={running || !messages.length} aria-label="清空对话"><Trash2 className="size-4" /></Button></div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto py-5 sm:px-2">
        {!messages.length && <div className="grid min-h-[48dvh] place-items-center text-center"><div><span className="mx-auto grid size-10 place-items-center rounded-md bg-primary text-sm font-bold text-primary-foreground">G</span><h2 className="mt-4 text-lg font-semibold">有什么可以帮忙的？</h2><p className="mt-1 text-sm text-muted-foreground">输入消息开始对话</p></div></div>}
        <div className="space-y-6">
          {messages.map((message) => message.role === "user" ? (
            <article key={message.id} className="flex justify-end">
              <div className="max-w-[88%] whitespace-pre-wrap rounded-md bg-secondary px-4 py-3 leading-6 text-secondary-foreground sm:max-w-[76%]">
                {message.files?.length ? <div className="mb-2 flex flex-wrap gap-2">{message.files.map((file) => <span key={file.id} className="rounded-md border bg-card px-2 py-1 text-xs">{file.name}</span>)}</div> : null}
                {message.display}
              </div>
            </article>
          ) : (
            <article key={message.id} className="grid grid-cols-[2rem_minmax(0,1fr)] gap-3">
              <span className="grid size-8 place-items-center rounded-md bg-primary text-xs font-bold text-primary-foreground">G</span>
              <div className="min-w-0 whitespace-pre-wrap pt-1 leading-7">
                {message.display || (running ? "正在思考..." : "")}
                {message.display && <button onClick={() => void navigator.clipboard.writeText(message.display).then(() => toast.success("已复制"))} className="mt-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground" aria-label="复制回答"><Copy className="size-3" />复制</button>}
              </div>
            </article>
          ))}
          <div ref={bottomRef} />
        </div>
      </div>

      <div className="sticky bottom-[calc(3.5rem+env(safe-area-inset-bottom))] z-20 mt-3 bg-background pb-2 pt-2 md:bottom-0">
        {settings && <div className="workspace-panel mb-2 grid gap-3 p-3 sm:grid-cols-2 lg:grid-cols-4"><label className="text-xs">推理强度<Select value={reasoning} onValueChange={setReasoning}><SelectTrigger className="mt-1"><SelectValue /></SelectTrigger><SelectContent>{["default", "none", "minimal", "low", "medium", "high", "xhigh"].map((value) => <SelectItem key={value} value={value}>{value === "default" ? "跟随配置" : value}</SelectItem>)}</SelectContent></Select></label><label className="text-xs">温度 {temperature.toFixed(2)}<input className="mt-3 w-full" type="range" min="0" max="2" step="0.05" value={temperature} onChange={(event) => setTemperature(Number(event.target.value))} /></label><label className="text-xs">Top P {topP.toFixed(2)}<input className="mt-3 w-full" type="range" min="0" max="1" step="0.05" value={topP} onChange={(event) => setTopP(Number(event.target.value))} /></label><label className="text-xs">系统提示<Textarea className="mt-1 min-h-20" value={system} onChange={(event) => setSystem(event.target.value)} /></label></div>}
        {files.length > 0 && <div className="mb-2 flex flex-wrap gap-2 px-1">{files.map((file) => <span key={file.id} className="flex items-center gap-1 rounded-md border bg-card px-2 py-1 text-xs">{file.name}<button onClick={() => setFiles((items) => items.filter((item) => item.id !== file.id))} aria-label={`移除 ${file.name}`}><X className="size-3" /></button></span>)}</div>}
        <div className="rounded-2xl bg-secondary p-1.5 shadow-none ring-1 ring-black/5 dark:ring-white/10">
          <Textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} onKeyDown={onKeyDown} className="max-h-36 min-h-12 resize-none border-0 bg-transparent px-2.5 py-2 shadow-none focus-visible:ring-0" placeholder="询问任何内容" />
          <div className="flex min-w-0 items-center gap-0.5 px-0.5 pb-0.5">
            <label className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="添加附件"><FilePlus2 className="size-4" /><input type="file" multiple className="hidden" onChange={(event) => void filesToAssets(event.target.files || []).then(setFiles)} /></label>
            <Select value={model} onValueChange={setModel}><SelectTrigger className="h-9 min-w-0 max-w-44 border-0 bg-transparent px-2 text-xs shadow-none focus-visible:ring-0"><SelectValue placeholder="选择模型" /></SelectTrigger><SelectContent>{models.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select>
            <Button variant="ghost" size="icon" className={cn("size-9 shrink-0", settings && "bg-accent text-foreground")} onClick={() => setSettings((value) => !value)} aria-label="聊天设置"><Settings2 className="size-4" /></Button>
            <div className="ml-auto shrink-0">{running ? <Button size="icon" variant="destructive" className="size-9 rounded-full" onClick={() => abortRef.current?.abort()} aria-label="停止生成"><Square className="size-4" /></Button> : <Button size="icon" className="size-9 rounded-full" onClick={() => void send()} disabled={!prompt.trim() && !files.length} aria-label="发送"><Send className="size-4" /></Button>}</div>
          </div>
        </div>
      </div>
    </section>
  );
}
