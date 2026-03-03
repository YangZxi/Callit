import { Button, ButtonGroup, Dropdown, DropdownItem, DropdownMenu, DropdownTrigger } from "@heroui/react";
import { ChevronDownIcon, DeleteDocumentIcon, EditDocumentIcon } from "./icons";

type EditorPanelActionProps = {
  disabled?: boolean;
  loading?: boolean;
  onSave: () => void | Promise<void>;
  onRename: () => void | Promise<void>;
  onDelete: () => void | Promise<void>;
};

const iconClasses = "text-xl text-default-500 pointer-events-none shrink-0";
export default function EditorPanelAction(props: EditorPanelActionProps) {
  const {
    disabled = false,
    loading = false,
    onSave,
    onRename,
  } = props;

  return (
    <ButtonGroup variant="flat">
      <Button isDisabled={disabled} isLoading={loading} 
        color="primary"
        onPress={onSave}
      >
        保存
      </Button>
      <Dropdown
        isDisabled={disabled}
        classNames={{
          base: "before:bg-default-200", // change arrow background
          content:
            "min-w-[140px] py-1 px-1 border border-default-200 bg-linear-to-br from-white to-default-200 dark:from-default-50 dark:to-black",
        }}
      >
        <DropdownTrigger>
          <Button isIconOnly aria-label="更多操作">
            <ChevronDownIcon />
          </Button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="编辑器操作菜单"
          variant="flat"
        >
          <DropdownItem 
            key="rename" color="default" description="重命名"
            startContent={<EditDocumentIcon className={iconClasses} />}
            onPress={onRename}
          >
            重命名文件
          </DropdownItem>
          <DropdownItem 
            key="delete" className="text-danger" color="danger" description="删除"
            startContent={<DeleteDocumentIcon className={iconClasses} />}
            onPress={props.onDelete}
          >
            删除文件
          </DropdownItem>
        </DropdownMenu>
      </Dropdown>
    </ButtonGroup>
  );
}
