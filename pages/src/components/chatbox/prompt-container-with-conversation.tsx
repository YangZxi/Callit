import { Button, ScrollShadow, Tab, Tabs } from "@heroui/react";
import { cn } from "@heroui/react";

import type { ChatMessage, ChatMode } from "./chatbox";
import PromptInputWithBottomActions from "./prompt-input-with-bottom-actions";
import Conversation from "./conversation";

type Props = {
  className?: string;
  scrollShadowClassname?: string;
  mode: ChatMode;
  loading: boolean;
  sending: boolean;
  files: string[];
  messages: ChatMessage[];
  onModeChange: (mode: ChatMode) => void;
  onSend: (message: string) => Promise<void> | void;
  onClearSession: () => Promise<void> | void;
  onUploadFiles: (files: FileList) => Promise<void> | void;
};

export default function PromptContainerWithConversation(props: Props) {
  const {
    className,
    scrollShadowClassname,
    mode,
    loading,
    sending,
    files,
    messages,
    onModeChange,
    onSend,
    onClearSession,
    onUploadFiles,
  } = props;

  return (
    <div className={cn("flex h-full min-h-0 w-full max-w-full flex-col gap-2", className)}>
      <div className="border-b-small border-divider flex w-full items-center justify-between gap-2 pb-2">
        <p className="text-base font-medium">Callit AI</p>
        <div className="flex items-center gap-2">
          <Tabs
            aria-label="聊天模式"
            selectedKey={mode}
            size="sm"
            onSelectionChange={(key) => {
              if (key === "chat" || key === "agent") {
                onModeChange(key);
              }
            }}
          >
            <Tab key="chat" title="Chat" />
            <Tab key="agent" title="Agent" />
          </Tabs>
          <Button color="default" size="sm" variant="flat" isDisabled={sending} onPress={onClearSession}>
            清空
          </Button>
        </div>
      </div>

      <ScrollShadow className={cn("flex h-full flex-col", scrollShadowClassname)} hideScrollBar size={4}>
        <Conversation loading={loading} messages={messages} />
      </ScrollShadow>

      <div className="flex flex-col gap-2">
        <PromptInputWithBottomActions
          files={files}
          sending={sending}
          onSend={onSend}
          onUploadFiles={onUploadFiles}
        />
      </div>
    </div>
  );
}
