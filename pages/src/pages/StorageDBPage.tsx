import { useCallback, useEffect, useRef, useState } from "react";
import { Button, Card, Spinner, toast } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useTranslation } from "react-i18next";
import { useBlocker } from "react-router-dom";
import clsx from "clsx";

import api from "@/lib/api";
import i18n from "@/i18n";
import { dbRowKey, displayDBValue, type DBQuery, type DBQueryResult, type DBRow } from "@/lib/storage-db";
import { resolvePendingDBNavigation } from "@/lib/storage-db-navigation";
import StorageDBWhere from "@/components/storage-db-where";
import StorageDBRowDrawer from "@/components/storage-db-row-drawer";

const initialQuery: DBQuery = { table: "", where: "", page: 1, page_size: 50, sort_column: "", sort_direction: "asc" };

export default function StorageDBPage() {
  const { t } = useTranslation();
  const [tables, setTables] = useState<string[]>([]);
  const [tableSearch, setTableSearch] = useState("");
  const [query, setQuery] = useState(initialQuery);
  const [where, setWhere] = useState("");
  const [result, setResult] = useState<DBQueryResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [committing, setCommitting] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [pending, setPending] = useState<Map<string, DBRow>>(new Map());
  const [editor, setEditor] = useState<{ row: DBRow | null } | null>(null);
  const [showSchema, setShowSchema] = useState(false);
  const requestID = useRef(0);
  const busyRef = useRef(false);
  const busy = loading || committing;
  const navigationBlocker = useBlocker(pending.size > 0);

  useEffect(() => {
    resolvePendingDBNavigation(navigationBlocker, committing, () => {
      if (!window.confirm(t("db.discardPending"))) return false;
      setPending(new Map());
      return true;
    });
  }, [navigationBlocker, committing, t]);

  const load = useCallback(async (nextQuery: DBQuery, refreshTables = false) => {
    const currentRequest = ++requestID.current;
    busyRef.current = true;
    setLoading(true);
    setError("");
    setQuery(nextQuery);
    setSelected(new Set());
    setResult((previous) => previous?.table.name === nextQuery.table ? { ...previous, rows: [], total: 0 } : null);
    try {
      if (refreshTables) {
        const names = await api.get<string[]>("/db/tables", { hideToast: true });
        if (currentRequest !== requestID.current) return;
        setTables(names);
        if (!names.includes(nextQuery.table)) {
          nextQuery = { ...initialQuery, table: names[0] || "", page_size: nextQuery.page_size };
          setQuery(nextQuery);
          setWhere("");
          setResult(null);
        }
      }
      if (!nextQuery.table) return;
      const data = await api.post<DBQueryResult>("/db/query", nextQuery, { hideToast: true });
      if (currentRequest !== requestID.current) return;
      setResult(data);
      setQuery({ ...nextQuery, page: data.page, page_size: data.page_size });
    } catch (failure) {
      if (currentRequest === requestID.current) setError((failure as { msg?: string } | null)?.msg || i18n.t("api.requestFailed"));
    } finally {
      if (currentRequest === requestID.current) {
        busyRef.current = false;
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void load(initialQuery, true);
    return () => { requestID.current++; };
  }, [load]);

  useEffect(() => {
    if (!pending.size) return;
    const preventUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", preventUnload);
    return () => window.removeEventListener("beforeunload", preventUnload);
  }, [pending.size]);

  const navigate = (nextQuery: DBQuery, refreshTables = false) => {
    if (busyRef.current || committing) return;
    if (pending.size && !window.confirm(t("db.discardPending"))) return;
    setPending(new Map());
    if (nextQuery.table !== query.table) setWhere(nextQuery.where);
    void load(nextQuery, refreshTables);
  };

  const markDeleted = () => {
    if (busy) return;
    setPending((previous) => {
      const next = new Map(previous);
      for (const row of result?.rows ?? []) {
        if (selected.has(dbRowKey(row))) next.set(dbRowKey(row), row);
      }
      return next;
    });
    setSelected(new Set());
  };

  const commitDeletes = async () => {
    if (busyRef.current || !pending.size) return;
    busyRef.current = true;
    setCommitting(true);
    setError("");
    try {
      await api.post("/db/rows/delete", {
        table: query.table,
        rows: Array.from(pending.values(), (row) => ({ identity: row.identity, version: row.version })),
      }, { hideToast: true });
      setPending(new Map());
      toast.success(t("db.deleted", { count: pending.size }));
      await load(query);
    } catch (failure) {
      setError((failure as { msg?: string } | null)?.msg || t("api.requestFailed"));
    } finally {
      busyRef.current = false;
      setCommitting(false);
    }
  };

  const canIdentify = Boolean(result?.table.identity_columns.length);
  const selectableRows = canIdentify ? result?.rows.filter((row) => !pending.has(dbRowKey(row))) ?? [] : [];
  const allSelected = selectableRows.length > 0 && selectableRows.every((row) => selected.has(dbRowKey(row)));
  const totalPages = Math.max(1, Math.ceil((result?.total ?? 0) / query.page_size));
  const toolbarButton = (label: string, icon: string, action: () => void, disabled = false, primary = false) => (
    <span title={label}>
      <Button aria-label={label} isIconOnly size="sm" variant={primary ? "primary" : "tertiary"} isDisabled={busy || disabled} onPress={action}>
        <Icon icon={icon} width={19} />
      </Button>
    </span>
  );

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-default-900">{t("db.title")}</h1>
        <p className="mt-1 text-sm text-default-500">{t("db.description")}</p>
      </div>
      <div className="grid min-h-[600px] gap-4 lg:grid-cols-[240px_minmax(0,1fr)]">
        <Card className="border border-default-200/70 bg-background/70 shadow-sm">
          <Card.Header>
            <Card.Title className="flex items-center justify-between text-base">
              <span>{t("db.tables")}</span>
              {toolbarButton(t("common.refresh"), "lucide:refresh-cw", () => navigate(query, true))}
            </Card.Title>
          </Card.Header>
          <Card.Content className="space-y-3">
            <input aria-label={t("db.searchTables")} placeholder={t("db.searchTables")} value={tableSearch} onChange={(event) => setTableSearch(event.target.value)} className="w-full rounded-lg border border-default-200 bg-background px-3 py-2 text-sm" />
            <div className="max-h-[65vh] space-y-1 overflow-auto">
              {tables.filter((name) => name.toLocaleLowerCase().includes(tableSearch.toLocaleLowerCase())).map((name) => (
                <button key={name} type="button" disabled={busy} aria-pressed={name === query.table} className={clsx("flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm hover:bg-default-100 disabled:opacity-50", name === query.table && "bg-default-100 font-medium")} onClick={() => navigate({ ...initialQuery, table: name, page_size: query.page_size })}>
                  <Icon icon="lucide:table-2" className="shrink-0 text-default-500" width={16} /><span className="truncate" title={name}>{name}</span>
                </button>
              ))}
              {!tables.length && !loading && <p className="py-4 text-sm text-default-500">{t("db.noTables")}</p>}
            </div>
          </Card.Content>
        </Card>
        <Card className="min-w-0 border border-default-200/70 bg-background/70 shadow-sm">
          <Card.Header className="gap-3">
            <Card.Title className="break-all text-base">{query.table || t("db.selectTable")}</Card.Title>
            <div className="flex flex-wrap items-center gap-1" role="toolbar" aria-label={t("db.actions")}>
              {toolbarButton(t("common.refresh"), "lucide:refresh-cw", () => navigate(query, true))}
              {toolbarButton(t("db.addRow"), "lucide:plus", () => setEditor({ row: null }), !result)}
              {toolbarButton(t("db.markDelete"), "lucide:trash-2", markDeleted, !selected.size)}
              {toolbarButton(t("db.commitDeletes"), "lucide:check", () => { void commitDeletes(); }, !pending.size, true)}
              {toolbarButton(t("db.undoDeletes"), "lucide:undo-2", () => setPending(new Map()), !pending.size)}
              <span className="mx-2 h-5 border-l border-default-200" />
              {toolbarButton(t("db.schema"), "lucide:columns-3", () => setShowSchema((previous) => !previous), !result)}
              {pending.size > 0 && <span role="status" className="ml-2 text-xs text-default-500">{t("db.pendingDeletes", { count: pending.size })}</span>}
            </div>
            <div className="flex w-full items-center gap-2">
              <span className="font-mono text-xs text-default-500">WHERE</span>
              <StorageDBWhere columns={result?.table.columns ?? []} value={where} disabled={busy || !query.table} onChange={setWhere} onQuery={() => navigate({ ...query, where, page: 1 })} />
              <Button size="sm" variant="secondary" isDisabled={busy || !query.table} onPress={() => navigate({ ...query, where, page: 1 })}>{t("db.query")}</Button>
            </div>
            <p className="text-xs text-default-500">{t("db.queryHint")}</p>
          </Card.Header>
          <Card.Content className="min-w-0 space-y-4">
            {error && <p role="alert" className="whitespace-pre-wrap break-words rounded-lg bg-danger/10 px-3 py-2 text-sm text-danger">{error}</p>}
            {showSchema && result && (
              <div className="space-y-3 rounded-lg border border-default-200 p-3">
                <h2 className="text-sm font-medium">{t("db.schema")}</h2>
                <div className="overflow-auto">
                  <table className="w-full whitespace-nowrap text-left text-xs">
                    <thead>
                      <tr className="border-b border-default-200">
                        <th className="p-2">{t("db.field")}</th>
                        <th className="p-2">{t("db.type")}</th>
                        <th className="p-2">{t("db.constraints")}</th>
                        <th className="p-2">{t("db.defaultValue")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {result.table.columns.map((column) => (
                        <tr key={column.name} className="border-b border-default-100">
                          <td className="p-2 font-mono">{column.name}</td>
                          <td className="p-2">{column.type || "ANY"}</td>
                          <td className="p-2">
                            {[
                              column.primary_key ? `PK ${column.primary_key}` : "",
                              column.not_null ? "NOT NULL" : "",
                              column.generated ? t("db.generated") : "",
                            ].filter(Boolean).join(" · ") || "—"}
                          </td>
                          <td className="p-2 font-mono">{column.default_value ?? "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <pre className="overflow-auto whitespace-pre-wrap break-all text-xs text-default-500">{result.table.sql}</pre>
              </div>
            )}
            {result && !canIdentify && <p className="text-sm text-default-500">{t("db.noIdentity")}</p>}
            {busy ? <div className="flex min-h-72 items-center justify-center gap-2"><Spinner size="sm" /><span className="text-sm text-default-500">{t("common.loading")}</span></div> : result && (
              <div className="max-h-[60vh] overflow-auto rounded-lg border border-default-200">
                <table className="w-full border-collapse whitespace-nowrap text-left text-sm">
                  <thead className="sticky top-0 z-10 bg-background">
                    <tr className="border-b border-default-200">
                      <th className="w-10 px-3 py-3">
                        <input
                          type="checkbox"
                          aria-label={t("db.selectPage")}
                          checked={allSelected}
                          disabled={!selectableRows.length}
                          ref={(element) => { if (element) element.indeterminate = selected.size > 0 && !allSelected; }}
                          onChange={(event) => setSelected(event.target.checked ? new Set(selectableRows.map(dbRowKey)) : new Set())}
                        />
                      </th>
                      {result.table.columns.map((column) => <th key={column.name} className="px-3 py-2 font-medium" aria-sort={query.sort_column === column.name ? query.sort_direction === "asc" ? "ascending" : "descending" : "none"}>
                        <button className="flex items-center gap-2" type="button" onClick={() => navigate({ ...query, page: 1, sort_column: column.name, sort_direction: query.sort_column === column.name && query.sort_direction === "asc" ? "desc" : "asc" })}>
                          <span>{column.name}<span className="ml-2 text-xs font-normal text-default-400">{column.type}</span></span>
                          <Icon icon={query.sort_column === column.name ? query.sort_direction === "asc" ? "lucide:arrow-up" : "lucide:arrow-down" : "lucide:arrow-up-down"} width={13} />
                        </button>
                      </th>)}
                      <th className="w-16 px-3 py-2">{t("db.actions")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.rows.map((row, index) => {
                      const key = canIdentify ? dbRowKey(row) : String(index);
                      const marked = pending.has(key);
                      return (
                        <tr key={key} className={clsx("border-b border-default-100", marked ? "bg-default-200 text-default-500" : selected.has(key) ? "bg-accent/10" : "hover:bg-default-50")}>
                          <td className="px-3 py-2">
                            <input
                              type="checkbox"
                              aria-label={t("db.selectRow", { number: index + 1 })}
                              checked={selected.has(key)}
                              disabled={!canIdentify || marked}
                              onChange={(event) => {
                                const checked = event.target.checked;
                                setSelected((previous) => {
                                  const next = new Set(previous);
                                  if (checked) next.add(key);
                                  else next.delete(key);
                                  return next;
                                });
                              }}
                            />
                          </td>
                          {result.table.columns.map((column) => (
                            <td
                              key={column.name}
                              className={clsx("max-w-80 truncate px-3 py-2 font-mono text-xs", row.values[column.name].type === "null" && "italic text-default-400")}
                              title={displayDBValue(row.values[column.name])}
                            >
                              {displayDBValue(row.values[column.name])}
                            </td>
                          ))}
                          <td className="px-3 py-1">
                            {marked
                              ? toolbarButton(t("db.undoDelete"), "lucide:undo-2", () => setPending((previous) => {
                                const next = new Map(previous);
                                next.delete(key);
                                return next;
                              }))
                              : toolbarButton(t("db.editRow"), "lucide:pencil", () => setEditor({ row }), !canIdentify)}
                          </td>
                        </tr>
                      );
                    })}
                    {!result.rows.length && <tr><td colSpan={result.table.columns.length + 2} className="py-16 text-center text-default-500">{t("db.noRows")}</td></tr>}
                  </tbody>
                </table>
              </div>
            )}
            {result && <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-default-500">
              <span>{t("db.rowCount", { count: result.total })}</span>
              <div className="flex items-center gap-2">
                <select aria-label={t("db.pageSize")} className="rounded-lg border border-default-200 bg-background px-2 py-1" disabled={busy} value={query.page_size} onChange={(event) => navigate({ ...query, page: 1, page_size: Number(event.target.value) })}>{[20, 50, 100].map((size) => <option key={size} value={size}>{t("db.perPage", { count: size })}</option>)}</select>
                {toolbarButton(t("db.previousPage"), "lucide:chevron-left", () => navigate({ ...query, page: query.page - 1 }), query.page <= 1)}
                <span>{query.page} / {totalPages}</span>
                {toolbarButton(t("db.nextPage"), "lucide:chevron-right", () => navigate({ ...query, page: query.page + 1 }), query.page >= totalPages)}
              </div>
            </div>}
          </Card.Content>
        </Card>
      </div>
      {editor && result && <StorageDBRowDrawer table={result.table} row={editor.row} onClose={() => setEditor(null)} onSaved={() => load(query)} />}
    </div>
  );
}
