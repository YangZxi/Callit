import { Button, Pagination } from "@heroui/react";

export type WorkerLogItem = {
  id: string;
  worker_id: string;
  request_id: string;
  status: number;
  stdout: string;
  stderr: string;
  result: string;
  error: string;
  duration_ms: number;
  created_at: string;
};

type WorkerLogListProps = {
  loading: boolean;
  items: WorkerLogItem[];
  expandedIds: Set<string>;
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
  onToggle: (id: string) => void;
  onPageChange: (page: number) => void;
};

function formatLogTime(raw: string): string {
  const dt = new Date(raw);
  if (Number.isNaN(dt.getTime())) {
    return raw;
  }
  return dt.toLocaleString();
}

function formatResultForDisplay(raw: string): string {
  if (!raw) return "";
  try {
    const parsed = JSON.parse(raw);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return raw;
  }
}

export default function WorkerLogList({
  loading,
  items,
  expandedIds,
  page,
  totalPages,
  onToggle,
  onPageChange,
}: WorkerLogListProps) {
  return (
    <div className="flex max-h-[60vh] flex-col gap-2 pr-1 relative">

      <div className="w-full flex justify-end">
        <Pagination showControls color="success" page={page} total={totalPages} onChange={onPageChange} />
      </div>
      <div className="overflow-auto h-[500px]">
        {loading ? (
          <p className="text-sm text-default-500">正在加载运行日志...</p>
        ) : null}
        {!loading && items.length === 0 ? (
          <div className="rounded-lg border border-default-200 px-3 py-4 text-sm text-default-500">
            暂无运行日志
          </div>
        ) : null}
        {!loading && items.length > 0 ? (
          items.map((item) => {
            const expanded = expandedIds.has(item.id);
            const formattedResult = formatResultForDisplay(item.result);
            return (
              <div key={item.id} className="rounded-lg border border-default-200 p-3">
                <div className="flex items-start justify-between gap-3" onClick={() => onToggle(item.id)}>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-default-800">request_id: {item.request_id}</p>
                    <p className="truncate text-xs text-default-500">
                      {formatLogTime(item.created_at)} · 状态 {item.status} · 耗时 {item.duration_ms}ms
                    </p>
                  </div>
                  <Button color="default" size="sm" variant="flat" onPress={() => onToggle(item.id)}>
                    {expanded ? "收起详情" : "展开详情"}
                  </Button>
                </div>
                {expanded ? (
                  <div className="mt-3 space-y-2">
                    {item.error ? (
                      <div>
                        <p className="mb-1 text-xs font-medium text-danger">error</p>
                        <pre className="max-h-40 overflow-auto rounded-md bg-danger-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-danger-700">{item.error}</pre>
                      </div>
                    ) : null}
                    {item.stderr ? (
                      <div>
                        <p className="mb-1 text-xs font-medium text-warning">stderr</p>
                        <pre className="max-h-40 overflow-auto rounded-md bg-warning-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-warning-700">{item.stderr}</pre>
                      </div>
                    ) : null}
                    {item.stdout ? (
                      <div>
                        <p className="mb-1 text-xs font-medium text-success">stdout</p>
                        <pre className="max-h-40 overflow-auto rounded-md bg-success-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-success-700">{item.stdout}</pre>
                      </div>
                    ) : null}
                    {item.result ? (
                      <div>
                        <p className="mb-1 text-xs font-medium text-primary">result</p>
                        <pre className="max-h-40 overflow-auto rounded-md bg-primary-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-primary-700">{formattedResult}</pre>
                      </div>
                    ) : null}
                    {!item.error && !item.stderr && !item.stdout && !item.result ? (
                      <p className="text-xs text-default-400">无详细输出</p>
                    ) : null}
                  </div>
                ) : null}
              </div>
            );
          })
        ) : null}
      </div>
    </div>
  );
}
