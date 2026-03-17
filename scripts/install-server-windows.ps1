# OWLmon 서버 Windows 서비스 설치 스크립트
# 관리자 권한으로 실행 필요
# 사용법: .\install-server-windows.ps1 -Password "비밀번호" -SmtpPassword "앱비밀번호" -SmtpTo "수신자@gmail.com"

param(
    [Parameter(Mandatory=$true)]
    [string]$Password,                          # OWLmon 로그인 비밀번호

    [string]$Username = "admin",                # OWLmon 로그인 아이디
    [string]$PrometheusURL = "http://localhost:9090",
    [string]$ListenAddr = ":8080",

    [string]$SmtpHost = "",                     # 비워두면 이메일 알림 비활성화
    [string]$SmtpPort = "587",
    [string]$SmtpUsername = "",
    [string]$SmtpPassword = "",
    [string]$SmtpFrom = "",
    [string]$SmtpTo = "",

    [string]$InstallDir = "C:\owlmon-server"
)

if (-NOT ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "관리자 권한으로 실행하세요. (우클릭 → 관리자로 실행)"
    exit 1
}

$ServiceName = "OWLmon-Server"
$ServerExe = "$InstallDir\owlmon-server.exe"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerSrc = Join-Path $ScriptDir "..\server"

Write-Host "=== OWLmon 서버 설치 ===" -ForegroundColor Cyan

# 1. 비밀번호 해시 생성
Write-Host "[1/4] 비밀번호 해시 생성 중..." -ForegroundColor Yellow
Push-Location $ServerSrc
$hashOutput = & go run ./cmd/hashpw $Password 2>&1
Pop-Location
$PasswordHash = $hashOutput | Where-Object { $_ -match '^\$2' } | Select-Object -First 1
if (-not $PasswordHash) {
    Write-Error "비밀번호 해시 생성 실패: $hashOutput"
    exit 1
}
Write-Host "      해시 생성 완료" -ForegroundColor Green

# 2. 빌드
Write-Host "[2/4] 서버 빌드 중..." -ForegroundColor Yellow
Push-Location $ServerSrc
& go build -o $ServerExe .
if ($LASTEXITCODE -ne 0) {
    Write-Error "빌드 실패"
    Pop-Location
    exit 1
}
Pop-Location
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Write-Host "      빌드 완료: $ServerExe" -ForegroundColor Green

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
sc.exe create $ServiceName binPath= "`"$ServerExe`"" start= auto DisplayName= "OWLmon Server" | Out-Null
sc.exe description $ServiceName "OWLmon 모니터링 서버 - Prometheus 프록시 및 이메일 알림" | Out-Null

# 환경변수 영구 저장 (레지스트리)
$envVars = @(
    "OWLMON_USERNAME=$Username",
    "OWLMON_PASSWORD_HASH=$PasswordHash",
    "OWLMON_PROMETHEUS_URL=$PrometheusURL",
    "OWLMON_LISTEN=$ListenAddr"
)
if ($SmtpHost -ne "") {
    $envVars += "SMTP_HOST=$SmtpHost"
    $envVars += "SMTP_PORT=$SmtpPort"
    $envVars += "SMTP_USERNAME=$SmtpUsername"
    $envVars += "SMTP_PASSWORD=$SmtpPassword"
    $envVars += "SMTP_FROM=$SmtpFrom"
    $envVars += "SMTP_TO=$SmtpTo"
}
$regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
Set-ItemProperty -Path $regPath -Name "Environment" -Value $envVars -Type MultiString

Start-Service -Name $ServiceName
$status = (Get-Service -Name $ServiceName).Status

Write-Host ""
Write-Host "=== 설치 완료 ===" -ForegroundColor Cyan
Write-Host "서비스 이름: $ServiceName"
Write-Host "상태: $status"
Write-Host "설치 경로: $InstallDir"
Write-Host "접속 주소: http://localhost$ListenAddr"
if ($SmtpHost -ne "") {
    Write-Host "이메일 알림: 활성화 ($SmtpTo)"
} else {
    Write-Host "이메일 알림: 비활성화 (SmtpHost 미설정)" -ForegroundColor Gray
}
Write-Host ""
Write-Host "서비스 관리 명령어:" -ForegroundColor Gray
Write-Host "  시작:  Start-Service $ServiceName"
Write-Host "  중지:  Stop-Service $ServiceName"
Write-Host "  상태:  Get-Service $ServiceName"
Write-Host ""
Write-Host "제거하려면: .\uninstall-server-windows.ps1" -ForegroundColor Gray
