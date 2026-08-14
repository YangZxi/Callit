# Callit

[简体中文](./README.md)

Callit is a lightweight, self-hosted personal serverless platform built around Docker.

It lets you write Workers in Python or Node.js and invoke them through HTTP routes. It is suited to lightweight APIs, HTML pages, and personal automation.

## Core capabilities

- HTTP-triggered Workers
- Python and Node.js runtimes
- File upload, file responses, and HTML responses
- Global dependency management
- Built-in Admin UI for Workers, configuration, and dependencies
- Persistent Worker data through KV and DB SDK APIs

Callit is designed for personal and lightweight use. For a mature enterprise serverless platform, consider OpenFaaS, Fission, AWS Lambda, Cloudflare Workers, or a similar service.

## Quick start

### Docker Compose

```yaml
services:
  callit:
    image: yangzxi/callit:latest
    container_name: callit
    environment:
      - ADMIN_TOKEN=
    ports:
      - "3100:3100"
    volumes:
      - ./data:/app/data
    cap_add:
      - SYS_ADMIN
    security_opt:
      - seccomp=unconfined
      - apparmor=unconfined
      - systempaths=unconfined
    restart: unless-stopped
    depends_on:
      - redis

  redis:
    image: redis:7-alpine
    container_name: callit-redis
    restart: unless-stopped
```

Start the services:

```bash
docker compose up -d
```

Then open:

- Router: `http://127.0.0.1:3100`
- Admin UI: `http://127.0.0.1:3100/admin/`

## Worker examples

### Python

```python
def handler(ctx):
    return {
        "status": 200,
        "body": {"message": "Hello, Callit!"},
        "headers": {"Content-Type": "application/json"},
    }
```

### Node.js

```javascript
export default function handler(ctx) {
  return {
    status: 200,
    body: { message: "Hello, Callit!" },
    headers: { "Content-Type": "application/json" },
  };
}
```

Each Worker must contain `main.py` or `main.js` and define `handler(ctx)`. See the [Worker documentation](./docs/worker_introduction.md) for the complete contract and SDK examples.

## Development

```bash
cd pages
pnpm install
pnpm run dev
```

To run the complete development stack:

```bash
docker compose -f docker-compose.dev.yml up --build
```

The default frontend port is `3180`; the backend port is `3100`.

## Data directories

- `data/app.db`: application SQLite database
- `data/workers/<worker_id>/`: Worker files
- `data/tmp/<request_id>/`: temporary request uploads, cleaned after a request
- `data/.lib/<runtime>/`: global runtime dependencies
- `public/`: built Admin UI assets

## License

Callit is distributed under [GNU General Public License v3.0](./LICENSE).
