import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, Label, Spinner, TextArea, toast, Input } from "@heroui/react";
import { Icon } from "@iconify/react";
import { Button } from "@heroui/react";
import clsx from "clsx";

import api from "@/lib/api";

type KVMetrics = {
  connected_clients: number;
  used_memory: number;
  used_memory_human: string;
  kv_keys: number;
};

type KVKeyItem = {
  key: string;
  namespace: string;
  name: string;
};

type KVKeyDetail = KVKeyItem & {
  ttl: number;
  value: string;
  exists: boolean;
};

type KVKeyGroup = {
  namespace: string;
  items: KVKeyItem[];
};

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size = size / 1024;
    index++;
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function formatTTL(ttl: number) {
  if (ttl === 0) return "永久";
  if (ttl === -2) return "不存在";
  if (ttl < 0) return `${ttl}`;
  return `${ttl}s`;
}

export default function StorageKVPage() {
  const [metrics, setMetrics] = useState<KVMetrics | null>(null);
  const [keys, setKeys] = useState<KVKeyItem[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [detail, setDetail] = useState<KVKeyDetail | null>(null);
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const metricItems = useMemo(() => {
    return [
      { label: "连接数", value: metrics?.connected_clients ?? 0 },
      { label: "内存", value: metrics?.used_memory_human || formatBytes(metrics?.used_memory ?? 0) },
      { label: "KV Keys", value: metrics?.kv_keys ?? 0 },
    ];
  }, [metrics]);

  const groupedKeys = useMemo<KVKeyGroup[]>(() => {
    const groups = new Map<string, KVKeyItem[]>();
    for (const item of keys) {
      const namespace = item.namespace || "default";
      const items = groups.get(namespace) || [];
      items.push(item);
      groups.set(namespace, items);
    }

    return Array.from(groups.entries())
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([namespace, items]) => ({
        namespace,
        items: [...items].sort((left, right) => left.name.localeCompare(right.name)),
      }));
  }, [keys]);

  const loadMetrics = useCallback(async () => {
    const data = await api.get<KVMetrics>("/kv/metrics");
    setMetrics(data);
  }, []);

  const loadKeys = useCallback(async (search: string = "") => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (search.trim()) params.set("key", search.trim());
      const data = await api.get<KVKeyItem[]>(`/kv/keys?${params.toString()}`);
      setKeys(data || []);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadDetail = useCallback(async (key: string) => {
    if (!key) return;
    setSelectedKey(key);
    setDetailLoading(true);
    try {
      const params = new URLSearchParams({ key });
      const data = await api.get<KVKeyDetail>(`/kv/key?${params.toString()}`);
      setDetail(data);
    } finally {
      setDetailLoading(false);
    }
  }, []);

  const reloadAll = useCallback(async () => {
    await Promise.all([loadMetrics(), loadKeys("")]);
  }, [loadKeys, loadMetrics]);

  useEffect(() => {
    void reloadAll();
  }, [reloadAll]);

  const submitSearch = () => {
    void loadKeys(keyword);
  };

  const deleteCurrentKey = async () => {
    if (!detail) return;
    const ok = window.confirm(`确认删除 Redis Key ${detail.key} ？`);
    if (!ok) return;

    setDeleting(true);
    try {
      await api.post("/kv/key/delete", { key: detail.key });
      toast.success("删除成功");
      setSelectedKey("");
      setDetail(null);
      await reloadAll();
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-default-900">KV Storage</h1>
          <p className="mt-1 text-sm text-default-500">管理 Callit Worker SDK 写入的 Redis KV 数据。</p>
        </div>
        <Button isDisabled={loading || detailLoading} size="sm" variant="secondary" onPress={() => void reloadAll()}>
          <Icon icon="solar:refresh-linear" className="size-4" />
          刷新
        </Button>
      </div>

      <div className="grid gap-3 rounded-lg border border-default-200/70 bg-background/70 p-4 shadow-sm md:grid-cols-3">
        {metricItems.map((item) => (
          <div key={item.label} className="min-w-0">
            <p className="text-xs text-default-500">{item.label}</p>
            <p className="mt-1 truncate text-lg font-semibold text-default-900" title={String(item.value)}>
              {item.value}
            </p>
          </div>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
        <Card className="min-h-[560px] border border-default-200/70 bg-background/70 shadow-sm">
          <Card.Header className="flex flex-col items-stretch gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <Card.Title className="text-base">Key List</Card.Title>
                <Card.Description>{keys.length} 个 key</Card.Description>
              </div>
              <Button isDisabled={loading} isIconOnly size="sm" variant="tertiary" onPress={() => void loadKeys("")}>
                <Icon icon="solar:refresh-linear" className="size-4" />
              </Button>
            </div>
            <div className="flex gap-2 w-full">
              <Input
                className="h-8"
                name="keyword"
                placeholder="搜索 key"
                fullWidth={true}
                value={keyword}
                onKeyDown={(event) => {
                  if (event.key === "Enter") submitSearch();
                }}
                onChange={(e) => setKeyword(e.target.value)}
              />
              <Button className="w-[80px] shrink-0" size="sm" variant="primary" onPress={submitSearch}>
                <Icon icon="solar:magnifer-linear" className="size-4" />
                搜索
              </Button>
            </div>
          </Card.Header>
          <Card.Content className="p-0">
            {loading ? (
              <div className="flex flex-col items-center justify-center gap-2 py-16">
                <Spinner />
                <span className="text-xs text-default-500">加载中...</span>
              </div>
            ) : keys.length === 0 ? (
              <div className="px-4 py-10 text-center text-sm text-default-500">暂无 KV key</div>
            ) : (
              <div className="max-h-[470px] overflow-auto pb-3">
                {groupedKeys.map((group) => (
                  <div key={group.namespace} className="mb-3 last:mb-0">
                    <div
                      className="sticky top-0 z-10 mb-1 flex items-center justify-between gap-2 bg-background/95 px-1 py-2 backdrop-blur"
                    >
                      <span className="truncate text-xs font-semibold uppercase text-default-500">
                        {group.namespace}
                      </span>
                      <span className="shrink-0 text-xs text-default-400">{group.items.length}</span>
                    </div>
                    <ul className="space-y-1 pl-1">
                      {group.items.map((item) => (
                        <li
                          key={item.key}
                          className={clsx(
                            "w-full cursor-pointer rounded-md px-2 py-1 text-left text-default-700 transition-colors",
                            "hover:bg-[var(--color-default-hover)] hover:text-default-900",
                            "rounded-xl",
                            selectedKey === item.key && "bg-primary/10 text-primary bg-[var(--color-default-hover)]",
                          )}
                          onClick={() => void loadDetail(item.key)}
                        >
                          <span className="block truncate text-sm font-medium">{item.name}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </Card.Content>
        </Card>

        <Card className="min-h-[560px] border border-default-200/70 bg-background/70 shadow-sm">
          <Card.Header className="flex !flex-row items-start justify-between gap-4">
            <div className="min-w-0">
              <Card.Title className="text-base">{detail?.key || "请选择左侧 key"}</Card.Title>
            </div>
            <Button
              className="shrink-0"
              isDisabled={!detail || detailLoading}
              isPending={deleting}
              size="sm"
              variant="danger-soft"
              onPress={deleteCurrentKey}
            >
              <Icon icon="solar:trash-bin-trash-linear" className="size-4" />
              删除
            </Button>
          </Card.Header>
          <Card.Content>
            {detailLoading ? (
              <div className="flex flex-col items-center justify-center gap-2 py-20">
                <Spinner />
                <span className="text-xs text-default-500">加载详情...</span>
              </div>
            ) : !detail ? (
              <div className="py-20 text-center text-sm text-default-500">未选择 key</div>
            ) : (
              <div className="space-y-5">
                <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_80px]">
                  <div className="min-w-0 rounded-md border border-default-200/70 p-3">
                    <p className="text-xs text-default-500">Namespace</p>
                    <p className="mt-1 truncate text-sm font-medium">{detail.namespace}</p>
                  </div>
                  <div className="min-w-0 rounded-md border border-default-200/70 p-3 md:w-[80px]">
                    <p className="text-xs text-default-500">TTL</p>
                    <p className="mt-1 truncate text-sm font-medium">{formatTTL(detail.ttl)}</p>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="kv-value">Value</Label>
                  <TextArea
                    readOnly
                    className="min-h-[360px] w-full font-mono text-sm"
                    id="kv-value"
                    value={detail.value}
                    variant="secondary"
                  />
                </div>
              </div>
            )}
          </Card.Content>
        </Card>
      </div>
    </div>
  );
}
