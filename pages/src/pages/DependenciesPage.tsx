import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	Modal,
	Tabs,
	toast,
} from "@heroui/react";
import { Button } from "@heroui/react";
import { Input } from "@/components/heroui";

import api, { BASE_API } from "@/lib/api";

type RuntimeType = "node" | "python";
type ManageAction = "install" | "remove";
type ModalMode = "manage" | "rebuild";

type DependencyInfo = {
	name: string;
	version: string;
};

type SSEEvent = {
	event: string;
	data: any;
};

async function streamDependencyManage(
	payload: { runtime: RuntimeType; action: ManageAction; package: string },
	onEvent: (event: SSEEvent) => void,
) {
	const res = await fetch(`${BASE_API}/dependencies/manage`, {
		method: "POST",
		credentials: "include",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(payload),
	});

	if (!res.ok) {
		const json = await res.json().catch(() => null);
		const msg = json?.msg || json?.error || "依赖管理请求失败";
		throw new Error(msg);
	}
	if (!res.body) {
		throw new Error("SSE 连接失败");
	}

	const reader = res.body.getReader();
	const decoder = new TextDecoder("utf-8");
	let buffer = "";

	const parseSSEChunk = (rawChunk: string): SSEEvent | null => {
		const lines = rawChunk
			.split("\n")
			.map((line) => line.trimEnd())
			.filter((line) => line.length > 0);
		if (lines.length === 0) return null;

		let event = "message";
		const dataLines: string[] = [];
		for (const line of lines) {
			if (line.startsWith("event:")) {
				event = line.slice("event:".length).trim();
				continue;
			}
			if (line.startsWith("data:")) {
				dataLines.push(line.slice("data:".length).trim());
			}
		}
		if (dataLines.length === 0) return null;

		const text = dataLines.join("\n");
		try {
			return { event, data: JSON.parse(text) };
		} catch {
			return { event, data: { text } };
		}
	};

	while (true) {
		const { done, value } = await reader.read();
		buffer += decoder.decode(value || new Uint8Array(), { stream: !done });

		let splitAt = buffer.indexOf("\n\n");
		while (splitAt >= 0) {
			const chunk = buffer.slice(0, splitAt).replace(/\r/g, "");
			buffer = buffer.slice(splitAt + 2);
			const event = parseSSEChunk(chunk);
			if (event) onEvent(event);
			splitAt = buffer.indexOf("\n\n");
		}
		if (done) break;
	}

	const tail = buffer.trim();
	if (tail) {
		const event = parseSSEChunk(tail);
		if (event) onEvent(event);
	}
}

async function streamDependencyRebuild(
	runtime: RuntimeType,
	onEvent: (event: SSEEvent) => void,
) {
	const res = await fetch(`${BASE_API}/dependencies/rebuild?runtime=${runtime}`, {
		method: "POST",
		credentials: "include",
	});

	if (!res.ok) {
		const json = await res.json().catch(() => null);
		const msg = json?.msg || json?.error || "环境重建请求失败";
		throw new Error(msg);
	}
	if (!res.body) {
		throw new Error("SSE 连接失败");
	}

	const reader = res.body.getReader();
	const decoder = new TextDecoder("utf-8");
	let buffer = "";

	const parseSSEChunk = (rawChunk: string): SSEEvent | null => {
		const lines = rawChunk
			.split("\n")
			.map((line) => line.trimEnd())
			.filter((line) => line.length > 0);
		if (lines.length === 0) return null;

		let event = "message";
		const dataLines: string[] = [];
		for (const line of lines) {
			if (line.startsWith("event:")) {
				event = line.slice("event:".length).trim();
				continue;
			}
			if (line.startsWith("data:")) {
				dataLines.push(line.slice("data:".length).trim());
			}
		}
		if (dataLines.length === 0) return null;

		const text = dataLines.join("\n");
		try {
			return { event, data: JSON.parse(text) };
		} catch {
			return { event, data: { text } };
		}
	};

	while (true) {
		const { done, value } = await reader.read();
		buffer += decoder.decode(value || new Uint8Array(), { stream: !done });

		let splitAt = buffer.indexOf("\n\n");
		while (splitAt >= 0) {
			const chunk = buffer.slice(0, splitAt).replace(/\r/g, "");
			buffer = buffer.slice(splitAt + 2);
			const event = parseSSEChunk(chunk);
			if (event) onEvent(event);
			splitAt = buffer.indexOf("\n\n");
		}
		if (done) break;
	}

	const tail = buffer.trim();
	if (tail) {
		const event = parseSSEChunk(tail);
		if (event) onEvent(event);
	}
}

export default function DependenciesPage() {
	const [runtime, setRuntime] = useState<RuntimeType>("node");
	const [dependencies, setDependencies] = useState<DependencyInfo[]>([]);
	const [loading, setLoading] = useState(false);

	const [modalOpen, setModalOpen] = useState(false);
	const [action, setAction] = useState<ManageAction>("install");
	const [modalMode, setModalMode] = useState<ModalMode>("manage");
	const [packageName, setPackageName] = useState("");
	const [running, setRunning] = useState(false);
	const [logs, setLogs] = useState<string[]>([]);
	const loadSeqRef = useRef(0);

	const title = useMemo(() => {
		return runtime === "node" ? "Node 依赖" : "Python 依赖";
	}, [runtime]);

	const loadDependencies = useCallback(async (target: RuntimeType) => {
		const seq = ++loadSeqRef.current;
		setLoading(true);
		try {
			const data = await api.get<DependencyInfo[]>(`/dependencies?runtime=${target}`);
			if (seq !== loadSeqRef.current) return;
			setDependencies(data || []);
		} finally {
			if (seq === loadSeqRef.current) {
				setLoading(false);
			}
		}
	}, []);

	useEffect(() => {
		void loadDependencies(runtime);
	}, [runtime, loadDependencies]);

	const openInstallModal = () => {
		setModalMode("manage");
		setAction("install");
		setPackageName("");
		setLogs([]);
		setModalOpen(true);
	};

	const openRemoveModal = (name: string) => {
		setModalMode("manage");
		setAction("remove");
		setPackageName(name);
		setLogs([]);
		setModalOpen(true);
	};

	const openRebuildModal = () => {
		if (runtime === "node") {
			toast.warning("node 环境暂不支持");
			return;
		}
		setModalMode("rebuild");
		setPackageName("");
		setLogs([]);
		setModalOpen(true);
	};

	const appendLog = (line: string) => {
		setLogs((prev) => [...prev, line]);
	};

	const doManage = async () => {
		if (running) return;
		const clean = packageName.trim();
		if (!clean) {
			toast.warning("依赖名不能为空");
			return;
		}

		setRunning(true);
		setLogs([]);

		let doneOK = false;
		try {
			await streamDependencyManage(
				{ runtime, action, package: clean },
				(event) => {
					if (event.event === "log") {
						const stream = event.data?.stream === "stderr" ? "stderr" : "stdout";
						const text = typeof event.data?.text === "string" ? event.data.text : "";
						if (text) appendLog(`[${stream}] ${text}`);
						return;
					}
					if (event.event === "error") {
						const msg = event.data?.message || "执行失败";
						appendLog(`[error] ${msg}`);
						return;
					}
					if (event.event === "done") {
						doneOK = event.data?.ok === true;
					}
				},
			);

			if (doneOK) {
				toast.success(action === "install" ? "依赖安装完成" : "依赖移除完成");
				await loadDependencies(runtime);
			} else {
				toast.danger("执行失败，请查看日志");
			}
		} catch (err) {
			const msg = err instanceof Error ? err.message : "依赖管理失败";
			appendLog(`[error] ${msg}`);
			toast.danger(msg);
		} finally {
			setRunning(false);
		}
	};

	const doRebuild = async () => {
		if (running) return;

		setRunning(true);
		setLogs([]);

		let doneOK = false;
		try {
			await streamDependencyRebuild(runtime, (event) => {
				if (event.event === "log") {
					const stream = event.data?.stream === "stderr" ? "stderr" : "stdout";
					const text = typeof event.data?.text === "string" ? event.data.text : "";
					if (text) appendLog(`[${stream}] ${text}`);
					return;
				}
				if (event.event === "error") {
					const msg = event.data?.message || "环境重建失败";
					appendLog(`[error] ${msg}`);
					return;
				}
				if (event.event === "done") {
					doneOK = event.data?.ok === true;
				}
			});

			if (doneOK) {
				toast.success("环境重建完成");
				await loadDependencies(runtime);
			} else {
				toast.danger("环境重建失败，请查看日志");
			}
		} catch (err) {
			const msg = err instanceof Error ? err.message : "环境重建失败";
			appendLog(`[error] ${msg}`);
			toast.danger(msg);
		} finally {
			setRunning(false);
		}
	};

	const modalHeading = modalMode === "rebuild"
		? "重建环境"
		: (action === "install" ? "安装依赖" : "移除依赖");

	return (
		<section className="pb-4 md:pb-2">
			<div className="flex items-center justify-between gap-3">
				<div className="flex flex-col gap-1">
					<h1 className="text-3xl font-semibold text-default-900 dark:text-default-700">Dependencies</h1>
					<p className="text-sm text-default-500">{title}</p>
				</div>
				<div className="flex items-center gap-2">
					<Button variant="secondary" onPress={openRebuildModal}>重建环境</Button>
					<Button variant="primary" onPress={openInstallModal}>安装依赖</Button>
				</div>
			</div>

			<div className="mt-4 rounded-xl border border-default-200 p-4">
				<Tabs
					aria-label="Dependencies Tabs"
					className="w-[200px]"
					orientation="horizontal"
					selectedKey={runtime}
					onSelectionChange={(key) => {
						if (key === "node" || key === "python") {
							setRuntime(key);
						}
					}}
				>
					<Tabs.ListContainer>
						<Tabs.List aria-label="Dependencies Tabs">
							<Tabs.Tab id="node">
								Node
								<Tabs.Indicator />
							</Tabs.Tab>
							<Tabs.Tab id="python">
								Python
								<Tabs.Indicator />
							</Tabs.Tab>
						</Tabs.List>
					</Tabs.ListContainer>
				</Tabs>

				<div className="mt-4">
					{loading ? <p className="text-sm text-default-500">正在加载依赖列表...</p> : null}
					{!loading && dependencies.length === 0 ? (
						<div className="rounded-xl border border-default-200 p-4 text-sm text-default-500">
							当前环境暂无可管理依赖。
						</div>
					) : null}
					{!loading && dependencies.length > 0 ? (
						<div className="flex flex-col gap-2">
							{dependencies.map((item) => (
								<div key={item.name} className="flex items-center justify-between rounded-xl border border-default-200 p-3">
									<div className="min-w-0">
										<p className="truncate font-medium text-default-800">{item.name}</p>
										<p className="text-xs text-default-500">{item.version || "-"}</p>
									</div>
									<Button size="sm" variant="danger-soft" onPress={() => openRemoveModal(item.name)}>
										移除
									</Button>
								</div>
							))}
						</div>
					) : null}
				</div>
			</div>

			<Modal
				isOpen={modalOpen}
				onOpenChange={(open) => {
					if (running) return;
					setModalOpen(open);
				}}
			>
				<Modal.Backdrop isDismissable={false} isKeyboardDismissDisabled={running}>
					<Modal.Container>
						<Modal.Dialog>
							<Modal.Header>
								<Modal.Heading>{modalHeading}</Modal.Heading>
							</Modal.Header>
							<Modal.Body>
								{modalMode === "manage" ? (
									<div className="mx-[2px] flex items-end gap-2">
										<Input
											className="flex-1"
											isDisabled={running || action === "remove"}
											isRequired
											label="依赖名"
											name="package-name"
											placeholder={runtime === "node" ? "例如 lodash" : "例如 requests"}
											value={packageName}
											onValueChange={setPackageName}
										/>
										<Button variant={action === "install" ? "primary" : "danger"} isPending={running} onPress={doManage}>
											{action === "install" ? "安装" : "移除"}
										</Button>
									</div>
								) : (
									<div className="rounded-lg border border-default-200 bg-default-50 px-3 py-2 text-sm text-default-600">
										将删除当前 Python venv，并根据 requirements.txt 重建环境。
									</div>
								)}
								<div className="mt-2 h-64 overflow-auto rounded-lg border border-default-200 bg-default-100 p-3 font-mono text-xs whitespace-pre-wrap break-words">
									{logs.length > 0 ? logs.join("\n") : "等待执行..."}
								</div>
							</Modal.Body>
							<Modal.Footer>
								{modalMode === "rebuild" ? (
									<Button variant="primary" isPending={running} onPress={doRebuild}>
										开始重建
									</Button>
								) : null}
								<Button
									variant="secondary"
									onPress={() => {
										if (running) return;
										setModalOpen(false);
									}}
								>
									关闭
								</Button>
							</Modal.Footer>
						</Modal.Dialog>
					</Modal.Container>
				</Modal.Backdrop>
			</Modal>
		</section>
	);
}
