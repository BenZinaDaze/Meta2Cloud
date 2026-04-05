Write-Host "🚀 Starting metadata2gd complete development environment..." -ForegroundColor Cyan

# 自动寻找虚拟环境的 Python
$pythonCmd = "python"
$venvPython = Join-Path $PSScriptRoot "..\.venv\Scripts\python.exe"
if (Test-Path $venvPython) {
    $pythonCmd = $venvPython
}

# 启动后端服务 (在新窗口中，防止多个进程混用控制台导致死锁)
Write-Host "============================================================" -ForegroundColor DarkGray
Write-Host "=> [Backend] Starting FastAPI Server on port 38765 in a new Terminal..." -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor DarkGray

$backendArgs = "-m", "uvicorn", "webui.api:app", "--host", "0.0.0.0", "--port", "38765", "--reload"
$backendProcess = Start-Process -FilePath $pythonCmd -ArgumentList $backendArgs -PassThru

# 稍微等待半秒以免互相交织不清
Start-Sleep -Seconds 1

# 启动前端服务
Write-Host "============================================================" -ForegroundColor DarkGray
Write-Host "=> [Frontend] Starting Vite Dev Server..." -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor DarkGray

Set-Location (Join-Path $PSScriptRoot "..\frontend")

try {
    # 在当前窗口前台拉起 Vite，并等待它执行
    & npm run dev
}
finally {
    # 捕捉到 Ctrl+C 或脚本退出时关闭新窗口后台挂起的后端进程
    Write-Host "`n[Auto-Cleanup] Shutting down backend uvicorn process..." -ForegroundColor Yellow
    if ($backendProcess -and -not $backendProcess.HasExited) {
        Stop-Process -Id $backendProcess.Id -Force -ErrorAction SilentlyContinue
    }
}
