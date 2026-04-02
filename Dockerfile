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
COPY organizer.py .
COPY pipeline.py  .
COPY server.py    .

# Webhook 端口
EXPOSE 46562

# 启动 webhook server（pipeline.py 由 server.py 在后台触发）
CMD ["python", "server.py", "--port", "46562"]
