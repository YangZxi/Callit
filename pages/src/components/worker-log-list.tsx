import { useEffect, useMemo, useState } from "react";
import { Button, Chip, Tabs } from "@heroui/react";

import Pagination from "@/components/heroui/Pagination";

export type WorkerLogItem = {
  id: string;
  worker_id: string;
  request_id: string;
  trigger?: string;
  status: number;
  stdin: string;
  stdout: string;
  stderr: string;
  exec_log: string;
  result: string;
  error: string;
  duration_ms: number;
  created_at: string;
};

type LogTabKey = "result" | "error" | "stdin" | "stdout" | "stderr" | "exec_log";

type WorkerLogListProps = {
  loading: boolean;
  items: WorkerLogItem[];
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  onRefresh: () => void;
};

type LogTabConfig = {
  key: LogTabKey;
  label: string;
  fixed?: boolean;
  content: string;
  toneClassName: string;
  labelClassName: string;
};

const logTabOrder: LogTabKey[] = ["result", "error", "stdin", "stdout", "stderr", "exec_log"];

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

function normalizeTrigger(trigger?: string): "http" | "cron" {
  return trigger === "cron" ? "cron" : "http";
}

function renderRunStatus(error: string) {
  if (error) {
    return <Chip color="danger" size="sm" variant="soft">Error</Chip>;
  }
  return <Chip color="success" size="sm" variant="soft">Success</Chip>;
}

function buildLogTabs(item: WorkerLogItem): LogTabConfig[] {
  const formattedResult = formatResultForDisplay(item.result);
  const formattedStdin = formatResultForDisplay(item.stdin);
  const tabMap: Record<LogTabKey, LogTabConfig> = {
    result: {
      key: "result",
      label: "result",
      fixed: true,
      content: formattedResult,
      toneClassName: "bg-accent/20 text-accent-600",
      labelClassName: "text-accent",
    },
    error: {
      key: "error",
      label: "error",
      content: item.error,
      toneClassName: "bg-danger/20 text-danger-600",
      labelClassName: "text-danger",
    },
    stdin: {
      key: "stdin",
      label: "stdin",
      fixed: true,
      content: formattedStdin,
      toneClassName: "bg-success/20 text-success-700",
      labelClassName: "text-success",
    },
    stdout: {
      key: "stdout",
      label: "stdout",
      fixed: true,
      content: item.stdout,
      toneClassName: "bg-default/40 text-default-700",
      labelClassName: "text-default-800",
    },
    stderr: {
      key: "stderr",
      label: "stderr",
      content: item.stderr,
      toneClassName: "bg-warning/20 text-warning-700",
      labelClassName: "text-warning",
    },
    exec_log: {
      key: "exec_log",
      label: "exec_log",
      fixed: true,
      content: item.exec_log,
      toneClassName: "bg-default/40 text-default-700",
      labelClassName: "text-default-800",
    },
  };

  return logTabOrder
    .map((key) => tabMap[key])
    .filter((tab) => tab.fixed === true || tab.content);
}

function getDefaultTabKey(tabs: LogTabConfig[]): LogTabKey | null {
  if (tabs.some((tab) => tab.key === "error")) {
    return "error";
  }
  return tabs[0]?.key ?? null;
}

export default function WorkerLogList({
  loading,
  items,
  page,
  totalPages,
  onPageChange,
  onRefresh,
}: WorkerLogListProps) {
  const [selectedLogId, setSelectedLogId] = useState("");
  const [activeTabsByLogId, setActiveTabsByLogId] = useState<Record<string, LogTabKey>>({});

  useEffect(() => {
    if (items.length === 0) {
      setSelectedLogId("");
      return;
    }

    setSelectedLogId((prev) => {
      if (prev && items.some((item) => item.id === prev)) {
        return prev;
      }
      return items[0].id;
    });
  }, [items]);

  const selectedItem = useMemo(
    () => items.find((item) => item.id === selectedLogId) ?? items[0] ?? null,
    [items, selectedLogId],
  );

  const availableTabs = useMemo(
    () => (selectedItem ? buildLogTabs(selectedItem) : []),
    [selectedItem],
  );

  const activeTabKey = useMemo(() => {
    if (!selectedItem) return null;
    const savedTabKey = activeTabsByLogId[selectedItem.id];
    if (savedTabKey && availableTabs.some((tab) => tab.key === savedTabKey)) {
      return savedTabKey;
    }
    return getDefaultTabKey(availableTabs);
  }, [activeTabsByLogId, availableTabs, selectedItem]);

  return (
    <div className="relative flex h-[calc(100vh-250px)] min-h-[500px] flex-col gap-4">
      <div className="flex justify-end gap-2">
        <Pagination
          page={page}
          totalPages={totalPages}
          onPageChange={onPageChange}
        />
        <Button size="sm" variant="secondary" onPress={onRefresh}>
          刷新
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-default-500">正在加载运行日志...</p>
      ) : null}

      {!loading && items.length === 0 ? (
        <div className="rounded-lg border border-default-200 px-3 py-4 text-sm text-default-500">
          暂无运行日志
        </div>
      ) : null}

      {!loading && items.length > 0 ? (
        <div className="grid min-h-0 flex-1 grid-cols-[200px_minmax(0,1fr)] gap-3">
          <div className="min-h-0 overflow-auto rounded-lg border border-default-200">
            {items.map((item) => {
              const trigger = normalizeTrigger(item.trigger);
              const isSelected = item.id === selectedItem?.id;
              return (
                <div
                  key={item.id}
                  className={[
                    "w-full cursor-pointer border-b border-default-200 px-3 py-3 text-left transition-all duration-150 last:border-b-0",
                    isSelected ? "bg-[#f1f1f1] dark:bg-[#464748]" : "hover:bg-[#eaebec] dark:hover:bg-[#464748] hover:shadow-sm",
                  ].join(" ")}
                  onClick={() => {
                    setSelectedLogId(item.id);
                    if (!activeTabKey) {
                      return;
                    }

                    const nextTabs = buildLogTabs(item);
                    if (!nextTabs.some((tab) => tab.key === activeTabKey)) {
                      return;
                    }

                    setActiveTabsByLogId((prev) => ({
                      ...prev,
                      [item.id]: activeTabKey,
                    }));
                  }}
                >
                  <div className="space-y-1">
                    <p className="truncate text-sm font-medium text-default-800" title={item.request_id}>
                      {item.request_id}
                    </p>
                    <p className="text-xs text-default-500">
                      触发类型：{trigger === "http" ? "HTTP" : "Cron"}
                    </p>
                    <p className="text-xs text-default-500">
                      时间：{formatLogTime(item.created_at)}
                    </p>
                  </div>
                </div>
              );
            })}
          </div>

          <div className="min-h-0 rounded-lg border border-default-200">
            {selectedItem ? (
              <div className="flex h-full min-h-0 flex-col">
                <div className="flex items-center justify-between border-b border-default-200 px-4 py-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-default-800" title={selectedItem.id}>
                      {selectedItem.request_id}
                    </p>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-default-500">
                      {renderRunStatus(selectedItem.error)}
                      <span>耗时 {selectedItem.duration_ms}ms</span>
                      {normalizeTrigger(selectedItem.trigger) === "http" ? (
                        <span>HTTP Status {selectedItem.status}</span>
                      ) : null}
                    </div>
                  </div>
                </div>

                <div className="min-h-0 flex-1 p-1">
                  {availableTabs.length > 0 && activeTabKey ? (
                    <Tabs
                      aria-label="日志详情"
                      className="flex h-full min-h-0 flex-col"
                      selectedKey={activeTabKey}
                      onSelectionChange={(key) => {
                        if (!selectedItem) return;
                        setActiveTabsByLogId((prev) => ({
                          ...prev,
                          [selectedItem.id]: String(key) as LogTabKey,
                        }));
                      }}
                    >
                      <Tabs.ListContainer className="px-1">
                        <Tabs.List aria-label="日志详情 Tab" className="w-[unset]">
                          {availableTabs.map((tab) => (
                            <Tabs.Tab id={tab.key} key={tab.key}>
                              {tab.label}
                              <Tabs.Indicator />
                            </Tabs.Tab>
                          ))}
                        </Tabs.List>
                      </Tabs.ListContainer>

                      {availableTabs.map((tab) => (
                        <Tabs.Panel className="min-h-0 flex-1 pt-2" id={tab.key} key={tab.key}>
                          <div className="flex h-full min-h-0 flex-col">
                            <pre className={`min-h-0 flex-1 overflow-auto rounded-md p-3 font-mono text-xs whitespace-pre-wrap break-words ${tab.toneClassName}`}>
                              {tab.content}
                            </pre>
                          </div>
                        </Tabs.Panel>
                      ))}
                    </Tabs>
                  ) : (
                    <div className="flex h-full items-center justify-center text-sm text-default-400">
                      无详细输出
                    </div>
                  )}
                </div>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
