import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, Label, Spinner, toast } from "@heroui/react";
import { Button } from "@heroui/react";

import api from "@/lib/api";
import { Input, Switch } from "@/components/heroui";

type AppConfigKey =
  | "MCP_ENABLE"
  | "MCP_TOKEN";

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
};

const fields: Array<{
  key: AppConfigKey;
  label: string;
  placeholder: string;
  type?: "text" | "password" | "number" | "switch";
}> = [
  { key: "MCP_ENABLE", label: "MCP Enable", placeholder: "false", type: "switch" },
  { key: "MCP_TOKEN", label: "MCP Token", placeholder: "mcp-token", type: "text" },
];

function labelOfSource(source: AdminConfigItem["source"]) {
  switch (source) {
    case "db":
      return "数据库";
    case "env":
      return "环境变量";
    default:
      return "默认值";
  }
}

export default function ConfigManagePage() {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [values, setValues] = useState<ConfigValues>(defaultValues);
  const [sources, setSources] = useState<Partial<Record<AppConfigKey, ConfigSource>>>({});
  const [submitAttempted, setSubmitAttempted] = useState(false);

  const intErrors = useMemo(() => ({} as Partial<Record<AppConfigKey, string>>), []);

  const sourceHint = (key: AppConfigKey) => {
    const meta = sources[key];
    if (!meta) return "当前来源：未知";
    return `当前来源：${labelOfSource(meta.source)}`;
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

    setSaving(true);
    try {
      await api.post("/config", { app_config: values });
      toast.success("保存成功");
      await load();
    } finally {
      setSaving(false);
    }
  };

  const renderFormField = (field: typeof fields[0], invalid: boolean) => {
    if (field.type === "switch") {
      return (
        <div className="flex flex-col gap-1">
          <Label>{field.label}</Label>
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
        errorMessage={invalid ? intErrors[field.key] : undefined}
        isInvalid={invalid}
        isRequired={field.type === "number"}
        label={field.label}
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
          <h1 className="text-2xl font-semibold text-default-900">配置管理</h1>
          <p className="mt-1 text-sm text-default-500">仅支持 AppConfig 白名单项。</p>
        </div>
        <Button variant="primary" isDisabled={loading} isPending={saving} onPress={save}>
          保存
        </Button>
      </div>

      <Card className="border border-default-200/70 bg-background/70 shadow-sm">
        <Card.Header 
          style={{flexDirection: "unset"}} 
          className="flex items-center justify-between gap-4"
        >
          <div>
            <p className="text-base font-semibold">AppConfig</p>
            <p className="mt-1 text-sm text-default-500">优先级：数据库 → 环境变量 → 默认值</p>
          </div>
          <Button isDisabled={loading} size="sm" variant="tertiary" onPress={load}>
            刷新
          </Button>
        </Card.Header>
        <Card.Content className="space-y-4">
          {loading ? (
            <div className="flex flex-col gap-2 items-center justify-center py-10">
              <Spinner />
              <span className="text-xs text-muted">加载中...</span>
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
