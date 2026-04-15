# Worker 文档
你可以把 Worker 理解为一个轻量级的函数计算环境，每个 Worker 都有独立的运行时和文件空间，通过 HTTP 触发执行。  

在 Worker 中，你可以使用 Python 或 Node.js 编写自定义代码来处理 HTTP 请求，实现动态响应、数据处理、文件操作等功能。 

除官方库之外，你还可以安装第三方依赖来扩展 Worker 的能力，例如在 Python Worker 中使用 `requests` 库发起 HTTP 请求，或在 Node Worker 中使用 `lodash` 进行数据处理。

## Worker 工作流程

当 HTTP 请求命中某个 Worker 路由后，系统会：
1. 读取并解析请求内容，包括请求方法、路径、查询参数、请求头和请求体等信息
2. 通过请求路径找到对应 Worker 程序
4. 执行 Worker 入口文件
5. 读取标准输出中的 `stdout` / `stderr`
6. 将 `stdout` 中的结构化结果解析为 HTTP 响应

* 当接收到文件上传时，系统会把文件暂存到服务器的运行时数据目录，并把文件信息（如路径、大小、类型）传入 `request.body[filename]` 中。沙箱内对应的可见路径为 `/tmp/upload`

Worker 中的脚本仅允许读写 `/tmp` 文件夹，其他文件皆为**只读**。

另外，平台内置了 `kv`、`db` 能力，你可以使用它来存储、读取持久化的字符串数据。具体规则参考 [Worker SDK 文档](./worker_sdk.md)。

## 环境变量使用说明

创建/编辑 Worker 时可设置环境变量，多个变量请使用分号 `;` 分隔，执行时会自动注入到 Worker 运行环境。  
在 Worker 内，你可以使用系统环境变量接口来访问这些变量：
- Python：`os.environ.get("KEY")`
- Node.js：`process.env.KEY`

## 入口文件要求

Worker 目录中必须包含主代码文件，文件名由 runtime 决定：

- Python Worker：必须包含 `main.py`
- Node Worker：必须包含 `main.js`

如果主文件缺失，Worker 执行会直接失败。

在 `main.xx` 入口文件中必须提供 `handler` 方法，用于接收执行上下文并返回响应结果。

如果脚本通过 `result.file` 返回文件路径：

- 相对路径会按 Worker 根目录解析
- 以 `/tmp/` 开头的路径会按运行时数据目录解析

### Python

`main.py` 必须定义：

```python
def handler(ctx):
    ...
```

如果未定义可调用的 `handler(ctx)`，运行时会报错

### Node

`main.js` 必须通过 CommonJS 导出 `handler`：

```javascript
function handler(ctx) {
  ...
}

module.exports = handler;
```

或：

```javascript
exports.handler = function (ctx) {
  ...
}
```

如果没有导出可调用的 `handler`，运行时会报错

## SDK 能力

Worker 运行时内置了 `kv` 和 `db` 两类 SDK 能力。

- `kv` 用于字符串键值存储
- `db` 用于访问 Worker 可用的共享数据库

详细的调用方式、参数规则、返回值结构和代码示例，请参考：
- [Worker SDK 文档](./worker_sdk.md)

## 模板示例

### Python

```python
def handler(ctx):
    request = ctx.get("request", {})

    return {
        "status": 200,
        "body": {
            "message": "Hello, Callit!",
            "request": request
        },
        "headers": {
            "Content-Type": "application/json"
        }
    }
```

### Node

```javascript
function handler(ctx) {
  const { request } = ctx;

  return {
    status: 200,
    body: {
      message: "Hello, Callit!",
      request,
    },
    headers: {
      "Content-Type": "application/json"
    }
  };
}

module.exports = handler
```

## context 结构

Worker 接收到的上下文模型为 `WorkerInput`：

```json
{
  "request": {},
  "event": {}
}
```

其中包含两部分：

- `request`：当前 HTTP 请求信息
- `event`：当前 Worker 执行事件信息

### request 字段说明

#### `request.method`

- 类型：`string`
- 来源：当前 HTTP 请求方法，如 `GET`、`POST`

#### `request.uri`

- 类型：`string`
- 来源：根据 Worker 路由和当前请求路径计算出的路由后缀
- 说明：
  - 如果 Worker 路由是 `/hello/*`
  - 实际请求路径是 `/hello/user/profile`
  - 则 `uri` 为 `/user/profile`
  - 如果没有额外后缀，则为 `/`

#### `request.url`

- 类型：`string`
- 来源：当前请求的完整 URL
- 说明：由请求的 scheme、host、path、query 重新拼装得到

#### `request.params`

- 类型：`map[string]string`
- 来源：URL query 参数
- 说明：
  - 来自 `?a=1&b=2`
  - 如果同名参数重复出现，保留最后一个值

#### `request.headers`

- 类型：`map[string]string`
- 来源：当前 HTTP 请求头
- 说明：
  - 所有请求头都会被传入
  - 同名 header 的多个值会使用逗号拼接成一个字符串

#### `request.body`

- 类型：`object | array`
- 来源：根据 `Content-Type` 对请求体做结构化解析后的结果
- 说明：
  - `application/json`：解析为 JSON 对象{}或数组[]
  - `application/x-www-form-urlencoded`：解析为 JSON 对象 {}
  - `multipart/form-data`：同 application/x-www-form-urlencoded，文件数据会以键值对对象形式传入，具体参考 [上传文件示例](#上传文件)
  - 其他类型：默认返回空对象 `{}`

#### `request.body_str`

- 类型：`string`
- 来源：原始 HTTP 请求体
- 说明：
  - 非 `multipart/form-data` 请求时，保留原始 body 字符串
  - `multipart/form-data` 请求时固定为空字符串
  - 这样做是为了避免把过大的上传内容直接塞进上下文

### event 字段说明

#### `event.request_id`

- 类型：`string`
- 来源：本次请求生成的唯一请求 ID
- 用途：可用于日志追踪、问题排查

#### `event.runtime`

- 类型：`string`
- 来源：当前 Worker 的运行时配置
- 可能值：`python` 或 `node`

#### `event.worker_id`

- 类型：`string`
- 来源：当前被执行 Worker 的唯一标识

#### `event.route`

- 类型：`string`
- 来源：当前命中的 Worker 路由规则

## stdin(Context) 样例

下面是一个典型的 `stdin` 样例。系统会把这段 JSON 文本写入脚本标准输入：

```json
{
  "request": {
    "method": "POST",
    "uri": "/user/profile",
    "url": "http://127.0.0.1:3100/api/hello/user/profile?name=callit",
    "params": {
      "name": "callit"
    },
    "headers": {
      "Content-Type": "application/json",
      "User-Agent": "curl/8.5.0"
    },
    "body": {
      "message": "hello"
    },
    "body_str": "{\"message\":\"hello\"}"
  },
  "event": {
    "request_id": "7b0d9f0a-9e95-45fd-ae43-3df2f6d2d001",
    "runtime": "python",
    "worker_id": "2bcf9922-c7d9-4d2e-a15d-b83de4ece1c6",
    "route": "/api/hello/*"
  }
}
```

## stdout 约定与样例

Worker 最终必须通过 `stdout` 输出一个合法 JSON，用于表示 HTTP 响应。

支持字段如下：

- `status`：HTTP 状态码，可选，默认 `200`
- `headers`：响应头，可选
- `file`：Worker 目录中的相对文件路径，可选
- `body`：响应体，必填

### stdout 样例

```json
{
  "status": 200,
  "headers": {
    "Content-Type": "application/json",
    "X-Trace": "abc"
  },
  "body": {
    "ok": true,
    "message": "hello"
  }
}
```

### 返回文件样例

如果希望直接返回 Worker 目录中的文件内容，可以输出：

```json
{
  "status": 200,
  "file": "output/report.txt",
  "headers": {
    "Content-Type": "text/plain; charset=utf-8"
  },
  "body": null
}
```

说明：

- `file` 必须是 Worker 目录内的相对路径
- 不允许越界访问 Worker 目录外的文件

## Worker 代码开发建议

- 保持 `handler(ctx)` 为纯函数风格，便于调试和测试
- 尽量返回结构化 JSON，不要手工拼接字符串响应
- 需要记录调试信息时，优先输出简洁日志，避免写入超大内容
- 如果依赖安装成功但运行时报找不到包，优先检查 runtime 是否一致

## Worker example

以下示例均以 Python Worker 为例，默认主文件名为 `main.py`。

### 上传文件

适用场景：

- 前端或客户端通过 `multipart/form-data` 上传文件
- Worker 读取上传文件元信息，必要时再读取文件内容

说明：

- `multipart/form-data` 请求时，`request.body_str` 会是空字符串
- 上传文件信息会出现在 `request.body`
- 文件会被暂存到服务器临时目录，字段中会包含可读取的 `path`

```python
import os


def handler(ctx):
    request = ctx.get("request", {})
    form = request.get("body", {}) or {}
    files = form.get("file", [])

    if not files:
        return {
            "status": 400,
            "body": {
                "error": "缺少上传文件，字段名应为 file"
            }
        }

    first = files[0]
    file_path = first.get("path", "")
    file_size = 0

    if file_path and os.path.exists(file_path):
        file_size = os.path.getsize(file_path)

    return {
        "status": 200,
        "body": {
            "message": "上传成功",
            "filename": first.get("filename"),
            "content_type": first.get("content_type"),
            "size": file_size,
            "tmp_path": file_path,
            "form_fields": form
        }
    }
```

对应的 `stdin` 结构通常类似：

```json
{
  "request": {
    "method": "POST",
    "body": {
      "title": "demo",
      // key 值是 form-data 中的字段名
      "file": [
        {
          "filename": "hello.txt",
          "content_type": "text/plain",
          "size": 12,
          "path": "/tmp/upload/hello.txt"
        }
      ]
    },
    "body_str": ""
  },
  "event": {}
}
```

### 下载文件

适用场景：

- Worker 动态生成文件后返回给客户端下载
- 或直接返回 Worker 目录中已有的文件

说明：

- Worker 在沙箱内只能写 `/tmp`，不能写 Worker 根目录
- 如果是动态生成文件，应该先把文件写到 `/tmp`，再通过 `file` 字段返回 `/tmp/...` 路径
- 如果返回的是 Worker 目录中预先存在的文件，仍然使用相对路径即可

```python
import os


def handler(ctx):
    output_dir = "/tmp/output"
    os.makedirs(output_dir, exist_ok=True)

    target = os.path.join(output_dir, "report.txt")
    with open(target, "w", encoding="utf-8") as f:
        f.write("hello from callit\n")
        f.write(f"request_id={ctx['event']['request_id']}\n")

    return {
        "status": 200,
        "file": "/tmp/output/report.txt",
        "headers": {
            "Content-Type": "text/plain; charset=utf-8",
            "Content-Disposition": "attachment; filename=report.txt"
        },
        "body": None
    }
```

如果只是返回 Worker 目录里已有文件，也可以直接：

```python
def handler(ctx):
    return {
        "status": 200,
        "file": "./manual.pdf",
        "headers": {
            "Content-Type": "application/pdf",
            "Content-Disposition": "attachment; filename=manual.pdf"
        },
        "body": None
    }
```

### 使用 Worker 返回 HTML 页面

适用场景：

- 返回简单静态页面
- 根据请求参数动态拼接 HTML
- 做轻量级页面渲染

说明：

- 返回 HTML 可以通过将内容放入`body` 字段，或通过 `file` 字段返回 Worker 目录中的 HTML 文件
- 设置 `Content-Type` 为 `text/html` 以确保浏览器正确解析
- `index.html` 必须存在于当前 Worker 目录下
- `file` 必须使用相对路径，不能写成绝对路径

1. 使用静态文件返回 HTML 页面（推荐） 

```python
def handler(ctx):
    return {
        "status": 200,
        "file": "index.html",
        "headers": {
            "Content-Type": "text/html; charset=utf-8"
        },
        "body": None
    }
```

1. 使用 HTML 代码直接返回（适合小页面）

```python
def handler(ctx):
    name = "Callit"
    html = f"""
<body>
  <h1>Hello, {name}</h1>
  <p>这是一个由 Worker 返回的 <span style="color: red;">HTML</span> 页面。</p>
</body>
""".strip()

    return {
        "status": 200,
        "headers": {
            "Content-Type": "text/html; charset=utf-8"
        },
        "body": html
    }
```
