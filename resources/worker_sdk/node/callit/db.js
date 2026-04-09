const http = require("node:http");
const https = require("node:https");
const { URL } = require("node:url");
const { normalizeNamespace } = require("./kv");

async function requestDBExec(payload) {
  const baseURL = (process.env.CALLIT_MAGIC_API_BASE_URL || "").replace(/\/$/, "");
  const workerID = process.env.CALLIT_WORKER_ID || "";
  const requestID = process.env.CALLIT_REQUEST_ID || "";
  if (!baseURL || !workerID || !requestID) {
    throw new Error("当前环境缺少 db 所需上下文");
  }

  const requestURL = new URL(`${baseURL}/db/exec`);
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
      reject(new Error(`请求 db 服务失败: ${error.message}`));
    });
    req.setTimeout(5000, () => {
      req.destroy(new Error("请求 db 服务超时"));
    });
    req.write(body);
    req.end();
  });

  const text = response.text.trim();
  if (!response.ok) {
    let message = text || `db 请求失败: ${response.status}`;
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

function ensureNonEmptyString(value, label) {
  if (typeof value !== "string" || !value.trim()) {
    throw new TypeError(`${label} 必须是非空 string`);
  }
  return value.trim();
}

function normalizeColumns(columns) {
  if (!columns.length) {
    return ["*"];
  }
  return columns.map((column) => ensureNonEmptyString(column, "列名"));
}

function normalizeValuesObject(values, label) {
  if (!values || typeof values !== "object" || Array.isArray(values)) {
    throw new TypeError(`${label} 必须是 object`);
  }
  const entries = Object.entries(values);
  if (!entries.length) {
    throw new TypeError(`${label} 不能为空`);
  }
  return entries.map(([key, value]) => [ensureNonEmptyString(key, "列名"), value]);
}

class BaseDBBuilder {
  constructor(client, table) {
    this.client = client;
    this.table = ensureNonEmptyString(table, "表名");
    this.whereClause = "";
    this.whereArgs = [];
    this.endSqlFragment = "";
  }

  getQualifiedTableName() {
    if (!this.client.namespace) {
      return this.table;
    }
    const namespacePrefix = `${this.client.namespace}_`;
    if (this.table.startsWith(namespacePrefix)) {
      return this.table;
    }
    return `${this.client.namespace}_${this.table}`;
  }

  where(condition, ...args) {
    this.whereClause = ensureNonEmptyString(condition, "where 条件");
    this.whereArgs = args;
    return this;
  }

  endSql(fragment) {
    this.endSqlFragment = ensureNonEmptyString(fragment, "endSql");
    return this;
  }

  buildTailParts() {
    const parts = [];
    if (this.whereClause) {
      parts.push(`where ${this.whereClause}`);
    }
    if (this.endSqlFragment) {
      parts.push(this.endSqlFragment);
    }
    return parts;
  }
}

class SelectBuilder extends BaseDBBuilder {
  constructor(client, table, columns) {
    super(client, table);
    this.columns = normalizeColumns(columns);
  }

  async exec() {
    const sql = [
      `select ${this.columns.join(", ")} from ${this.getQualifiedTableName()}`,
      ...this.buildTailParts(),
    ].join(" ");
    const result = await this.client.exec(sql, ...this.whereArgs);
    return result && Array.isArray(result.rows) ? result.rows : [];
  }
}

class InsertBuilder extends BaseDBBuilder {
  constructor(client, table) {
    super(client, table);
    this.insertValues = new Map();
  }

  values(values) {
    const entries = normalizeValuesObject(values, "insert.values");
    for (const [key, value] of entries) {
      this.insertValues.set(key, value);
    }
    return this;
  }

  async exec() {
    if (!this.insertValues.size) {
      throw new Error("insert.exec 前必须先调用 values()");
    }
    const columns = Array.from(this.insertValues.keys());
    const args = Array.from(this.insertValues.values());
    const placeholders = columns.map(() => "?").join(", ");
    const parts = [
      `insert into ${this.getQualifiedTableName()}(${columns.join(", ")}) values(${placeholders})`,
    ];
    if (this.endSqlFragment) {
      parts.push(this.endSqlFragment);
    }
    const result = await this.client.exec(parts.join(" "), ...args);
    return result && typeof result.last_insert_id === "number" ? result.last_insert_id : 0;
  }
}

class UpdateBuilder extends BaseDBBuilder {
  constructor(client, table) {
    super(client, table);
    this.updateAssignments = new Map();
  }

  set(column, value) {
    this.updateAssignments.set(ensureNonEmptyString(column, "列名"), value);
    return this;
  }

  values(values) {
    const entries = normalizeValuesObject(values, "update.values");
    for (const [key, value] of entries) {
      this.updateAssignments.set(key, value);
    }
    return this;
  }

  async exec() {
    if (!this.updateAssignments.size) {
      throw new Error("update.exec 前必须至少调用一次 set() 或 values()");
    }
    const setClauses = [];
    const args = [];
    for (const [column, value] of this.updateAssignments.entries()) {
      setClauses.push(`${column} = ?`);
      args.push(value);
    }
    const sql = [
      `update ${this.getQualifiedTableName()} set ${setClauses.join(", ")}`,
      ...this.buildTailParts(),
    ].join(" ");
    const result = await this.client.exec(sql, ...args, ...this.whereArgs);
    return result && typeof result.rows_affected === "number" ? result.rows_affected : 0;
  }
}

class DeleteBuilder extends BaseDBBuilder {
  async exec() {
    const sql = [`delete from ${this.getQualifiedTableName()}`, ...this.buildTailParts()].join(" ");
    const result = await this.client.exec(sql, ...this.whereArgs);
    return result && typeof result.rows_affected === "number" ? result.rows_affected : 0;
  }
}

function createDBClient(namespace) {
  return {
    namespace: normalizeNamespace(namespace),
    async exec(sql, ...args) {
      if (typeof sql !== "string" || !sql.trim()) {
        throw new TypeError("db client 的 sql 必须是非空 string");
      }
      return (await requestDBExec({ sql, args })) || null;
    },
    select(table, ...columns) {
      return new SelectBuilder(this, table, columns);
    },
    insert(table) {
      return new InsertBuilder(this, table);
    },
    update(table) {
      return new UpdateBuilder(this, table);
    },
    delete(table) {
      return new DeleteBuilder(this, table);
    },
  };
}

const db = {
  newClient(namespace) {
    return createDBClient(namespace);
  },
};

module.exports = {
  db,
};
