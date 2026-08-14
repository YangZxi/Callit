import { Button, Tooltip } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useTranslation } from "react-i18next";

import { changeLocale, chineseLocale, currentLocale } from "@/i18n";

export function LanguageSwitch() {
  const { t } = useTranslation();
  const isChinese = currentLocale() === chineseLocale;
  const nextLabel = isChinese ? t("language.switchToEnglish") : t("language.switchToChinese");

  return (
    <Tooltip delay={400}>
      <Button isIconOnly size="sm" variant="tertiary" aria-label={nextLabel} onPress={() => void changeLocale(isChinese ? "en" : chineseLocale)}>
        <Icon aria-hidden="true" icon="lucide:languages" width={20} />
      </Button>
      <Tooltip.Content>{nextLabel}</Tooltip.Content>
    </Tooltip>
  );
}
