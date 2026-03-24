import React from "react";
import { FieldError, Input as HeroInput, Label, TextField } from "@heroui/react";

type InputProps = {
	label?: React.ReactNode;
	description?: React.ReactNode;
	placeholder?: string;
	value?: string;
	name?: string;
	type?: React.HTMLInputTypeAttribute;
	className?: string;
	maxLength?: number;
	isRequired?: boolean;
	isDisabled?: boolean;
	isInvalid?: boolean;
	errorMessage?: React.ReactNode;
	autoFocus?: boolean;
	autoComplete?: string;
  variant?: string;
	onValueChange?: (value: string) => void;
	onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>;
};

export default function Input({
	label,
	description,
	placeholder,
	value,
	name,
	type = "text",
	className,
	maxLength,
	isRequired,
	isDisabled,
	isInvalid,
	errorMessage,
	autoFocus,
	autoComplete,
	onValueChange,
	onKeyDown,
}: InputProps) {
	return (
		<TextField
      aria-label={name}
			isRequired={isRequired}
			name={name}
			type={type}
			validate={() => {
				if (!isInvalid) return null;
				return typeof errorMessage === "string" ? errorMessage : "输入不合法";
			}}
		>
			{label ? <Label>{label}</Label> : null}
			<HeroInput
			  className={className}
				aria-label={typeof label === "string" && label ? label : placeholder || name || "input"}
				autoComplete={autoComplete}
				autoFocus={autoFocus}
				disabled={isDisabled}
				maxLength={maxLength}
				placeholder={placeholder}
				value={value}
				onKeyDown={onKeyDown}
				onChange={(event) => onValueChange?.(event.target.value)}
			/>
			{description ? <div className="text-xs text-default-500">{description}</div> : null}
			<FieldError>{typeof errorMessage === "string" ? errorMessage : undefined}</FieldError>
		</TextField>
	);
}
