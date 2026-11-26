# OWLmon 에이전트 Windows 서비스 설치 스크립트
# 관리자 권한으로 실행 필요. 보통은 웹 화면 [서버 추가]가 만들어 준 한 줄 명령으로 실행:
#   & ([scriptblock]::Create((iwr -UseBasicParsing https://<OWLmon 서버>/api/agent/install-agent.ps1).Content)) `
#     -Server https://<OWLmon 서버> -EnrollToken <화면에서 발급한 토큰> [-ServerCertSha256 <인증서 지문>]
# 바이너리는 OWLmon 서버에서 받음 (-Build: ..\agent 소스로 빌드, Go 필요)
# 예전 방식 (공용 키, 서버에서 OWLMON_LEGACY_SHARED_KEY=true일 때만 받음):
#   .\install-agent-windows.ps1 -Build -Endpoint "192.168.1.10:4317" -AgentKey "<공용 키>"

param(
    [string]$Server = "",
    [string]$EnrollToken = "",
    [string]$ServerCertSha256 = "",
    [string]$Endpoint = "",
    [string]$InstallDir = "C:\owlmon",
    [string]$ConfigPath = "",
    [string]$AgentKey = "",
    [switch]$Build
)
$ErrorActionPreference = "Stop"

# 관리자 권한 확인
if (-NOT ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "관리자 권한으로 실행하세요. (우클릭 → 관리자로 실행)"
    exit 1
}
if ($EnrollToken -eq "" -and $AgentKey -eq "") {
    Write-Error "-EnrollToken 이 필요합니다. 웹 화면 [서버 추가]에서 설치 명령을 받으세요."
    exit 1
}
# 등록 토큰과 응답으로 받는 서버 전용 키가 평문으로 오가지 않게 https만 (이 PC의 localhost 시험은 예외)
if ($EnrollToken -ne "" -and $Server -notlike "https://*" -and $Server -notmatch '^http://(localhost|127\.0\.0\.1)(:|/|$)') {
    Write-Error "등록 토큰은 https 서버 주소로만 보냅니다: -Server https://<OWLmon 서버>"
    exit 1
}

# 서버 인증서 확인 — 자체 서명이면 인증서 지문을 고정(검증을 끄지 않음), 아니면 Windows 기본 검증
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
if ($ServerCertSha256 -ne "") {
    $pin = $ServerCertSha256.Replace(":", "")
    [Net.ServicePointManager]::ServerCertificateValidationCallback = {
        param($s, $c)
        ([BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($c.GetRawCertData())) -replace '-') -eq $pin
    }.GetNewClosure()
}

# https 서버면 메트릭도 같은 주소(443)로 TLS 전송
$OtlpTls = "false"
if ($Server -like "https://*") {
    $OtlpTls = "true"
    if ($Endpoint -eq "") {
        $u = [Uri]$Server
        $Endpoint = "$($u.Host):$($u.Port)"
    }
}
if ($Endpoint -eq "") { $Endpoint = "localhost:4317" }

$ServiceName = "OWLmon-Agent"
$AgentExe = "$InstallDir\owlmon-agent.exe"
$ScriptDir = if ($MyInvocation.MyCommand.Path) { Split-Path -Parent $MyInvocation.MyCommand.Path } else { "." }
$AgentSrc = Join-Path $ScriptDir "..\agent"

Write-Host "=== OWLmon 에이전트 설치 ===" -ForegroundColor Cyan

# 0. 기존 서비스 제거 (실행 중인 exe를 덮어쓰지 않게 먼저)
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "[0/4] 기존 서비스 제거 중..." -ForegroundColor Yellow
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
}

# 1. 설치 디렉토리 — 키가 저장되므로 SYSTEM·Administrators만 접근
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
icacls $InstallDir /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null

# 2. 바이너리
if ($Build) {
    Write-Host "[1/4] 에이전트 빌드 중..." -ForegroundColor Yellow
    Push-Location $AgentSrc
    & go build -o $AgentExe .
    $code = $LASTEXITCODE
    Pop-Location
    if ($code -ne 0) { Write-Error "빌드 실패"; exit 1 }
} else {
    Write-Host "[1/4] OWLmon 서버에서 에이전트 받는 중..." -ForegroundColor Yellow
    # 아직 키가 없으므로 등록 토큰으로 받음 (토큰은 소진되지 않고, 에이전트가 처음 실행될 때 키로 바뀜)
    try {
        Invoke-WebRequest -UseBasicParsing -Headers @{ "X-Enroll-Token" = $EnrollToken } `
            -Uri "$Server/api/agent/binary?os=windows&arch=amd64" -OutFile $AgentExe
    } catch {
        Write-Error "받기 실패 — 토큰이 만료·사용됐거나, 서버에 Windows 에이전트가 올라가 있지 않습니다: $_"
        exit 1
    }
}
Write-Host "      $AgentExe" -ForegroundColor Green

# 3. 설정
Write-Host "[2/4] config.yaml 준비 중..." -ForegroundColor Yellow
if ($ConfigPath -ne "" -and (Test-Path $ConfigPath)) {
    Copy-Item $ConfigPath "$InstallDir\config.yaml" -Force
} else {
    $yaml = @"
# OWLmon 에이전트 설정 (install-agent-windows.ps1 자동 생성)
otlp_endpoint: "$Endpoint"
otlp_tls: $OtlpTls
server_cert_sha256: "$ServerCertSha256"
# 일회용 — 처음 실행 때 이 서버 전용 키로 바뀌어 agent.key에 저장됨
enroll_token: "$EnrollToken"
checks: []
logs:
  enabled: true
  server_url: "$Server"
  agent_key: "$AgentKey"
  tails: []
"@
    [IO.File]::WriteAllText("$InstallDir\config.yaml", $yaml)  # BOM 없이
}
# 새 토큰으로 다시 설치하면 예전 키는 버리고 새로 등록
if ($EnrollToken -ne "") { Remove-Item "$InstallDir\agent.key" -ErrorAction SilentlyContinue }

# 4. 서비스 등록
Write-Host "[3/4] Windows 서비스 등록 중..." -ForegroundColor Yellow
$binPath = "`"$AgentExe`""
sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= "OWLmon Monitoring Agent" | Out-Null
sc.exe description $ServiceName "OWLmon 서버 모니터링 에이전트 - CPU/메모리/디스크 메트릭 수집" | Out-Null
# 등록 실패(서버 일시 불통 등)로 종료되면 1분 뒤 다시 시작
sc.exe failure $ServiceName reset= 86400 actions= restart/60000/restart/60000/restart/60000 | Out-Null

# 서비스는 C:\Windows\System32에서 시작하므로 설정 경로를 항상 지정
$regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
$envVars = @("OWLMON_OTLP_ENDPOINT=$Endpoint", "OWLMON_CONFIG=$InstallDir\config.yaml")
Set-ItemProperty -Path $regPath -Name "Environment" -Value $envVars -Type MultiString

Write-Host "[4/4] 서비스 시작 중..." -ForegroundColor Yellow
Start-Service -Name $ServiceName
Start-Sleep -Seconds 3
$status = (Get-Service -Name $ServiceName).Status

Write-Host ""
Write-Host "=== 설치 완료 ===" -ForegroundColor Cyan
Write-Host "서비스 이름: $ServiceName"
Write-Host "상태: $status"
Write-Host "설치 경로: $InstallDir"
Write-Host "서버: $Server (메트릭: $Endpoint, TLS: $OtlpTls)"
Write-Host ""
Write-Host "서비스 관리 명령어:" -ForegroundColor Gray
Write-Host "  시작:  Start-Service $ServiceName"
Write-Host "  중지:  Stop-Service $ServiceName"
Write-Host "  상태:  Get-Service $ServiceName"
Write-Host "  로그:  Get-EventLog -LogName Application -Source $ServiceName -Newest 20"
Write-Host ""
Write-Host "제거하려면: .\uninstall-agent-windows.ps1" -ForegroundColor Gray
