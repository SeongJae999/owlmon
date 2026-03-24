#!/usr/bin/env bash
# OWLmon 개발 환경 전체 종료 스크립트 (macOS / Linux)
# 사용법: make stop  또는  ./stop.sh

ROOT="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$ROOT/.dev-pids.json"

echo "=== OWLmon 개발 환경 종료 ==="

# PID 파일 기반 종료
if [[ -f "$PID_FILE" ]]; then
    SERVER_PID=$(python3 -c "import json,sys; d=json.load(open('$PID_FILE')); print(d.get('server',''))" 2>/dev/null)
    AGENT_PID=$(python3  -c "import json,sys; d=json.load(open('$PID_FILE')); print(d.get('agent',''))"  2>/dev/null)
    WEB_PID=$(python3    -c "import json,sys; d=json.load(open('$PID_FILE')); print(d.get('web',''))"    2>/dev/null)

    for pid in $SERVER_PID $AGENT_PID $WEB_PID; do
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null && echo "      PID $pid 종료"
        fi
    done
    rm -f "$PID_FILE"
fi

# 프로세스 이름으로 직접 종료 (PID 파일 누락 대비)
for name in owlmon-server-dev owlmon-agent-dev; do
    pids=$(pgrep -f "$name" 2>/dev/null || true)
    for pid in $pids; do
        echo "      $name (PID: $pid) 종료"
        kill -9 "$pid" 2>/dev/null || true
    done
done

# 웹 개발 서버 종료 (vite)
pids=$(pgrep -f "vite" 2>/dev/null || true)
for pid in $pids; do
    echo "      vite (PID: $pid) 종료"
    kill "$pid" 2>/dev/null || true
done

# Docker 종료
if command -v docker &>/dev/null; then
    echo "Docker 중지 중..."
    docker compose -f "$ROOT/docker-compose.yml" down
else
    echo "[경고] docker 명령어를 찾을 수 없어 Docker 컨테이너 종료를 건너뜁니다."
fi

echo "완료"
