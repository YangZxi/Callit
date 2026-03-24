import { ButtonGroup, Dropdown, Button } from "@heroui/react";
import { ChevronDownIcon, DeleteDocumentIcon, EditDocumentIcon } from "./icons";

type EditorPanelActionProps = {
  disabled?: boolean;
  loading?: boolean;
  onSave: () => void | Promise<void>;
  onRename: () => void | Promise<void>;
  onDelete: () => void | Promise<void>;
};

const iconClasses = "text-default-500 pointer-events-none shrink-0";
export default function EditorPanelAction(props: EditorPanelActionProps) {
  const {
    disabled = false,
    loading = false,
    onSave,
    onRename,
    onDelete,
  } = props;

  return (
    <ButtonGroup variant="tertiary">
      <Button isDisabled={disabled} isPending={loading} size="sm" variant="primary" onPress={onSave}>
        保存
      </Button>
      <Dropdown>
        <Button 
          aria-label="更多操作" size="sm" variant="tertiary"
          isIconOnly isDisabled={disabled} 
        >
          <ButtonGroup.Separator />
          <ChevronDownIcon />
        </Button>
        <Dropdown.Popover
          placement="bottom end"
          style={{
            width: "110px",
            minWidth: "110px",
          }}
        >
          <Dropdown.Menu aria-label="编辑器操作菜单">
            <Dropdown.Item
              id="rename"
              textValue="重命名"
              onPress={onRename}
            >
              <EditDocumentIcon className={iconClasses} />
              重命名
            </Dropdown.Item>
            <Dropdown.Item
              id="delete"
              className="text-danger"
              textValue="删除"
              onPress={onDelete}
            >
              <DeleteDocumentIcon className={iconClasses} />
              删除
            </Dropdown.Item>
          </Dropdown.Menu>
        </Dropdown.Popover>
      </Dropdown>
    </ButtonGroup>
  );
}
