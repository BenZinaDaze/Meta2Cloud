#!/usr/bin/env bash

# metadata2gd 本地全栈开发启动脚本
# 一键并联启动 fastapi 后端和 vite 前端

echo "🚀 Starting metadata2gd complete development environment..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PYTHON_CMD="python"
BACKEND_PID=""

# 优先使用项目虚拟环境，避免误用系统 Python 导致缺少 uvicorn/fastapi 依赖
if [ -x "$PROJECT_ROOT/.venv/bin/python" ]; then
  PYTHON_CMD="$PROJECT_ROOT/.venv/bin/python"
fi

cleanup() {
  if [ -n "$BACKEND_PID" ] && kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
    kill "$BACKEND_PID" >/dev/null 2>&1 || true
  fi
}

# 捕捉中断信号或退出，关闭后台 uvicorn 进程
trap cleanup SIGINT SIGTERM EXIT

if ! "$PYTHON_CMD" -c "import uvicorn" >/dev/null 2>&1; then
  echo "❌ Backend dependencies are missing for: $PYTHON_CMD"
  echo "   Please create/use a virtualenv and install requirements first:"
  echo "   cd $PROJECT_ROOT && python -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt"
  exit 1
fi

# 1. 启动后端 (Uvicorn)
echo "============================================================"
echo "=> [Backend] Starting FastAPI Server on port 38765..."
echo "============================================================"
# 这里使用 python -m uvicorn，以防 uvicorn 不在系统 PATH 中
"$PYTHON_CMD" -m uvicorn webui.api:app --host 0.0.0.0 --port 38765 --reload &
BACKEND_PID=$!

# 稍微等待半秒以免前端的日志先刷掉后端的启动日志
sleep 0.5 

# 2. 启动前端 (Vite)
echo "============================================================"
echo "=> [Frontend] Starting Vite Dev Server..."
echo "============================================================"
cd "$PROJECT_ROOT/frontend" && npm run dev
