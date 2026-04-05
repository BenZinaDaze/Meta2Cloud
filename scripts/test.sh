#!/usr/bin/env bash

# metadata2gd 本地全栈开发启动脚本
# 一键并联启动 fastapi 后端和 vite 前端

echo "🚀 Starting metadata2gd complete development environment..."

# 捕捉中断信号 (Ctrl+C) 或退出时，杀死当前进程组下的所有子进程 (kill 0)
trap "kill 0" SIGINT SIGTERM EXIT

# 1. 启动后端 (Uvicorn)
echo "============================================================"
echo "=> [Backend] Starting FastAPI Server on port 38765..."
echo "============================================================"
# 这里使用 python -m uvicorn，以防 uvicorn 不在系统 PATH 中
python -m uvicorn webui.api:app --host 0.0.0.0 --port 38765 --reload &

# 稍微等待半秒以免前端的日志先刷掉后端的启动日志
sleep 0.5 

# 2. 启动前端 (Vite)
echo "============================================================"
echo "=> [Frontend] Starting Vite Dev Server..."
echo "============================================================"
cd frontend && npm run dev
