# OWLmon 개발 환경 전체 시작 스크립트
# 사용법: .\dev.ps1

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "=== OWLmon 개발 환경 시작 ===" -ForegroundColor Cyan

# ---------------------------------------------------------------------------
# 0. 필수 도구 체크 & PATH 자동 보정
# ---------------------------------------------------------------------------
# 일반적인 설치 경로를 PATH에 추가 (이미 있으면 무시)
$extraPaths = @(
    "$env:ProgramFiles\Docker\Docker\resources\bin",
    "$env:ProgramFiles\Go\bin",
    "$env:ProgramFiles\nodejs",
    "$env:LOCALAPPDATA\Programs\nodejs",
    "$env:USERPROFILE\go\bin"
)
foreach ($p in $extraPaths) {
    if ((Test-Path $p) -and ($env:PATH -notlike "*$p*")) {
        $env:PATH = "$p;$env:PATH"
    }
}

function Assert-Command {
    param([string]$Name, [string]$HelpURL)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        Write-Host "[오류] '$Name' 명령어를 찾을 수 없습니다." -ForegroundColor Red
        Write-Host "       설치 가이드: $HelpURL" -ForegroundColor Gray
        exit 1
    }
}

Assert-Command "docker"  "https://docs.docker.com/desktop/install/windows-install/"
Assert-Command "go"      "https://go.dev/dl/"
Assert-Command "node"    "https://nodejs.org/"
Assert-Command "npm"     "https://nodejs.org/"

Write-Host "      필수 도구 확인 완료 (docker, go, node, npm)" -ForegroundColor Green

# ---------------------------------------------------------------------------
# 0.5 포트 8080 점유 프로세스 정리
# ---------------------------------------------------------------------------
$portCheck = netstat -ano 2>$null | Select-String ":8080\s.*LISTENING"
if ($portCheck) {
    foreach ($line in $portCheck) {
        $pid = ($line -split '\s+')[-1]
        if ($pid -and $pid -ne "0") {
            Write-Host "      포트 8080 점유 프로세스 종료 (PID: $pid)" -ForegroundColor Yellow
            Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        }
    }
    Start-Sleep -Seconds 1
}

# ---------------------------------------------------------------------------
# 1. .env 자동 생성 (없으면 .env.example 기반)
# ---------------------------------------------------------------------------
if (-not (Test-Path "$root\server\.env")) {
    Write-Host "[준비] .env 파일 생성 중..." -ForegroundColor Yellow
    if (-not (Test-Path "$root\server\.env.example")) {
        Write-Host "[오류] server/.env.example 파일이 없습니다." -ForegroundColor Red
        exit 1
    }

    # JWT 시크릿 랜덤 생성
    $jwtBytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($jwtBytes)
    $jwtSecret = ($jwtBytes | ForEach-Object { $_.ToString("x2") }) -join ""

    # 기본 비밀번호(admin) 해시 생성
    Push-Location "$root\server"
    $hashOutput = go run ./cmd/hashpw admin 2>&1
    Pop-Location
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[오류] 비밀번호 해시 생성 실패: $hashOutput" -ForegroundColor Red
        exit 1
    }
    $passwordHash = ($hashOutput | Select-Object -Last 1).Trim()

    # .env 파일 작성
    @"
OWLMON_JWT_SECRET=$jwtSecret
OWLMON_PASSWORD_HASH='$passwordHash'
POSTGRES_DSN=postgres://owlmon:owlmon@localhost:5433/owlmon
"@ | Set-Content "$root\server\.env" -Encoding UTF8

    Write-Host "      .env 생성 완료 (admin/admin)" -ForegroundColor Green
} else {
    Write-Host "      .env 이미 존재 — 건너뜀" -ForegroundColor Gray
}

# ---------------------------------------------------------------------------
# 2. 의존성 설치
# ---------------------------------------------------------------------------
# Go 모듈 다운로드
Write-Host "[준비] Go 모듈 확인 중..." -ForegroundColor Yellow
Push-Location "$root\server"
go mod download
Pop-Location
if ($LASTEXITCODE -ne 0) { Write-Error "Go 모듈 다운로드 실패"; exit 1 }

Push-Location "$root\agent"
go mod download
Pop-Location
if ($LASTEXITCODE -ne 0) { Write-Error "Go 모듈 다운로드 실패 (agent)"; exit 1 }
Write-Host "      Go 모듈 준비 완료" -ForegroundColor Green

# npm install (node_modules 없으면)
if (-not (Test-Path "$root\web\node_modules")) {
    Write-Host "[준비] npm install 실행 중..." -ForegroundColor Yellow
    Push-Location "$root\web"
    npm install
    Pop-Location
    if ($LASTEXITCODE -ne 0) { Write-Error "npm install 실패"; exit 1 }
    Write-Host "      npm install 완료" -ForegroundColor Green
} else {
    Write-Host "      node_modules 이미 존재 — 건너뜀" -ForegroundColor Gray
}

# ---------------------------------------------------------------------------
# 3. Docker 인프라
# ---------------------------------------------------------------------------
Write-Host "[1/4] Docker 인프라 시작..." -ForegroundColor Yellow
docker compose -f "$root\docker-compose.yml" up -d
if ($LASTEXITCODE -ne 0) { Write-Error "Docker 실패"; exit 1 }
Write-Host "      완료" -ForegroundColor Green

# PostgreSQL ready 대기 (최대 30초)
Write-Host "      PostgreSQL 준비 대기 중..." -ForegroundColor Yellow
$pgReady = $false
for ($i = 0; $i -lt 30; $i++) {
    $result = docker exec owlmon-postgres pg_isready -U owlmon 2>$null
    if ($LASTEXITCODE -eq 0) {
        $pgReady = $true
        break
    }
    Start-Sleep -Seconds 1
}
if ($pgReady) {
    Write-Host "      PostgreSQL 준비 완료" -ForegroundColor Green
} else {
    Write-Host "      [경고] PostgreSQL이 30초 내에 준비되지 않았습니다. 서버가 파일 기반 저장으로 폴백합니다." -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# 4. 서버 빌드 & 실행
# ---------------------------------------------------------------------------
Write-Host "[2/4] 서버 빌드 중..." -ForegroundColor Yellow
Push-Location "$root\server"
go build -o owlmon-server-dev.exe .
if ($LASTEXITCODE -ne 0) { Write-Error "서버 빌드 실패"; Pop-Location; exit 1 }
Pop-Location

# 자동 재시작 래퍼 스크립트
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

# ---------------------------------------------------------------------------
# 5. 에이전트 빌드 & 실행
# ---------------------------------------------------------------------------
Write-Host "[3/4] 에이전트 빌드 중..." -ForegroundColor Yellow
Push-Location "$root\agent"
go build -o owlmon-agent-dev.exe .
if ($LASTEXITCODE -ne 0) { Write-Error "에이전트 빌드 실패"; Pop-Location; exit 1 }
Pop-Location
Write-Host "      에이전트 시작 (자동 재시작 활성화)..." -ForegroundColor Yellow
$agent = Start-Job -ScriptBlock $restartWrapper -ArgumentList "$root\agent\owlmon-agent-dev.exe", "$root\agent", "Agent"
Write-Host "      Job ID: $($agent.Id)" -ForegroundColor Green

# ---------------------------------------------------------------------------
# 6. 웹 개발 서버
# ---------------------------------------------------------------------------
Write-Host "[4/4] 웹 개발 서버 시작..." -ForegroundColor Yellow
Start-Process -FilePath "powershell" -ArgumentList "-NoExit", "-Command", "cd '$root\web'; npm run dev" -WindowStyle Normal

Write-Host ""
Write-Host "=== 실행 완료 ===" -ForegroundColor Cyan
Write-Host "대시보드:  http://localhost:5173" -ForegroundColor White
Write-Host "API 서버:  http://localhost:8080" -ForegroundColor White
Write-Host "Prometheus: http://localhost:9090" -ForegroundColor Gray
Write-Host ""
Write-Host "종료하려면: .\stop.ps1" -ForegroundColor Gray

# PID 저장 (stop.ps1에서 사용)
@{ server = $server.Id; agent = $agent.Id } | ConvertTo-Json | Set-Content "$root\.dev-pids.json"
