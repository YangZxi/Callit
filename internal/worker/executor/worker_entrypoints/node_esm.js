const fs = require("fs");
const path = require("path");
const { pathToFileURL } = require("url");

const RES_SEPARATOR = "\n**=====^=====**\n";
const LOG_SEPARATOR = "\n===============\n";

const raw = fs.readFileSync(0, "utf8");
const ctx = raw ? JSON.parse(raw) : {};
let hasFinished = false;
let hasWrittenResult = false;
let hasClosedLogBlock = false;

async function loadHandler() {
	const entryPath = path.join(process.cwd(), "main.js");
	const loaded = await import(pathToFileURL(entryPath).href);

	if (loaded && typeof loaded.default === "function") {
		return loaded.default;
	}
	if (loaded && typeof loaded.handler === "function") {
		return loaded.handler;
	}

	throw new Error("main.js 必须导出默认 handler(ctx) 或命名导出 handler(ctx)");
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
		console.error("Worker 未正常返回结果");
		writeResult({
			status: 500,
			body: {},
		});
	}
	closeLogBlock();
});

process.stdout.write(LOG_SEPARATOR);
Promise.resolve()
	.then(async () => (await loadHandler())(ctx))
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
