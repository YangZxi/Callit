import React from "react";
import { cn } from "@heroui/react";

type PromptInputProps = React.TextareaHTMLAttributes<HTMLTextAreaElement> & {
  endContent?: React.ReactNode;
};

const PromptInput = React.forwardRef<HTMLTextAreaElement, PromptInputProps>(
  ({ className, endContent, onChange, rows = 1, ...props }, ref) => {
    return (
      <div className="w-full">
        <div className="w-full rounded-large bg-transparent">
          <div className="relative w-full">
            <textarea
              {...props}
              ref={ref}
              aria-label="Prompt"
              className={cn(
                "min-h-[40px] w-full resize-none border-0 bg-transparent px-0 py-0 text-default-700 shadow-none outline-none",
                "placeholder:text-default-400 focus-visible:outline-none",
                "disabled:cursor-not-allowed disabled:opacity-60",
                className,
              )}
              rows={rows}
              onChange={onChange}
            />
            {endContent ? (
              <div className="pointer-events-none absolute right-0 bottom-0">
                <div className="pointer-events-auto">
                  {endContent}
                </div>
              </div>
            ) : null}
          </div>
        </div>
      </div>
    );
  },
);

export default PromptInput;

PromptInput.displayName = "PromptInput";
