# callit

轻量级、自建、基于 Docker 的个人 Serverless 平台。

## 技术栈

- Backend: Go + Gin
- Frontend: React + Vite + HeroUI
- Database: SQLite3
- Runtime: Python3 / Node.js

## 端口

- Router: `3100`
- Admin: `3101`

## 运行前准备

```bash
export ADMIN_TOKEN=your-token
```

## 本地运行

先构建前端并复制到 `/public`：

```bash
cd pages
npm run build
cd ..
rm -rf public/*
cp -r pages/dist/* public/
```

再启动后端：

```bash
go run ./cmd
```

## Docker 运行

```bash
docker compose up --build
```

## 数据目录

- `data/app.db`
- `data/workers/<worker_id>/...`
- `data/temps/<request_id>/...`（请求结束自动清理）
- `public`（Admin 前端构建产物目录）

## 前端开发

```bash
cd pages
pnpm run dev
```

默认端口 `3180`，并已将 `/api` 代理到 `http://localhost:3101`。

## 脚本标准输入（stdin）示例

Router 会将请求上下文序列化为 JSON，通过标准输入传给 Worker：

```json
{
	"request": {
		"method": "POST",
		"uri": "/api/js?data=123&data=456",
		"url": "http://127.0.0.1:3100/api/js?data=123&data=456",
		"route_suffix": "/",
		"params": {
            // `params` 规则：
            // - `?data=123` -> `{"data":"123"}`
            // - `?data=123&data=456` -> `{"data":"456"}`（同名参数以后出现的值覆盖前值）
			"data": "456"
		},
		"headers": {
			"Content-Type": "application/json",
			"X-Trace": "abc"
		},
		"body": "{\"name\":\"callit\"}",
		"json": {
			"name": "callit"
		}
	},
	"event": {
		"request_id": "f3f3f1f6-8ad0-4f42-b31a-a90f9e8b2b31",
		"runtime": "node",
		"worker_id": "2bcf9922-c7d9-4d2e-a15d-b83de4ece1c6",
		"route": "/api/js"
	}
}
```

## 脚本标准输出（stdout）示例

Worker 必须输出合法 JSON 到标准输出：

```json
{
	"status": 200,
	"headers": {
		"X-Trace": "abc"
	},
	"file": "index.html",
	"body": {
		"data": {},
		"message": "hello"
	}
}
```

说明：
- `status` 可选，该值决定了 HTTP Status Code，默认 200。
- `headers` 可选。
- `body` 为业务响应体，支持 JSON 对象/数组，也支持字符串（例如 HTML 或纯文本）。
- `file` 可选，表示 Worker 目录下的相对路径（如 `index.html`、`./index.html`、`/index.html`）。
- 当 `file` 和 `body` 都不存在，固定返回 `404`（忽略 Worker 提供的 `status` 与 `Content-Type`）。
