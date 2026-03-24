import React from "react";
import {Avatar, Badge, Link, Tooltip} from "@heroui/react";
import {Icon} from "@iconify/react";
import {cn} from "@heroui/react";
import { Button } from "@heroui/react";

export type MessageCardProps = React.HTMLAttributes<HTMLDivElement> & {
  avatar?: string;
  showFeedback?: boolean;
  message?: React.ReactNode;
  currentAttempt?: number;
  status?: "success" | "failed";
  attempts?: number;
  messageClassName?: string;
  onAttemptChange?: (attempt: number) => void;
  onMessageCopy?: (content: string | string[]) => void;
  onFeedback?: (feedback: "like" | "dislike") => void;
  onAttemptFeedback?: (feedback: "like" | "dislike" | "same") => void;
};

const MessageCard = React.forwardRef<HTMLDivElement, MessageCardProps>(
  (
    {
      avatar,
      message,
      showFeedback,
      attempts = 1,
      currentAttempt = 1,
      status,
      onMessageCopy,
      onAttemptChange,
      onFeedback,
      onAttemptFeedback,
      className,
      messageClassName,
      ...props
    },
    ref,
  ) => {
    const [feedback, setFeedback] = React.useState<"like" | "dislike">();
    const [attemptFeedback, setAttemptFeedback] = React.useState<"like" | "dislike" | "same">();

    const messageRef = React.useRef<HTMLDivElement>(null);

    const [copied, setCopied] = React.useState(false);

    const failedMessageClassName =
      status === "failed" ? "bg-danger-100/50 border border-danger-100 text-foreground" : "";
    const failedMessage = (
      <p>
        Something went wrong, if the issue persists please contact us through our help center
        at&nbsp;
        <Link href="mailto:support@acmeai.com" size="sm">
          support@acmeai.com
        </Link>
      </p>
    );

    const hasFailed = status === "failed";

    const handleCopy = React.useCallback(() => {
      let stringValue = "";

      if (typeof message === "string") {
        stringValue = message;
      } else if (Array.isArray(message)) {
        message.forEach((child) => {
          // @ts-ignore
          const childString =
            typeof child === "string" ? child : child?.props?.children?.toString();

          if (childString) {
            stringValue += childString + "\n";
          }
        });
      }

      const valueToCopy = stringValue || messageRef.current?.textContent || "";

      void navigator.clipboard.writeText(valueToCopy);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);

      onMessageCopy?.(valueToCopy);
    }, [message, onMessageCopy]);

    const handleFeedback = React.useCallback(
      (liked: boolean) => {
        setFeedback(liked ? "like" : "dislike");

        onFeedback?.(liked ? "like" : "dislike");
      },
      [onFeedback],
    );

    const handleAttemptFeedback = React.useCallback(
      (feedback: "like" | "dislike" | "same") => {
        setAttemptFeedback(feedback);

        onAttemptFeedback?.(feedback);
      },
      [onAttemptFeedback],
    );

    return (
      <div {...props} ref={ref} className={cn("flex gap-3", className)}>
        <div className="relative flex-none">
          <Badge
            isOneChar
            color="danger"
            content={<Icon className="text-background" icon="gravity-ui:circle-exclamation-fill" />}
            isInvisible={!hasFailed}
            placement="bottom-right"
            shape="circle"
          >
            <Avatar src={avatar} />
          </Badge>
        </div>
        <div className="flex w-full flex-col gap-4">
          <div
            className={cn(
              "rounded-medium bg-content2 text-default-600 relative w-full px-4 py-3",
              failedMessageClassName,
              messageClassName,
            )}
          >
            <div ref={messageRef} className={"text-small pr-20"}>
              {hasFailed ? failedMessage : message}
            </div>
            {showFeedback && !hasFailed && (
              <div className="bg-content2 shadow-small absolute top-2 right-2 flex rounded-full">
                <Button isIconOnly className="rounded-full" size="sm" variant="ghost" onPress={handleCopy}>
                  {copied ? (
                    <Icon className="text-default-600 text-lg" icon="gravity-ui:check" />
                  ) : (
                    <Icon className="text-default-600 text-lg" icon="gravity-ui:copy" />
                  )}
                </Button>
                <Button
                  isIconOnly
                  className="rounded-full"
                  size="sm"
                  variant="ghost"
                  onPress={() => handleFeedback(true)}
                >
                  {feedback === "like" ? (
                    <Icon className="text-default-600 text-lg" icon="gravity-ui:thumbs-up-fill" />
                  ) : (
                    <Icon className="text-default-600 text-lg" icon="gravity-ui:thumbs-up" />
                  )}
                </Button>
                <Button
                  isIconOnly
                  className="rounded-full"
                  size="sm"
                  variant="ghost"
                  onPress={() => handleFeedback(false)}
                >
                  {feedback === "dislike" ? (
                    <Icon className="text-default-600 text-lg" icon="gravity-ui:thumbs-down-fill" />
                  ) : (
                    <Icon className="text-default-600 text-lg" icon="gravity-ui:thumbs-down" />
                  )}
                </Button>
              </div>
            )}
            {attempts > 1 && !hasFailed && (
              <div className="flex w-full items-center justify-end">
                <button
                  onClick={() => onAttemptChange?.(currentAttempt > 1 ? currentAttempt - 1 : 1)}
                >
                  <Icon
                    className="text-default-400 hover:text-default-500 cursor-pointer"
                    icon="gravity-ui:circle-arrow-left"
                  />
                </button>
                <button
                  onClick={() =>
                    onAttemptChange?.(currentAttempt < attempts ? currentAttempt + 1 : attempts)
                  }
                >
                  <Icon
                    className="text-default-400 hover:text-default-500 cursor-pointer"
                    icon="gravity-ui:circle-arrow-right"
                  />
                </button>
                <p className="text-tiny text-default-500 px-1 font-medium">
                  {currentAttempt}/{attempts}
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    );
  },
);

export default MessageCard;

MessageCard.displayName = "MessageCard";
