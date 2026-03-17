# OWLmon 에이전트 Windows 서비스 설치 스크립트
# 관리자 권한으로 실행 필요
# 사용법: .\install-agent-windows.ps1 -Endpoint "192.168.1.10:4317"

param(
    [string]$Endpoint = "localhost:4317",
    [string]$InstallDir = "C:\owlmon",
    [string]$ConfigPath = ""
)

# 관리자 권한 확인
if (-NOT ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "관리자 권한으로 실행하세요. (우클릭 → 관리자로 실행)"
    exit 1
}

$ServiceName = "OWLmon-Agent"
$AgentExe = "$InstallDir\owlmon-agent.exe"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$AgentSrc = Join-Path $ScriptDir "..\agent"

Write-Host "=== OWLmon 에이전트 설치 ===" -ForegroundColor Cyan

# 1. 빌드
Write-Host "[1/4] 에이전트 빌드 중..." -ForegroundColor Yellow
Push-Location $AgentSrc
& go build -o $AgentExe .
if ($LASTEXITCODE -ne 0) {
    Write-Error "빌드 실패"
    Pop-Location
    exit 1
}
Pop-Location
Write-Host "      빌드 완료: $AgentExe" -ForegroundColor Green

# 2. 설치 디렉토리 준비
Write-Host "[2/4] 설치 디렉토리 준비 중..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# config.yaml 복사 (지정된 경우)
if ($ConfigPath -ne "" -and (Test-Path $ConfigPath)) {
    Copy-Item $ConfigPath "$InstallDir\config.yaml" -Force
    Write-Host "      config.yaml 복사 완료" -ForegroundColor Green
} elseif (Test-Path "$AgentSrc\config.yaml") {
    Copy-Item "$AgentSrc\config.yaml" "$InstallDir\config.yaml" -Force
    Write-Host "      config.yaml 복사 완료 (기본값)" -ForegroundColor Green
}

# 3. 기존 서비스 제거
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "[3/4] 기존 서비스 제거 중..." -ForegroundColor Yellow
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
} else {
    Write-Host "[3/4] 기존 서비스 없음, 신규 설치" -ForegroundColor Yellow
}

# 4. 서비스 등록
Write-Host "[4/4] Windows 서비스 등록 중..." -ForegroundColor Yellow

$binPath = "`"$AgentExe`""
sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= "OWLmon Monitoring Agent" | Out-Null
sc.exe description $ServiceName "OWLmon 서버 모니터링 에이전트 - CPU/메모리/디스크 메트릭 수집" | Out-Null

# 환경변수 설정 (레지스트리)
$regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
$envVars = "OWLMON_OTLP_ENDPOINT=$Endpoint"
if ($ConfigPath -ne "") {
    $envVars += "`0OWLMON_CONFIG=$InstallDir\config.yaml"
}
Set-ItemProperty -Path $regPath -Name "Environment" -Value $envVars.Split("`0") -Type MultiString

# 서비스 시작
Start-Service -Name $ServiceName
$status = (Get-Service -Name $ServiceName).Status

Write-Host ""
Write-Host "=== 설치 완료 ===" -ForegroundColor Cyan
Write-Host "서비스 이름: $ServiceName"
Write-Host "상태: $status"
Write-Host "설치 경로: $InstallDir"
Write-Host "OTLP 엔드포인트: $Endpoint"
Write-Host ""
Write-Host "서비스 관리 명령어:" -ForegroundColor Gray
Write-Host "  시작:  Start-Service $ServiceName"
Write-Host "  중지:  Stop-Service $ServiceName"
Write-Host "  상태:  Get-Service $ServiceName"
Write-Host "  로그:  Get-EventLog -LogName Application -Source $ServiceName -Newest 20"
Write-Host ""
Write-Host "제거하려면: .\uninstall-agent-windows.ps1" -ForegroundColor Gray
