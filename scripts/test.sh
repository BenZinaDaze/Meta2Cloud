#!/usr/bin/env bash

# meta2cloud 后端测试脚本

set -e
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/.."

echo "🧪 go vet..."
go vet ./...

echo "🧪 go test..."
go test ./internal/... -count=1

echo "✅ All tests passed."
