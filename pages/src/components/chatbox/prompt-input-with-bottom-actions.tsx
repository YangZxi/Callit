import type { Selection } from "@heroui/react";

import React from "react";
import {
  Button,
  Tooltip,
  ButtonGroup,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
} from "@heroui/react";
import { Icon } from "@iconify/react";
import { cn } from "@heroui/react";

import PromptInput from "./prompt-input";
import { ChevronDownIcon } from "../icons";

type Props = {
  files: string[];
  sending?: boolean;
  onSend: (message: string) => Promise<void> | void;
  onUploadFiles: (files: FileList) => Promise<void> | void;
};

export default function PromptInputWithBottomActions(props: Props) {
  const { files, sending = false, onSend, onUploadFiles } = props;
  const uploadRef = React.useRef<HTMLInputElement | null>(null);
  const [selectedKeys, setSelectedKeys] = React.useState<Set<string>>(new Set<string>());
  const [prompt, setPrompt] = React.useState<string>("");

  const handleSubmit = async () => {
    if (sending) return;
    const refs = Array.from(selectedKeys)
      .filter((name) => name.trim() !== "")
      .map((name) => `[${name}](${name})`);
    const cleanPrompt = prompt.trim();
    const mergedPrompt = refs.length > 0 ? `${refs.join("\n")}\n\n${cleanPrompt}`.trim() : cleanPrompt;
    if (!mergedPrompt) return;
    setPrompt("");
    setSelectedKeys(new Set<string>());
    await onSend(mergedPrompt);
  };

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const selected = event.target.files;
    if (!selected || selected.length === 0) return;
    await onUploadFiles(selected);
    event.target.value = "";
  };

  return (
    <div className="flex w-full flex-col gap-2">
      <form
        className="rounded-medium bg-default-100 hover:bg-default-200/70 flex w-full flex-col items-start transition-colors"
        onSubmit={(event) => {
          event.preventDefault();
          void handleSubmit();
        }}
      >
        <PromptInput
          classNames={{
            inputWrapper: "bg-transparent! shadow-none",
            innerWrapper: "relative",
            input: "pt-1 pl-2 pb-6 pr-10! text-medium",
          }}
          endContent={(
            <div className="flex items-end gap-2">
              <Tooltip showArrow content="发送消息">
                <Button
                  isIconOnly
                  type="submit"
                  color={!prompt.trim() ? "default" : "primary"}
                  isDisabled={!prompt.trim() || sending}
                  isLoading={sending}
                  radius="lg"
                  size="sm"
                  variant="solid"
                >
                  <Icon
                    className={cn(
                      "[&>path]:stroke-[2px]",
                      !prompt.trim() ? "text-default-600" : "text-primary-foreground",
                    )}
                    icon="solar:arrow-up-linear"
                    width={20}
                  />
                </Button>
              </Tooltip>
            </div>
          )}
          minRows={3}
          placeholder="输入你的问题或改造需求..."
          radius="lg"
          value={prompt}
          variant="flat"
          onValueChange={setPrompt}
        />
        <div className="flex w-full items-center justify-between gap-2 overflow-auto px-4 pb-4">
          <div className="flex w-full gap-1 md:gap-3">
            <ButtonGroup variant="flat">
              <Button
                size="sm"
                isDisabled={sending}
                startContent={<Icon className="text-default-500" icon="solar:paperclip-linear" width={18} />}
                variant="flat"
                onPress={() => uploadRef.current?.click()}
              >
                Attach
              </Button>
              <Dropdown
                classNames={{
                  base: "before:bg-default-200",
                  content: "max-h-[240px] min-w-[220px] py-1 px-1 border border-default-200 bg-linear-to-br from-white to-default-200 dark:from-default-50 dark:to-black",
                }}
              >
                <DropdownTrigger>
                  <Button size="sm" isIconOnly aria-label="引用文件" isDisabled={sending}>
                    <ChevronDownIcon />
                  </Button>
                </DropdownTrigger>
                <DropdownMenu
                  aria-label="Worker 文件引用"
                  emptyContent="当前 Worker 没有可选文件"
                  variant="flat"
                  selectedKeys={selectedKeys}
                  selectionMode="multiple"
                  items={files.map((item) => ({ key: item, label: item }))}
                  onSelectionChange={(selection: Selection) => {
                    if (selection === "all") {
                      setSelectedKeys(new Set<string>());
                      return;
                    }
                    setSelectedKeys(new Set(Array.from(selection).map((item) => String(item))));
                  }}
                >
                  {(item) => <DropdownItem key={item.key}>{item.label}</DropdownItem>}
                </DropdownMenu>
              </Dropdown>
            </ButtonGroup>
            <input ref={uploadRef} multiple className="hidden" type="file" onChange={handleUpload} />
          </div>
          <p className="text-tiny text-default-400 py-1">{prompt.length}/2000</p>
        </div>
      </form>
    </div>
  );
}
