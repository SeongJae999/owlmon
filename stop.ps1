# OWLmon 개발 환경 전체 종료 스크립트
# 사용법: .\stop.ps1

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$pidFile = "$root\.dev-pids.json"

Write-Host "=== OWLmon 개발 환경 종료 ===" -ForegroundColor Cyan

# PATH 자동 보정 (Docker 등)
$extraPaths = @(
    "$env:ProgramFiles\Docker\Docker\resources\bin",
    "$env:ProgramFiles\Go\bin"
)
foreach ($p in $extraPaths) {
    if ((Test-Path $p) -and ($env:PATH -notlike "*$p*")) {
        $env:PATH = "$p;$env:PATH"
    }
}

# Job 종료 (PID 파일 기반)
if (Test-Path $pidFile) {
    $pids = Get-Content $pidFile | ConvertFrom-Json
    foreach ($id in @($pids.server, $pids.agent)) {
        if ($id) {
            Stop-Job -Id $id -ErrorAction SilentlyContinue
            Remove-Job -Id $id -Force -ErrorAction SilentlyContinue
            Write-Host "      Job $id 종료" -ForegroundColor Gray
        }
    }
    Remove-Item $pidFile
}

# 프로세스 이름으로 직접 종료 (Job이 누락된 경우 대비)
foreach ($name in @("owlmon-server-dev", "owlmon-agent-dev")) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        Write-Host "      $name (PID: $($_.Id)) 종료" -ForegroundColor Gray
        Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
    }
}

# 웹 개발 서버 종료 (vite)
Get-Process -Name "node" -ErrorAction SilentlyContinue | Where-Object {
    $_.MainWindowTitle -like "*vite*" -or $_.CommandLine -like "*vite*"
} | Stop-Process -Force -ErrorAction SilentlyContinue

# Docker 종료
if (Get-Command docker -ErrorAction SilentlyContinue) {
    Write-Host "Docker 중지 중..." -ForegroundColor Yellow
    docker compose -f "$root\docker-compose.yml" down
} else {
    Write-Host "[경고] docker 명령어를 찾을 수 없어 Docker 컨테이너 종료를 건너뜁니다." -ForegroundColor Yellow
}

Write-Host "완료" -ForegroundColor Green
