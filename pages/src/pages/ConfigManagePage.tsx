import { useCallback, useEffect, useMemo, useState } from "react";
import { Button, Card, CardBody, CardHeader, Input, Spinner, Switch, addToast } from "@heroui/react";

import api from "@/lib/api";

type AppConfigKey =
  | "AI_BASE_URL"
  | "AI_API_KEY"
  | "AI_MODEL"
  | "AI_MAX_CONTEXT_TOKENS"
  | "AI_TIMEOUT_MS"
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
  AI_BASE_URL: "",
  AI_API_KEY: "",
  AI_MODEL: "",
  AI_MAX_CONTEXT_TOKENS: "",
  AI_TIMEOUT_MS: "",
  MCP_ENABLE: "false",
  MCP_TOKEN: "",
};

const fields: Array<{
  key: AppConfigKey;
  label: string;
  placeholder: string;
  type?: "text" | "password" | "number" | "switch";
}> = [
  { key: "AI_BASE_URL", label: "AI Base URL", placeholder: "https://api.openai.com/v1" },
  { key: "AI_API_KEY", label: "AI API Key", placeholder: "sk-xxx", type: "password" },
  { key: "AI_MODEL", label: "AI 模型", placeholder: "gpt-5" },
  { key: "AI_MAX_CONTEXT_TOKENS", label: "最大上下文 Token", placeholder: "16000", type: "number" },
  { key: "AI_TIMEOUT_MS", label: "请求超时(ms)", placeholder: "60000", type: "number" },
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

  const intErrors = useMemo(() => {
    const errors: Partial<Record<AppConfigKey, string>> = {};
    const intKeys: AppConfigKey[] = ["AI_MAX_CONTEXT_TOKENS", "AI_TIMEOUT_MS"];
    for (const key of intKeys) {
      const raw = values[key].trim();
      const n = Number(raw);
      if (!raw || Number.isNaN(n) || !Number.isInteger(n) || n <= 0) {
        errors[key] = "请输入正整数";
      }
    }
    return errors;
  }, [values]);

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
    if (intErrors.AI_MAX_CONTEXT_TOKENS || intErrors.AI_TIMEOUT_MS) {
      addToast({ title: "请先修正数字类型配置", color: "danger", variant: "flat" });
      return;
    }

    setSaving(true);
    try {
      await api.post("/config", { app_config: values });
      addToast({ title: "保存成功", color: "success", variant: "flat" });
      await load();
    } finally {
      setSaving(false);
    }
  };

  const renderFormField = (field: typeof fields[0], invalid: boolean) => {
    if (field.type === "switch") {
      return (
        <label>
          {field.label}
          <Switch
            isSelected={values[field.key] === "true"}
            onValueChange={(selected) => {
              setValues((prev) => ({ ...prev, [field.key]: selected ? "true" : "false" }));
            }}
          >
            {`/ ${sourceHint(field.key)}`}
          </Switch>
        </label>
      );
    } else {
      return <Input
        key={field.key}
        errorMessage={invalid ? intErrors[field.key] : undefined}
        isInvalid={invalid}
        label={`${field.label} / ${sourceHint(field.key)}`}
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
        <Button color="primary" isDisabled={loading} isLoading={saving} onPress={save}>
          保存
        </Button>
      </div>

      <Card className="border border-default-200/70 bg-background/70 shadow-sm">
        <CardHeader className="flex items-center justify-between">
          <div>
            <p className="text-base font-semibold">AppConfig</p>
            <p className="mt-1 text-sm text-default-500">优先级：数据库 → 环境变量 → 默认值</p>
          </div>
          <Button isDisabled={loading} size="sm" variant="light" onPress={load}>
            刷新
          </Button>
        </CardHeader>
        <CardBody className="space-y-4">
          {loading ? (
            <div className="flex items-center justify-center py-10">
              <Spinner label="加载中..." />
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
        </CardBody>
      </Card>
    </div>
  );
}
