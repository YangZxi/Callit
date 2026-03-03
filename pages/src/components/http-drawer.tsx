import { Dispatch, SetStateAction, useEffect, useMemo, useState } from "react";
import { addToast, Button, Drawer, DrawerBody, DrawerContent, DrawerHeader, Input, Select, SelectItem, Tab, Tabs, Textarea } from "@heroui/react";

type TabKey = "headers" | "params" | "body";
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE";

type HttpDrawerProps = {
  isOpen: boolean;
  onClose: () => void;
  defaultUrl: string;
  defaultMethod?: HttpMethod;
};

type KeyValueRow = {
  id: string;
  key: string;
  value: string;
};

function createRow(): KeyValueRow {
  return {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    key: "",
    value: "",
  };
}

function rowsToMap(rows: KeyValueRow[]): Record<string, string> {
  const output: Record<string, string> = {};
  rows.forEach((item) => {
    const key = item.key.trim();
    if (!key) return;
    output[key] = item.value;
  });
  return output;
}

type KeyValueRowsEditorProps = {
  rows: KeyValueRow[];
  setRows: Dispatch<SetStateAction<KeyValueRow[]>>;
};

function KeyValueRowsEditor({ rows, setRows }: KeyValueRowsEditorProps) {
  const updateRow = (id: string, field: "key" | "value", value: string) => {
    setRows((prev) => prev.map((item) => (item.id === id ? { ...item, [field]: value } : item)));
  };

  const insertRowAfter = (id: string) => {
    setRows((prev) => {
      const index = prev.findIndex((item) => item.id === id);
      if (index < 0) return [...prev, createRow()];
      const next = [...prev];
      next.splice(index + 1, 0, createRow());
      return next;
    });
  };

  const removeRow = (id: string) => {
    setRows((prev) => {
      if (prev.length <= 1) return prev;
      return prev.filter((item) => item.id !== id);
    });
  };

  return (
    <div className="space-y-1">
      {rows.map((row) => (
        <div key={row.id} className="flex gap-2 items-center">
          <Input
            size="sm"
            placeholder="Key"
            value={row.key}
            onValueChange={(value) => updateRow(row.id, "key", value)}
          />
          <Input
            size="sm"
            placeholder="Value"
            value={row.value}
            onValueChange={(value) => updateRow(row.id, "value", value)}
          />
          <Button
            isIconOnly
            size="sm"
            variant="flat"
            isDisabled={rows.length <= 1}
            onPress={() => removeRow(row.id)}
          >
            <svg aria-hidden="true" fill="none" height="16" viewBox="0 0 24 24" width="16">
              <path d="M7 11h10v2H7z" fill="currentColor" />
            </svg>
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="flat"
            onPress={() => insertRowAfter(row.id)}
          >
            <svg aria-hidden="true" fill="none" height="16" viewBox="0 0 24 24" width="16">
              <path d="M11 5h2v14h-2zM5 11h14v2H5z" fill="currentColor" />
            </svg>
          </Button>
        </div>
      ))}
    </div>
  );
}

export default function HttpDrawer({ isOpen, onClose, defaultUrl, defaultMethod = "GET" }: HttpDrawerProps) {
  const [activeTab, setActiveTab] = useState<TabKey>("params");
  const [method, setMethod] = useState<HttpMethod>(defaultMethod);
  const [url, setUrl] = useState(defaultUrl);
  const [headersRows, setHeadersRows] = useState<KeyValueRow[]>([createRow()]);
  const [paramsRows, setParamsRows] = useState<KeyValueRow[]>([createRow()]);
  const [bodyText, setBodyText] = useState("");
  const [loading, setLoading] = useState(false);
  const [responseText, setResponseText] = useState("等待请求...");

  useEffect(() => {
    setUrl(defaultUrl);
  }, [defaultUrl]);

  useEffect(() => {
    setMethod(defaultMethod);
  }, [defaultMethod]);

  const methodList = useMemo<HttpMethod[]>(() => ["GET", "POST", "PUT", "DELETE"], []);

  const sendRequest = async () => {
    if (!url.trim()) {
      addToast({ title: "URL 不能为空", color: "danger", variant: "flat", timeout: 1800 });
      return;
    }

    setLoading(true);
    try {
      const headers = rowsToMap(headersRows);
      const params = rowsToMap(paramsRows);

      const target = new URL(url, window.location.origin);
      Object.entries(params).forEach(([key, val]) => {
        target.searchParams.set(key, val);
      });

      const init: RequestInit = {
        method,
        headers,
      };
      if (method !== "GET" && method !== "DELETE" && bodyText.trim()) {
        init.body = bodyText;
      }

      const res = await fetch(target.toString(), init);
      const resHeaders: Record<string, string> = {};
      res.headers.forEach((value, key) => {
        resHeaders[key] = value;
      });

      const contentType = res.headers.get("content-type") || "";
      const body = contentType.includes("application/json") ? await res.json().catch(() => null) : await res.text();

      setResponseText(JSON.stringify({
        ok: res.ok,
        status: res.status,
        statusText: res.statusText,
        url: target.toString(),
        headers: resHeaders,
        body,
      }, null, 2));
    } catch (err) {
      setResponseText(JSON.stringify({
        error: err instanceof Error ? err.message : "请求失败",
      }, null, 2));
    }
    finally {
      setLoading(false);
    }
  };

  return (
    <Drawer
      isOpen={isOpen}
      placement="right"
      size="2xl"
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DrawerContent>
        <>
          <DrawerHeader className="flex items-center justify-between gap-2">
            <span className="text-base font-semibold text-default-900">HTTP 测试</span>
          </DrawerHeader>
          <DrawerBody className="">
            <div className="grid grid-cols-[110px_1fr_auto] items-end gap-2">
              <Select
                label="Method"
                labelPlacement="outside"
                disallowEmptySelection
                selectedKeys={new Set([method])}
                size="sm"
                onSelectionChange={(keys) => {
                  if (keys === "all") return;
                  const first = Array.from(keys)[0];
                  if (typeof first === "string") {
                    setMethod(first as HttpMethod);
                  }
                }}
              >
                {methodList.map((m) => (
                  <SelectItem key={m}>{m}</SelectItem>
                ))}
              </Select>

              <Input
                label="URL"
                labelPlacement="outside"
                placeholder="输入请求 URL"
                size="sm"
                value={url}
                onValueChange={setUrl}
              />

              <Button
                isIconOnly
                color="primary"
                isLoading={loading}
                size="sm"
                onPress={sendRequest}
              >
                <svg aria-hidden="true" fill="none" height="18" viewBox="0 0 24 24" width="18">
                  <path d="M5 3l14 9-14 9 3.5-9L5 3z" fill="currentColor" />
                </svg>
              </Button>
            </div>

            <div>
              <Tabs
                aria-label="HTTP Tabs"
                selectedKey={activeTab}
                onSelectionChange={(key) => setActiveTab(String(key) as TabKey)}
              >
                <Tab key="params" title="Params">
                  <KeyValueRowsEditor rows={paramsRows} setRows={setParamsRows} />
                </Tab>
                <Tab key="body" title="Body">
                  <Textarea
                    label="Body"
                    labelPlacement="outside"
                    minRows={6}
                    placeholder='{"name":"callit"}'
                    value={bodyText}
                    onValueChange={setBodyText}
                  />
                </Tab>
                <Tab key="headers" title="Headers">
                  <KeyValueRowsEditor rows={headersRows} setRows={setHeadersRows} />
                </Tab>
              </Tabs>
            </div>

            <div className="rounded-lg border border-default-200 bg-content1 p-3">
              <p className="mb-2 text-sm font-medium text-default-700">Response</p>
              <pre className="max-h-[320px] overflow-auto whitespace-pre-wrap break-words text-xs text-default-600">{responseText}</pre>
            </div>
          </DrawerBody>
        </>
      </DrawerContent>
    </Drawer>
  );
}
