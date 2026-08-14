import { ChangeEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button, Chip, toast } from "@heroui/react";
import Editor, { DiffEditor, MonacoDiffEditor, OnMount } from "@monaco-editor/react";
import { useNavigate, useParams } from "react-router-dom";
import type { IDisposable } from "monaco-editor";

import api from "@/lib/api";
import EditorPanelAction from "@/components/editor-panel-action";
import HttpDrawer from "@/components/http-drawer";
import XModal from "@/components/modal";
import WorkerLogList, { WorkerLogItem } from "@/components/worker-log-list";
import WorkerFileTree from "@/components/worker-file-tree";
import { Icon } from "@iconify/react";
import { useTheme } from "@/lib/theme";

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

function normalizeWorkerPathInput(raw: string): string {
  const trimmed = raw.trim();
  const isDirectory = trimmed.endsWith("/");
  const withoutLeadingSlash = trimmed.replace(/^\/+/, "");
  const pathWithoutMarker = isDirectory ? withoutLeadingSlash.slice(0, -1) : withoutLeadingSlash;
  const normalized = pathWithoutMarker.split("/").map((segment) => segment.trim()).join("/");
  return isDirectory ? `${normalized}/` : normalized;
}

export default function WorkerDetailPage() {
  const navigate = useNavigate();
  const { id = "" } = useParams();
  const { isDark } = useTheme();
  const uploadRef = useRef<HTMLInputElement | null>(null);
  const uploadDirectoryRef = useRef("");
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
  const [packageDeleteTarget, setPackageDeleteTarget] = useState("");
  const [directoryDeleteTarget, setDirectoryDeleteTarget] = useState("");
  const [treeRevealPath, setTreeRevealPath] = useState("");
  const [logLoading, setLogLoading] = useState(false);
  const [logPage, setLogPage] = useState(1);
  const [logTotal, setLogTotal] = useState(0);
  const [logItems, setLogItems] = useState<WorkerLogItem[]>([]);

  const defaultRunUrl = useMemo(() => {
    if (!workerInfo) return "";
    let fullUrl = `${window.location.origin}${workerInfo.route}`;
    if (fullUrl.endsWith("/*")) {
      fullUrl = fullUrl.slice(0, -1);
    }
    return fullUrl;
  }, [workerInfo]);
  const isDiffMode = activeDiffFile !== "";
  const mainFilename = workerInfo?.runtime === "node" ? "main.js" : "main.py";
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
    return api.get<FileContentResp>(`/workers/${id}/file?filename=${target}`, options);
  }, [id]);

  const loadFiles = useCallback(async (prefer?: string) => {
    const data = await api.get<{ files: string[] }>(`/workers/${id}/files`);
    const nextAllFiles = data.files || [];
    const visibleEntries = nextAllFiles.filter((filename) => !/\.diff(?:\.[^./\\]+)?$/.test(filename));
    const visibleFiles = visibleEntries.filter((entry) => !entry.endsWith("/")).sort((a, b) => {
      const aMain = a === "main.py" || a === "main.js";
      const bMain = b === "main.py" || b === "main.js";
      if (aMain && !bMain) return -1;
      if (!aMain && bMain) return 1;
      return a.localeCompare(b);
    });
    setFiles(visibleEntries);

    if (visibleFiles.length === 0) {
      setSelectedFile("");
      setContent("");
      setOriginContent("");
      setActiveDiffFile("");
      setFileMediaType("text");
      setPreviewDataURL("");
      return;
    }

    if (prefer) setTreeRevealPath(prefer);
    if (prefer && !prefer.endsWith("/") && visibleFiles.includes(prefer)) {
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
    // 尝试加载 diff 文件，暂时禁用
    // const maybeDiff = buildDiffFilename(filename);
    // try {
    //   const diffData = await fetchFileContent(maybeDiff, { hideToast: true });
    //   if (diffData.media_type === "image") {
    //     setOriginContent("");
    //     setActiveDiffFile("");
    //     return;
    //   }
    //   setOriginContent(data.content || "");
    //   setContent(diffData.content || "");
    //   setActiveDiffFile(maybeDiff);
    // } catch {
    //   setOriginContent("");
    //   setActiveDiffFile("");
    // }
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

  const createFile = async (directory = "") => {
    const filename = window.prompt(
      directory ? `在 ${directory} 中输入新文件名` : "输入相对路径（目录以 / 结尾，例如 src/ 或 src/helper.py）",
      "",
    );
    if (!filename) return;
    const clean = normalizeWorkerPathInput(filename);
    if (!clean) return;
    if (directory && clean.endsWith("/")) {
      toast.warning("请输入文件名");
      return;
    }
    const targetPath = normalizeWorkerPathInput(`${directory}${clean}`);

    setFileBusy(true);
    try {
      await api.post<{ files: string[] }>(`/workers/${id}/files/update`, {
        filename: targetPath,
        content: "",
      });
      await loadFiles(targetPath);
    } finally {
      setFileBusy(false);
    }
  };

  const performDeleteEntry = useCallback(async (entry: string) => {
    if (!entry || fileBusy) return;
    setFileBusy(true);
    try {
      await api.post<{ ok: boolean }>(`/workers/${id}/files/delete`, {
        filename: entry,
      });
      if (selectedFile === entry || (entry.endsWith("/") && selectedFile.startsWith(entry))) {
        setSelectedFile("");
        setContent("");
        setOriginContent("");
        setActiveDiffFile("");
        setPreviewDataURL("");
      }
      await loadFiles();
    } finally {
      setFileBusy(false);
    }
  }, [fileBusy, id, loadFiles, selectedFile]);

  const requestDeleteEntry = useCallback(async (entry: string) => {
    if (!entry || fileBusy || entry === mainFilename) return;
    if (entry.endsWith("/")) {
      setDirectoryDeleteTarget(entry);
      return;
    }
    if (entry === "package.json") {
      setPackageDeleteTarget(entry);
      return;
    }
    const ok = window.confirm(`确认删除文件 ${entry} ？`);
    if (!ok) return;
    await performDeleteEntry(entry);
  }, [fileBusy, mainFilename, performDeleteEntry]);

  const deleteCurrentFile = async () => {
    await requestDeleteEntry(selectedFile);
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
    const directory = uploadDirectoryRef.current;
    const uploadedFirst = normalizeWorkerPathInput(`${directory}${selected[0]?.name || ""}`);
    setFileBusy(true);
    try {
      const formData = new FormData();
      Array.from(selected).forEach((file) => {
        formData.append("files", file);
      });
      const directoryQuery = directory ? `?directory=${encodeURIComponent(directory)}` : "";
      await api.post<{ files: string[] }>(`/workers/${id}/files/upload${directoryQuery}`, formData);
      await loadFiles(uploadedFirst || undefined);
    } finally {
      setFileBusy(false);
      uploadDirectoryRef.current = "";
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
          <aside className="flex min-h-0 w-[220px] shrink-0 flex-col border-r border-default-200">
            <div className="p-2 border-b border-default-200 flex flex-col gap-2">
              <Button
                className="w-full"
                size="sm"
                variant="tertiary"
                onPress={() => {
                  uploadDirectoryRef.current = "";
                  uploadRef.current?.click();
                }}
              >
                上传文件
              </Button>
              <Button className="w-full" isPending={fileBusy} size="sm" variant="tertiary" onPress={() => void createFile()}>
                新建文件
              </Button>
              <input ref={uploadRef} multiple className="hidden" type="file" onChange={uploadFiles} />
            </div>
            <div className="flex-1 min-h-0 overflow-auto">
              <WorkerFileTree
                entries={files}
                selectedFile={selectedFile}
                mainFilename={mainFilename}
                revealPath={treeRevealPath}
                disabled={fileBusy}
                onSelectFile={setSelectedFile}
                onRequestUpload={(directory) => {
                  uploadDirectoryRef.current = directory;
                  uploadRef.current?.click();
                }}
                onRequestCreateFile={(directory) => void createFile(directory)}
                onRequestDelete={(entry) => void requestDeleteEntry(entry)}
              />
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
        isOpen={packageDeleteTarget !== ""}
        size="lg"
        header="删除 package.json 提示"
        isDismissable={!fileBusy}
        isKeyboardDismissDisabled={fileBusy}
        onOpenChange={(open) => {
          if (fileBusy) return;
          if (!open) setPackageDeleteTarget("");
        }}
        footer={
          <>
            <Button
              variant="secondary"
              isDisabled={fileBusy}
              onPress={() => setPackageDeleteTarget("")}
            >
              取消
            </Button>
            <Button
              variant="danger"
              isPending={fileBusy}
              onPress={() => {
                void (async () => {
                  await performDeleteEntry(packageDeleteTarget);
                  setPackageDeleteTarget("");
                })();
              }}
            >
              删除
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
        isOpen={directoryDeleteTarget !== ""}
        size="lg"
        header="删除目录"
        isDismissable={!fileBusy}
        isKeyboardDismissDisabled={fileBusy}
        onOpenChange={(open) => {
          if (fileBusy) return;
          if (!open) setDirectoryDeleteTarget("");
        }}
        footer={
          <>
            <Button
              variant="secondary"
              isDisabled={fileBusy}
              onPress={() => setDirectoryDeleteTarget("")}
            >
              取消
            </Button>
            <Button
              variant="danger"
              isPending={fileBusy}
              onPress={() => {
                void (async () => {
                  await performDeleteEntry(directoryDeleteTarget);
                  setDirectoryDeleteTarget("");
                })();
              }}
            >
              删除目录
            </Button>
          </>
        }
      >
        <p className="text-sm leading-6 text-default-700">
          删除目录 {directoryDeleteTarget} 会同时移除目录下的所有文件和目录，此操作无法撤销。
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
