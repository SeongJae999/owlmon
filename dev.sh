#!/usr/bin/env bash
# OWLmon 개발 환경 전체 시작 스크립트 (macOS / Linux)
# 사용법: make dev  또는  ./dev.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "=== OWLmon 개발 환경 시작 ==="

# ---------------------------------------------------------------------------
# 0. PATH 자동 보정
# ---------------------------------------------------------------------------
for p in /opt/homebrew/bin /usr/local/bin /usr/local/go/bin "$HOME/go/bin"; do
    [[ -d "$p" && ":$PATH:" != *":$p:"* ]] && export PATH="$p:$PATH"
done

# ---------------------------------------------------------------------------
# 0. 필수 도구 체크
# ---------------------------------------------------------------------------
assert_cmd() {
    local name=$1 url=$2
    if ! command -v "$name" &>/dev/null; then
        echo "[오류] '$name' 명령어를 찾을 수 없습니다."
        echo "       설치 가이드: $url"
        exit 1
    fi
}

assert_cmd docker "https://docs.docker.com/desktop/install/mac-install/"
assert_cmd go     "https://go.dev/dl/"
assert_cmd node   "https://nodejs.org/"
assert_cmd npm    "https://nodejs.org/"

echo "      필수 도구 확인 완료 (docker, go, node, npm)"

# ---------------------------------------------------------------------------
# 0.5 Docker 데몬 자동 시작
# ---------------------------------------------------------------------------
if ! docker info &>/dev/null; then
    echo "[준비] Docker 데몬이 꺼져있습니다. Docker Desktop 시작 중..."
    open -a Docker
    echo -n "      Docker 준비 대기 중"
    for i in $(seq 1 60); do
        if docker info &>/dev/null; then
            echo ""
            echo "      Docker 준비 완료"
            break
        fi
        if [[ $i -eq 60 ]]; then
            echo ""
            echo "[오류] Docker가 60초 내에 시작되지 않았습니다. Docker Desktop을 수동으로 실행해주세요."
            exit 1
        fi
        echo -n "."
        sleep 1
    done
fi

# ---------------------------------------------------------------------------
# 0.6 포트 8080 점유 프로세스 정리
# ---------------------------------------------------------------------------
if lsof -ti:8080 &>/dev/null; then
    echo "      포트 8080 점유 프로세스 종료 중..."
    lsof -ti:8080 | xargs kill -9 2>/dev/null || true
    sleep 1
fi

# ---------------------------------------------------------------------------
# 1. .env 자동 생성 (없으면 .env.example 기반)
# ---------------------------------------------------------------------------
ENV_FILE="$ROOT/server/.env"
ENV_EXAMPLE="$ROOT/server/.env.example"

if [[ ! -f "$ENV_FILE" ]]; then
    echo "[준비] .env 파일 생성 중..."
    if [[ ! -f "$ENV_EXAMPLE" ]]; then
        echo "[오류] server/.env.example 파일이 없습니다."
        exit 1
    fi

    JWT_SECRET=$(openssl rand -hex 32)

    pushd "$ROOT/server" > /dev/null
    HASH_OUTPUT=$(go run ./cmd/hashpw admin 2>&1)
    popd > /dev/null
    PASSWORD_HASH=$(echo "$HASH_OUTPUT" | head -1 | tr -d '[:space:]')

    cat > "$ENV_FILE" <<EOF
OWLMON_JWT_SECRET=$JWT_SECRET
OWLMON_PASSWORD_HASH='$PASSWORD_HASH'
POSTGRES_DSN=postgres://owlmon:owlmon@localhost:5433/owlmon
EOF

    echo "      .env 생성 완료 (admin/admin)"
else
    echo "      .env 이미 존재 — 건너뜀"
fi

# ---------------------------------------------------------------------------
# 2. 의존성 설치
# ---------------------------------------------------------------------------
echo "[준비] Go 모듈 확인 중..."
(cd "$ROOT/server" && go mod download)
(cd "$ROOT/agent"  && go mod download)
echo "      Go 모듈 준비 완료"

if [[ ! -d "$ROOT/web/node_modules" ]]; then
    echo "[준비] npm install 실행 중..."
    (cd "$ROOT/web" && npm install)
    echo "      npm install 완료"
else
    echo "      node_modules 이미 존재 — 건너뜀"
fi

# ---------------------------------------------------------------------------
# 3. Docker 인프라
# ---------------------------------------------------------------------------
echo "[1/4] Docker 인프라 시작..."
docker compose -f "$ROOT/docker-compose.yml" up -d
echo "      완료"

# PostgreSQL ready 대기 (최대 30초)
echo "      PostgreSQL 준비 대기 중..."
for i in $(seq 1 30); do
    if docker exec owlmon-postgres pg_isready -U owlmon &>/dev/null; then
        echo "      PostgreSQL 준비 완료"
        break
    fi
    if [[ $i -eq 30 ]]; then
        echo "      [경고] PostgreSQL이 30초 내에 준비되지 않았습니다. 서버가 파일 기반 저장으로 폴백합니다."
    fi
    sleep 1
done

# ---------------------------------------------------------------------------
# 4. 서버 빌드 & 실행 (크래시 시 자동 재시작)
# ---------------------------------------------------------------------------
echo "[2/4] 서버 빌드 중..."
(cd "$ROOT/server" && go build -o owlmon-server-dev .)

echo "      서버 시작 (자동 재시작 활성화)..."
(
    cd "$ROOT/server"
    while true; do
        ./owlmon-server-dev &
        SERVER_PID=$!
        wait $SERVER_PID
        EXIT_CODE=$?
        [[ $EXIT_CODE -eq 0 ]] && break
        echo "[Server] 크래시 감지 (ExitCode: $EXIT_CODE) — 5초 후 재시작..."
        sleep 5
    done
) &
SERVER_JOB_PID=$!
echo "      PID: $SERVER_JOB_PID"

# ---------------------------------------------------------------------------
# 5. 에이전트 빌드 & 실행 (크래시 시 자동 재시작)
# ---------------------------------------------------------------------------
echo "[3/4] 에이전트 빌드 중..."
(cd "$ROOT/agent" && go build -o owlmon-agent-dev .)

echo "      에이전트 시작 (자동 재시작 활성화)..."
(
    cd "$ROOT/agent"
    while true; do
        ./owlmon-agent-dev &
        AGENT_PID=$!
        wait $AGENT_PID
        EXIT_CODE=$?
        [[ $EXIT_CODE -eq 0 ]] && break
        echo "[Agent] 크래시 감지 (ExitCode: $EXIT_CODE) — 5초 후 재시작..."
        sleep 5
    done
) &
AGENT_JOB_PID=$!
echo "      PID: $AGENT_JOB_PID"

# ---------------------------------------------------------------------------
# 6. 웹 개발 서버 (백그라운드, 로그 파일)
# ---------------------------------------------------------------------------
echo "[4/4] 웹 개발 서버 시작..."
(cd "$ROOT/web" && npm run dev > "$ROOT/web-dev.log" 2>&1) &
WEB_JOB_PID=$!
echo "      PID: $WEB_JOB_PID (로그: web-dev.log)"

# ---------------------------------------------------------------------------
# PID 저장 (stop.sh에서 사용)
# ---------------------------------------------------------------------------
echo "{\"server\": $SERVER_JOB_PID, \"agent\": $AGENT_JOB_PID, \"web\": $WEB_JOB_PID}" > "$ROOT/.dev-pids.json"

echo ""
echo "=== 실행 완료 ==="
sleep 2 && open "http://localhost:5173" &
echo "대시보드:   http://localhost:5173"
echo "API 서버:   http://localhost:8080"
echo "Prometheus: http://localhost:9090"
echo ""
echo "웹 로그 확인: tail -f web-dev.log"
echo "종료하려면:  make stop"
