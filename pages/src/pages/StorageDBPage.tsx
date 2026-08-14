import { Card } from "@heroui/react";
import { useTranslation } from "react-i18next";

export default function StorageDBPage() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-default-900">{t("storage.dbTitle")}</h1>
        <p className="mt-1 text-sm text-default-500">{t("storage.dbDescription")}</p>
      </div>

      <Card className="border border-default-200/70 bg-background/70 shadow-sm">
        <Card.Header>
          <Card.Title className="text-base">{t("storage.workerDb")}</Card.Title>
          <Card.Description>{t("storage.dbUnavailable")}</Card.Description>
        </Card.Header>
        <Card.Content>
          <div className="rounded-md border border-dashed border-default-300 px-4 py-12 text-center text-sm text-default-500">
            {t("storage.dbFuture")}
          </div>
        </Card.Content>
      </Card>
    </div>
  );
}
