# 程序配置说明

本文档基于 [internal/config/config.go](./internal/config/config.go) 的当前实现整理。

## 配置加载规则

程序配置分为两类：

- 基础运行配置：仅支持环境变量配置，不支持数据库配置。
- AppConfig：支持环境变量配置，也支持数据库配置覆盖。

其中 AppConfig 的优先级为：

`数据库配置 > 环境变量 > 硬编码默认值`

说明：

- 程序启动时先加载基础运行配置。
- 随后会加载 AppConfig。
- 如果数据库中存在对应 key 且 value 非空，则会覆盖环境变量值。

## 基础运行配置

### `ADMIN_TOKEN`
Admin 接口访问令牌。生产环境必须显式设置为高强度随机值。
- 必填：是
- 类型：`string`
- 默认值：`123123`

### `ADMIN_PREFIX`
Admin 服务访问前缀。程序会自动保证以 `/` 开头，并去掉末尾 `/`。
- 类型：`string`
- 默认值：`admin`
- 说明：如果你希望自定义 `Admin` 后台的访问路径，可以通过设置该环境变量来实现。例如，设置 `ADMIN_PREFIX=backend` 后，Admin 后台的访问地址将变为 `http://localhost:3100/backend`。

### `SERVER_PORT`
程序监听端口，Router 与 Admin 共用该端口。
- 类型：`int`
- 默认值：`3100`

### `LOG_LEVEL`
日志级别。
- 类型：`string`
- 默认值：`info`


## AppConfig 配置

以下配置项属于 AppConfig 白名单，支持数据库配置。

### `AI_BASE_URL`
AI 服务基础地址。
- 类型：`string`
- 默认值：`https://api.openai.com/v1`
- 数据库配置：支持数据库配置（数据库的 key: `AI_BASE_URL`）

### `AI_API_KEY`
AI 服务访问密钥。
- 类型：`string`
- 默认值：空字符串
- 数据库配置：支持数据库配置（数据库的 key: `AI_API_KEY`）

### `AI_MODEL`
默认使用的 AI 模型名称。
- 类型：`string`
- 默认值：`gpt-5`
- 数据库配置：支持数据库配置（数据库的 key: `AI_MODEL`）

### `AI_MAX_CONTEXT_TOKENS`
单次对话允许的最大上下文 Token 数。
- 类型：`int`
- 默认值：`16000`
- 数据库配置：支持数据库配置（数据库的 key: `AI_MAX_CONTEXT_TOKENS`）

### `AI_TIMEOUT_MS`
AI 请求超时时间，单位毫秒。
- 类型：`int`
- 默认值：`60000`
- 数据库配置：支持数据库配置（数据库的 key: `AI_TIMEOUT_MS`）

## 数据库存储说明

支持数据库配置的项会存储在 `app_config` 表中，字段为：

- `key`：配置 key
- `value`：配置值

当前仅允许写入 AppConfig 白名单中的 key，非白名单配置项不会被接受。

## 补充说明

- 基础运行配置在程序启动时读取，主要用于服务启动、目录定位、数据库初始化等核心能力。
- AppConfig 主要用于运行中的应用级配置，当前主要是 AI 相关设置。
- 如果数据库中某个 AppConfig key 存在但值为空，启动同步时不会覆盖环境变量或默认值。
