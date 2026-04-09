const http = require("node:http");
const https = require("node:https");
const { URL } = require("node:url");

async function request(action, payload) {
  const baseURL = (process.env.CALLIT_MAGIC_API_BASE_URL || "").replace(/\/$/, "");
  const workerID = process.env.CALLIT_WORKER_ID || "";
  const requestID = process.env.CALLIT_REQUEST_ID || "";
  if (!baseURL || !workerID || !requestID) {
    throw new Error("当前环境缺少 kv 所需上下文");
  }

  const requestURL = new URL(`${baseURL}/kv/${action}`);
  const body = JSON.stringify(payload);
  const transport = requestURL.protocol === "https:" ? https : http;

  const response = await new Promise((resolve, reject) => {
    const req = transport.request({
      protocol: requestURL.protocol,
      hostname: requestURL.hostname,
      port: requestURL.port,
      path: `${requestURL.pathname}${requestURL.search}`,
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Content-Length": Buffer.byteLength(body),
        "X-Callit-Worker-Id": workerID,
        "X-Callit-Request-Id": requestID,
      },
    }, (res) => {
      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => {
        resolve({
          ok: (res.statusCode || 0) >= 200 && (res.statusCode || 0) < 300,
          status: res.statusCode || 0,
          text: Buffer.concat(chunks).toString("utf-8"),
        });
      });
    });

    req.on("error", (error) => {
      reject(new Error(`请求 kv 服务失败: ${error.message}`));
    });
    req.setTimeout(5000, () => {
      req.destroy(new Error("请求 kv 服务超时"));
    });
    req.write(body);
    req.end();
  });

  const text = response.text.trim();
  if (!response.ok) {
    let message = text || `kv 请求失败: ${response.status}`;
    try {
      const parsed = text ? JSON.parse(text) : {};
      if (parsed && parsed.error) {
        message = parsed.error;
      }
    } catch (_) {
      // ignore
    }
    throw new Error(message);
  }
  return text ? JSON.parse(text) : null;
}

function normalizeNamespace(namespace) {
  const workerID = (process.env.CALLIT_WORKER_ID || "").trim();
  if (!workerID) {
    throw new Error("当前环境缺少 CALLIT_WORKER_ID");
  }
  if (namespace === null || namespace === undefined) {
    return workerID;
  }
  if (typeof namespace !== "string") {
    throw new TypeError("kv.newClient 的 namespace 必须是 string、null 或 undefined");
  }
  const normalizedNamespace = namespace.trim();
  if (!normalizedNamespace) {
    return workerID;
  }
  return normalizedNamespace;
}

function createKVClient(namespace) {
  const normalizedNamespace = normalizeNamespace(namespace);

  return {
    async set(key, value, seconds) {
      if (typeof value !== "string") {
        throw new TypeError("kv client 的 value 必须是 string");
      }
      await request("set", {
        namespace: normalizedNamespace,
        key,
        value,
        seconds,
      });
    },
    async get(key) {
      const response = (await request("get", { namespace: normalizedNamespace, key })) || {};
      return response.value;
    },
    async delete(key) {
      await request("delete", { namespace: normalizedNamespace, key });
    },
    async increment(key, step = 1) {
      const response = (await request("increment", { namespace: normalizedNamespace, key, step })) || {};
      return Number.parseInt(String(response.int_value || 0), 10);
    },
    async expire(key, seconds) {
      await request("expire", { namespace: normalizedNamespace, key, seconds });
    },
    async ttl(key) {
      const response = (await request("ttl", { namespace: normalizedNamespace, key })) || {};
      return Number.parseInt(String(response.seconds || 0), 10);
    },
    async has(key) {
      const response = (await request("has", { namespace: normalizedNamespace, key })) || {};
      return Boolean(response.exists);
    },
  };
}

const kv = {
  newClient(namespace) {
    return createKVClient(namespace);
  },
};

module.exports = {
  kv,
  normalizeNamespace,
};
