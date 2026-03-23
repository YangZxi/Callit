# syntax=docker/dockerfile:1.7

FROM node:22-bookworm AS frontend-builder
WORKDIR /app/pages

COPY pages ./
RUN npm install -g pnpm@latest-10 && pnpm install --frozen-lockfile && pnpm run build

FROM golang:1.25-bookworm AS backend-builder
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . ./
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /dist/callit ./cmd

FROM debian:bookworm-slim AS runtime
WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       python3 \
       python3-pip \
       python3-venv \
       nodejs \
       npm \
       sqlite3 \
    && npm install -g pnpm@10 \
    && groupadd -g 10001 callit \
    && useradd -u 10001 -g callit -d /app -s /usr/sbin/nologin callit \
    && rm -rf /var/lib/apt/lists/*

COPY --from=backend-builder --chown=callit:callit /dist/callit /app/callit
COPY --from=backend-builder --chown=callit:callit /app/resources /app/resources
COPY --from=frontend-builder --chown=callit:callit /app/pages/dist /app/public/admin

RUN mkdir -p /app/data/workers /app/data/temps \
    && chown -R callit:callit /app/data

ENV GIN_MODE=release
ENV DATA_DIR=/app/data
ENV ADMIN_PORT=3100

EXPOSE 3100

CMD ["/app/callit"]
