import { Dispatch, SetStateAction, useEffect, useMemo, useState } from "react";
import { toast, Drawer, Tabs, TextArea, Label } from "@heroui/react";
import { Button } from "@heroui/react";
import { Input, Select } from "@/components/heroui";

type TabKey = "headers" | "params" | "body";
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE";

type HttpDrawerProps = {
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
            className="flex-1"
            label=""
            name={`kv-key-${row.id}`}
            placeholder="Key"
            value={row.key}
            onValueChange={(value) => updateRow(row.id, "key", value)}
          />
          <Input
            className="flex-1"
            label=""
            name={`kv-value-${row.id}`}
            placeholder="Value"
            value={row.value}
            onValueChange={(value) => updateRow(row.id, "value", value)}
          />
          <Button
            isIconOnly
            size="sm"
            variant="secondary"
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
            variant="secondary"
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

export default function HttpDrawer({ defaultUrl, defaultMethod = "GET" }: HttpDrawerProps) {
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
      toast.danger("URL 不能为空");
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
    <Drawer>
      <Button size="sm" variant="tertiary">Testing</Button>
      <Drawer.Backdrop>
        <Drawer.Content placement="right">
          <Drawer.Dialog className="w-[min(96vw,36rem)]">
            <Drawer.Handle />
            <Drawer.CloseTrigger />
            <Drawer.Header>
              <Drawer.Heading>HTTP 测试</Drawer.Heading>
            </Drawer.Header>
            <Drawer.Body className="flex flex-col gap-4">
              <div className="grid grid-cols-[110px_1fr_auto] items-end gap-2">
                <Select
                  className="w-full"
                  label="Method"
                  options={methodList.map((m) => ({ label: m, value: m }))}
                  value={method}
                  onValueChange={(value) => setMethod(value as HttpMethod)}
                />

                <Input
                  label="URL"
                  name="http-url"
                  placeholder="输入请求 URL"
                  value={url}
                  onValueChange={setUrl}
                />

                <Button
                  isIconOnly
                  variant="primary"
                  isPending={loading}
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
                  <Tabs.ListContainer>
                    <Tabs.List aria-label="HTTP Tabs">
                      <Tabs.Tab id="params">
                        Params
                        <Tabs.Indicator />
                      </Tabs.Tab>
                      <Tabs.Tab id="body">
                        Body
                        <Tabs.Indicator />
                      </Tabs.Tab>
                      <Tabs.Tab id="headers">
                        Headers
                        <Tabs.Indicator />
                      </Tabs.Tab>
                    </Tabs.List>
                  </Tabs.ListContainer>
                  <Tabs.Panel className="pt-4" id="params">
                    <KeyValueRowsEditor rows={paramsRows} setRows={setParamsRows} />
                  </Tabs.Panel>
                  <Tabs.Panel className="pt-4" id="body">
                    <div className="flex flex-col gap-1">
                      <Label htmlFor="textarea-body">Short feedback</Label>
                    <TextArea
                      id="textarea-body"
                      rows={6}
                      placeholder='{"name":"callit"}'
                      value={bodyText}
                      onChange={(e) => setBodyText(e.target.value)}
                    />
                    </div>
                  </Tabs.Panel>
                  <Tabs.Panel className="pt-4" id="headers">
                    <KeyValueRowsEditor rows={headersRows} setRows={setHeadersRows} />
                  </Tabs.Panel>
                </Tabs>
              </div>

              <div className="rounded-lg border border-default-200 bg-content1 p-3">
                <p className="mb-2 text-sm font-medium text-default-700">Response</p>
                <pre className="max-h-[320px] overflow-auto whitespace-pre-wrap break-words text-xs text-default-600">{responseText}</pre>
              </div>
            </Drawer.Body>
            <Drawer.Footer className="flex justify-end">
              <Button slot="close" size="sm" variant="secondary">
                关闭
              </Button>
            </Drawer.Footer>
          </Drawer.Dialog>
        </Drawer.Content>
      </Drawer.Backdrop>
    </Drawer>
  );
}
