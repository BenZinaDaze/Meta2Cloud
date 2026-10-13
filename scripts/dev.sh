#!/usr/bin/env bash

# meta2cloud 本地全栈开发启动脚本
# 一键并联启动 Go 后端和 Vite 前端

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
    echo ""
    echo "🛑 Stopping dev servers..."
    [ -n "$FRONTEND_PID" ] && kill "$FRONTEND_PID" 2>/dev/null
    [ -n "$BACKEND_PID" ] && kill "$BACKEND_PID" 2>/dev/null
    wait 2>/dev/null
    exit 0
}
trap cleanup INT TERM

echo "🚀 Starting meta2cloud development environment..."

# 启动 Go 后端（端口 38765）
cd "$PROJECT_ROOT"
go build -o /tmp/meta2cloud-dev ./cmd/meta2cloud
/tmp/meta2cloud-dev serve --host 0.0.0.0 --port 38765 --root "$PROJECT_ROOT" &
BACKEND_PID=$!
echo "✅ Backend:  http://localhost:38765 (pid $BACKEND_PID)"

# 启动前端开发服务器（端口 5173，代理 /api 到 38765）
cd "$PROJECT_ROOT/frontend"
npm run dev &
FRONTEND_PID=$!
echo "✅ Frontend: http://localhost:5173 (pid $FRONTEND_PID)"

echo ""
echo "Press Ctrl+C to stop."
wait
