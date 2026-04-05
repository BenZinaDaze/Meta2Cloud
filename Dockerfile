FROM node:22-bookworm-slim AS frontend-builder

ARG APP_VERSION=dev


WORKDIR /frontend

# 先复制依赖文件，利用 Docker 层缓存
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# 复制前端源码并构建
COPY frontend/ ./
RUN VITE_APP_VERSION=${APP_VERSION} npm run build


FROM python:3.11-slim

ARG APP_VERSION=dev
ENV APP_VERSION=${APP_VERSION}

# 系统依赖（requests 需要 CA 证书）
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 先复制依赖文件，利用 Docker 层缓存
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# 复制源代码（config/ 目录通过 volume 挂载，不打包进镜像）
COPY drive/       ./drive/
COPY mediaparser/ ./mediaparser/
COPY nfo/         ./nfo/
COPY webui/       ./webui/
COPY organizer.py ./
COPY pipeline.py  ./

# 复制前端构建产物
COPY --from=frontend-builder /frontend/dist ./frontend/dist

# 运行时数据目录（DB 文件由程序在容器内自动创建）
RUN mkdir -p /app/data

# WebUI + Webhook 统一端口
EXPOSE 38765

# 单进程启动：WebUI API + Webhook /trigger 均在 38765
CMD ["uvicorn", "webui.api:app", "--host", "0.0.0.0", "--port", "38765"]
