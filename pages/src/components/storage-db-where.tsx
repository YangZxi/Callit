import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import clsx from "clsx";

import { dbFieldCompletion, type DBColumn } from "@/lib/storage-db";

type Props = {
  columns: DBColumn[];
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
  onQuery: () => void;
};

export default function StorageDBWhere({ columns, value, disabled, onChange, onQuery }: Props) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const listID = useId();
  const [cursor, setCursor] = useState(0);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const completion = dbFieldCompletion(value, cursor, columns);
  const candidates = open ? completion?.candidates ?? [] : [];
  const activeIndex = Math.min(active, Math.max(0, candidates.length - 1));

  useEffect(() => {
    listRef.current?.children[activeIndex]?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

  const insertColumn = (column: DBColumn) => {
    if (!completion) return;
    const identifier = `"${column.name.replace(/"/g, '""')}"`;
    const next = value.slice(0, completion.start) + identifier + value.slice(completion.end);
    const nextCursor = completion.start + identifier.length;
    onChange(next);
    setCursor(nextCursor);
    setOpen(false);
    requestAnimationFrame(() => {
      inputRef.current?.focus();
      inputRef.current?.setSelectionRange(nextCursor, nextCursor);
    });
  };

  return (
    <div className="relative min-w-0 flex-1">
      <input
        ref={inputRef}
        aria-label={t("db.where")}
        aria-autocomplete="list"
        aria-controls={listID}
        aria-expanded={candidates.length > 0}
        aria-activedescendant={candidates.length ? `${listID}-${activeIndex}` : undefined}
        autoComplete="off"
        spellCheck={false}
        role="combobox"
        className="w-full rounded-lg border border-default-200 bg-background px-3 py-2 font-mono text-sm outline-none focus:border-accent disabled:opacity-50"
        disabled={disabled}
        placeholder={t("db.wherePlaceholder")}
        value={value}
        onBlur={() => setOpen(false)}
        onChange={(event) => {
          onChange(event.target.value);
          setCursor(event.target.selectionStart ?? event.target.value.length);
          setActive(0);
          setOpen(true);
        }}
        onSelect={(event) => setCursor(event.currentTarget.selectionStart ?? value.length)}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
            event.preventDefault();
            setOpen(false);
            onQuery();
          } else if ((event.key === "ArrowDown" || event.key === "ArrowUp") && completion) {
            event.preventDefault();
            setOpen(true);
            setActive(open ? (activeIndex + (event.key === "ArrowDown" ? 1 : -1) + completion.candidates.length) % completion.candidates.length : 0);
          } else if (event.key === "Enter" && candidates.length) {
            event.preventDefault();
            insertColumn(candidates[activeIndex]);
          } else if (event.key === "Escape") {
            setOpen(false);
          }
        }}
      />
      {candidates.length > 0 && (
        <ul ref={listRef} id={listID} role="listbox" aria-label={t("db.fieldSuggestions")} className="absolute top-full right-0 left-0 z-30 max-h-56 overflow-y-auto rounded-b-lg border border-default-200 bg-background py-1 shadow-lg">
          {candidates.map((column, index) => (
            <li
              key={column.name}
              id={`${listID}-${index}`}
              role="option"
              aria-selected={index === activeIndex}
              className={clsx("flex cursor-pointer items-center justify-between gap-3 px-3 py-2 text-sm", index === activeIndex && "bg-default-100")}
              onMouseDown={(event) => event.preventDefault()}
              onMouseEnter={() => setActive(index)}
              onClick={() => insertColumn(column)}
            >
              <span className="font-mono">{column.name}</span>
              <span className="text-xs text-default-500">{column.type}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
