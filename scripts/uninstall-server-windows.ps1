# OWLmon 서버 Windows 서비스 제거 스크립트
param(
    [string]$InstallDir = "C:\owlmon-server"
)

if (-NOT ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "관리자 권한으로 실행하세요."
    exit 1
}

$ServiceName = "OWLmon-Server"

Write-Host "=== OWLmon 서버 제거 ===" -ForegroundColor Cyan

$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
    Write-Host "서비스 제거 완료" -ForegroundColor Green
} else {
    Write-Host "서비스가 설치되어 있지 않습니다." -ForegroundColor Gray
}

if (Test-Path $InstallDir) {
    Remove-Item -Path $InstallDir -Recurse -Force
    Write-Host "설치 디렉토리 제거 완료: $InstallDir" -ForegroundColor Green
}

Write-Host "제거 완료" -ForegroundColor Cyan
