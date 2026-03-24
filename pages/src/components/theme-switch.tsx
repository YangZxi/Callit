import { FC, useEffect, useState } from "react";
import clsx from "clsx";
import { Button } from "@heroui/react";

import { useTheme } from "@/lib/theme.ts";
import { SunFilledIcon, MoonFilledIcon } from "@/components/icons.tsx";

export interface ThemeSwitchProps {
  className?: string;
}

export const ThemeSwitch: FC<ThemeSwitchProps> = ({
  className,
}) => {
  const [isMounted, setIsMounted] = useState(false);
  const { theme, toggleTheme } = useTheme();
  const isSelected = theme === "dark";

  useEffect(() => {
    setIsMounted(true);
  }, []);

  if (!isMounted) {
    return <span className="inline-flex h-9 w-9" aria-hidden="true" />;
  }

  return (
    <Button
      isIconOnly
      variant="tertiary"
      size="sm"
      onPress={toggleTheme}
      aria-label={isSelected ? "切换到浅色模式" : "切换到深色模式"}
      aria-pressed={isSelected}
      className={clsx(
        "shrink-0",
        className,
      )}
    >
      {isSelected ? (
        <MoonFilledIcon size={22} />
      ) : (
        <SunFilledIcon size={22} />
      )}
    </Button>
  );
};
