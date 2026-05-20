#!/bin/bash
# OWLmon 에이전트 Linux 서비스 설치 스크립트
#
# 사용법 (root 권한 필수):
#   sudo bash install-agent-linux.sh \
#     --server   http://192.168.0.30:8080 \
#     --otlp     192.168.0.30:4317 \
#     --agent-key YOUR_AGENT_KEY \
#     [--collect-interval 15s] \
#     [--journald]                  # journald 로그 수집 켜기 (Debian 12 / CentOS 8+ 권장)
#
# 빌드 모드 (Go 필요):
#   --build                          # 현재 디렉토리의 agent/ 소스로 빌드 (기본)
#   --binary /path/to/owlmon-agent   # 미리 빌드된 바이너리 사용 (Go 불필요)

set -e

# ─── 기본값 ──────────────────────────────────────────────
ENDPOINT="localhost:4317"
SERVER_URL="http://localhost:8080"
AGENT_KEY=""
COLLECT_INTERVAL="15s"
ENABLE_JOURNALD=false
USE_BINARY=""
INSTALL_DIR="/opt/owlmon"
SERVICE_NAME="owlmon-agent"
SERVICE_USER="owlmon-agent"
WAL_DIR="/var/lib/owlmon"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_SRC="$SCRIPT_DIR/../agent"

# ─── 인자 파싱 ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --endpoint|--otlp) ENDPOINT="$2"; shift 2 ;;
        --server)          SERVER_URL="$2"; shift 2 ;;
        --agent-key)       AGENT_KEY="$2"; shift 2 ;;
        --collect-interval) COLLECT_INTERVAL="$2"; shift 2 ;;
        --journald)        ENABLE_JOURNALD=true; shift ;;
        --build)           USE_BINARY=""; shift ;;
        --binary)          USE_BINARY="$2"; shift 2 ;;
        --install-dir)     INSTALL_DIR="$2"; shift 2 ;;
        -h|--help)
            sed -n '2,15p' "$0"
            exit 0
            ;;
        *) echo "알 수 없는 옵션: $1"; exit 1 ;;
    esac
done

# ─── 사전 점검 ────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
    echo "[오류] root 권한으로 실행하세요: sudo bash $0 ..."
    exit 1
fi

if [[ -z "$AGENT_KEY" ]]; then
    echo "[경고] --agent-key 가 비어있습니다. 로그 수집이 401로 거부될 수 있습니다."
fi

echo "=== OWLmon 에이전트 설치 ==="
echo "  Server URL:       $SERVER_URL"
echo "  OTLP endpoint:    $ENDPOINT"
echo "  Collect interval: $COLLECT_INTERVAL"
echo "  Journald 수집:    $ENABLE_JOURNALD"
echo "  Install dir:      $INSTALL_DIR"
echo ""

# ─── 1. 바이너리 준비 (빌드 또는 복사) ───────────────────
mkdir -p "$INSTALL_DIR" "$WAL_DIR"

if [[ -n "$USE_BINARY" ]]; then
    echo "[1/6] 사전 빌드 바이너리 복사: $USE_BINARY"
    if [[ ! -f "$USE_BINARY" ]]; then
        echo "      [오류] 파일 없음: $USE_BINARY"; exit 1
    fi
    cp "$USE_BINARY" "$INSTALL_DIR/owlmon-agent"
else
    echo "[1/6] 에이전트 빌드 중..."
    if ! command -v go &>/dev/null; then
        echo "      [오류] Go가 설치돼있지 않습니다. --binary 옵션으로 사전 빌드 바이너리 사용 권장."
        exit 1
    fi
    if [[ ! -d "$AGENT_SRC" ]]; then
        echo "      [오류] agent 소스 디렉토리 없음: $AGENT_SRC"; exit 1
    fi
    (cd "$AGENT_SRC" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/owlmon-agent" .)
fi
chmod +x "$INSTALL_DIR/owlmon-agent"

# ─── 2. config.yaml 동적 생성 ───────────────────────────
echo "[2/6] config.yaml 생성 중..."
cat > "$INSTALL_DIR/config.yaml" <<YAML
# OWLmon 에이전트 설정 (install-agent-linux.sh 자동 생성)
otlp_endpoint: "$ENDPOINT"

checks:
  - name: "OWLmon Server Reachable"
    type: tcp
    target: "${ENDPOINT}"
    interval: 60s

logs:
  enabled: true
  server_url: "$SERVER_URL"
  agent_key: "$AGENT_KEY"
  wal_path: "$WAL_DIR/logs-wal.json"

  journald:
    enabled: $ENABLE_JOURNALD
    source: "journald"
    # 매칭된 라인만 송신 — DB 폭증 방지. 빈 배열이면 안전 기본값 사용
    include_patterns: ["error", "fatal", "panic", "warning", "warn", "fail", "denied", "OOM", "kill"]

  tails: []
YAML
echo "      $INSTALL_DIR/config.yaml"

# ─── 3. 전용 시스템 사용자 + 그룹 ──────────────────────
echo "[3/6] 서비스 사용자/권한 설정 중..."
if ! id "$SERVICE_USER" &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER" 2>/dev/null \
      || useradd --system --no-create-home --shell /sbin/nologin "$SERVICE_USER"
    echo "      사용자 생성: $SERVICE_USER"
else
    echo "      사용자 이미 존재: $SERVICE_USER"
fi

# journald 수집 시 systemd-journal 그룹 필요
if [[ "$ENABLE_JOURNALD" == "true" ]] && getent group systemd-journal >/dev/null; then
    usermod -aG systemd-journal "$SERVICE_USER"
    echo "      systemd-journal 그룹 추가 (journald 읽기 권한)"
fi

chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR" "$WAL_DIR"

# ─── 4. systemd unit ────────────────────────────────────
echo "[4/6] systemd 서비스 등록 중..."
cat > /etc/systemd/system/$SERVICE_NAME.service <<UNIT
[Unit]
Description=OWLmon Monitoring Agent
Documentation=https://github.com/SeongJae999/owlmon
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/owlmon-agent
Restart=always
RestartSec=10
Environment=OWLMON_OTLP_ENDPOINT=$ENDPOINT
Environment=OWLMON_SERVER_URL=$SERVER_URL
Environment=OWLMON_CONFIG=$INSTALL_DIR/config.yaml
Environment=OWLMON_COLLECT_INTERVAL=$COLLECT_INTERVAL

# 보안 강화
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$WAL_DIR

StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
UNIT

# ─── 5. 시작 + 활성화 ───────────────────────────────────
echo "[5/6] 서비스 시작 중..."
systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

sleep 3

# ─── 6. 검증 ────────────────────────────────────────────
echo "[6/6] 검증 중..."
STATUS=$(systemctl is-active "$SERVICE_NAME" || echo "inactive")
RECENT_LOG=$(journalctl -u "$SERVICE_NAME" -n 20 --no-pager 2>/dev/null | grep -E "호스트 스펙|owlmon-agent 시작" | tail -2)

echo ""
echo "=== 설치 완료 ==="
echo "  서비스 상태:  $STATUS"
echo "  설치 경로:    $INSTALL_DIR"
echo "  Server:       $SERVER_URL"
echo ""
if [[ -n "$RECENT_LOG" ]]; then
    echo "[최근 로그]"
    echo "$RECENT_LOG" | sed 's/^/  /'
fi
echo ""
echo "서비스 관리:"
echo "  상태:  systemctl status $SERVICE_NAME"
echo "  로그:  journalctl -u $SERVICE_NAME -f"
echo "  중지:  systemctl stop $SERVICE_NAME"
echo "  제거:  sudo bash uninstall-agent-linux.sh"

if [[ "$STATUS" != "active" ]]; then
    echo ""
    echo "[경고] 서비스가 active 상태가 아닙니다. 로그 확인: journalctl -u $SERVICE_NAME -n 30"
    exit 1
fi
