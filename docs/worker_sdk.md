# Worker SDK 文档

本文档说明 Worker 运行时内置的 SDK 能力，目前包含：

- `kv`
- `db`

你可以在 Worker 中直接使用：

- Python: `from callit import kv, db`
- Node: `const { kv, db } = require("callit")`

## kv 能力

`kv` 是一个 client 工厂。未显式指定 `namespace` 时，默认使用当前 `worker_id` 作为隔离命名空间。

### 创建 client

```python
from callit import kv

kv_client = kv.new_client("group1")
default_client = kv.new_client()
```

```javascript
const { kv } = require("callit");

const kvClient = kv.newClient("group1");
const defaultClient = kv.newClient();
```

### namespace 规则

- `namespace` 可为 `None/null/""/非空字符串`。当 `namespace` 为 `None/null/""` 时，SDK 会自动使用当前 `worker_id`
- `namespace` 只用于当前 kv client 的 key 隔离，不影响其他 client

### 方法说明

- `set(key, value, seconds)`
  - `value` 必须是 `string`
  - 成功时不返回内容
- `get(key)`
  - 返回 `string`
- `delete(key)`
  - 成功时不返回内容
- `increment(key, step=1)`
  - 返回递增后的整数值
- `expire(key, seconds)`
  - 成功时不返回内容
- `ttl(key)`
  - 返回剩余秒数
  - `-2` 表示 key 不存在
  - `-1` 表示 key 存在但没有过期时间
- `has(key)`
  - 返回 `true` / `false`

### 调用示例

```python
import json
from callit import kv

kv_client = kv.new_client("group1")
kv_client.set("session", json.dumps({"id": 123}), 300)
value = kv_client.get("session")
exists = kv_client.has("session")
ttl = kv_client.ttl("session")
```

```javascript
const { kv } = require("callit");

const kvClient = kv.newClient("group1");
await kvClient.set("session", JSON.stringify({ id: 123 }), 300);
const value = await kvClient.get("session");
const exists = await kvClient.has("session");
const ttl = await kvClient.ttl("session");
```

## db 能力

`db` 是一个 client 工厂，用于访问 Worker 可用的共享数据库。默认使用当前 worker_id 作为隔离命名空间。

### 创建 client

```python
from callit import db

db_client = db.new_client()
group_client = db.new_client("group1")
```

```javascript
const { db } = require("callit");

const dbClient = db.newClient();
const groupClient = db.newClient("group1");
```

### namespace 规则

- `namespace` 可为 `None/null/""/非空字符串`。当 `namespace` 为 `None/null/""` 时，SDK 会自动使用当前 `worker_id`
- `namespace` 只作用于 builder 生成的表名
- builder 在执行 `select / insert / update / delete` 时，会把表名改写为：
  - `<namespace>_<table_name>`
  - 但如果传入表名已经以精确的 `<namespace>_` 开头，则不会重复追加前缀。前缀匹配严格区分大小写
- `db.exec(sql, ...args)` 不会自动改写表名

示例：

- `db.newClient("group1").select("users").exec()` 实际访问 `group1_users`
- `db.newClient("group1").select("group1_users").exec()` 实际访问 `group1_users`
- `db.newClient("group1").select("GROUP1_users").exec()` 实际访问 `group1_GROUP1_users`
- `db.newClient().select("users").exec()` 实际访问 `<worker_id>_users`
- `db.newClient("group1").exec("select * from users")` 仍然访问 `users`

### 原始 SQL 接口

client 提供原始接口：

- `exec(sql, ...args)`

约定如下：

- 使用 `?` 作为占位符传参
- 平台执行时会使用参数绑定，不会把参数直接拼接到 SQL 字符串中
- 所有 Worker 共用同一个数据库
- `db` 能力不进行额外权限隔离，请谨慎处理表结构与数据冲突

#### `exec` 返回值

`exec(sql, ...args)` 始终返回原始结果对象：

```json
{
  "rows": [],
  "rows_affected": 0,
  "last_insert_id": 0
}
```

规则如下：

- 查询类 SQL：主要读取 `rows`
- `update/delete`：主要读取 `rows_affected`
- `insert`：主要读取 `last_insert_id`

#### `exec` 示例

```python
from callit import db

db_client = db.new_client()
result = db_client.exec("select * from users where status = ?", 1)
rows = result["rows"]
```

```javascript
const { db } = require("callit");

const dbClient = db.newClient();
const result = await dbClient.exec("select * from users where status = ?", 1);
const rows = result.rows;
```

### Builder 接口

除原始 `exec` 外，client 还提供四类 builder：

- `select(table, ...columns)`
- `insert(table)`
- `update(table)`
- `delete(table)`

这些 builder 只在 SDK 内部拼接 SQL，最终仍然调用原始 `exec`。

#### `select(table, ...columns)`

- 当 `columns` 不传时，默认使用 `*`
- 可配合：
  - `.where(condition, ...args)`
  - `.endSql(fragment)`
  - `.exec()`

示例：

```python
rows = (
    db.new_client("group1")
    .select("users")
    .where("age > ? or status = ?", 18, 1)
    .endSql("limit 10 order by created_at desc")
    .exec()
)
```

```javascript
const rows = await db
  .newClient("group1")
  .select("users")
  .where("age > ? or status = ?", 18, 1)
  .endSql("limit 10 order by created_at desc")
  .exec();
```

返回值：

- 固定返回数组，例如：

```json
[
  {"id": 1, "name": "John"},
  {"id": 2, "name": "Alice"}
]
```

#### `insert(table)`

- 使用 `.values({...})` 指定插入字段
- `values` 可重复调用；同名字段以后一次为准
- 可配合：
  - `.endSql(fragment)`
  - `.exec()`

示例：

```python
last_insert_id = (
    db.new_client("group1")
    .insert("users")
    .values({"name": "John", "age": 25, "status": 1})
    .exec()
)
```

```javascript
const lastInsertId = await db
  .newClient("group1")
  .insert("users")
  .values({ name: "John", age: 25, status: 1 })
  .exec();
```

返回值：

- 固定返回 `last_insert_id`
- 例如：`200001`

#### `update(table)`

- 使用 `.set(column, value)` 设置单列
- 也可以使用 `.values({...})` 批量设置
- `set` 与 `values` 可混用
- 同名字段以后一次为准
- 可配合：
  - `.where(condition, ...args)`
  - `.endSql(fragment)`
  - `.exec()`

示例：

```python
rows_affected = (
    db.new_client("group1")
    .update("users")
    .set("name", "John")
    .set("age", 25)
    .values({"status": 1, "age": 30})
    .where("id = ?", 9)
    .endSql("limit 1")
    .exec()
)
```

```javascript
const rowsAffected = await db
  .newClient("group1")
  .update("users")
  .set("name", "John")
  .set("age", 25)
  .values({ status: 1, age: 30 })
  .where("id = ?", 9)
  .endSql("limit 1")
  .exec();
```

返回值：

- 固定返回影响行数
- 例如：`1`

#### `delete(table)`

- 可配合：
  - `.where(condition, ...args)`
  - `.endSql(fragment)`
  - `.exec()`

示例：

```python
rows_affected = (
    db.new_client("group1")
    .delete("users")
    .where("status = ?", 0)
    .exec()
)
```

```javascript
const rowsAffected = await db
  .newClient("group1")
  .delete("users")
  .where("status = ?", 0)
  .exec();
```

返回值：

- 固定返回影响行数
- 例如：`1`

### Builder 规则汇总

- `select(table, ...columns)` 未传列名时默认 `*`
- `where(condition, ...args)` 重复调用时，后一次覆盖前一次
- `endSql(fragment)` 会在完整 SQL 末尾追加；重复调用时，后一次覆盖前一次
- `insert.values({...})` 可重复调用；同名字段以后一次为准
- `update.set()` / `update.values()` 可混用；同名字段以后一次为准
- builder 会自动处理参数顺序，无需手动拼接 SQL 参数列表
