# meta2cloud 后端测试脚本 (PowerShell)
Set-Location (Split-Path $PSScriptRoot -Parent)

Write-Host "go vet..."
go vet ./...

Write-Host "go test..."
go test ./internal/... -count=1

Write-Host "All tests passed."
