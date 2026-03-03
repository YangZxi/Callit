import React from "react";

import type { ChatMessage } from "./chatbox";
import MessageCard from "./message-card";
import MarkdownMessage from "./markdown-message";

type Props = {
  messages: ChatMessage[];
  loading?: boolean;
};

export default function Conversation({ messages, loading = false }: Props) {
  const endRef = React.useRef<HTMLDivElement | null>(null);

  React.useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages]);

  if (loading) {
    return <div className="px-1 py-2 text-xs text-default-500">正在加载会话...</div>;
  }

  if (messages.length === 0) {
    return <div className="px-1 py-2 text-xs text-default-500">暂无会话消息，开始提问吧。</div>;
  }

  return (
    <div className="flex flex-col gap-4 px-1 pb-1">
      {messages.map((item) => (
        <MessageCard
          key={item.id}
          avatar={
            item.role === "assistant"
              ? "https://nextuipro.nyc3.cdn.digitaloceanspaces.com/components-images/avatar_ai.png"
              : "https://d2u8k2ocievbld.cloudfront.net/memojis/male/6.png"
          }
          message={item.role === "assistant" ? <MarkdownMessage content={item.content} /> : item.content}
          messageClassName={item.role === "user" ? "bg-content3 text-content3-foreground" : ""}
          showFeedback={item.role === "assistant"}
        />
      ))}
      <div ref={endRef} />
    </div>
  );
}
