import { FormEvent, KeyboardEvent, useEffect, useMemo, useState } from "react";
import { Button, Chip, Input, Modal, ModalBody, ModalContent, ModalHeader, Select, SelectItem, Switch, addToast } from "@heroui/react";
import { useNavigate } from "react-router-dom";

import api from "@/lib/api";
import XModal from "@/components/modal";
import WorkerCronList from "@/components/worker-cron-list";

type WorkerItem = {
  id: string;
  name: string;
  runtime: "python" | "node";
  route: string;
  timeout_ms: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type CreateWorkerForm = {
  name: string;
  runtime: "python" | "node";
  route: string;
  timeoutMS: string;
  enabled: boolean;
};

type ModalMode = "create" | "edit";

const initialForm: CreateWorkerForm = {
  name: "",
  runtime: "python",
  route: "",
  timeoutMS: "5000",
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
      addToast({ title: "name 不能为空", color: "warning", variant: "flat", timeout: 2500 });
      return;
    }
    if (!form.route.trim().startsWith("/")) {
      addToast({ title: "route 必须以 / 开头", color: "warning", variant: "flat", timeout: 2500 });
      return;
    }
    if (!isValidWorkerRoute(form.route.trim())) {
      addToast({ title: "route 使用通配符时只支持结尾 /* 形式", color: "warning", variant: "flat", timeout: 2500 });
      return;
    }
    if (!Number.isFinite(timeout) || timeout <= 0) {
      addToast({ title: "timeout_ms 必须大于 0", color: "warning", variant: "flat", timeout: 2500 });
      return;
    }
    setCreating(true);
    try {
      if (modalMode === "create") {
        await api.post<WorkerItem>("/workers/create", {
          name: form.name.trim(),
          runtime: form.runtime,
          route: form.route.trim(),
          timeout_ms: timeout,
          enabled: form.enabled,
        });
      } else {
        await api.post<WorkerItem>("/workers/update", {
          id: editingID,
          name: form.name.trim(),
          runtime: form.runtime,
          route: form.route.trim(),
          timeout_ms: timeout,
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
      runtime: item.runtime,
      route: item.route,
      timeoutMS: String(item.timeout_ms),
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
    <section className="py-2 md:py-6 pb-4 md:pb-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-semibold text-default-900 dark:text-default-700">Worker 列表</h1>
          <p className="text-sm text-default-500">共 {workers.length} 个 Worker</p>
        </div>
        <div className="flex items-center gap-2">
          <Input
            className="w-40"
            placeholder=""
            size="sm"
            value={keyword}
            onKeyDown={onKeywordKeyDown}
            onValueChange={setKeyword}
          />
          <Button color="primary" onPress={openCreateModal}>新增 Worker</Button>
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
            className="w-full rounded-xl border border-default-200 p-4 text-left hover:border-primary transition-colors"
            role="button"
            tabIndex={0}
            onClick={() => navigate(`${window.__BASE_PREFIX__}/workers/${item.id}`)}
            onKeyDown={(event) => onCardKeyDown(event, item.id)}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="flex flex-col gap-2">
                <div className="flex items-center gap-2">
                  <p className="text-lg font-medium text-default-900">{item.name}</p>
                  <Chip color={item.enabled ? "success" : "default"} size="sm" variant="flat">
                    {item.enabled ? "已启用" : "已停用"}
                  </Chip>
                </div>
                <p className="text-sm text-default-600">{item.route}</p>
                <p className="text-xs text-default-500">{item.runtime} · {item.timeout_ms}ms</p>
                <p className="text-xs text-default-400">UUID: {item.id}</p>
              </div>
              <div className="flex items-center gap-2" onClick={(event) => event.stopPropagation()}>
                <Button
                  color="secondary"
                  size="sm"
                  variant="flat"
                  onPress={() => openCronModal(item)}
                >
                  Cron
                </Button>
                <Button
                  color="primary"
                  isLoading={actioningID === item.id}
                  size="sm"
                  variant="flat"
                  onPress={() => openEditModal(item)}
                >
                  编辑
                </Button>
                {item.enabled ? (
                  <Button
                    color="warning"
                    isLoading={actioningID === item.id}
                    size="sm"
                    variant="flat"
                    onPress={() => setEnabled(item, false)}
                  >
                    停用
                  </Button>
                ) : (
                  <Button
                    color="success"
                    isLoading={actioningID === item.id}
                    size="sm"
                    variant="flat"
                    onPress={() => setEnabled(item, true)}
                  >
                    启用
                  </Button>
                )}
                <Button
                  color="danger"
                  isLoading={actioningID === item.id}
                  size="sm"
                  variant="flat"
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
        onOpenChange={(open) => setIsOpen(open)}
        onSubmit={() => {
          void submitForm();
        }}
      >
        <form className="flex flex-col gap-3" onSubmit={onCreateSubmit}>
          <Input isRequired label="Worker 名" value={form.name} onValueChange={(value) => setForm((prev) => ({ ...prev, name: value }))} />
          <Input isRequired label="路由" value={form.route} onValueChange={(value) => setForm((prev) => ({ ...prev, route: value }))} />
          <Select
            isDisabled={modalMode === "edit"}
            label="Runtime"
            selectedKeys={[form.runtime]}
            onSelectionChange={(keys) => {
              if (modalMode === "edit") return;
              if (keys === "all") return;
              const first = Array.from(keys)[0];
              if (first === "python" || first === "node") {
                setForm((prev) => ({ ...prev, runtime: first }));
              }
            }}
          >
            <SelectItem key="python">python</SelectItem>
            <SelectItem key="node">node</SelectItem>
          </Select>
          <Input
            isRequired
            label="超时(ms)"
            type="number"
            value={form.timeoutMS}
            onValueChange={(value) => setForm((prev) => ({ ...prev, timeoutMS: value }))}
          />
          <Switch 
            isSelected={form.enabled} 
            onValueChange={(value) => setForm((prev) => ({ ...prev, enabled: value }))}
          >
			      启用 Worker
          </Switch>
          <button className="hidden" type="submit">创建</button>
        </form>
      </XModal>

      <Modal
        hideCloseButton={false}
        isDismissable={false}
        isKeyboardDismissDisabled
        isOpen={cronModalOpen}
        scrollBehavior="inside"
        size="sm"
        onOpenChange={(open) => {
          if (!open) {
            setCronModalOpen(false);
            setCronWorker(null);
          }
        }}
      >
        <ModalContent>
          {() => (
            <>
              <ModalHeader className="flex flex-col gap-1 text-default-900 dark:text-default-700">
                {cronWorker ? `${cronWorker.name} 的 Cron` : "Cron"}
              </ModalHeader>
              <ModalBody className="pb-5">
                {cronWorker ? <WorkerCronList isOpen={cronModalOpen} workerId={cronWorker.id} /> : null}
              </ModalBody>
            </>
          )}
        </ModalContent>
      </Modal>
    </section>
  );
}
