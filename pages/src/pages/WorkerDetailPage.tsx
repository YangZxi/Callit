import { ChangeEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	Button,
	Chip,
	Listbox,
	ListboxItem,
	Modal,
	ModalBody,
	ModalContent,
	ModalFooter,
	ModalHeader,
	Pagination,
	addToast,
} from "@heroui/react";
import Editor, { DiffEditor, OnMount } from "@monaco-editor/react";
import { useNavigate, useParams } from "react-router-dom";

import api from "@/lib/api";
import Chatbox from "@/components/chatbox/chatbox";
import EditorPanelAction from "@/components/editor-panel-action";
import HttpDrawer from "@/components/http-drawer";
import { useTheme } from "@/lib/theme";

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

type FileContentResp = {
	filename: string;
	content: string;
	media_type?: "text" | "image";
	mime_type?: string;
	preview_data_url?: string;
};

type WorkerLogItem = {
	id: string;
	worker_id: string;
	request_id: string;
	status: number;
	stdout: string;
	stderr: string;
	error: string;
	duration_ms: number;
	created_at: string;
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

function formatLogTime(raw: string): string {
	const dt = new Date(raw);
	if (Number.isNaN(dt.getTime())) {
		return raw;
	}
	return dt.toLocaleString();
}

export default function WorkerDetailPage() {
	const navigate = useNavigate();
	const { id = "" } = useParams();
	const { isDark } = useTheme();
	const uploadRef = useRef<HTMLInputElement | null>(null);
	const saveCurrentFileRef = useRef<(() => Promise<void>) | null>(null);

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
	const [httpDrawerOpen, setHttpDrawerOpen] = useState(false);
	const [logModalOpen, setLogModalOpen] = useState(false);
	const [logLoading, setLogLoading] = useState(false);
	const [logPage, setLogPage] = useState(1);
	const [logTotal, setLogTotal] = useState(0);
	const [logItems, setLogItems] = useState<WorkerLogItem[]>([]);
	const [expandedLogIds, setExpandedLogIds] = useState<Set<string>>(new Set());

	const selectedKeys = useMemo(() => (selectedFile ? new Set<string>([selectedFile]) : new Set<string>()), [selectedFile]);
	const defaultRunUrl = useMemo(() => {
		if (!workerInfo) return "";
		return `${window.location.protocol}//${window.location.host}${workerInfo.route}`;
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
		return api.get<FileContentResp>(`/workers/${id}/files/content?filename=${target}`, options);
	}, [id]);

	const loadFiles = useCallback(async (prefer?: string) => {
		const data = await api.get<{ files: string[] }>(`/workers/${id}/files`);
		const nextAllFiles = data.files || [];
		const visibleFiles = nextAllFiles.filter((filename) => !/\.diff(?:\.[^./\\]+)?$/.test(filename));
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
			addToast({ title: "缺少 WorkerId", color: "danger", variant: "flat", timeout: 2500 });
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
			addToast({ title: "当前为 Diff 模式，请使用“应用并清理 diff”", color: "warning", variant: "flat", timeout: 2200 });
			return;
		}
		if (fileMediaType === "image") {
			addToast({ title: "图片预览文件不支持在线编辑", color: "warning", variant: "flat", timeout: 2200 });
			return;
		}
		setSaving(true);
		try {
			const target = encodeURIComponent(selectedFile);
			await api.put<{ files: string[] }>(`/workers/${id}/files/content?filename=${target}`, { content });
			addToast({ title: "保存成功", color: "success", variant: "flat", timeout: 1800 });
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
			const originTarget = encodeURIComponent(selectedFile);
			await api.put<{ files: string[] }>(`/workers/${id}/files/content?filename=${originTarget}`, { content });
			const diffTarget = encodeURIComponent(activeDiffFile);
			await api.delete<{ ok: boolean }>(`/workers/${id}/files?filename=${diffTarget}`);
			addToast({ title: "合并完成", color: "success", variant: "flat", timeout: 1800 });
			await loadFiles(selectedFile);
			await loadFileContent(selectedFile);
		} finally {
			setSaving(false);
		}
	}, [activeDiffFile, selectedFile, saving, fileBusy, id, content, loadFiles, loadFileContent]);

	useEffect(() => {
		saveCurrentFileRef.current = isDiffMode ? applyDiffMerge : saveCurrentFile;
	}, [isDiffMode, applyDiffMerge, saveCurrentFile]);

	const handleEditorMount: OnMount = (editor, monaco) => {
		editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
			void saveCurrentFileRef.current?.();
		});
	};

	const handleDiffEditorMount = (editor: any, monaco: any) => {
		const modifiedEditor = editor.getModifiedEditor();
		modifiedEditor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
			void saveCurrentFileRef.current?.();
		});
	};

	const createFile = async () => {
		const filename = window.prompt("输入新文件名（例如 helper.py）", "");
		if (!filename) return;
		const clean = filename.trim();
		if (!clean) return;

		setFileBusy(true);
		try {
			const target = encodeURIComponent(clean);
			await api.put<{ files: string[] }>(`/workers/${id}/files/content?filename=${target}`, { content: "" });
			await loadFiles(clean);
			await loadFileContent(clean);
		} finally {
			setFileBusy(false);
		}
	};

	const deleteCurrentFile = async () => {
		if (!selectedFile || fileBusy) return;
		const ok = window.confirm(`确认删除文件 ${selectedFile} ？`);
		if (!ok) return;

		setFileBusy(true);
		try {
			const target = encodeURIComponent(selectedFile);
			await api.delete<{ ok: boolean }>(`/workers/${id}/files?filename=${target}`);
			await loadFiles();
		} finally {
			setFileBusy(false);
		}
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
			addToast({ title: "重命名成功", color: "success", variant: "flat", timeout: 1800 });
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
			const data = await api.post<{ files: string[] }>(`/workers/${id}/files`, formData);
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
			const data = await api.post<WorkerItem>(`/workers/${id}/${nextPath}`);
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
			await api.delete<{ ok: boolean }>(`/workers/${id}`);
			navigate("/workers", { replace: true });
		} finally {
			setFnBusy(false);
		}
	};

	const openWorkerLogsModal = () => {
		setLogPage(1);
		setExpandedLogIds(new Set());
		setLogModalOpen(true);
	};

	const toggleLogDetails = (logID: string) => {
		setExpandedLogIds((prev) => {
			const next = new Set(prev);
			if (next.has(logID)) {
				next.delete(logID);
			} else {
				next.add(logID);
			}
			return next;
		});
	};

	const handleChatFilesChanged = useCallback(async () => {
		await loadFiles(selectedFile || undefined);
		if (selectedFile) {
			await loadFileContent(selectedFile);
		}
	}, [loadFiles, loadFileContent, selectedFile]);

	if (loading) {
		return <section className="py-6 text-sm text-default-500">正在加载 Worker 详情...</section>;
	}

	if (!workerInfo) {
		return <section className="py-6 text-sm text-danger">Worker 不存在或已删除</section>;
	}

	return (
		<section className="relative h-[calc(var(--main-height)-36px)] py-2">
			<div className="flex h-full flex-col overflow-hidden rounded-xl border border-default-200 bg-background/70">
				<div className="border-b border-default-200 p-2">
					<div className="flex items-center justify-between gap-3">
						<div className="min-w-0">
							<div className="flex items-center gap-2">
								<h1 className="truncate text-base font-semibold text-default-900">{workerInfo.name}</h1>
								<Chip color={workerInfo.enabled ? "success" : "default"} size="sm" variant="flat">
									{workerInfo.enabled ? "已启用" : "已停用"}
								</Chip>
							</div>
							<p className="truncate text-xs text-default-500">
								{workerInfo.route} · {workerInfo.runtime}
							</p>
						</div>
						<div className="flex items-center justify-end gap-2">
							<Button color="primary" size="sm" variant="flat" onPress={() => setHttpDrawerOpen(true)}>Testing</Button>
							<Button color="secondary" size="sm" variant="flat" onPress={openWorkerLogsModal}>运行日志</Button>
							<Button color="default" size="sm" variant="flat" onPress={() => navigate("/workers")}>返回列表</Button>
							<Button color={workerInfo.enabled ? "warning" : "success"} isLoading={fnBusy} size="sm" variant="flat" onPress={toggleWorkerEnabled}>
								{workerInfo.enabled ? "停用 Worker" : "启用 Worker"}
							</Button>
							<Button color="danger" isLoading={fnBusy} size="sm" variant="flat" onPress={removeWorker}>删除 Worker</Button>
						</div>
					</div>
				</div>
				<div className="flex min-h-0 flex-1">
					<aside className="flex min-h-0 w-[150px] shrink-0 flex-col border-r border-default-200">
						<div className="p-2 border-b border-default-200 flex flex-col gap-2">
							<Button color="default" size="sm" variant="flat" onPress={() => uploadRef.current?.click()}>
								上传文件
							</Button>
							<Button color="default" isLoading={fileBusy} size="sm" variant="flat" onPress={createFile}>
								新建文件
							</Button>
							<input ref={uploadRef} multiple className="hidden" type="file" onChange={uploadFiles} />
						</div>
						<div className="flex-1 min-h-0 overflow-auto">
							<Listbox
								aria-label="Worker 文件列表"
								className="p-1"
								disallowEmptySelection
								selectedKeys={selectedKeys}
								selectionMode="single"
								onSelectionChange={(keys) => {
									if (keys === "all") return;
									const first = Array.from(keys)[0];
									if (typeof first === "string") {
										setSelectedFile(first);
									}
								}}
							>
								{files.map((file) => (
									<ListboxItem key={file}>{file}</ListboxItem>
								))}
							</Listbox>
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
									<Button
										color="warning"
										size="sm"
										variant="flat"
										isDisabled={saving || fileBusy}
										onPress={applyDiffMerge}
									>
										应用并清理 diff
									</Button>
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
										onChange={(value: string | undefined) => setContent(value ?? "")}
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
			<HttpDrawer
				isOpen={httpDrawerOpen}
				onClose={() => setHttpDrawerOpen(false)}
				defaultUrl={defaultRunUrl}
			/>
			<Modal
				isOpen={logModalOpen}
				size="5xl"
				onOpenChange={(open) => {
					setLogModalOpen(open);
					if (!open) {
						setExpandedLogIds(new Set());
					}
				}}
			>
				<ModalContent>
					{(close) => (
						<>
							<ModalHeader>最近运行日志</ModalHeader>
							<ModalBody>
								{logLoading ? (
									<p className="text-sm text-default-500">正在加载运行日志...</p>
								) : null}
								{!logLoading && logItems.length === 0 ? (
									<div className="rounded-lg border border-default-200 px-3 py-4 text-sm text-default-500">
										暂无运行日志
									</div>
								) : null}
								{!logLoading && logItems.length > 0 ? (
									<div className="flex max-h-[60vh] flex-col gap-2 overflow-auto pr-1">
										{logItems.map((item) => {
											const expanded = expandedLogIds.has(item.id);
											return (
												<div key={item.id} className="rounded-lg border border-default-200 p-3">
													<div className="flex items-start justify-between gap-3">
														<div className="min-w-0">
															<p className="truncate text-sm font-medium text-default-800">
																request_id: {item.request_id}
															</p>
															<p className="truncate text-xs text-default-500">
																{formatLogTime(item.created_at)} · 状态 {item.status} · 耗时 {item.duration_ms}ms
															</p>
														</div>
														<Button color="default" size="sm" variant="flat" onPress={() => toggleLogDetails(item.id)}>
															{expanded ? "收起详情" : "展开详情"}
														</Button>
													</div>
													{expanded ? (
														<div className="mt-3 space-y-2">
															{item.error ? (
																<div>
																	<p className="mb-1 text-xs font-medium text-danger">error</p>
																	<pre className="max-h-40 overflow-auto rounded-md bg-danger-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-danger-700">{item.error}</pre>
																</div>
															) : null}
															{item.stderr ? (
																<div>
																	<p className="mb-1 text-xs font-medium text-warning">stderr</p>
																	<pre className="max-h-40 overflow-auto rounded-md bg-warning-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-warning-700">{item.stderr}</pre>
																</div>
															) : null}
															{item.stdout ? (
																<div>
																	<p className="mb-1 text-xs font-medium text-success">stdout</p>
																	<pre className="max-h-40 overflow-auto rounded-md bg-success-50 p-2 font-mono text-xs whitespace-pre-wrap break-words text-success-700">{item.stdout}</pre>
																</div>
															) : null}
															{!item.error && !item.stderr && !item.stdout ? (
																<p className="text-xs text-default-400">无详细输出</p>
															) : null}
														</div>
													) : null}
												</div>
											);
										})}
									</div>
								) : null}
							</ModalBody>
							<ModalFooter className="flex items-center justify-between">
								<div>
									{logTotal > workerLogsPageSize ? (
										<Pagination
											showControls
											color="success"
											page={logPage}
											total={logTotalPages}
											onChange={setLogPage}
										/>
									) : null}
								</div>
								<Button variant="flat" onPress={close}>关闭</Button>
							</ModalFooter>
						</>
					)}
				</ModalContent>
			</Modal>
			<div className="absolute z-10000 bottom-4 right-4 z-40">
				<Chatbox workerId={id} onFilesChanged={handleChatFilesChanged} />
			</div>
		</section>
	);
}
