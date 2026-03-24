import { Label, Switch as HeroSwitch } from "@heroui/react";

type SwitchProps = {
  label?: string;
  value?: boolean;
  className?: string;
  isDisabled?: boolean;
  onValueChange?: (value: boolean) => void;
};

export default function Switch({
  label,
  value,
  className,
  isDisabled,
  onValueChange,
}: SwitchProps) {
  return (
    <HeroSwitch
      className={className}
      isDisabled={isDisabled}
      isSelected={value}
      onChange={onValueChange}
    >
      <HeroSwitch.Control>
        <HeroSwitch.Thumb />
      </HeroSwitch.Control>
      <HeroSwitch.Content>
        {label ? <Label className="text-sm">{label}</Label> : null}
      </HeroSwitch.Content>
    </HeroSwitch>
  );
}
