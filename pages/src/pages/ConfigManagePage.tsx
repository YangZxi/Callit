import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, Label, Spinner, toast } from "@heroui/react";
import { Button } from "@heroui/react";

import api from "@/lib/api";
import { Input, Switch } from "@/components/heroui";
import { useTranslation } from "react-i18next";

type AppConfigKey =
  | "MCP_ENABLE"
  | "MCP_TOKEN"
  | "WORKER_LOG_RETENTION_DAYS";

type AdminConfigItem = {
  key: string;
  value: string;
  source: "db" | "env" | "default";
  db?: string;
};

type ConfigSource = {
  source: AdminConfigItem["source"];
  db?: string;
};

type ConfigValues = Record<AppConfigKey, string>;

const defaultValues: ConfigValues = {
  MCP_ENABLE: "false",
  MCP_TOKEN: "",
  WORKER_LOG_RETENTION_DAYS: "30",
};

const fields: Array<{
  key: AppConfigKey;
  label: string;
  labelKey?: string;
  placeholder: string;
  type?: "text" | "password" | "number" | "switch";
  min?: number;
  descriptionKey?: string;
}> = [
  { key: "MCP_ENABLE", label: "MCP Enable", placeholder: "false", type: "switch" },
  { key: "MCP_TOKEN", label: "MCP Token", placeholder: "mcp-token", type: "text" },
  {
    key: "WORKER_LOG_RETENTION_DAYS",
    label: "Worker Log Retention Days",
    labelKey: "config.workerLogRetentionDays",
    placeholder: "30",
    type: "number",
    min: 1,
    descriptionKey: "config.workerLogRetentionDescription",
  },
];

function labelOfSource(source: AdminConfigItem["source"], t: (key: string) => string) {
  switch (source) {
    case "db":
      return t("config.db");
    case "env":
      return t("config.env");
    default:
      return t("config.default");
  }
}

export default function ConfigManagePage() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [values, setValues] = useState<ConfigValues>(defaultValues);
  const [sources, setSources] = useState<Partial<Record<AppConfigKey, ConfigSource>>>({});
  const [submitAttempted, setSubmitAttempted] = useState(false);

  const intErrors = useMemo(() => {
    const errors: Partial<Record<AppConfigKey, string>> = {};
    const retentionDays = values.WORKER_LOG_RETENTION_DAYS.trim();
    if (!/^\d+$/.test(retentionDays) || Number(retentionDays) <= 0) {
      errors.WORKER_LOG_RETENTION_DAYS = t("config.positiveInteger");
    }
    return errors;
  }, [t, values.WORKER_LOG_RETENTION_DAYS]);

  const sourceHint = (key: AppConfigKey) => {
    const meta = sources[key];
    if (!meta) return t("config.sourceUnknown");
    return `${t("config.sourceUnknown").replace("未知", "")}${labelOfSource(meta.source, t)}`;
  };

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.get<AdminConfigItem[]>("/config");
      const next: ConfigValues = { ...defaultValues };
      const nextSources: Partial<Record<AppConfigKey, ConfigSource>> = {};
      for (const it of res || []) {
        if (!(it.key in next)) continue;
        const key = it.key as AppConfigKey;
        next[key] = it.value ?? "";
        nextSources[key] = {
          source: it.source,
          db: it.db,
        };
      }
      setValues(next);
      setSources(nextSources);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    setSubmitAttempted(true);
    if (Object.keys(intErrors).length > 0) return;

    setSaving(true);
    try {
      await api.post("/config", { app_config: values });
      toast.success(t("config.saveSuccess"));
      await load();
    } finally {
      setSaving(false);
    }
  };

  const renderFormField = (field: typeof fields[0], invalid: boolean) => {
    const label = field.labelKey ? t(field.labelKey) : field.label;
    if (field.type === "switch") {
      return (
        <div className="flex flex-col gap-1">
          <Label>{label}</Label>
          <Switch
            value={values[field.key] === "true"}
            onValueChange={(value) => {
              setValues((prev) => ({ ...prev, [field.key]: value ? "true" : "false" }));
            }}
          />
        </div>
      );
    } else {
      return <Input
        key={field.key}
        // description={sourceHint(field.key)}
        description={field.descriptionKey ? t(field.descriptionKey) : undefined}
        errorMessage={invalid ? intErrors[field.key] : undefined}
        isInvalid={invalid}
        isRequired={field.type === "number"}
        label={label}
        min={field.min}
        name={field.key}
        placeholder={field.placeholder}
        type={field.type ?? "text"}
        value={values[field.key]}
        onValueChange={(value) => {
          setValues((prev) => ({ ...prev, [field.key]: value }));
        }}
      />
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-default-900">{t("config.title")}</h1>
          <p className="mt-1 text-sm text-default-500">{t("config.description")}</p>
        </div>
        <Button variant="primary" isDisabled={loading} isPending={saving} onPress={save}>
          {t("common.save")}
        </Button>
      </div>

      <Card className="border border-default-200/70 bg-background/70 shadow-sm">
        <Card.Header 
          style={{flexDirection: "unset"}} 
          className="flex items-center justify-between gap-4"
        >
          <div>
            <p className="text-base font-semibold">{t("config.appConfig")}</p>
          </div>
          <Button isDisabled={loading} size="sm" variant="tertiary" onPress={load}>
            {t("common.refresh")}
          </Button>
        </Card.Header>
        <Card.Content className="space-y-4">
          {loading ? (
            <div className="flex flex-col gap-2 items-center justify-center py-10">
              <Spinner />
              <span className="text-xs text-muted">{t("common.loading")}</span>
            </div>
          ) : (
            <div className="flex flex-col gap-4">
              {fields.map((field) => {
                const invalid = submitAttempted && !!intErrors[field.key];
                return (
                  renderFormField(field, invalid)
                );
              })}
            </div>
          )}
        </Card.Content>
      </Card>
    </div>
  );
}
