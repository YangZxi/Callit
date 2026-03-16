import React from "react";
import { Avatar, Badge, addToast } from "@heroui/react";

import api, { API_BASE } from "@/lib/api";

import PromptContainerWithConversation from "./prompt-container-with-conversation";
import { OpenAIIcon } from "../icons";

export type ChatMode = "chat" | "agent";

export type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  mode: ChatMode;
  content: string;
  created_at: string;
};

type ChatSessionResponse = {
  worker_id: string;
  agent_initialized: boolean;
  messages: ChatMessage[];
};

type ChatboxProps = {
  workerId: string;
  onFilesChanged?: () => void | Promise<void>;
};

type SSEEvent = {
  event: string;
  data: any;
};

async function streamChat(
  workerId: string,
  payload: { mode: ChatMode; message: string; history_limit: number },
  onEvent: (event: SSEEvent) => void,
) {
  const res = await fetch(`${API_BASE}/workers/${workerId}/chat/stream`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  if (!res.ok) {
    const json = await res.json().catch(() => null);
    const msg = json?.msg || json?.error || "聊天请求失败";
    throw new Error(msg);
  }
  if (!res.body) {
    throw new Error("SSE 连接失败");
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";
  const parseSSEChunk = (rawChunk: string): SSEEvent | null => {
    const lines = rawChunk
      .split("\n")
      .map((line) => line.trimEnd())
      .filter((line) => line.length > 0);
    if (lines.length === 0) return null;

    let event = "message";
    const dataLines: string[] = [];
    for (const line of lines) {
      if (line.startsWith("event:")) {
        event = line.slice("event:".length).trim();
        continue;
      }
      if (line.startsWith("data:")) {
        dataLines.push(line.slice("data:".length).trim());
      }
    }
    if (dataLines.length === 0) {
      return null;
    }

    const text = dataLines.join("\n");
    try {
      return {
        event,
        data: JSON.parse(text),
      };
    } catch {
      return {
        event,
        data: { text },
      };
    }
  };

  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done });

    let splitAt = buffer.indexOf("\n\n");
    while (splitAt >= 0) {
      const chunk = buffer.slice(0, splitAt).replace(/\r/g, "");
      buffer = buffer.slice(splitAt + 2);
      const event = parseSSEChunk(chunk);
      if (event) {
        onEvent(event);
      }
      splitAt = buffer.indexOf("\n\n");
    }

    if (done) {
      break;
    }
  }

  const tail = buffer.trim();
  if (tail) {
    const event = parseSSEChunk(tail);
    if (event) {
      onEvent(event);
    }
  }
}

export default function Chatbox({ workerId, onFilesChanged }: ChatboxProps) {
  const [chatOpen, setChatOpen] = React.useState(false);
  const [mode, setMode] = React.useState<ChatMode>("chat");
  const [messages, setMessages] = React.useState<ChatMessage[]>([]);
  const [files, setFiles] = React.useState<string[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [sending, setSending] = React.useState(false);

  const loadSession = React.useCallback(async () => {
    if (!workerId) return;
    setLoading(true);
    try {
      const data = await api.get<ChatSessionResponse>(`/workers/${workerId}/chat/session`);
      setMessages(data.messages || []);
    } finally {
      setLoading(false);
    }
  }, [workerId]);

  const loadFiles = React.useCallback(async () => {
    if (!workerId) return;
    const data = await api.get<{ files: string[] }>(`/workers/${workerId}/files`);
    setFiles((data.files || []).filter((name) => !/\.diff(?:\.[^./\\]+)?$/.test(name)));
  }, [workerId]);

  React.useEffect(() => {
    if (!chatOpen || !workerId) return;
    void loadSession();
    void loadFiles();
  }, [chatOpen, workerId, loadSession, loadFiles]);

  const clearSession = React.useCallback(async () => {
    if (!workerId || sending) return;
    await api.post<{ ok: boolean }>(`/workers/${workerId}/chat/session/clear`);
    setMessages([]);
    addToast({ title: "会话已清空", color: "success", variant: "flat", timeout: 1500 });
  }, [workerId, sending]);

  const handleUpload = React.useCallback(async (selected: FileList) => {
    if (!workerId || selected.length === 0) return;
    const formData = new FormData();
    Array.from(selected).forEach((file) => {
      formData.append("files", file);
    });
    await api.post<{ files: string[] }>(`/workers/${workerId}/files`, formData);
    await loadFiles();
    await onFilesChanged?.();
  }, [workerId, loadFiles, onFilesChanged]);

  const sendMessage = React.useCallback(async (text: string) => {
    const clean = text.trim();
    if (!clean || !workerId || sending) return;

    const now = new Date().toISOString();
    const userMsg: ChatMessage = {
      id: `user-${Date.now()}`,
      role: "user",
      mode,
      content: clean,
      created_at: now,
    };
    const assistantID = `assistant-${Date.now()}`;
    const assistantPlaceholder: ChatMessage = {
      id: assistantID,
      role: "assistant",
      mode,
      content: "",
      created_at: now,
    };
    setMessages((prev) => [...prev, userMsg, assistantPlaceholder]);
    setSending(true);

    try {
      await streamChat(
        workerId,
        { mode, message: clean, history_limit: 10 },
        (event) => {
          if (event.event === "delta") {
            const delta = typeof event.data?.text === "string" ? event.data.text : "";
            if (!delta) return;
            setMessages((prev) =>
              prev.map((item) =>
                item.id === assistantID ? { ...item, content: item.content + delta } : item,
              ),
            );
            return;
          }
          if (event.event === "agent_files") {
            void loadFiles();
            void onFilesChanged?.();
            return;
          }
          if (event.event === "error") {
            const msg = event.data?.message || "聊天失败";
            addToast({ title: msg, color: "danger", variant: "flat", timeout: 2200 });
          }
        },
      );
      await loadSession();
      await loadFiles();
      await onFilesChanged?.();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "聊天失败";
      addToast({ title: msg, color: "danger", variant: "flat", timeout: 2200 });
      setMessages((prev) =>
        prev.map((item) =>
          item.id === assistantID && !item.content
            ? { ...item, content: "请求失败，请稍后重试。" }
            : item,
        ),
      );
    } finally {
      setSending(false);
    }
  }, [workerId, sending, mode, loadSession, loadFiles, onFilesChanged]);

  return (
    <div className="flex flex-col items-end gap-3">
      {chatOpen ? (
        <div className="h-[500px] w-[520px] p-2 overflow-hidden rounded-xl border border-default-200 bg-background shadow-2xl">
          <main className="flex h-full min-h-0">
            <div className="relative flex h-full min-h-0 w-full flex-col gap-2">
              <PromptContainerWithConversation
                className="h-full min-h-0 max-w-full px-0"
                scrollShadowClassname="h-full min-h-0"
                mode={mode}
                loading={loading}
                sending={sending}
                files={files}
                messages={messages}
                onModeChange={setMode}
                onSend={sendMessage}
                onClearSession={clearSession}
                onUploadFiles={handleUpload}
              />
            </div>
          </main>
        </div>
      ) : null}
      <button
        type="button"
        aria-label={chatOpen ? "收起聊天窗口" : "展开聊天窗口"}
        className="rounded-xl transition-transform hover:scale-105"
        onClick={() => setChatOpen((prev) => !prev)}
      >
        <Badge color="secondary" content={messages.length > 0 ? String(messages.length) : 0} variant="solid">
          <Avatar radius="md" 
            icon={<OpenAIIcon />}
          />
        </Badge>
      </button>
    </div>
  );
}
