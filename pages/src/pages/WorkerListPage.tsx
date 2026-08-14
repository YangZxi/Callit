import { FormEvent, KeyboardEvent, useEffect, useMemo, useState } from "react";
import { Chip, Label, SearchField, TextArea, toast } from "@heroui/react";
import { Button } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useNavigate } from "react-router-dom";

import api from "@/lib/api";
import XModal from "@/components/modal";
import WorkerCronList from "@/components/worker-cron-list";
import { Input, Select, Switch } from "@/components/heroui";
import { copyText } from "@/lib/utils";
import { useTranslation } from "react-i18next";

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

function formatMillisecondsToSeconds(ms: number): string {
  const seconds = Math.round(ms / 100) / 10;
  return Number.isInteger(seconds) ? String(seconds) : seconds.toFixed(1);
}

function isValidWorkerRoute(route: string): boolean {
  if (!route.startsWith("/")) {
    return false;
  }
  if (route === "/*") {
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
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [isOpen, setIsOpen] = useState(false);
  const [modalMode, setModalMode] = useState<ModalMode>("create");
  const [editingID, setEditingID] = useState<string>("");
  const [listLoading, setListLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [actioningID, setActioningID] = useState<string>("");
  const [workers, setWorkers] = useState<WorkerItem[]>([]);
  const [form, setForm] = useState<CreateWorkerForm>(initialForm);
  const [timeoutSeconds, setTimeoutSeconds] = useState(() => formatMillisecondsToSeconds(Number(initialForm.timeoutMS)));
  const [cronModalOpen, setCronModalOpen] = useState(false);
  const [cronWorker, setCronWorker] = useState<WorkerItem | null>(null);
  const [keyword, setKeyword] = useState("");

  const empty = useMemo(() => !listLoading && workers.length === 0, [listLoading, workers.length]);

  const normalizeEnvInput = (value: string): string => {
    return value
      .split(/\r\n|\n|\r/)
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .join("\n");
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

  const updateTimeoutSeconds = (value: string) => {
    setTimeoutSeconds(value);
    const timeoutSValue = value.trim();
    if (!/^\d+(?:\.\d)?$/.test(timeoutSValue)) {
      return;
    }

    const seconds = Number(timeoutSValue);
    if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 300) {
      return;
    }

    setForm((prev) => ({
      ...prev,
      timeoutMS: String(Math.round(seconds * 1000)),
    }));
  };

  const submitForm = async () => {
    if (creating) return;
    const timeoutMS = Number(form.timeoutMS);
    const timeoutSeconds = timeoutMS / 1000;

    if (!form.name.trim()) {
      toast.warning(t("workers.nameRequired"));
      return;
    }
    if ([...form.description].length > 200) {
      toast.warning(t("workers.descriptionTooLong"));
      return;
    }
    if (!form.route.trim().startsWith("/")) {
      toast.warning(t("workers.routeRequired"));
      return;
    }
    if (form.route.trim() === "/*") {
      toast.warning(t("workers.routeRoot"));
      return;
    }
    if (!isValidWorkerRoute(form.route.trim())) {
      toast.warning(t("workers.routeWildcard"));
      return;
    }
    if (!Number.isFinite(timeoutMS) || timeoutMS <= 0 || timeoutSeconds > 300) {
      toast.warning(t("workers.timeoutInvalid"));
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
          timeout_ms: timeoutMS,
          env: normalizeEnvInput(form.env),
          enabled: form.enabled,
        });
      } else {
        await api.post<WorkerItem>("/workers/update", {
          id: editingID,
          name: form.name.trim(),
          description: form.description.trim(),
          route: form.route.trim(),
          timeout_ms: timeoutMS,
          env: normalizeEnvInput(form.env),
          enabled: form.enabled,
        });
      }
      setForm(initialForm);
      setTimeoutSeconds(formatMillisecondsToSeconds(Number(initialForm.timeoutMS)));
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
    setTimeoutSeconds(formatMillisecondsToSeconds(Number(initialForm.timeoutMS)));
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
    setTimeoutSeconds(formatMillisecondsToSeconds(item.timeout_ms));
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
    const ok = window.confirm(t("workers.deleteConfirm", { name: item.name }));
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
    <div className="pb-4 md:pb-2">
      <header className="flex flex-col md:flex-row md:items-end justify-between mb-6 gap-6">
        <div>
          <div className="flex items-center gap-3 mb-2">
            <h1 className="text-4xl font-extrabold font-headline tracking-tight text-[#2c3437] dark:text-white">{t("workers.title")}</h1>
          </div>
          <p className="text-[#596064] dark:text-gray-400 font-medium">{t("workers.count", { count: workers.length })}</p>
        </div>
        <div className="flex items-center gap-4">
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
          <Button
            variant="primary"
            onPress={openCreateModal}
          >
            {t("workers.add")}
          </Button>
        </div>
      </header>

      <div className="space-y-2 mb-6">
        {listLoading && <div className="text-sm text-default-500">{t("common.loading")}</div>}
        {empty && (
          <div className="bg-white/50 backdrop-blur-md rounded-2xl p-12 text-center border border-white/40">
            <p className="text-[#596064]">{t("workers.noWorkersHint")}</p>
          </div>
        )}
        {!listLoading && workers.map((item) => (
          <div
            key={item.id}
            className="group bg-white/30 dark:bg-white/5 backdrop-blur-md rounded-2xl p-4 shadow-[0_4px_24px_0_rgba(0,0,0,0.02)] hover:shadow-[0_8px_32px_0_rgba(0,0,0,0.06)] transition-all duration-500 flex flex-col lg:flex-row lg:items-center justify-between gap-6 border border-white/40 dark:border-white/10 relative cursor-pointer"
            role="button"
            tabIndex={0}
            onClick={() => navigate(`${window.__BASE_PREFIX__}/workers/${item.id}`)}
            onKeyDown={(event) => onCardKeyDown(event, item.id)}
          >
            <div className="flex-1">
              <div className="flex flex-col mb-2">
                <div className="flex items-center gap-3">
                  <h3 className="text-xl font-bold leading-tight text-[#2c3437] dark:text-white">{item.name}</h3>
                  <Chip
                    color={item.enabled ? "success" : "default"}
                    size="sm" variant="soft"
                  >
                    {item.enabled ? t("common.enabled") : t("common.disabled")}
                  </Chip>
                </div>
                <p className="text-[11px] font-mono text-[#acb3b7] mt-1">ID: {item.id}</p>
              </div>
              <div className="grid grid-cols-2 md:grid-cols-6 gap-6">
                <div className="space-y-1">
                  <p className="text-[10px] font-bold text-[#acb3b7] uppercase tracking-widest text-nowrap">Runtime</p>
                  <div className="flex items-center gap-1 text-[#2c3437] dark:text-gray-200">
                    <Icon icon="ic:baseline-terminal" width="22" height="22" className="text-[#0055d7]" />
                    <span className="font-mono text-sm font-medium leading-[22px] capitalize">{item.runtime}</span>
                  </div>
                </div>
                <div className="space-y-1">
                  <p className="text-[10px] font-bold text-[#acb3b7] uppercase tracking-widest text-nowrap">Timeout</p>
                  <div className="flex items-center gap-1 text-[#2c3437] dark:text-gray-200">
                    <Icon icon="mingcute:time-duration-line" width="22" height="22" className="text-[#0055d7]" />
                    <span className="font-mono text-sm font-medium leading-[22px]">{item.timeout_ms}ms</span>
                  </div>
                </div>
                <div className="space-y-1 md:col-span-2">
                  <p className="text-[10px] font-bold text-[#acb3b7] uppercase tracking-widest text-nowrap">Route</p>
                  <div
                    className="flex items-center gap-1 text-[#2c3437] dark:text-gray-200 cursor-pointer hover:text-[#0055d7] transition-colors max-w-fit"
                    title={item.route}
                    onClick={(e) => {
                      e.stopPropagation();
                      let fullUrl = `${window.location.origin}${item.route}`;
                      if (fullUrl.endsWith("/*")) {
                        fullUrl = fullUrl.slice(0, -1);
                      }
                      void copyText(fullUrl);
                      toast.success(t("workers.copied"));
                    }}
                  >
                    <Icon icon="ic:baseline-link" width="22" height="22" className="text-[#0055d7]" />
                    <span className="font-mono text-sm font-medium leading-[22px] truncate max-w-[200px]">{item.route}</span>
                  </div>
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2" onClick={(event) => event.stopPropagation()}>
              <Button
                size="sm"
                variant="tertiary"
                className="text-xs text-purple-600 dark:text-purple-400"
                onPress={() => openCronModal(item)}
              >
                Cron
              </Button>
              <Button
                isPending={actioningID === item.id}
                size="sm"
                variant="secondary"
                className="text-xs"
                onPress={() => openEditModal(item)}
              >
                {t("workers.edit")}
              </Button>
              {item.enabled ? (
                <Button
                  isPending={actioningID === item.id}
                  size="sm"
                  variant="tertiary"
                  className="text-xs text-warning"
                  onPress={() => setEnabled(item, false)}
                >
                  {t("workers.disable")}
                </Button>
              ) : (
                <Button
                  isPending={actioningID === item.id}
                  size="sm"
                  variant="tertiary"
                  className="text-xs"
                  onPress={() => setEnabled(item, true)}
                >
                  {t("workers.enable")}
                </Button>
              )}
              <Button
                isPending={actioningID === item.id}
                size="sm"
                variant="danger-soft"
                className="text-xs"
                onPress={() => removeWorker(item)}
              >
                {t("common.delete")}
              </Button>
            </div>
          </div>
        ))}
      </div>

      <XModal
        header={modalMode === "create" ? t("workers.create") : t("workers.edit")}
        isDismissable={!creating}
        isKeyboardDismissDisabled={creating}
        isOpen={isOpen}
        scrollBehavior="inside"
        submitText={creating ? t("common.loading") : (modalMode === "create" ? t("workers.create") : t("common.save"))}
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
          <div className="flex items-start gap-4">
            <div className="min-w-0 flex-1">
              <Input
                isRequired
                className="w-full"
                label={t("workers.name")}
                name="worker-name"
                value={form.name}
                onValueChange={(value) => setForm((prev) => ({ ...prev, name: value }))}
              />
            </div>
            <div className="textfield w-[60px] shrink-0">
              <label className="label mb-2">{t("workers.enable")}</label>
              <Switch
                value={form.enabled}
                onValueChange={(value) => setForm((prev) => ({ ...prev, enabled: value }))}
              />
            </div>
          </div>
          <Input
            description={`${[...form.description].length}/200`}
            label={t("workers.description")}
            maxLength={200}
            name="worker-description"
            value={form.description}
            onValueChange={(value) => setForm((prev) => ({ ...prev, description: value }))}
          />
          <Input
            isRequired
            label={t("workers.route")}
            name="worker-route"
            value={form.route}
            onValueChange={(value) => setForm((prev) => ({ ...prev, route: value }))}
          />
          <div className="grid grid-cols-2 gap-2">
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
              label={t("workers.timeout")}
              name="worker-timeout"
              type="number"
              step="0.1"
              value={timeoutSeconds}
              onValueChange={updateTimeoutSeconds}
            />
          </div>
          <div className="flex flex-col gap-1 mb-1">
            <Label htmlFor="worker-env">{t("workers.environment")}</Label>
            <TextArea
              id="worker-env"
              className="w-full"
              rows={3}
              placeholder={[`KEY=value`, `KEY2=value2`, `...`].join("\n")}
              value={form.env}
              onChange={(event) => setForm((prev) => ({ ...prev, env: event.target.value }))}
            />
          </div>
          <button className="hidden" type="submit">{t("workers.create")}</button>
        </form>
      </XModal>

      <XModal
        header={cronWorker ? `${cronWorker.name} ${t("workers.cron")}` : "Cron"}
        isDismissable={false}
        isKeyboardDismissDisabled
        isOpen={cronModalOpen}
        scrollBehavior="inside"
        size="sm"
        onOpenChange={(open) => {
          setCronModalOpen(open);
          if (!open) {
            setCronWorker(null);
          }
        }}
      >
        {cronWorker ? <WorkerCronList isOpen={cronModalOpen} workerId={cronWorker.id} /> : null}
      </XModal>
    </div>
  );
}
