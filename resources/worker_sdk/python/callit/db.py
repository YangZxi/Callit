import json
import os
import urllib.error
import urllib.request

from .kv import normalize_namespace


def _request_db_exec(payload):
    base_url = (os.environ.get("CALLIT_MAGIC_API_BASE_URL") or "").rstrip("/")
    worker_id = os.environ.get("CALLIT_WORKER_ID") or ""
    request_id = os.environ.get("CALLIT_REQUEST_ID") or ""
    if not base_url or not worker_id or not request_id:
        raise RuntimeError("当前环境缺少 db 所需上下文")

    request = urllib.request.Request(
        url=f"{base_url}/db/exec",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "X-Callit-Worker-Id": worker_id,
            "X-Callit-Request-Id": request_id,
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request) as response:
            raw = response.read().decode("utf-8").strip()
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="ignore").strip()
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {}
        message = parsed.get("error") or body or f"db 请求失败: {error.code}"
        raise RuntimeError(message)
    except urllib.error.URLError as error:
        raise RuntimeError(f"请求 db 服务失败: {error}") from error


def _ensure_non_empty_string(value, label):
    if not isinstance(value, str) or not value.strip():
        raise TypeError(f"{label} 必须是非空 string")
    return value.strip()


def _normalize_columns(columns):
    if not columns:
        return ["*"]
    return [_ensure_non_empty_string(column, "列名") for column in columns]


def _normalize_values_object(values, label):
    if not isinstance(values, dict):
        raise TypeError(f"{label} 必须是 dict")
    if not values:
        raise TypeError(f"{label} 不能为空")
    return [(_ensure_non_empty_string(key, "列名"), value) for key, value in values.items()]


class _BaseDBBuilder:
    def __init__(self, client, table):
        self._client = client
        self._table = _ensure_non_empty_string(table, "表名")
        self._where_clause = ""
        self._where_args = []
        self._end_sql_fragment = ""

    def where(self, condition, *args):
        self._where_clause = _ensure_non_empty_string(condition, "where 条件")
        self._where_args = list(args)
        return self

    def endSql(self, fragment):
        self._end_sql_fragment = _ensure_non_empty_string(fragment, "endSql")
        return self

    def _build_tail_parts(self):
        parts = []
        if self._where_clause:
            parts.append(f"where {self._where_clause}")
        if self._end_sql_fragment:
            parts.append(self._end_sql_fragment)
        return parts

    def _qualified_table_name(self):
        if not self._client._namespace:
            return self._table
        namespace_prefix = f"{self._client._namespace}_"
        if self._table.startswith(namespace_prefix):
            return self._table
        return f"{self._client._namespace}_{self._table}"


class _SelectBuilder(_BaseDBBuilder):
    def __init__(self, client, table, columns):
        super().__init__(client, table)
        self._columns = _normalize_columns(columns)

    def exec(self):
        sql = " ".join([
            f"select {', '.join(self._columns)} from {self._qualified_table_name()}",
            *self._build_tail_parts(),
        ]).strip()
        result = self._client.exec(sql, *self._where_args) or {}
        rows = result.get("rows")
        return rows if isinstance(rows, list) else []


class _InsertBuilder(_BaseDBBuilder):
    def __init__(self, client, table):
        super().__init__(client, table)
        self._insert_values = {}

    def values(self, values):
        for key, value in _normalize_values_object(values, "insert.values"):
            self._insert_values[key] = value
        return self

    def exec(self):
        if not self._insert_values:
            raise RuntimeError("insert.exec 前必须先调用 values()")
        columns = list(self._insert_values.keys())
        args = [self._insert_values[column] for column in columns]
        placeholders = ", ".join(["?"] * len(columns))
        parts = [
            f"insert into {self._qualified_table_name()}({', '.join(columns)}) values({placeholders})"
        ]
        if self._end_sql_fragment:
            parts.append(self._end_sql_fragment)
        result = self._client.exec(" ".join(parts), *args) or {}
        last_insert_id = result.get("last_insert_id")
        return last_insert_id if isinstance(last_insert_id, int) else 0


class _UpdateBuilder(_BaseDBBuilder):
    def __init__(self, client, table):
        super().__init__(client, table)
        self._update_assignments = {}

    def set(self, column, value):
        self._update_assignments[_ensure_non_empty_string(column, "列名")] = value
        return self

    def values(self, values):
        for key, value in _normalize_values_object(values, "update.values"):
            self._update_assignments[key] = value
        return self

    def exec(self):
        if not self._update_assignments:
            raise RuntimeError("update.exec 前必须至少调用一次 set() 或 values()")
        columns = list(self._update_assignments.keys())
        set_clause = ", ".join([f"{column} = ?" for column in columns])
        args = [self._update_assignments[column] for column in columns]
        sql = " ".join([
            f"update {self._qualified_table_name()} set {set_clause}",
            *self._build_tail_parts(),
        ]).strip()
        result = self._client.exec(sql, *args, *self._where_args) or {}
        rows_affected = result.get("rows_affected")
        return rows_affected if isinstance(rows_affected, int) else 0


class _DeleteBuilder(_BaseDBBuilder):
    def exec(self):
        sql = " ".join([
            f"delete from {self._qualified_table_name()}",
            *self._build_tail_parts(),
        ]).strip()
        result = self._client.exec(sql, *self._where_args) or {}
        rows_affected = result.get("rows_affected")
        return rows_affected if isinstance(rows_affected, int) else 0


class _DBClient:
    def __init__(self, namespace=None):
        self._namespace = normalize_namespace(namespace)

    def exec(self, sql, *args):
        if not isinstance(sql, str) or not sql.strip():
            raise TypeError("db client 的 sql 必须是非空 string")
        return _request_db_exec({"sql": sql, "args": list(args)})

    def select(self, table, *columns):
        return _SelectBuilder(self, table, columns)

    def insert(self, table):
        return _InsertBuilder(self, table)

    def update(self, table):
        return _UpdateBuilder(self, table)

    def delete(self, table):
        return _DeleteBuilder(self, table)


class _DBFactory:
    def new_client(self, namespace=None):
        return _DBClient(namespace)

    def newClient(self, namespace=None):
        return self.new_client(namespace)


db = _DBFactory()
