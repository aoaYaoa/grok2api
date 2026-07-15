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

  useEffect(() => { void publicFetch<{ data?: Array<{ id?: string }> }>(key, publicEndpoints.models).then((payload) => { const values = (payload.data || []).map((item) => item.id || "").filter(Boolean); setModels(values); setModel((current) => current || values[0] || "grok-4.1-fast"); }).catch((error) => toast.error(error.message)); }, [key]);
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [messages]);

  async function run(history: Message[]) {
    const assistantID = crypto.randomUUID();
    setMessages([...history, { id: assistantID, role: "assistant", content: "", display: "" }]);
    setRunning(true);
    const controller = new AbortController(); abortRef.current = controller;
    const apiMessages: Array<{ role: "system" | "user" | "assistant"; content: MessageContent }> = history.map(({ role, content }) => ({ role, content }));
    if (system.trim()) apiMessages.unshift({ role: "system" as const, content: system.trim() });
    const body: Record<string, unknown> = { model, messages: apiMessages, stream: true, temperature, top_p: topP };
    if (reasoning !== "default") body.reasoning_effort = reasoning;
    try {
      const response = await fetch(publicEndpoints.chat, { method: "POST", headers: { "Content-Type": "application/json", ...(key ? { Authorization: `Bearer ${key}` } : {}) }, body: JSON.stringify(body), signal: controller.signal });
      if (!response.ok || !response.body) throw new Error((await response.text()) || `请求失败: ${response.status}`);
      const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ""; let output = "";
      while (true) {
        const { done, value } = await reader.read(); buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
        const frames = buffer.split("\n\n"); buffer = frames.pop() || "";
        for (const frame of frames) for (const line of frame.split("\n")) if (line.startsWith("data:")) { const raw = line.slice(5).trim(); if (!raw || raw === "[DONE]") continue; try { output += textDelta(JSON.parse(raw)); setMessages((current) => current.map((item) => item.id === assistantID ? { ...item, content: output, display: output } : item)); } catch { /* ignore keepalive */ } }
        if (done) break;
      }
    } catch (error) { if (!controller.signal.aborted) toast.error(error instanceof Error ? error.message : "聊天请求失败"); }
    finally { setRunning(false); abortRef.current = null; }
  }

  async function send() {
    const text = prompt.trim(); if ((!text && files.length === 0) || running) return;
    const blocks: MessageContent = files.length ? [...(text ? [{ type: "text" as const, text }] : []), ...files.map((file) => ({ type: "file" as const, file: { file_data: file.data } }))] : text;
    const next = [...messages, { id: crypto.randomUUID(), role: "user" as const, content: blocks, display: text, files }];
    setMessages(next); setPrompt(""); setFiles([]); await run(next);
  }
  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); void send(); } }
  const retry = () => { const index = messages.map((item) => item.role).lastIndexOf("user"); if (index >= 0 && !running) void run(messages.slice(0, index + 1)); };

  return (
    <section className="mx-auto flex min-h-[calc(100dvh-7.5rem)] max-w-5xl flex-col">
      <div className="mb-4 flex items-center justify-between gap-3"><div><h1 className="text-xl font-semibold">Chat 聊天</h1><p className="text-sm text-muted-foreground">{running ? "正在生成" : "就绪"}</p></div><div className="flex gap-1"><Button variant="ghost" size="icon" onClick={retry} disabled={running || !messages.length} aria-label="重试"><RotateCcw className="size-4" /></Button><Button variant="ghost" size="icon" onClick={() => setMessages([])} disabled={running || !messages.length} aria-label="清空对话"><Trash2 className="size-4" /></Button></div></div>
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto border-y py-5">
        {!messages.length && <div className="grid min-h-64 place-items-center text-sm text-muted-foreground">输入一条消息开始对话</div>}
        {messages.map((message) => <article key={message.id} className={cn("flex", message.role === "user" ? "justify-end" : "justify-start")}><div className={cn("max-w-[88%] whitespace-pre-wrap rounded-md px-4 py-3 leading-6", message.role === "user" ? "bg-primary text-primary-foreground" : "border bg-card")}>
          {message.files?.length ? <div className="mb-2 flex flex-wrap gap-2">{message.files.map((file) => <span key={file.id} className="rounded border px-2 py-1 text-xs">{file.name}</span>)}</div> : null}{message.display || (message.role === "assistant" && running ? "正在思考..." : "")}
          {message.role === "assistant" && message.display && <button onClick={() => void navigator.clipboard.writeText(message.display).then(() => toast.success("已复制"))} className="mt-2 flex items-center gap-1 text-xs text-muted-foreground" aria-label="复制回答"><Copy className="size-3" />复制</button>}
        </div></article>)}<div ref={bottomRef} />
      </div>
      <div className="sticky bottom-16 mt-4 border bg-background p-3 md:bottom-0">
        {files.length > 0 && <div className="mb-2 flex flex-wrap gap-2">{files.map((file) => <span key={file.id} className="flex items-center gap-1 rounded border px-2 py-1 text-xs">{file.name}<button onClick={() => setFiles((items) => items.filter((item) => item.id !== file.id))} aria-label={`移除 ${file.name}`}><X className="size-3" /></button></span>)}</div>}
        {settings && <div className="mb-3 grid gap-3 border-b pb-3 sm:grid-cols-2 lg:grid-cols-4"><label className="text-xs">推理强度<Select value={reasoning} onValueChange={setReasoning}><SelectTrigger className="mt-1"><SelectValue /></SelectTrigger><SelectContent>{["default","none","minimal","low","medium","high","xhigh"].map((value) => <SelectItem key={value} value={value}>{value === "default" ? "跟随配置" : value}</SelectItem>)}</SelectContent></Select></label><label className="text-xs">温度 {temperature.toFixed(2)}<input className="mt-3 w-full" type="range" min="0" max="2" step="0.05" value={temperature} onChange={(event) => setTemperature(Number(event.target.value))} /></label><label className="text-xs">Top P {topP.toFixed(2)}<input className="mt-3 w-full" type="range" min="0" max="1" step="0.05" value={topP} onChange={(event) => setTopP(Number(event.target.value))} /></label><label className="text-xs">系统提示<Textarea className="mt-1 min-h-20" value={system} onChange={(event) => setSystem(event.target.value)} /></label></div>}
        <div className="flex items-end gap-2"><Select value={model} onValueChange={setModel}><SelectTrigger className="w-48 shrink-0"><SelectValue placeholder="选择模型" /></SelectTrigger><SelectContent>{models.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select><label className="grid size-10 shrink-0 cursor-pointer place-items-center rounded-md hover:bg-accent" aria-label="添加附件"><FilePlus2 className="size-4" /><input type="file" multiple className="hidden" onChange={(event) => void filesToAssets(event.target.files || []).then(setFiles)} /></label><Textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} onKeyDown={onKeyDown} className="min-h-10 max-h-40 flex-1 resize-none" placeholder="询问任何内容" /><Button variant="ghost" size="icon" onClick={() => setSettings((value) => !value)} aria-label="聊天设置"><Settings2 className="size-4" /></Button>{running ? <Button size="icon" variant="destructive" onClick={() => abortRef.current?.abort()} aria-label="停止生成"><Square className="size-4" /></Button> : <Button size="icon" onClick={() => void send()} disabled={!prompt.trim() && !files.length} aria-label="发送"><Send className="size-4" /></Button>}</div>
      </div>
    </section>
  );
}
