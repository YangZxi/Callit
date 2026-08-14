import { Button } from "@heroui/react";
import { Modal } from "@heroui/react";
import { useTranslation } from "react-i18next";

type ModalSize = React.ComponentProps<typeof Modal.Container>["size"];
type ModalPlacement = React.ComponentProps<typeof Modal.Container>["placement"];
type ModalScroll = React.ComponentProps<typeof Modal.Container>["scroll"];

type XModalProps = {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  isDismissable?: boolean;
  isKeyboardDismissDisabled?: boolean;
  size?: ModalSize;
  placement?: ModalPlacement;
  scrollBehavior?: ModalScroll;
  classNames?: {
    header?: string;
    body?: string;
    footer?: string;
  };
  header?: string | React.ReactNode;
  footer?: React.ReactNode;
  children: React.ReactNode;
  submitText?: string;
  onSubmit?: () => void;
};

export default function XModal({
  header,
  children,
  footer,
  submitText,
  onSubmit,
  isOpen,
  onOpenChange,
  isDismissable = true,
  isKeyboardDismissDisabled,
  size,
  placement,
  scrollBehavior,
  classNames,
  ...props
}: XModalProps) {
  const { t } = useTranslation();
  const close = () => onOpenChange(false);

  if (onSubmit && footer == null) {
    footer = <>
      <Button variant="secondary" onPress={close}>{t("common.close")}</Button>
      <Button variant="primary" onPress={onSubmit}>{submitText || t("common.submit")}</Button>
    </>;
  }

  return (
    <Modal isOpen={isOpen} onOpenChange={onOpenChange} {...props}>
      <Button className={"hidden"}>Open Modal</Button>
      <Modal.Backdrop
        isDismissable={isDismissable}
        isKeyboardDismissDisabled={isKeyboardDismissDisabled}
      >
        <Modal.Container placement={placement} scroll={scrollBehavior} size={size}>
          <Modal.Dialog>
            <Modal.Header className={`flex flex-col gap-1 text-default-900 dark:text-default-700 ${classNames?.header}`}>
              <Modal.Heading>{header}</Modal.Heading>
            </Modal.Header>
            <Modal.Body className={`${classNames?.body}`}>
              {children}
            </Modal.Body>
            <Modal.Footer className={`flex justify-end gap-2 ${classNames?.footer}`}>
              {footer ? <>
                {footer}
              </> : <>
                <Button variant="primary" onPress={close}>{t("modal.acknowledge")}</Button>
              </>}
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
