import { useEffect, useMemo, useState } from "react";
import { Alert, Tooltip, toast } from "@heroui/react";
import { Button } from "@heroui/react";
import { Icon } from "@iconify/react";

import api from "@/lib/api";
import { Input } from "@/components/heroui";

export type WorkerCronItem = {
  id: string;
  cron: string;
  worker_id: string;
  created_at: string;
  updated_at: string;
};

type WorkerCronListProps = {
  workerId: string;
  isOpen: boolean;
};

type CronRow = {
  key: string;
  id?: string;
  cron: string;
  draft: string;
  mode: "view" | "edit" | "create";
  submitting: boolean;
  deleting: boolean;
};

function toViewRow(item: WorkerCronItem): CronRow {
  return {
    key: `cron-${item.id}`,
    id: item.id,
    cron: item.cron,
    draft: item.cron,
    mode: "view",
    submitting: false,
    deleting: false,
  };
}

export default function WorkerCronList({ workerId, isOpen }: WorkerCronListProps) {
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState<CronRow[]>([]);

  const hasPendingCreate = useMemo(() => rows.some((item) => item.mode === "create"), [rows]);

  const loadCrons = async () => {
    if (!workerId) return;
    setLoading(true);
    try {
      const data = await api.get<WorkerCronItem[]>(`/workers/${workerId}/crons`);
      setRows((data || []).map(toViewRow));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!isOpen || !workerId) return;
    void loadCrons();
  }, [isOpen, workerId]);

  const updateRow = (key: string, updater: (current: CronRow) => CronRow) => {
    setRows((prev) => prev.map((item) => (item.key === key ? updater(item) : item)));
  };

  const removeRow = (key: string) => {
    setRows((prev) => prev.filter((item) => item.key !== key));
  };

  const startCreate = () => {
    setRows((prev) => [...prev, {
      key: `new-${Date.now()}`,
      cron: "",
      draft: "",
      mode: "create",
      submitting: false,
      deleting: false,
    }]);
  };

  const startEdit = (key: string) => {
    updateRow(key, (current) => ({
      ...current,
      draft: current.cron,
      mode: "edit",
    }));
  };

  const cancelEdit = (key: string) => {
    const row = rows.find((item) => item.key === key);
    if (!row) return;
    if (row.mode === "create") {
      removeRow(key);
      return;
    }
    updateRow(key, (current) => ({
      ...current,
      draft: current.cron,
      mode: "view",
      submitting: false,
    }));
  };

  const submitRow = async (key: string) => {
    const row = rows.find((item) => item.key === key);
    if (!row) return;

    const cron = row.draft.trim();
    if (!cron) {
      toast.warning("cron 不能为空");
      return;
    }

    updateRow(key, (current) => ({ ...current, submitting: true }));
    try {
      if (row.mode === "create") {
        const created = await api.post<WorkerCronItem>(`/workers/${workerId}/crons/create`, { cron });
        updateRow(key, () => toViewRow(created));
        return;
      }

      if (!row.id) {
        throw new Error("缺少 cron id");
      }
      const updated = await api.post<WorkerCronItem>(`/workers/${workerId}/crons/update`, { id: row.id, cron });
      updateRow(key, () => toViewRow(updated));
    } catch {
      updateRow(key, (current) => ({ ...current, submitting: false }));
    }
  };

  const deleteRow = async (key: string) => {
    const row = rows.find((item) => item.key === key);
    if (!row?.id) return;
    if (!window.confirm(`确认删除 cron：${row.cron}？`)) {
      return;
    }

    updateRow(key, (current) => ({ ...current, deleting: true }));
    try {
      await api.post<{ ok: boolean }>(`/workers/${workerId}/crons/delete`, { id: row.id });
      removeRow(key);
    } catch {
      updateRow(key, (current) => ({ ...current, deleting: false }));
    }
  };

  return (
    <div className="flex min-h-[220px] flex-col gap-3">
      <Tooltip>
        <Button isIconOnly variant="tertiary">
          <Icon icon="ri:question-line" width={20} />
        </Button>
        <Tooltip.Content>
          <div className="text-sm flex flex-col gap-2">
            支持标准的 Cron 表达式格式，示例：
            <p>
              每小时的第 10 分钟执行
              <span className="ml-2 rounded bg-default-200 px-1.5 py-0.5 font-mono text-xs">10 * * * *</span>
            </p>
            <p>
              每 30 秒执行一次
              <span className="ml-2 rounded bg-default-200 px-1.5 py-0.5 font-mono text-xs">*/30 * * * * *</span>
            </p>
          </div>
        </Tooltip.Content>
      </Tooltip>

      {loading ? <p className="text-sm text-default-500">正在加载 Cron 列表...</p> : null}

      {!loading && rows.length === 0 ? (
        <div className="rounded-lg border border-dashed border-default-300 px-4 py-2 text-sm text-default-500">
          暂无 Cron 表达式
        </div>
      ) : null}

      <div className="flex flex-col gap-2">
        {rows.map((row) => (
          <div key={row.key} className="flex items-center gap-2 rounded-lg border border-default-200 px-3 py-2">
            <div className="min-w-0 flex-1">
              {row.mode === "view" ? (
                <p className="truncate text-sm text-default-800">{row.cron}</p>
              ) : (
                <Input
                  autoFocus
                  className="w-full"
                  name={`cron-${row.key}`}
                  placeholder="请输入 cron 表达式"
                  value={row.draft}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      event.preventDefault();
                      void submitRow(row.key);
                    }
                  }}
                  onValueChange={(value) => updateRow(row.key, (current) => ({ ...current, draft: value }))}
                />
              )}
            </div>

            {row.mode === "view" ? (
              <>
                <Button isIconOnly size="sm" variant="secondary" onPress={() => startEdit(row.key)}>
                  <Icon icon="solar:pen-2-linear" width={18} />
                </Button>
                <Button
                  isIconOnly
                  isPending={row.deleting}
                  size="sm"
                  variant="danger-soft"
                  onPress={() => {
                    void deleteRow(row.key);
                  }}
                >
                  <Icon icon="solar:trash-bin-trash-linear" width={18} />
                </Button>
              </>
            ) : (
              <>
                <Button isIconOnly size="sm" variant="secondary" onPress={() => cancelEdit(row.key)}>
                  <Icon icon="solar:close-circle-linear" width={18} />
                </Button>
                <Button
                  isIconOnly
                  isPending={row.submitting}
                  size="sm"
                  variant="primary"
                  onPress={() => {
                    void submitRow(row.key);
                  }}
                >
                  <Icon icon="solar:check-circle-linear" width={18} />
                </Button>
              </>
            )}
          </div>
        ))}
      </div>

      <div className="flex justify-end">
        <Button
          isIconOnly
          isDisabled={hasPendingCreate}
          size="sm"
          variant="primary"
          onPress={startCreate}
        >
          <Icon icon="lucide:circle-plus" width={20} />
        </Button>
      </div>
    </div>
  );
}
