import { ChangeEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Chip, ListBox, toast } from "@heroui/react";
import Editor, { DiffEditor, MonacoDiffEditor, OnMount } from "@monaco-editor/react";
import { useNavigate, useParams } from "react-router-dom";
import type { IDisposable } from "monaco-editor";

import api from "@/lib/api";
import EditorPanelAction from "@/components/editor-panel-action";
import HttpDrawer from "@/components/http-drawer";
import XModal from "@/components/modal";
import WorkerLogList, { WorkerLogItem } from "@/components/worker-log-list";
import { Icon } from "@iconify/react";
import { useTheme } from "@/lib/theme";
import { Button } from "@heroui/react";

type WorkerItem = {
  id: string;
  name: string;
  description: string;
  runtime: "python" | "node";
  route: string;
  timeout_ms: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type FileContentResp = {
  filename: string;
  content: string;
  media_type?: "text" | "image";
  mime_type?: string;
  preview_data_url?: string;
};

type WorkerLogsPageResp = {
  page: number;
  page_size: number;
  total: number;
  data: WorkerLogItem[];
};

const workerLogsPageSize = 20;

function languageFromFilename(filename: string): string {
  if (filename.endsWith(".py")) return "python";
  if (filename.endsWith(".js")) return "javascript";
  if (filename.endsWith(".ts")) return "typescript";
  if (filename.endsWith(".json")) return "json";
  if (filename.endsWith(".md")) return "markdown";
  if (filename.endsWith(".html")) return "html";
  if (filename.endsWith(".css")) return "css";
  return "plaintext";
}

function buildDiffFilename(original: string): string {
  const marker = ".diff";
  const dotIndex = original.lastIndexOf(".");
  if (dotIndex <= 0) {
    return `${original}${marker}`;
  }
  return `${original.slice(0, dotIndex)}${marker}${original.slice(dotIndex)}`;
}

export default function WorkerDetailPage() {
  const navigate = useNavigate();
  const { id = "" } = useParams();
  const { isDark } = useTheme();
  const uploadRef = useRef<HTMLInputElement | null>(null);
  const saveCurrentFileRef = useRef<(() => Promise<void>) | null>(null);
  const diffEditorChangeRef = useRef<IDisposable | null>(null);

  const [workerInfo, setWorkerInfo] = useState<WorkerItem | null>(null);
  const [files, setFiles] = useState<string[]>([]);
  const [selectedFile, setSelectedFile] = useState("");
  const [content, setContent] = useState("");
  const [originContent, setOriginContent] = useState("");
  const [activeDiffFile, setActiveDiffFile] = useState("");
  const [fileMediaType, setFileMediaType] = useState<"text" | "image">("text");
  const [previewDataURL, setPreviewDataURL] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [fileBusy, setFileBusy] = useState(false);
  const [fnBusy, setFnBusy] = useState(false);
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [deleteFileConfirmOpen, setDeleteFileConfirmOpen] = useState(false);
  const [logLoading, setLogLoading] = useState(false);
  const [logPage, setLogPage] = useState(1);
  const [logTotal, setLogTotal] = useState(0);
  const [logItems, setLogItems] = useState<WorkerLogItem[]>([]);

  const selectedKeys = useMemo(() => (selectedFile ? new Set<string>([selectedFile]) : new Set<string>()), [selectedFile]);
  const defaultRunUrl = useMemo(() => {
    if (!workerInfo) return "";
    let fullUrl = `${window.location.origin}${workerInfo.route}`;
    if (fullUrl.endsWith("/*")) {
      fullUrl = fullUrl.slice(0, -1);
    }
    return fullUrl;
  }, [workerInfo]);
  const isDiffMode = activeDiffFile !== "";
  const logTotalPages = useMemo(() => Math.max(1, Math.ceil(logTotal / workerLogsPageSize)), [logTotal]);

  const loadWorkerInfo = useCallback(async () => {
    const data = await api.get<WorkerItem>(`/workers/${id}`);
    setWorkerInfo(data);
  }, [id]);

  const loadWorkerLogs = useCallback(async (page: number) => {
    setLogLoading(true);
    try {
      const data = await api.get<WorkerLogsPageResp>(`/workers/${id}/logs?page=${page}&page_size=${workerLogsPageSize}`);
      setLogItems(data.data || []);
      setLogTotal(data.total || 0);
    } finally {
      setLogLoading(false);
    }
  }, [id]);

  const fetchFileContent = useCallback(async (filename: string, options?: { hideToast?: boolean }) => {
    const target = encodeURIComponent(filename);
    return api.get<FileContentResp>(`/workers/${id}/files/${target}`, options);
  }, [id]);

  const loadFiles = useCallback(async (prefer?: string) => {
    const data = await api.get<{ files: string[] }>(`/workers/${id}/files`);
    const nextAllFiles = data.files || [];
    const visibleFiles = nextAllFiles
      .filter((filename) => !/\.diff(?:\.[^./\\]+)?$/.test(filename))
      .sort((a, b) => {
        const aMain = a.startsWith("main.");
        const bMain = b.startsWith("main.");
        if (aMain && !bMain) return -1;
        if (!aMain && bMain) return 1;
        return a.localeCompare(b);
      });
    setFiles(visibleFiles);

    if (visibleFiles.length === 0) {
      setSelectedFile("");
      setContent("");
      setOriginContent("");
      setActiveDiffFile("");
      setFileMediaType("text");
      setPreviewDataURL("");
      return;
    }

    if (prefer && visibleFiles.includes(prefer)) {
      setSelectedFile(prefer);
      return;
    }
    setSelectedFile((prev) => (prev && visibleFiles.includes(prev) ? prev : visibleFiles[0]));
  }, [id]);

  const loadFileContent = useCallback(async (filename: string) => {
    if (!filename) return;
    const data = await fetchFileContent(filename);
    const mediaType = data.media_type === "image" ? "image" : "text";
    setFileMediaType(mediaType);
    setContent(data.content || "");
    setPreviewDataURL(mediaType === "image" ? (data.preview_data_url || "") : "");

    if (mediaType !== "text") {
      setOriginContent("");
      setActiveDiffFile("");
      return;
    }
    const maybeDiff = buildDiffFilename(filename);
    try {
      const diffData = await fetchFileContent(maybeDiff, { hideToast: true });
      if (diffData.media_type === "image") {
        setOriginContent("");
        setActiveDiffFile("");
        return;
      }
      setOriginContent(data.content || "");
      setContent(diffData.content || "");
      setActiveDiffFile(maybeDiff);
    } catch {
      setOriginContent("");
      setActiveDiffFile("");
    }
  }, [fetchFileContent]);

  useEffect(() => {
    if (!id) {
      toast.danger("缺少 WorkerId");
      navigate("/workers", { replace: true });
      return;
    }

    let active = true;
    const boot = async () => {
      setLoading(true);
      try {
        await loadWorkerInfo();
        await loadFiles();
      } finally {
        if (active) setLoading(false);
      }
    };

    void boot();
    return () => {
      active = false;
    };
  }, [id, navigate, loadWorkerInfo, loadFiles]);

  useEffect(() => {
    if (!selectedFile) return;
    void loadFileContent(selectedFile);
  }, [selectedFile, loadFileContent]);

  useEffect(() => {
    if (!logModalOpen) return;
    void loadWorkerLogs(logPage);
  }, [logModalOpen, logPage, loadWorkerLogs]);

  const saveCurrentFile = useCallback(async () => {
    if (!selectedFile || saving) return;
    if (isDiffMode) {
      toast.warning("当前为 Diff 模式，请使用“应用并清理 diff”");
      return;
    }
    if (fileMediaType === "image") {
      toast.warning("图片预览文件不支持在线编辑");
      return;
    }
    setSaving(true);
    try {
      await api.post<{ files: string[] }>(`/workers/${id}/files/update`, {
        filename: selectedFile,
        content,
      });
      toast.success("保存成功");
      await loadFiles(selectedFile);
    } finally {
      setSaving(false);
    }
  }, [selectedFile, saving, isDiffMode, fileMediaType, id, content, loadFiles]);

  const applyDiffMerge = useCallback(async () => {
    if (!activeDiffFile || !selectedFile || saving || fileBusy) return;
    const ok = window.confirm(`将 ${activeDiffFile} 的内容合并回 ${selectedFile} 并删除 diff 文件，确认继续？`);
    if (!ok) return;

    setSaving(true);
    try {
      await api.post<{ files: string[] }>(`/workers/${id}/files/update`, {
        filename: selectedFile,
        content,
      });
      await api.post<{ ok: boolean }>(`/workers/${id}/files/delete`, {
        filename: activeDiffFile,
      });
      toast.success("合并完成");
      await loadFiles(selectedFile);
      await loadFileContent(selectedFile);
    } finally {
      setSaving(false);
    }
  }, [activeDiffFile, selectedFile, saving, fileBusy, id, content, loadFiles, loadFileContent]);

  const cancelDiffMerge = useCallback(async () => {
    if (!activeDiffFile || fileBusy) return;
    setFileBusy(true);
    try {
      await api.post<{ ok: boolean }>(`/workers/${id}/files/delete`, {
        filename: activeDiffFile,
      });
      toast.warning("已取消 Diff 合并");
      await loadFiles(selectedFile || undefined);
      if (selectedFile) {
        await loadFileContent(selectedFile);
      }
    } finally {
      setFileBusy(false);
    }
  }, [activeDiffFile, fileBusy, id, loadFiles, loadFileContent, selectedFile]);

  useEffect(() => {
    saveCurrentFileRef.current = isDiffMode ? applyDiffMerge : saveCurrentFile;
  }, [isDiffMode, applyDiffMerge, saveCurrentFile]);

  const handleEditorMount: OnMount = (editor, monaco) => {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      void saveCurrentFileRef.current?.();
    });
  };

  const handleDiffEditorMount = (editor: MonacoDiffEditor, monaco: Parameters<OnMount>[1]) => {
    const modifiedEditor = editor.getModifiedEditor();
    diffEditorChangeRef.current?.dispose();
    diffEditorChangeRef.current = modifiedEditor.onDidChangeModelContent(() => {
      setContent(modifiedEditor.getValue());
    });
    modifiedEditor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      void saveCurrentFileRef.current?.();
    });
  };

  useEffect(() => {
    return () => {
      diffEditorChangeRef.current?.dispose();
      diffEditorChangeRef.current = null;
    };
  }, []);

  const createFile = async () => {
    const filename = window.prompt("输入新文件名（例如 helper.py）", "");
    if (!filename) return;
    const clean = filename.trim();
    if (!clean) return;

    setFileBusy(true);
    try {
      await api.post<{ files: string[] }>(`/workers/${id}/files/update`, {
        filename: clean,
        content: "",
      });
      await loadFiles(clean);
      await loadFileContent(clean);
    } finally {
      setFileBusy(false);
    }
  };

  const performDeleteCurrentFile = useCallback(async () => {
    if (!selectedFile || fileBusy) return;
    setFileBusy(true);
    try {
      await api.post<{ ok: boolean }>(`/workers/${id}/files/delete`, {
        filename: selectedFile,
      });
      await loadFiles();
    } finally {
      setFileBusy(false);
    }
  }, [selectedFile, fileBusy, id, loadFiles]);

  const deleteCurrentFile = async () => {
    if (!selectedFile || fileBusy) return;
    if (selectedFile === "package.json") {
      setDeleteFileConfirmOpen(true);
      return;
    }
    const ok = window.confirm(`确认删除文件 ${selectedFile} ？`);
    if (!ok) return;
    await performDeleteCurrentFile();
  };

  const renameCurrentFile = async () => {
    if (!selectedFile || fileBusy) return;
    const nextName = window.prompt("输入新的文件名", selectedFile);
    if (nextName === null) return;
    const clean = nextName.trim();
    if (!clean) return;
    if (clean === selectedFile) return;

    setFileBusy(true);
    try {
      const data = await api.post<{ files: string[]; filename: string }>(`/workers/${id}/files/rename`, {
        filename: selectedFile,
        new_filename: clean,
      });
      const renamed = data.filename || clean;
      toast.success("重命名成功");
      await loadFiles(renamed);
      await loadFileContent(renamed);
    } finally {
      setFileBusy(false);
    }
  };

  const uploadFiles = async (event: ChangeEvent<HTMLInputElement>) => {
    const selected = event.target.files;
    if (!selected || selected.length === 0) return;
    setFileBusy(true);
    try {
      const formData = new FormData();
      Array.from(selected).forEach((file) => {
        formData.append("files", file);
      });
      const data = await api.post<{ files: string[] }>(`/workers/${id}/files/upload`, formData);
      const uploadedFirst = data.files?.[0] || "";
      await loadFiles(uploadedFirst || undefined);
      if (uploadedFirst) {
        await loadFileContent(uploadedFirst);
      }
    } finally {
      setFileBusy(false);
      event.target.value = "";
    }
  };

  const toggleWorkerEnabled = async () => {
    if (!workerInfo || fnBusy) return;
    setFnBusy(true);
    try {
      const nextPath = workerInfo.enabled ? "disable" : "enable";
      const data = await api.post<WorkerItem>(`/workers/${nextPath}`, { id });
      setWorkerInfo(data);
    } finally {
      setFnBusy(false);
    }
  };

  const removeWorker = async () => {
    if (!workerInfo || fnBusy) return;
    const ok = window.confirm(`确认删除 Worker ${workerInfo.name} ？`);
    if (!ok) return;
    setFnBusy(true);
    try {
      await api.post<{ ok: boolean }>("/workers/delete", { id });
      navigate("/workers", { replace: true });
    } finally {
      setFnBusy(false);
    }
  };

  const openWorkerLogsModal = () => {
    setLogPage(1);
    setLogModalOpen(true);
  };

  if (loading) {
    return <section className="py-6 text-sm text-default-500">正在加载 Worker 详情...</section>;
  }

  if (!workerInfo) {
    return <section className="py-6 text-sm text-danger">Worker 不存在或已删除</section>;
  }

  return (
    <section className="relative h-[calc(var(--main-height)-36px)]">
      <div className="flex h-full flex-col overflow-hidden rounded-xl border border-default-200 bg-background/70">
        <div className="border-b border-default-200 p-2">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <Button size="sm" variant="secondary"
                isIconOnly
                onPress={() => navigate(`${window.__BASE_PREFIX__}/workers`)}
              >
                <Icon icon="material-symbols:arrow-back-ios-new-rounded" width="24" height="30" />
              </Button>
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <h1 className="truncate text-base font-semibold text-default-900">{workerInfo.name}</h1>
                  <Chip color={workerInfo.enabled ? "success" : "default"} size="sm" variant="soft">
                    {workerInfo.enabled ? "已启用" : "已停用"}
                  </Chip>
                </div>
                <p className="truncate text-xs text-default-500">
                  {workerInfo.route} · {workerInfo.runtime}
                </p>
              </div>
            </div>
            <div className="flex items-center justify-end gap-2">
              <HttpDrawer defaultUrl={defaultRunUrl} />
              <Button size="sm" variant="secondary" onPress={openWorkerLogsModal}>运行日志</Button>
              <Button
                isPending={fnBusy}
                size="sm"
                variant={workerInfo.enabled ? "tertiary" : "tertiary"}
                className={workerInfo.enabled ? "border-warning text-warning" : undefined}
                onPress={toggleWorkerEnabled}
              >
                {workerInfo.enabled ? "停用" : "启用"}
              </Button>
              <Button isPending={fnBusy} size="sm" variant="danger-soft" onPress={removeWorker}>删除 Worker</Button>
            </div>
          </div>
        </div>
        <div className="flex min-h-0 flex-1">
          <aside className="flex min-h-0 w-[150px] shrink-0 flex-col border-r border-default-200">
            <div className="p-2 border-b border-default-200 flex flex-col gap-2">
              <Button className="w-full" size="sm" variant="tertiary" onPress={() => uploadRef.current?.click()}>
                上传文件
              </Button>
              <Button className="w-full" isPending={fileBusy} size="sm" variant="tertiary" onPress={createFile}>
                新建文件
              </Button>
              <input ref={uploadRef} multiple className="hidden" type="file" onChange={uploadFiles} />
            </div>
            <div className="flex-1 min-h-0 overflow-auto">
              <ListBox
                aria-label="Worker 文件列表"
                className="p-1"
                selectedKeys={selectedKeys}
                selectionMode="single"
                onSelectionChange={(keys) => {
                  console.log("selected keys", keys);
                  if (keys === "all") return;
                  const first = Array.from(keys)[0];
                  if (typeof first === "string") {
                    setSelectedFile(first);
                  }
                }}
              >
                {files.map((file) => (
                  <ListBox.Item key={file} id={file} textValue={file}>
                    {file}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                ))}
              </ListBox>
            </div>
          </aside>

          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div className="border-b border-default-200 p-3 flex items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-default-700">
                  {selectedFile || "请选择一个文件"}
                </p>
                {isDiffMode ? (
                  <p className="truncate text-xs text-warning">
                    Diff 模式：{selectedFile} ↔ {activeDiffFile}
                  </p>
                ) : null}
              </div>
              <div className="flex items-center gap-2">
                {isDiffMode ? (
                  <>
                    <Button
                      size="sm"
                      variant="outline"
                      className="border-orange-300 text-orange-400"
                      isDisabled={saving || fileBusy}
                      onPress={applyDiffMerge}
                    >
                      应用 Diff
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      isDisabled={saving || fileBusy}
                      onPress={cancelDiffMerge}
                    >
                      取消 Diff
                    </Button>
                  </>
                ) : null}
                <EditorPanelAction
                  loading={saving}
                  disabled={!selectedFile || fileBusy}
                  onRename={renameCurrentFile}
                  onDelete={deleteCurrentFile}
                  onSave={isDiffMode ? applyDiffMerge : saveCurrentFile}
                />
              </div>
            </div>

            <div className="flex-1 min-h-0">
              {selectedFile ? (
                fileMediaType === "image" ? (
                  <div className="h-full w-full overflow-auto bg-default-50 p-4">
                    <div className="mx-auto flex h-full max-w-5xl items-center justify-center">
                      {previewDataURL ? (
                        <img alt={selectedFile} className="max-h-full max-w-full object-contain" src={previewDataURL} />
                      ) : (
                        <p className="text-sm text-default-500">图片预览加载失败</p>
                      )}
                    </div>
                  </div>
                ) : isDiffMode ? (
                  <DiffEditor
                    height="100%"
                    language={languageFromFilename(selectedFile)}
                    modified={content}
                    original={originContent}
                    theme={isDark ? "vs-dark" : "vs"}
                    onMount={handleDiffEditorMount}
                    options={{
                      automaticLayout: true,
                      fontSize: 14,
                      minimap: { enabled: false },
                      scrollBeyondLastLine: false,
                      renderSideBySide: true,
                    }}
                  />
                ) : (
                  <Editor
                    defaultLanguage={languageFromFilename(selectedFile)}
                    height="100%"
                    language={languageFromFilename(selectedFile)}
                    onMount={handleEditorMount}
                    options={{
                      automaticLayout: true,
                      fontSize: 14,
                      minimap: { enabled: false },
                      scrollBeyondLastLine: false,
                    }}
                    path={selectedFile}
                    theme={isDark ? "vs-dark" : "vs"}
                    value={content}
                    onChange={(value) => setContent(value ?? "")}
                  />
                )
              ) : (
                <div className="h-full flex items-center justify-center text-default-500 text-sm">
                  当前无文件，请先上传或新建文件
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
      <XModal
        isOpen={deleteFileConfirmOpen}
        size="lg"
        header="删除 package.json 提示"
        isDismissable={!fileBusy}
        isKeyboardDismissDisabled={fileBusy}
        onOpenChange={(open) => {
          if (fileBusy) return;
          setDeleteFileConfirmOpen(open);
        }}
        footer={
          <>
            <Button
              variant="secondary"
              isDisabled={fileBusy}
              onPress={() => setDeleteFileConfirmOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              isPending={fileBusy}
              onPress={() => {
                void (async () => {
                  await performDeleteCurrentFile();
                  setDeleteFileConfirmOpen(false);
                })();
              }}
            >
              知道了
            </Button>
          </>
        }
      >
        <p className="text-sm leading-6 text-default-700">
          删除 package.json 文件会使 Worker 转为 CommonJS，ESM 中的特性将不再可用，
          你需要修改 JS 代码为 CJS 的标准导出和导入后 Worker 才能正常运行
        </p>
      </XModal>
      <XModal
        isOpen={logModalOpen}
        size="cover"
        header="最近运行日志"
        onOpenChange={(open) => {
          setLogModalOpen(open);
        }}
      >
        <WorkerLogList
          loading={logLoading}
          items={logItems}
          page={logPage}
          totalPages={logTotalPages}
          onPageChange={setLogPage}
          onRefresh={() => void loadWorkerLogs(logPage)}
        />
      </XModal>
    </section>
  );
}
