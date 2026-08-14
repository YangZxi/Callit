import { ButtonGroup, Dropdown, Button } from "@heroui/react";
import { ChevronDownIcon, DeleteDocumentIcon, EditDocumentIcon } from "./icons";
import { useTranslation } from "react-i18next";

type EditorPanelActionProps = {
  disabled?: boolean;
  loading?: boolean;
  onSave: () => void | Promise<void>;
  onRename: () => void | Promise<void>;
  onDelete: () => void | Promise<void>;
};

const iconClasses = "text-default-500 pointer-events-none shrink-0";
export default function EditorPanelAction(props: EditorPanelActionProps) {
  const { t } = useTranslation();
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
        {t("common.save")}
      </Button>
      <Dropdown>
        <Button 
          aria-label={t("workers.moreActions")} size="sm" variant="tertiary"
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
          <Dropdown.Menu aria-label={t("workers.editorActions")}>
            <Dropdown.Item
              id="rename"
              textValue={t("workers.rename")}
              onPress={onRename}
            >
              <EditDocumentIcon className={iconClasses} />
              {t("workers.rename")}
            </Dropdown.Item>
            <Dropdown.Item
              id="delete"
              className="text-danger"
              textValue={t("common.delete")}
              onPress={onDelete}
            >
              <DeleteDocumentIcon className={iconClasses} />
              {t("common.delete")}
            </Dropdown.Item>
          </Dropdown.Menu>
        </Dropdown.Popover>
      </Dropdown>
    </ButtonGroup>
  );
}
