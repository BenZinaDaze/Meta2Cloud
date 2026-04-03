FROM python:3.11-slim

# 系统依赖（requests 需要 CA 证书）
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 先复制依赖文件，利用 Docker 层缓存
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 复制源代码（config/ 目录通过 volume 挂载，不打包进镜像）
COPY drive/       ./drive/
COPY mediaparser/ ./mediaparser/
COPY nfo/         ./nfo/
COPY webui/       ./webui/
COPY organizer.py .
COPY pipeline.py  .

# 前端构建产物（npm run build 产生 frontend/dist/）
# 如果未构建则跳过，WebUI 静态服务不可用但 API 正常工作
COPY frontend/dist/ ./frontend/dist/

# 运行时数据目录（DB 文件由程序在容器内自动创建）
RUN mkdir -p /app/data

# WebUI + Webhook 统一端口
EXPOSE 8765

# 单进程启动：WebUI API + Webhook /trigger 均在 8765
CMD ["uvicorn", "webui.api:app", "--host", "0.0.0.0", "--port", "8765"]
