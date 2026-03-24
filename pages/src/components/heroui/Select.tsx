import { Label, ListBox, Select as HeroSelect } from "@heroui/react";

type SelectOption = {
	label: string;
	value: string;
};

type SelectProps = {
	className?: string;
	placeholder?: string;
	label?: string;
	value?: string;
	options: SelectOption[];
	isDisabled?: boolean;
	onValueChange?: (value: string) => void;
};

export default function Select({
	className,
	placeholder,
	label,
	value,
	options,
	isDisabled,
	onValueChange,
}: SelectProps) {
	return (
		<HeroSelect
			className={className}
			isDisabled={isDisabled}
			placeholder={placeholder}
			selectedKey={value}
			onSelectionChange={(key) => {
				if (key == null) return;
				onValueChange?.(String(key));
			}}
		>
			{label ? <Label>{label}</Label> : null}
			<HeroSelect.Trigger>
				<HeroSelect.Value />
				<HeroSelect.Indicator />
			</HeroSelect.Trigger>
			<HeroSelect.Popover>
				<ListBox>
					{options.map((option) => (
						<ListBox.Item id={option.value} key={option.value} textValue={option.label}>
							{option.label}
							<ListBox.ItemIndicator />
						</ListBox.Item>
					))}
				</ListBox>
			</HeroSelect.Popover>
		</HeroSelect>
	);
}
