import React from "react";
import {
  ButtonGroup,
  Button,
  Dropdown,
  Label,
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
        className={
          "rounded-4xl bg-[#ececee] hover:bg-[#e1e1e1] flex w-full flex-col items-start " +
          "p-1"
        }
        onSubmit={(event) => {
          event.preventDefault();
          void handleSubmit();
        }}
      >
        <PromptInput
          className="pt-1 pr-10 pb-6 pl-2 text-medium"
          endContent={(
            <div className="flex items-end gap-2">
              {/* <Tooltip showArrow content="发送消息"> */}
              <Button
                isIconOnly
                type="submit"
                variant={!prompt.trim() ? "tertiary" : "primary"}
                isDisabled={!prompt.trim() || sending}
                isPending={sending}
                className={`rounded-4xl ${!prompt.trim() ? "bg-[#d0d0d3]" : ""}`}
                size="sm"
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
              {/* </Tooltip> */}
            </div>
          )}
          rows={4}
          placeholder="输入你的问题或改造需求..."
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
        />
        <div className="flex w-full items-center justify-between gap-2 overflow-auto px-4 pb-4">
          <div className="flex w-full gap-1 md:gap-3">
            <ButtonGroup isDisabled={sending} size="sm" variant="secondary">
              <Button
                variant="tertiary"
                className={"bg-[#e7e7e9]"}
                onPress={() => uploadRef.current?.click()}
              >
                <Icon className="text-default-500" icon="solar:paperclip-linear" width={18} />
                上传文件
              </Button>
              <Dropdown>
                <Button
                  isIconOnly
                  aria-label="选择要引用的 Worker 文件"
                  className={"bg-[#e7e7e9] text-default"}
                  isDisabled={files.length === 0}
                >
                  <ButtonGroup.Separator />
                  <ChevronDownIcon />
                </Button>
                <Dropdown.Popover placement="bottom end"

                  style={{
                    width: "100px",
                    minWidth: "100px",
                    maxWidth: "160px",
                    overflowX: "hidden",
                  }}
                >
                  <Dropdown.Menu
                    aria-label="Worker 文件引用"
                    selectedKeys={selectedKeys}
                    selectionMode="multiple"
                    onSelectionChange={(selection) => {
                      if (selection === "all") {
                        setSelectedKeys(new Set(files));
                        return;
                      }
                      setSelectedKeys(new Set(Array.from(selection).map((item) => String(item))));
                    }}
                  >
                    {files.length === 0 ? (
                      <Dropdown.Item
                        id="no-files"
                        isDisabled
                        textValue="当前 Worker 没有可选文件"
                      >
                        <Label>当前 Worker 没有可选文件</Label>
                      </Dropdown.Item>
                    ) : files.map((item) => (
                      <Dropdown.Item
                        key={item}
                        id={item}
                        textValue={item}
                      >
                        {item}
                      </Dropdown.Item>
                    ))}
                  </Dropdown.Menu>
                </Dropdown.Popover>
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
