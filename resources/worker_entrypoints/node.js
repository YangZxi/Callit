const fs = require("fs");
const vm = require("vm");
const path = require("path");

const raw = fs.readFileSync(0, "utf8");
const ctx = raw ? JSON.parse(raw) : {};
const source = fs.readFileSync(path.join(process.cwd(), "main.js"), "utf8");

const sandbox = {
	module: { exports: {} },
	exports: {},
	require,
	console,
	setTimeout,
	clearTimeout,
	Promise,
};

vm.createContext(sandbox);
vm.runInContext(source, sandbox, { filename: "main.js" });

let handler = null;
if (typeof sandbox.handler === "function") {
	handler = sandbox.handler;
} else if (typeof sandbox.module.exports === "function") {
	handler = sandbox.module.exports;
} else if (sandbox.module.exports && typeof sandbox.module.exports.handler === "function") {
	handler = sandbox.module.exports.handler;
} else if (sandbox.exports && typeof sandbox.exports.handler === "function") {
	handler = sandbox.exports.handler;
}

if (typeof handler !== "function") {
	throw new Error("main.js 必须定义 handler(ctx) 或导出 handler");
}

Promise.resolve(handler(ctx)).then((out) => {
	process.stdout.write(JSON.stringify(out));
}).catch((err) => {
	console.error(err && err.stack ? err.stack : String(err));
	process.exit(1);
});
