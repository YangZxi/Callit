const fs = require("fs");
const path = require("path");

const RES_SEPARATOR = "\n**=====^=====**\n";
const LOG_SEPARATOR = "\n===============\n";

const raw = fs.readFileSync(0, "utf8");
const ctx = raw ? JSON.parse(raw) : {};

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

process.stdout.write(LOG_SEPARATOR);

Promise.resolve(loadHandler()(ctx)).then((out) => {
	process.stdout.write(RES_SEPARATOR);
	process.stdout.write(JSON.stringify(out));
	process.stdout.write(RES_SEPARATOR);
}).catch((err) => {
	console.error(err && err.stack ? err.stack : String(err));
	process.exit(1);
});
process.stdout.write(LOG_SEPARATOR.trim() + "\n\n");
