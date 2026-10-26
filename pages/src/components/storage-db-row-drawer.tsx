import { useState } from "react";
import { Button, Drawer, TextArea, toast } from "@heroui/react";
import { useTranslation } from "react-i18next";

import api from "@/lib/api";
import { displayDBValue, type DBRow, type DBTable, type DBValue } from "@/lib/storage-db";

type FieldValue = { type: DBValue["type"] | "default"; value: string };
type Props = {
  table: DBTable;
  row: DBRow | null;
  onClose: () => void;
  onSaved: () => Promise<void>;
};

export default function StorageDBRowDrawer({ table, row, onClose, onSaved }: Props) {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [fields, setFields] = useState<Record<string, FieldValue>>(() => Object.fromEntries(
    table.columns.map((column) => [column.name, row?.values[column.name] ?? { type: "default", value: "" }]),
  ));
  const changed = !row || table.columns.some((column) => !column.generated && JSON.stringify(fields[column.name]) !== JSON.stringify(row.values[column.name]));

  const save = async () => {
    if (saving || !changed) return;
    const values: Record<string, DBValue> = Object.create(null);
    for (const column of table.columns) {
      const field = fields[column.name];
      if (column.generated || field.type === "default") continue;
      if (row && JSON.stringify(field) === JSON.stringify(row.values[column.name])) continue;
      values[column.name] = { type: field.type, value: field.type === "null" ? "" : field.value };
    }
    setSaving(true);
    setError("");
    try {
      await api.post(`/db/rows/${row ? "update" : "create"}`, {
        table: table.name,
        values,
        identity: row?.identity,
        version: row?.version,
      }, { hideToast: true });
    } catch (failure) {
      setError((failure as { msg?: string } | null)?.msg || t("api.requestFailed"));
      setSaving(false);
      return;
    }
    toast.success(t("db.saved"));
    await onSaved();
    onClose();
  };

  return (
    <Drawer isOpen onOpenChange={(open) => { if (!open && !saving) onClose(); }}>
      <Drawer.Backdrop isDismissable={!saving} isKeyboardDismissDisabled={saving}>
        <Drawer.Content placement="right">
          <Drawer.Dialog className="w-[min(96vw,40rem)]">
            <Drawer.Header>
              <Drawer.Heading>{t(row ? "db.editRow" : "db.addRow")} · {table.name}</Drawer.Heading>
            </Drawer.Header>
            <Drawer.Body className="space-y-5">
              {!row && <p className="text-sm text-default-500">{t("db.defaultHint")}</p>}
              {table.columns.map((column, index) => {
                const field = fields[column.name];
                const inputID = `db-field-${index}`;
                return (
                  <div key={column.name} className="space-y-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <label htmlFor={inputID} className="font-mono text-sm font-medium">{column.name}</label>
                      <span className="text-xs text-default-500">
                        {column.type || "ANY"}{column.primary_key ? " · PK" : ""}{column.not_null ? " · NOT NULL" : ""}
                      </span>
                    </div>
                    {column.generated ? (
                      <p className="break-all rounded-lg bg-default-100 p-3 text-sm text-default-500">{row ? displayDBValue(row.values[column.name]) : t("db.generated")}</p>
                    ) : (
                      <>
                        <select
                          aria-label={t("db.valueType", { name: column.name })}
                          className="w-full rounded-lg border border-default-200 bg-background px-3 py-2 text-sm"
                          disabled={saving}
                          value={field.type}
                          onChange={(event) => setFields((previous) => ({ ...previous, [column.name]: { ...field, type: event.target.value as FieldValue["type"] } }))}
                        >
                          {!row && <option value="default">{t("db.useDefault")}</option>}
                          <option value="text">{t("db.textType")}</option>
                          <option value="integer">{t("db.integerType")}</option>
                          <option value="real">{t("db.realType")}</option>
                          <option value="null">NULL</option>
                          <option value="blob">{t("db.blobType")}</option>
                        </select>
                        {field.type !== "default" && field.type !== "null" && (
                          <TextArea
                            id={inputID}
                            aria-label={column.name}
                            className="w-full font-mono text-sm"
                            rows={field.type === "text" || field.type === "blob" ? 3 : 1}
                            disabled={saving}
                            value={field.value}
                            onChange={(event) => setFields((previous) => ({ ...previous, [column.name]: { ...field, value: event.target.value } }))}
                          />
                        )}
                        {field.type === "default" && column.default_value !== null && <p className="break-all font-mono text-xs text-default-500">{column.default_value}</p>}
                      </>
                    )}
                  </div>
                );
              })}
              {error && <p role="alert" className="whitespace-pre-wrap break-words text-sm text-danger">{error}</p>}
            </Drawer.Body>
            <Drawer.Footer className="flex justify-end gap-2">
              <Button variant="secondary" isDisabled={saving} onPress={onClose}>{t("common.cancel")}</Button>
              <Button variant="primary" isPending={saving} isDisabled={!changed || saving} onPress={() => { void save(); }}>{t("common.submit")}</Button>
            </Drawer.Footer>
          </Drawer.Dialog>
        </Drawer.Content>
      </Drawer.Backdrop>
    </Drawer>
  );
}
