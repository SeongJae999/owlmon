# OWLmon 서버 개발 실행 스크립트
# 사용법: .\run.ps1

$exe = ".\owlmon-server-dev.exe"

Write-Host "빌드 중..." -ForegroundColor Yellow
go build -o $exe .
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "서버 시작" -ForegroundColor Green
& $exe
