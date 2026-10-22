FROM node:22-bookworm-slim AS frontend-builder

ARG APP_VERSION=dev


WORKDIR /frontend

# 先复制依赖文件，利用 Docker 层缓存
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# 复制前端源码并构建
COPY frontend/ ./
RUN VITE_APP_VERSION=${APP_VERSION} npm run build


FROM golang:1.27.3-bookworm AS backend-builder

ARG APP_VERSION=dev

WORKDIR /src

# 先复制依赖文件，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制后端源码并构建（纯 Go 依赖，CGO 关闭便于静态链接）
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${APP_VERSION}" \
    -o /out/meta2cloud ./cmd/meta2cloud


FROM debian:bookworm-slim

ARG APP_VERSION=dev
ENV APP_VERSION=${APP_VERSION}

# 系统依赖（HTTPS 请求需要 CA 证书；tzdata 提供时区）
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制后端二进制
COPY --from=backend-builder /out/meta2cloud /app/meta2cloud

# 复制配置样例与前端构建产物
COPY config/config.example.yaml /app/config/config.example.yaml
COPY --from=frontend-builder /frontend/dist ./frontend/dist

# 运行时数据目录（DB 文件由程序在容器内自动创建）
RUN mkdir -p /app/config/data /app/data

# WebUI + Webhook 统一端口
EXPOSE 38765

# 单进程启动：WebUI API + Webhook /trigger + 后台监听均在 38765
CMD ["/app/meta2cloud", "serve", "--host", "0.0.0.0", "--port", "38765", "--root", "/app"]
