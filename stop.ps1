# OWLmon 개발 환경 전체 종료 스크립트
# 사용법: .\stop.ps1

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$pidFile = "$root\.dev-pids.json"

Write-Host "=== OWLmon 개발 환경 종료 ===" -ForegroundColor Cyan

# 서버/에이전트 종료
if (Test-Path $pidFile) {
    $pids = Get-Content $pidFile | ConvertFrom-Json
    foreach ($id in @($pids.server, $pids.agent)) {
        if ($id) {
            Stop-Process -Id $id -Force -ErrorAction SilentlyContinue
            Write-Host "      PID $id 종료" -ForegroundColor Gray
        }
    }
    Remove-Item $pidFile
}

# 웹 개발 서버 종료 (vite)
Get-Process -Name "node" -ErrorAction SilentlyContinue | Where-Object {
    $_.MainWindowTitle -like "*vite*" -or $_.CommandLine -like "*vite*"
} | Stop-Process -Force -ErrorAction SilentlyContinue

# Docker 종료
Write-Host "Docker 중지 중..." -ForegroundColor Yellow
docker compose -f "$root\docker-compose.yml" down

Write-Host "완료" -ForegroundColor Green
