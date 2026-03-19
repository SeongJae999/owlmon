# OWLmon 개발 환경 전체 시작 스크립트
# 사용법: .\dev.ps1

$root = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "=== OWLmon 개발 환경 시작 ===" -ForegroundColor Cyan

# 1. Docker 인프라
Write-Host "[1/4] Docker 인프라 시작..." -ForegroundColor Yellow
docker compose -f "$root\docker-compose.yml" up -d
if ($LASTEXITCODE -ne 0) { Write-Error "Docker 실패"; exit 1 }
Write-Host "      완료" -ForegroundColor Green

# 2. 서버 빌드 & 실행
Write-Host "[2/4] 서버 빌드 중..." -ForegroundColor Yellow
Push-Location "$root\server"
go build -o owlmon-server-dev.exe .
if ($LASTEXITCODE -ne 0) { Write-Error "서버 빌드 실패"; Pop-Location; exit 1 }
Pop-Location
# 자동 재시작 래퍼 스크립트 생성
$restartWrapper = {
    param($exe, $workdir, $name)
    while ($true) {
        $proc = Start-Process -FilePath $exe -WorkingDirectory $workdir -PassThru -WindowStyle Minimized
        $proc.WaitForExit()
        if ($proc.ExitCode -eq 0) { break }  # 정상 종료면 재시작 안 함
        Write-Host "[$name] 크래시 감지 (ExitCode: $($proc.ExitCode)) — 5초 후 재시작..." -ForegroundColor Red
        Start-Sleep -Seconds 5
    }
}

Write-Host "      서버 시작 (자동 재시작 활성화)..." -ForegroundColor Yellow
$server = Start-Job -ScriptBlock $restartWrapper -ArgumentList "$root\server\owlmon-server-dev.exe", "$root\server", "Server"
Write-Host "      Job ID: $($server.Id)" -ForegroundColor Green

# 3. 에이전트 빌드 & 실행
Write-Host "[3/4] 에이전트 빌드 중..." -ForegroundColor Yellow
Push-Location "$root\agent"
go build -o owlmon-agent-dev.exe .
if ($LASTEXITCODE -ne 0) { Write-Error "에이전트 빌드 실패"; Pop-Location; exit 1 }
Pop-Location
Write-Host "      에이전트 시작 (자동 재시작 활성화)..." -ForegroundColor Yellow
$agent = Start-Job -ScriptBlock $restartWrapper -ArgumentList "$root\agent\owlmon-agent-dev.exe", "$root\agent", "Agent"
Write-Host "      Job ID: $($agent.Id)" -ForegroundColor Green

# 4. 웹 개발 서버
Write-Host "[4/4] 웹 개발 서버 시작..." -ForegroundColor Yellow
Start-Process -FilePath "powershell" -ArgumentList "-NoExit", "-Command", "cd '$root\web'; npm run dev" -WindowStyle Normal

Write-Host ""
Write-Host "=== 실행 완료 ===" -ForegroundColor Cyan
Write-Host "대시보드:  http://localhost:5173" -ForegroundColor White
Write-Host "Prometheus: http://localhost:9090" -ForegroundColor Gray
Write-Host ""
Write-Host "종료하려면: .\stop.ps1" -ForegroundColor Gray

# PID 저장 (stop.ps1에서 사용)
@{ server = $server.Id; agent = $agent.Id } | ConvertTo-Json | Set-Content "$root\.dev-pids.json"
