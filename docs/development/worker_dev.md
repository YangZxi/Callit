# Worker 开发文档

本文档面向开发者，补充 Worker 的运行时行为、日志来源、依赖加载规则和调试建议。

## 工作原理

每个 Worker 对应一个独立目录，目录路径通常为：

`{DATA_DIR}/workers/{worker_id}`

当 HTTP 请求命中某个 Worker 路由后，系统会：

1. 读取并解析请求内容。
2. 组装 `WorkerInput` 上下文。
3. 将 `WorkerInput` 以 JSON 形式写入脚本进程的 `stdin`。
4. 执行 Worker 主文件。
5. 读取脚本的 `stdout` / `stderr`。
6. 将 `stdout` 中的结构化结果解析为 HTTP 响应。
7. 当接收到文件上传时，系统会把文件暂存到服务器的运行时数据目录，并把文件信息（如路径、大小、类型）传入 `request.body[filename]` 中。沙箱内对应的可见路径为 `/tmp/upload`。

Worker 中的脚本仅允许读写 `/tmp` 文件夹，其他文件皆为**只读**。

## stderr 的来源

`stderr` 是 Worker 脚本进程的标准错误输出，系统会完整采集并记录到运行日志中。

常见来源包括：

- `main.py` / `main.js` 不存在或无法正确加载
- 主文件中没有定义合法的 `handler`
- 代码语法错误
- 运行时抛出异常
- 导入依赖失败，如 Python `import` 失败、Node `require` 失败
- 主动写入标准错误流
  - Python：例如 `print("error", file=sys.stderr)`
  - Node：例如 `console.error("error")`

### 什么情况下会产生 stderr

以下情况通常会出现 `stderr`：

- Worker 启动阶段失败
- `handler(ctx)` 执行过程中抛异常
- 第三方依赖缺失或版本不兼容
- 代码显式打印错误日志到标准错误流

需要注意：

- 普通 `print(...)`（Python）或 `console.log(...)`（Node）通常进入 `stdout`
- 这些日志不会作为最终响应体返回，而是作为执行日志保存
- 如果最终 `stdout` 不是合法的协议输出，平台会返回 500

## 依赖库来源

Worker 运行时依赖来自全局依赖目录，而不是单个 Worker 私有目录。

依赖通过 Admin 后台的 `/dependencies` 页面统一安装和管理。

依赖存储路径为：

`{DATA_DIR}/.lib/{runtime}`

例如：

- Node：`{DATA_DIR}/.lib/node`
- Python：`{DATA_DIR}/.lib/python`

### Node 依赖

- 通过 `/dependencies` 页面执行 `pnpm add <package>`
- 安装位置在 `{DATA_DIR}/.lib/node`
- 实际加载时会把 `{DATA_DIR}/.lib/node/node_modules` 注入 `NODE_PATH`

### Python 依赖

- 通过 `/dependencies` 页面执行 `pip install <package>`
- 安装位置在 `{DATA_DIR}/.lib/python`
- 该目录会被初始化为 Python 虚拟环境
- 执行时会自动探测并把 `site-packages` 注入 `PYTHONPATH`
