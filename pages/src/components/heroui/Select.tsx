import { Label, ListBox, Select as HeroSelect, Avatar } from "@heroui/react";

type SelectOption = {
	label: string;
	value: string;
	description?: string;
};

type SelectProps = {
	className?: string;
	placeholder?: string;
	label?: string;
	ariaLabel?: string;
	"aria-label"?: string;
	"aria-labelledby"?: string;
	value?: string;
	options: SelectOption[];
	isDisabled?: boolean;
	onValueChange?: (value: string) => void;
};

export default function Select({
	className,
	placeholder,
	label,
	"aria-label": ariaLabel,
	value,
	options,
	isDisabled,
	onValueChange,
}: SelectProps) {
	return (
		<HeroSelect
			aria-label={ariaLabel ?? label ?? placeholder ?? "select"}
			className={className}
			isDisabled={isDisabled}
			placeholder={placeholder}
			value={value}
			onChange={(key) => {
				if (key == null) return;
				onValueChange?.(String(key));
			}}
		>
			{label ? <Label>{label}</Label> : null}
			<HeroSelect.Trigger>
				<HeroSelect.Value>
          {({defaultChildren, isPlaceholder, state}) => {
            if (isPlaceholder || state.selectedItems.length === 0) {
              return defaultChildren;
            }
            const selectedItems = state.selectedItems;
            if (selectedItems.length > 1) {
              return `${selectedItems.length} users selected`;
            }
            const selectedItem = options.find((opt) => opt.value === selectedItems[0]?.key);
            if (!selectedItem) {
              return defaultChildren;
            }
            return (
              <div className="min-w-0">
								<div className="truncate">{selectedItem.label}</div>
							</div>
            );
          }}
        </HeroSelect.Value>
				<HeroSelect.Indicator />
			</HeroSelect.Trigger>
			<HeroSelect.Popover>
				<ListBox>
					{options.map((option) => (
						<ListBox.Item id={option.value} key={option.value} textValue={option.label}>
							<div className="min-w-0">
								<div className="truncate">{option.label}</div>
								{option.description ? (
									<div className="truncate text-xs text-default-500">{option.description}</div>
								) : null}
							</div>
							<ListBox.ItemIndicator />
						</ListBox.Item>
					))}
				</ListBox>
			</HeroSelect.Popover>
		</HeroSelect>
	);
}
