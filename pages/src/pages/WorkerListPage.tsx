import { FormEvent, KeyboardEvent, useEffect, useMemo, useState } from "react";
import { Chip, Label, SearchField, TextArea, toast } from "@heroui/react";
import { Button } from "@heroui/react";
import { useNavigate } from "react-router-dom";

import api from "@/lib/api";
import XModal from "@/components/modal";
import WorkerCronList from "@/components/worker-cron-list";
import { Input, Select, Switch } from "@/components/heroui";

type WorkerItem = {
  id: string;
  name: string;
  description: string;
  runtime: "python" | "node";
  route: string;
  timeout_ms: number;
  env: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type CreateWorkerForm = {
  name: string;
  description: string;
  runtime: "python" | "node";
  route: string;
  timeoutMS: string;
  env: string;
  enabled: boolean;
};

type ModalMode = "create" | "edit";

const initialForm: CreateWorkerForm = {
  name: "",
  description: "",
  runtime: "python",
  route: "",
  timeoutMS: "5000",
  env: "",
  enabled: true,
};

function isValidWorkerRoute(route: string): boolean {
  if (!route.startsWith("/")) {
    return false;
  }
  if (!route.includes("*")) {
    return true;
  }
  const first = route.indexOf("*");
  const last = route.lastIndexOf("*");
  return first === last && route.endsWith("/*");
}

export default function WorkerListPage() {
  const navigate = useNavigate();
  const [isOpen, setIsOpen] = useState(false);
  const [modalMode, setModalMode] = useState<ModalMode>("create");
  const [editingID, setEditingID] = useState<string>("");
  const [listLoading, setListLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [actioningID, setActioningID] = useState<string>("");
  const [workers, setWorkers] = useState<WorkerItem[]>([]);
  const [form, setForm] = useState<CreateWorkerForm>(initialForm);
  const [cronModalOpen, setCronModalOpen] = useState(false);
  const [cronWorker, setCronWorker] = useState<WorkerItem | null>(null);
  const [keyword, setKeyword] = useState("");

  const empty = useMemo(() => !listLoading && workers.length === 0, [listLoading, workers.length]);

  const normalizeEnvInput = (value: string): string => {
    return value
      .split(/[\n;]/)
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .join(";");
  };

  const loadWorkers = async (searchKeyword: string = keyword) => {
    setListLoading(true);
    try {
      const params = new URLSearchParams();
      params.set("keyword", searchKeyword);
      const data = await api.get<WorkerItem[]>(`/workers?${params.toString()}`);
      setWorkers(data);
    } finally {
      setListLoading(false);
    }
  };

  useEffect(() => {
    void loadWorkers("");
  }, []);

  const submitForm = async () => {
    if (creating) return;
    const timeout = Number(form.timeoutMS);

    if (!form.name.trim()) {
      toast.warning("name 不能为空");
      return;
    }
    if ([...form.description].length > 200) {
      toast.warning("description 不能超过 200 字符");
      return;
    }
    if (!form.route.trim().startsWith("/")) {
      toast.warning("route 必须以 / 开头");
      return;
    }
    if (!isValidWorkerRoute(form.route.trim())) {
      toast.warning("route 使用通配符时只支持结尾 /* 形式");
      return;
    }
    if (!Number.isFinite(timeout) || timeout <= 0) {
      toast.warning("timeout_ms 必须大于 0");
      return;
    }
    setCreating(true);
    try {
      if (modalMode === "create") {
        await api.post<WorkerItem>("/workers/create", {
          name: form.name.trim(),
          description: form.description.trim(),
          runtime: form.runtime,
          route: form.route.trim(),
          timeout_ms: timeout,
          env: normalizeEnvInput(form.env),
          enabled: form.enabled,
        });
      } else {
        await api.post<WorkerItem>("/workers/update", {
          id: editingID,
          name: form.name.trim(),
          description: form.description.trim(),
          route: form.route.trim(),
          timeout_ms: timeout,
          env: normalizeEnvInput(form.env),
          enabled: form.enabled,
        });
      }
      setForm(initialForm);
      setModalMode("create");
      setEditingID("");
      setIsOpen(false);
      await loadWorkers();
    } finally {
      setCreating(false);
    }
  };

  const openCreateModal = () => {
    setModalMode("create");
    setEditingID("");
    setForm(initialForm);
    setIsOpen(true);
  };

  const openEditModal = (item: WorkerItem) => {
    setModalMode("edit");
    setEditingID(item.id);
    setForm({
      name: item.name,
      description: item.description || "",
      runtime: item.runtime,
      route: item.route,
      timeoutMS: String(item.timeout_ms),
      env: item.env || "",
      enabled: item.enabled,
    });
    setIsOpen(true);
  };

  const openCronModal = (item: WorkerItem) => {
    setCronWorker(item);
    setCronModalOpen(true);
  };

  const setEnabled = async (item: WorkerItem, enabled: boolean) => {
    if (actioningID) return;
    setActioningID(item.id);
    try {
      await api.post<WorkerItem>(`/workers/${enabled ? "enable" : "disable"}`, { id: item.id });
      await loadWorkers();
    } finally {
      setActioningID("");
    }
  };

  const removeWorker = async (item: WorkerItem) => {
    if (actioningID) return;
    const ok = window.confirm(`确认删除 Worker ${item.name} ？`);
    if (!ok) return;
    setActioningID(item.id);
    try {
      await api.post<{ ok: boolean }>("/workers/delete", { id: item.id });
      await loadWorkers();
    } finally {
      setActioningID("");
    }
  };

  const onCreateSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void submitForm();
  };

  const onCardKeyDown = (event: KeyboardEvent<HTMLDivElement>, workerID: string) => {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    navigate(`/workers/${workerID}`);
  };

  const onKeywordKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter") {
      return;
    }
    event.preventDefault();
    void loadWorkers(keyword.trim());
  };

  return (
    <section className="pb-4 md:pb-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-semibold text-default-900 dark:text-default-700">Worker 列表</h1>
          <p className="text-sm text-default-500">共 {workers.length} 个 Worker</p>
        </div>
        <div className="flex items-center gap-2">
          <SearchField name="search" 
            aria-label="Search workers"
            onChange={setKeyword} onKeyDown={onKeywordKeyDown}
            onClear={() => {
              setKeyword("");
              loadWorkers("");
             }}
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input 
                className="w-[120px]" placeholder="Search..."
                value={keyword}
              />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
          <Button variant="primary" onPress={openCreateModal}>新增 Worker</Button>
        </div>
      </div>

      <div className="mt-4 flex flex-col gap-3">
        {listLoading && <div className="text-sm text-default-500">正在加载 Worker 列表...</div>}
        {empty && (
          <div className="rounded-xl border border-default-200 p-5 text-sm text-default-500">暂无 Worker，点击右上角新增。</div>
        )}
        {!listLoading && workers.map((item) => (
          <div
            key={item.id}
            className="w-full rounded-xl border border-default-200 p-4 text-left hover:border-accent transition-colors"
            role="button"
            tabIndex={0}
            onClick={() => navigate(`${window.__BASE_PREFIX__}/workers/${item.id}`)}
            onKeyDown={(event) => onCardKeyDown(event, item.id)}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="flex flex-col gap-2">
                <div className="flex items-center gap-2">
                  <p className="text-lg font-medium text-default-900">{item.name}</p>
                  <Chip color={item.enabled ? "success" : "default"} size="sm" variant="soft">
                    {item.enabled ? "已启用" : "已停用"}
                  </Chip>
                </div>
                <p className="text-sm text-default-600">{item.route}</p>
                <p className="text-xs text-default-500">{item.runtime} · {item.timeout_ms}ms</p>
                <p className="text-xs text-default-400">UUID: {item.id}</p>
              </div>
              <div className="flex items-center gap-2" onClick={(event) => event.stopPropagation()}>
                <Button
                  size="sm"
                  variant="tertiary"
                  className={"text-xs text-purple-600 dark:text-purple-400"}
                  onPress={() => openCronModal(item)}
                >
                  Cron
                </Button>
                <Button
                  isPending={actioningID === item.id}
                  size="sm"
                  variant="secondary"
                  className={"text-xs"}
                  onPress={() => openEditModal(item)}
                >
                  编辑
                </Button>
                {item.enabled ? (
                  <Button
                    isPending={actioningID === item.id}
                    size="sm"
                    variant="tertiary"
                    className="text-xs text-warning"
                    onPress={() => setEnabled(item, false)}
                  >
                    停用
                  </Button>
                ) : (
                  <Button
                    isPending={actioningID === item.id}
                    size="sm"
                    variant="tertiary"
                    className={"text-xs"}
                    onPress={() => setEnabled(item, true)}
                  >
                    启用
                  </Button>
                )}
                <Button
                  isPending={actioningID === item.id}
                  size="sm"
                  variant="danger-soft"
                  className={"text-xs"}
                  onPress={() => removeWorker(item)}
                >
                  删除
                </Button>
              </div>
            </div>
          </div>
        ))}
      </div>

      <XModal
        header={modalMode === "create" ? "创建 Worker" : "编辑 Worker"}
        isDismissable={!creating}
        isKeyboardDismissDisabled={creating}
        isOpen={isOpen}
        scrollBehavior="inside"
        submitText={creating ? (modalMode === "create" ? "创建中..." : "保存中...") : (modalMode === "create" ? "创建" : "保存")}
        classNames={{
          body: "px-[4px]"
        }}
        size="lg"
        onOpenChange={(open) => setIsOpen(open)}
        onSubmit={() => {
          void submitForm();
        }}
      >
        <form className="flex flex-col gap-3" onSubmit={onCreateSubmit}>
          <Input
            isRequired
            label="Worker 名"
            name="worker-name"
            value={form.name}
            onValueChange={(value) => setForm((prev) => ({ ...prev, name: value }))}
          />
          <Input
            description={`${[...form.description].length}/200`}
            label="描述"
            maxLength={200}
            name="worker-description"
            value={form.description}
            onValueChange={(value) => setForm((prev) => ({ ...prev, description: value }))}
          />
          <Input
            isRequired
            label="路由"
            name="worker-route"
            value={form.route}
            onValueChange={(value) => setForm((prev) => ({ ...prev, route: value }))}
          />
          <Select
            className="w-full"
            isDisabled={modalMode === "edit"}
            label="Runtime"
            options={[
              { label: "python", value: "python" },
              { label: "node", value: "node" },
            ]}
            value={form.runtime}
            onValueChange={(value) => {
              if (value === "python" || value === "node") {
                setForm((prev) => ({ ...prev, runtime: value }));
              }
            }}
          />
          <Input
            isRequired
            label="超时(ms)"
            name="worker-timeout"
            type="number"
            value={form.timeoutMS}
            onValueChange={(value) => setForm((prev) => ({ ...prev, timeoutMS: value }))}
          />
          <div className="flex flex-col gap-1">
            <Label htmlFor="worker-env">环境变量</Label>
            <TextArea
              id="worker-env"
              className="w-full"
              rows={3}
              placeholder="KEY=value;KEY2=value2;..."
              value={form.env}
              onChange={(event) => setForm((prev) => ({ ...prev, env: event.target.value }))}
            />
          </div>
          <Switch
            label="启用 Worker"
            value={form.enabled}
            onValueChange={(value) => setForm((prev) => ({ ...prev, enabled: value }))}
          />
          <button className="hidden" type="submit">创建</button>
        </form>
      </XModal>

      <XModal
        header={cronWorker ? `${cronWorker.name} 的 Cron` : "Cron"}
        isDismissable={false}
        isKeyboardDismissDisabled
        isOpen={cronModalOpen}
        scrollBehavior="inside"
        size="sm"
        // classNames={{
        //   body: "mx-[30px]"
        // }}
        onOpenChange={(open) => {
          setCronModalOpen(open);
          if (!open) {
            setCronWorker(null);
          }
        }}
      >
        {cronWorker ? <WorkerCronList isOpen={cronModalOpen} workerId={cronWorker.id} /> : null}
      </XModal>
    </section>
  );
}
