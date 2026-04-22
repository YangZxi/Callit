import { useCallback, useEffect, useMemo, useState } from "react";
import type { Key, ReactNode } from "react";
import { Button, Card, Chip, Skeleton, Tabs } from "@heroui/react";
import { Icon } from "@iconify/react";
import {
  Bar,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts";

import { Select } from "@/components/heroui";
import api from "@/lib/api";

type WorkerItem = {
  id: string;
  name: string;
  enabled: boolean;
};

type DashboardWorkerCounts = {
  total: number;
  enabled: number;
  disabled: number;
};

type DashboardSummary = {
  success_rate_24h: number | null;
  success_rate_6h: number | null;
  avg_duration_ms_24h: number | null;
  total_calls_24h: number;
  success_calls_24h: number;
  failed_calls_24h: number;
  total_calls_6h: number;
  failed_calls_6h: number;
  last_failed_workers_count: number;
};

type DashboardLastFailedWorker = {
  worker_id: string;
  worker_name: string;
  last_log_id: string;
  last_request_id: string;
  last_status: number;
  last_duration_ms: number;
  last_failed_at: string;
  last_error: string;
};

type DashboardSlowWorkerRankItem = {
  worker_id: string;
  worker_name: string;
  calls: number;
  success_rate: number | null;
  avg_duration_ms: number;
};

type DashboardReliableWorkerRankItem = {
  worker_id: string;
  worker_name: string;
  calls: number;
  success_rate: number | null;
  failed: number;
};

type DashboardWorkerRankings = {
  slowest: DashboardSlowWorkerRankItem[];
  least_reliable: DashboardReliableWorkerRankItem[];
};

type DashboardMetricsResponse = {
  generated_at: string;
  workers: DashboardWorkerCounts;
  summary: DashboardSummary;
  last_failed_workers: DashboardLastFailedWorker[];
  worker_rankings: DashboardWorkerRankings;
};

type DashboardTrendPoint = {
  time: string;
  total: number;
  success: number;
  failed: number;
  success_rate: number | null;
  avg_duration_ms: number | null;
};

type HealthRankItem = {
  worker_id: string;
  worker_name: string;
  calls: number;
  success_rate: number | null;
  avg_duration_ms: number | null;
  failed?: number;
};

const allWorkersValue = "all";

function formatPercent(value: number | null | undefined): string {
  if (value == null) return "--";
  return `${value.toFixed(2)}%`;
}

function formatDuration(value: number | null | undefined): string {
  if (value == null) return "--";
  if (value >= 1000) {
    const seconds = value / 1000;
    return `${seconds >= 10 ? seconds.toFixed(0) : seconds.toFixed(1)}s`;
  }
  return `${value}ms`;
}

function formatDurationAxis(value: number): string {
  if (!Number.isFinite(value)) return "--";
  if (value >= 1000) {
    const seconds = value / 1000;
    return `${seconds >= 10 ? seconds.toFixed(0) : seconds.toFixed(1)}s`;
  }
  return `${Math.round(value)}ms`;
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat(undefined, {
    notation: value >= 1000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(value);
}

function formatDateTime(raw: string): string {
  const dt = new Date(raw);
  if (Number.isNaN(dt.getTime())) return "--";
  return dt.toLocaleString();
}

function formatClock(raw: string): string {
  const dt = new Date(raw);
  if (Number.isNaN(dt.getTime())) return "--";
  return dt.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatHour(raw: string): string {
  const dt = new Date(raw);
  if (Number.isNaN(dt.getTime())) return "--";
  return dt.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatHourRange(raw: string): string {
  const start = new Date(raw);
  if (Number.isNaN(start.getTime())) return "--";
  const end = new Date(start.getTime() + 60 * 60 * 1000);
  const startDate = start.toLocaleDateString(undefined, { month: "2-digit", day: "2-digit" });
  const endDate = end.toLocaleDateString(undefined, { month: "2-digit", day: "2-digit" });
  const startTime = start.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  const endTime = end.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });

  if (startDate === endDate) {
    return `${startDate} ${startTime} - ${endTime}`;
  }
  return `${startDate} ${startTime} - ${endDate} ${endTime}`;
}

function formatRelativeTime(raw: string): string {
  const dt = new Date(raw);
  if (Number.isNaN(dt.getTime())) return "--";
  const diffSeconds = Math.max(0, Math.floor((Date.now() - dt.getTime()) / 1000));
  if (diffSeconds < 60) return `${diffSeconds}s ago`;
  const diffMinutes = Math.floor(diffSeconds / 60);
  if (diffMinutes < 60) return `${diffMinutes}m ago`;
  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

function successToneClass(value: number | null | undefined): string {
  if (value == null) return "text-default-500";
  if (value >= 95) return "text-success";
  if (value >= 90) return "text-orange-500";
  return "text-warning";
}

function buildHealthRanking(metrics: DashboardMetricsResponse | null): HealthRankItem[] {
  if (!metrics) return [];
  const byWorker = new Map<string, HealthRankItem>();

  metrics.worker_rankings.least_reliable.forEach((item) => {
    byWorker.set(item.worker_id, {
      worker_id: item.worker_id,
      worker_name: item.worker_name,
      calls: item.calls,
      success_rate: item.success_rate,
      avg_duration_ms: null,
      failed: item.failed,
    });
  });

  metrics.worker_rankings.slowest.forEach((item) => {
    const existing = byWorker.get(item.worker_id);
    byWorker.set(item.worker_id, {
      worker_id: item.worker_id,
      worker_name: item.worker_name,
      calls: existing?.calls ?? item.calls,
      success_rate: existing?.success_rate ?? item.success_rate,
      avg_duration_ms: item.avg_duration_ms,
      failed: existing?.failed,
    });
  });

  return Array.from(byWorker.values())
    .sort((a, b) => {
      const leftRate = a.success_rate ?? 101;
      const rightRate = b.success_rate ?? 101;
      if (leftRate !== rightRate) return leftRate - rightRate;
      const leftDuration = a.avg_duration_ms ?? -1;
      const rightDuration = b.avg_duration_ms ?? -1;
      return rightDuration - leftDuration;
    })
    .slice(0, 5);
}

function chartTooltipLabel(label: ReactNode, payload?: readonly { payload?: { time?: string } }[]): string {
  const rawTime = payload?.[0]?.payload?.time;
  if (rawTime) {
    return `Time: ${formatHourRange(rawTime)}`;
  }
  return `Time: ${label ?? "--"}`;
}

function KpiSkeleton() {
  return (
    <Card className="min-h-[132px] border border-default-200/70 bg-background/70 shadow-[0_8px_30px_rgba(44,52,55,0.05)]">
      <Card.Content className="space-y-4 p-5">
        <Skeleton className="h-3 w-28 rounded-full" />
        <Skeleton className="h-8 w-24 rounded-lg" />
        <Skeleton className="h-4 w-full rounded-full" />
      </Card.Content>
    </Card>
  );
}

type KpiCardProps = {
  label: string;
  value: string;
  icon: string;
  toneClassName?: string;
  footer?: ReactNode;
};

function KpiCard({ label, value, icon, toneClassName = "text-default-900", footer }: KpiCardProps) {
  return (
    <Card className="h-[132px] max-h-[132px] border border-default-200/70 bg-background/70 shadow-[0_8px_30px_rgba(44,52,55,0.05)] backdrop-blur-xl">
      <Card.Content className="flex h-full flex-col justify-between gap-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-[10px] font-bold uppercase tracking-widest text-default-500">{label}</p>
            <p className={`mt-2 truncate text-3xl font-extrabold ${toneClassName}`}>{value}</p>
          </div>
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default-100 text-primary">
            <Icon icon={icon} width="20" height="20" />
          </div>
        </div>
        {footer ? <div className="min-w-0 text-xs text-default-500">{footer}</div> : null}
      </Card.Content>
    </Card>
  );
}

export default function Dashboard() {
  const [metrics, setMetrics] = useState<DashboardMetricsResponse | null>(null);
  const [trend, setTrend] = useState<DashboardTrendPoint[]>([]);
  const [workers, setWorkers] = useState<WorkerItem[]>([]);
  const [selectedWorkerId, setSelectedWorkerId] = useState(allWorkersValue);
  const [chartTab, setChartTab] = useState<"call" | "duration">("call");
  const [loading, setLoading] = useState(true);
  const [trendLoading, setTrendLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const loadTrend = useCallback(async (workerId: string) => {
    setTrendLoading(true);
    try {
      const data = await api.get<DashboardTrendPoint[]>(`/dashboard/workers/trend?worker_id=${encodeURIComponent(workerId)}`);
      setTrend(data || []);
    } finally {
      setTrendLoading(false);
    }
  }, []);

  const loadDashboard = useCallback(async (workerId: string, options?: { silent?: boolean }) => {
    if (!options?.silent) {
      setLoading(true);
    } else {
      setRefreshing(true);
    }
    setTrendLoading(true);
    try {
      const [metricsData, workersData, trendData] = await Promise.all([
        api.get<DashboardMetricsResponse>("/dashboard/metrics"),
        api.get<WorkerItem[]>("/workers?keyword="),
        api.get<DashboardTrendPoint[]>(`/dashboard/workers/trend?worker_id=${encodeURIComponent(workerId)}`),
      ]);
      setMetrics(metricsData);
      setWorkers(workersData || []);
      setTrend(trendData || []);
    } finally {
      setLoading(false);
      setTrendLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void loadDashboard(allWorkersValue);
  }, [loadDashboard]);

  const workerOptions = useMemo(() => [
    { label: "所有 Worker", value: allWorkersValue, description: "" },
    ...workers.map((worker) => ({
      label: worker.name,
      value: worker.id,
      description: worker.id,
    })),
  ], [workers]);

  const chartData = useMemo(() => trend.map((item) => ({
    ...item,
    hour: formatHour(item.time),
    successRate: item.success_rate,
    successRateLine: item.success_rate ?? 0,
    avgDuration: item.avg_duration_ms,
    avgDurationLine: item.avg_duration_ms ?? 0,
  })), [trend]);

  const healthRanking = useMemo(() => buildHealthRanking(metrics), [metrics]);
  const enabledRate = metrics && metrics.workers.total > 0
    ? Math.round((metrics.workers.enabled / metrics.workers.total) * 100)
    : 0;
  const failedWorkerNames = metrics?.last_failed_workers.map((item) => item.worker_name).join("、") || "";
  const hasCallTrendData = trend.some((item) => item.total > 0);
  const hasDurationTrendData = trend.some((item) => item.avg_duration_ms != null);

  const onWorkerChange = (workerId: string) => {
    setSelectedWorkerId(workerId || allWorkersValue);
    void loadTrend(workerId || allWorkersValue);
  };

  const onChartTabChange = (key: Key) => {
    setChartTab(String(key) === "duration" ? "duration" : "call");
  };

  const onRefresh = () => {
    void loadDashboard(selectedWorkerId, { silent: true });
  };

  return (
    <div className="pb-8">
      <header className="mb-8 flex flex-col justify-between gap-4 md:flex-row md:items-end">
        <div className="min-w-0">
          <h1 className="text-4xl font-extrabold tracking-tight text-default-900">Dashboard</h1>
          <p className="mt-2 text-sm font-medium text-default-500">System Worker Execution Health Overview</p>
        </div>
        <div className="flex max-w-full items-center gap-2 rounded-xl border border-default-200 bg-background/70 px-3 py-2 shadow-sm">
          <Icon icon="mingcute:time-duration-line" width="18" height="18" className="shrink-0 text-primary" />
          <span className="truncate text-[11px] font-semibold uppercase tracking-wider text-default-500">
            Last Update: {metrics ? formatClock(metrics.generated_at) : "--"}
          </span>
          <Button
            isIconOnly
            size="sm"
            variant="tertiary"
            isPending={refreshing}
            onPress={onRefresh}
          >
            <Icon icon="material-symbols:refresh-rounded" width="18" height="18" />
          </Button>
        </div>
      </header>

      <section className="mb-8 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
        {loading ? (
          <>
            <KpiSkeleton />
            <KpiSkeleton />
            <KpiSkeleton />
            <KpiSkeleton />
            <KpiSkeleton />
          </>
        ) : (
          <>
            <KpiCard
              label="Worker Overview"
              value={`${metrics?.workers.total ?? 0} Total`}
              icon="icon-park-outline:more-app"
              toneClassName={successToneClass(null)}
              footer={`${metrics?.workers.enabled ?? 0} Enabled`}
            />
            <KpiCard
              label="24h Success Rate"
              value={formatPercent(metrics?.summary.success_rate_24h)}
              icon="material-symbols:trending-up-rounded"
              toneClassName={successToneClass(metrics?.summary.success_rate_24h)}
              footer={`${formatNumber(metrics?.summary.total_calls_24h ?? 0)} calls · ${formatNumber(metrics?.summary.failed_calls_24h ?? 0)} failed`}
            />
            <KpiCard
              label="6h Success Rate"
              value={formatPercent(metrics?.summary.success_rate_6h)}
              icon="material-symbols:query-stats-rounded"
              toneClassName={successToneClass(metrics?.summary.success_rate_6h)}
              footer={`${formatNumber(metrics?.summary.total_calls_6h ?? 0)} calls · ${formatNumber(metrics?.summary.failed_calls_6h ?? 0)} failed`}
            />
            <KpiCard
              label="Avg Execution"
              value={formatDuration(metrics?.summary.avg_duration_ms_24h)}
              icon="material-symbols:speed-rounded"
              footer="24h average execution time"
            />
            <KpiCard
              label="Recently Failed"
              value={`${metrics?.summary.last_failed_workers_count ?? 0}`}
              icon="material-symbols:error-outline-rounded"
              toneClassName="text-orange-500"
              footer={
                <div className="truncate" title={failedWorkerNames || "暂无失败 Worker"}>
                  {failedWorkerNames || "暂无失败 Worker"}
                </div>
              }
            />
          </>
        )}
      </section>

      <Card className="mb-8 border border-default-200/70 bg-background/70 shadow-[0_8px_30px_rgba(44,52,55,0.05)] backdrop-blur-xl">
        <Card.Header className="flex flex-col items-start justify-between gap-4 p-5 md:flex-row md:items-center">
          <div>
            <Card.Title className="text-xl font-bold tracking-tight text-default-900">
              {chartTab === "call" ? "Call Success Rate Trend" : "Average Duration Trend"}
            </Card.Title>
            <Card.Description>Last 24 Hours · Hourly Aggregation</Card.Description>
          </div>
          <div className="flex items-center gap-2">
            <Tabs className="text-center"
              selectedKey={chartTab}
              onSelectionChange={onChartTabChange}
            >
              <Tabs.ListContainer>
                <Tabs.List
                  aria-label="Options"
                >
                  <Tabs.Tab id="call">
                    Call
                    <Tabs.Indicator />
                  </Tabs.Tab>
                  <Tabs.Tab id="duration">
                    Duration
                    <Tabs.Indicator />
                  </Tabs.Tab>
                </Tabs.List>
              </Tabs.ListContainer>
            </Tabs>
            <Select
              aria-label="选择 Worker"
              className="w-full md:w-[220px]"
              options={workerOptions}
              value={selectedWorkerId}
              onValueChange={onWorkerChange}
            />
          </div>
        </Card.Header>
        <Card.Content className="p-5 pt-0">
          <div className="h-[280px] min-h-[280px] min-w-0 rounded-xl border border-default-200 bg-default-50/60 p-3 dark:bg-default-100/10">
            {trendLoading ? (
              <div className="flex h-full items-center justify-center">
                <Skeleton className="h-full w-full rounded-xl" />
              </div>
            ) : chartTab === "call" ? (
              hasCallTrendData ? (
                <ResponsiveContainer
                  width="100%"
                  height="100%"
                  minWidth={0}
                  minHeight={240}
                  initialDimension={{ width: 1, height: 240 }}
                >
                  <ComposedChart data={chartData} margin={{ top: 8, right: 8, bottom: 0, left: -18 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                    <XAxis
                      dataKey="hour"
                      tick={{ fontSize: 11, fill: "var(--muted)" }}
                      tickLine={false}
                      axisLine={false}
                      minTickGap={18}
                    />
                    <YAxis
                      yAxisId="calls"
                      tick={{ fontSize: 11, fill: "var(--muted)" }}
                      tickLine={false}
                      axisLine={false}
                      allowDecimals={false}
                    />
                    <YAxis
                      yAxisId="rate"
                      orientation="right"
                      domain={[0, 100]}
                      tick={{ fontSize: 11, fill: "var(--muted)" }}
                      tickLine={false}
                      axisLine={false}
                      tickFormatter={(value) => `${value}%`}
                    />
                    <RechartsTooltip
                      labelFormatter={chartTooltipLabel}
                      formatter={(value, name) => {
                        if (name === "successRate") return [`${Number(value).toFixed(2)}%`, "Success Rate"];
                        if (name === "total") return [formatNumber(Number(value)), "Total Calls"];
                        if (name === "failed") return [formatNumber(Number(value)), "Failures"];
                        return [String(value), String(name)];
                      }}
                      labelStyle={{ color: "var(--foreground)" }}
                      itemStyle={{ color: "var(--foreground)" }}
                      contentStyle={{
                        fontSize: 14,
                        background: "var(--overlay)",
                        border: "1px solid var(--border)",
                        borderRadius: "10px",
                        color: "var(--foreground)",
                      }}
                    />
                    <Bar yAxisId="calls" dataKey="total" name="total" radius={[5, 5, 0, 0]} fill="var(--accent)" fillOpacity={0.18} />
                    <Bar yAxisId="calls" dataKey="failed" name="failed" radius={[5, 5, 0, 0]} barSize={8}>
                      {chartData.map((entry) => (
                        <Cell key={entry.time} fill={entry.failed > 0 ? "var(--danger)" : "transparent"} />
                      ))}
                    </Bar>
                    <Line
                      yAxisId="rate"
                      type="monotone"
                      dataKey="successRateLine"
                      name="successRate"
                      stroke="var(--accent)"
                      strokeWidth={3}
                      dot={{ r: 3, fill: "var(--accent)", strokeWidth: 0 }}
                      activeDot={{ r: 5 }}
                      connectNulls
                    />
                  </ComposedChart>
                </ResponsiveContainer>
              ) : (
                <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-default-500">
                  <Icon icon="material-symbols:monitoring-rounded" width="36" height="36" />
                  <p className="text-sm font-medium">最近 24 小时暂无调用数据</p>
                </div>
              )
            ) : hasDurationTrendData ? (
              <ResponsiveContainer
                width="100%"
                height="100%"
                minWidth={0}
                minHeight={240}
                initialDimension={{ width: 1, height: 240 }}
              >
                <ComposedChart data={chartData} margin={{ top: 8, right: 8, bottom: 0, left: -18 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                  <XAxis
                    dataKey="hour"
                    tick={{ fontSize: 11, fill: "var(--muted)" }}
                    tickLine={false}
                    axisLine={false}
                    minTickGap={18}
                  />
                  <YAxis
                    yAxisId="duration"
                    tick={{ fontSize: 11, fill: "var(--muted)" }}
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(value) => formatDurationAxis(Number(value))}
                  />
                  <RechartsTooltip
                    labelFormatter={chartTooltipLabel}
                    formatter={(value, name) => {
                      if (name === "avgDuration") return [formatDuration(Number(value)), "Avg Duration"];
                      return [String(value), String(name)];
                    }}
                    labelStyle={{ color: "var(--foreground)" }}
                    itemStyle={{ color: "var(--foreground)" }}
                    contentStyle={{
                      fontSize: 14,
                      background: "var(--overlay)",
                      border: "1px solid var(--border)",
                      borderRadius: "10px",
                      color: "var(--foreground)",
                    }}
                  />
                  <Line
                    yAxisId="duration"
                    type="monotone"
                    dataKey="avgDurationLine"
                    name="avgDuration"
                    stroke="var(--success)"
                    strokeWidth={3}
                    dot={{ r: 3, fill: "var(--success)", strokeWidth: 0 }}
                    activeDot={{ r: 5 }}
                    connectNulls
                  />
                </ComposedChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-default-500">
                <Icon icon="material-symbols:speed-rounded" width="36" height="36" />
                <p className="text-sm font-medium">最近 24 小时暂无耗时数据</p>
              </div>
            )}
          </div>
          <div className="mt-4 flex flex-col justify-between gap-3 text-[10px] font-bold uppercase tracking-widest text-default-500 md:flex-row">
            {chartTab === "call" ? (
              <div className="flex flex-wrap gap-5">
                <span className="flex items-center gap-2"><span className="h-3 w-3 rounded-sm bg-primary/20" /> Total Calls</span>
                <span className="flex items-center gap-2"><span className="h-0.5 w-4 bg-primary" /> Success Rate</span>
                <span className="flex items-center gap-2"><span className="h-2 w-2 rounded-full bg-danger" /> Failure Events</span>
              </div>
            ) : (
              <div className="flex flex-wrap gap-5">
                <span className="flex items-center gap-2"><span className="h-0.5 w-4 bg-success" /> Avg Duration</span>
              </div>
            )}
            <span>Worker: {workerOptions.find((item) => item.value === selectedWorkerId)?.label ?? "所有 Worker"}</span>
          </div>
        </Card.Content>
      </Card>

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
        <Card className="border border-default-200/70 bg-background/70 shadow-[0_8px_30px_rgba(44,52,55,0.05)] backdrop-blur-xl">
          <Card.Header className="p-3">
            <Card.Title className="text-lg font-bold tracking-tight text-default-900">Worker Health Ranking</Card.Title>
            <Card.Description>Last 24 Hours</Card.Description>
          </Card.Header>
          <Card.Content className="p-3 pt-0">
            <div className="grid grid-cols-[minmax(0,1.4fr)_0.6fr_0.7fr_0.7fr] gap-3 px-2 py-2 text-[10px] font-bold uppercase tracking-widest text-default-500">
              <span>Worker</span>
              <span className="text-right">Calls</span>
              <span className="text-right">Success</span>
              <span className="text-right">Avg</span>
            </div>
            <div className="space-y-1">
              {healthRanking.length === 0 ? (
                <div className="rounded-xl border border-default-200 px-3 py-6 text-center text-sm text-default-500">暂无排行数据</div>
              ) : healthRanking.map((item) => (
                <div key={item.worker_id} className="grid grid-cols-[minmax(0,1.4fr)_0.6fr_0.7fr_0.7fr] items-center gap-3 rounded-xl px-2 py-3 transition-colors hover:bg-default-100">
                  <span className="truncate font-mono text-sm font-semibold text-primary" title={item.worker_name}>{item.worker_name}</span>
                  <span className="text-right text-sm font-medium text-default-700">{formatNumber(item.calls)}</span>
                  <span className={`text-right text-sm font-bold ${successToneClass(item.success_rate)}`}>{formatPercent(item.success_rate)}</span>
                  <span className="text-right font-mono text-sm text-default-500">{formatDuration(item.avg_duration_ms)}</span>
                </div>
              ))}
            </div>
          </Card.Content>
        </Card>

        <Card className="border border-default-200/70 bg-background/70 shadow-[0_8px_30px_rgba(44,52,55,0.05)] backdrop-blur-xl">
          <Card.Header className="p-3">
            <Card.Title className="text-lg font-bold tracking-tight text-default-900">Current Failures</Card.Title>
            <Card.Description>Workers Whose Latest Call Result Is Failure</Card.Description>
          </Card.Header>
          <Card.Content className="p-3 pt-0">
            <div className="overflow-x-auto">
              <div className="min-w-[560px]">
                <div className="grid grid-cols-[1fr_0.45fr_0.65fr_1.2fr] gap-3 border-b border-default-200 px-2 pb-3 text-[10px] font-bold uppercase tracking-widest text-default-500">
                  <span>Worker</span>
                  <span className="text-center">Status</span>
                  <span>Last Failed</span>
                  <span>Error Summary</span>
                </div>
                {metrics?.last_failed_workers.length === 0 ? (
                  <div className="px-2 py-6 text-center text-sm text-default-500">暂无当前失败 Worker</div>
                ) : metrics?.last_failed_workers.map((item) => (
                  <div key={item.worker_id} className="grid grid-cols-[1fr_0.45fr_0.65fr_1.2fr] items-center gap-3 border-b border-default-100 px-2 py-4 last:border-b-0 hover:bg-default-100">
                    <span className="truncate font-mono text-sm font-semibold text-default-900" title={item.worker_name}>{item.worker_name}</span>
                    <span className="text-center text-sm font-bold text-danger">{item.last_status}</span>
                    <span className="text-xs text-default-500" title={formatDateTime(item.last_failed_at)}>{formatRelativeTime(item.last_failed_at)}</span>
                    <span className="truncate text-xs italic text-default-600" title={item.last_error || item.last_request_id}>
                      {item.last_error || item.last_request_id}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </Card.Content>
        </Card>
      </div>
    </div>
  );
}
