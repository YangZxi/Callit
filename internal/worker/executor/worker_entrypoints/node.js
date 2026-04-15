const fs = require("fs");
const path = require("path");
const callit = require("callit");

const RES_SEPARATOR = "\n**=====^=====**\n";
const LOG_SEPARATOR = "\n===============\n";

const raw = fs.readFileSync(0, "utf8");
const ctx = raw ? JSON.parse(raw) : {};
let hasFinished = false;
let hasWrittenResult = false;
let hasClosedLogBlock = false;

function loadHandler() {
	const entryPath = path.join(process.cwd(), "main.js");
	const loaded = require(entryPath);

	if (typeof loaded === "function") {
		return loaded;
	}
	if (loaded && typeof loaded.handler === "function") {
		return loaded.handler;
	}

	throw new Error("main.js 必须导出 handler(ctx) 或直接导出函数");
}

function writeResult(out) {
	if (hasWrittenResult) {
		return;
	}
	hasWrittenResult = true;
	process.stdout.write(RES_SEPARATOR);
	process.stdout.write(JSON.stringify(out));
	process.stdout.write(RES_SEPARATOR);
}

function closeLogBlock() {
	if (hasClosedLogBlock) {
		return;
	}
	hasClosedLogBlock = true;
	process.stdout.write(LOG_SEPARATOR.trim() + "\n\n");
}

process.once("beforeExit", () => {
	if (!hasFinished && !hasWrittenResult) {
		// Worker 在没有正常结束的情况下退出
		// 可能是 Worker 中有出于 Pending 的 Promise 导致的
		const message = "Worker 未正常返回结果";
		console.error(message);
		writeResult({
			status: 500,
			body: {},
		});
	}
	closeLogBlock();
});

process.stdout.write(LOG_SEPARATOR);
Promise.resolve()
	.then(() => loadHandler()(ctx))
	.then((out) => {
		hasFinished = true;
		writeResult(out);
	})
	.catch((err) => {
		hasFinished = true;
		console.error(err && err.stack ? err.stack : String(err));
		process.exitCode = 1;
	})
	.finally(() => {
		closeLogBlock();
	});
