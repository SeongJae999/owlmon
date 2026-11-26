#!/bin/bash
# OWLmon 에이전트 Linux 서비스 설치 스크립트
#
# 사용법 (root 권한 필수) — 보통은 웹 화면 [서버 추가]가 만들어 준 한 줄 명령으로 실행:
#   curl -fsS https://<OWLmon 서버>/api/agent/install-agent.sh | sudo bash -s -- \
#     --server https://<OWLmon 서버> --enroll-token <화면에서 발급한 토큰> \
#     [--pin sha256//<공개키 지문> --server-cert-sha256 <인증서 지문>]  # 자체 서명 인증서일 때 (화면이 넣어 줌)
#     [--collect-interval 15s] \
#     [--journald]                  # journald 로그 수집 켜기 (Debian 12 / CentOS 8+ 권장)
#
# 바이너리: 등록 토큰이 있으면 OWLmon 서버에서 받음 (기본)
#   --build                          # 현재 디렉토리의 agent/ 소스로 빌드 (Go 필요)
#   --binary /path/to/owlmon-agent   # 미리 빌드된 바이너리 사용
# 예전 방식 (공용 키, 서버에서 OWLMON_LEGACY_SHARED_KEY=true일 때만 받음):
#   --server http://<서버>:8080 --otlp <서버>:4317 --agent-key <공용 키>

set -e

# ─── 기본값 ──────────────────────────────────────────────
ENDPOINT=""
SERVER_URL="http://localhost:8080"
AGENT_KEY=""
ENROLL_TOKEN=""
PIN=""
CERT_SHA256=""
USE_BUILD=false
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
        --enroll-token)    ENROLL_TOKEN="$2"; shift 2 ;;
        --pin)             PIN="$2"; shift 2 ;;
        --server-cert-sha256) CERT_SHA256="$2"; shift 2 ;;
        --collect-interval) COLLECT_INTERVAL="$2"; shift 2 ;;
        --journald)        ENABLE_JOURNALD=true; shift ;;
        --build)           USE_BUILD=true; shift ;;
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

if [[ -z "$AGENT_KEY" && -z "$ENROLL_TOKEN" ]]; then
    echo "[오류] --enroll-token 이 필요합니다. 웹 화면 [서버 추가]에서 설치 명령을 받으세요."
    exit 1
fi

# 등록 토큰과 응답으로 받는 서버 전용 키가 평문으로 오가지 않게 https만 (이 서버 안 localhost 시험은 예외)
if [[ -n "$ENROLL_TOKEN" && "$SERVER_URL" != https://* && ! "$SERVER_URL" =~ ^http://(localhost|127\.0\.0\.1)(:|/|$) ]]; then
    echo "[오류] 등록 토큰은 https 서버 주소로만 보냅니다: --server https://<OWLmon 서버>"
    exit 1
fi

# 서버 인증서 확인 — 자체 서명이면 공개키 지문 고정(--pin), 아니면 시스템 CA로 검증. 검증을 끄지 않는다
CURL=(curl -fsS --retry 2)
[[ -n "$PIN" ]] && CURL+=(-k --pinnedpubkey "$PIN")

# https 서버면 메트릭도 같은 주소(443)로 TLS 전송 (nginx가 키 확인 후 Collector로 넘김)
OTLP_TLS=false
if [[ "$SERVER_URL" == https://* ]]; then
    OTLP_TLS=true
    HOSTPORT="${SERVER_URL#https://}"; HOSTPORT="${HOSTPORT%%/*}"
    [[ "$HOSTPORT" == *:* ]] || HOSTPORT="$HOSTPORT:443"
    ENDPOINT="${ENDPOINT:-$HOSTPORT}"
fi
ENDPOINT="${ENDPOINT:-localhost:4317}"

echo "=== OWLmon 에이전트 설치 ==="
echo "  Server URL:       $SERVER_URL"
echo "  OTLP endpoint:    $ENDPOINT (TLS: $OTLP_TLS)"
echo "  Collect interval: $COLLECT_INTERVAL"
echo "  Journald 수집:    $ENABLE_JOURNALD"
echo "  Install dir:      $INSTALL_DIR"
echo ""

# ─── 1. 바이너리 준비 (빌드 또는 복사) ───────────────────
mkdir -p "$INSTALL_DIR" "$WAL_DIR"

# 다시 설치할 때 실행 중인 바이너리를 덮어쓰지 않게 먼저 멈춤
systemctl stop "$SERVICE_NAME" 2>/dev/null || true

if [[ -n "$USE_BINARY" ]]; then
    echo "[1/6] 사전 빌드 바이너리 복사: $USE_BINARY"
    if [[ ! -f "$USE_BINARY" ]]; then
        echo "      [오류] 파일 없음: $USE_BINARY"; exit 1
    fi
    cp "$USE_BINARY" "$INSTALL_DIR/owlmon-agent"
elif [[ -n "$ENROLL_TOKEN" && "$USE_BUILD" != true ]]; then
    case "$(uname -m)" in
        x86_64|amd64) ARCH=amd64 ;;
        aarch64|arm64) ARCH=arm64 ;;
        *) echo "      [오류] 지원하지 않는 CPU: $(uname -m)"; exit 1 ;;
    esac
    echo "[1/6] OWLmon 서버에서 에이전트 받는 중 (linux/$ARCH)..."
    # 아직 키가 없으므로 등록 토큰으로 받음 (토큰은 소진되지 않고, 에이전트가 처음 실행될 때 키로 바뀜)
    if ! "${CURL[@]}" -H "X-Enroll-Token: $ENROLL_TOKEN" -o "$INSTALL_DIR/owlmon-agent.new" \
        "$SERVER_URL/api/agent/binary?os=linux&arch=$ARCH"; then
        rm -f "$INSTALL_DIR/owlmon-agent.new"
        echo "      [오류] 받기 실패 — 토큰이 만료·사용됐거나, 서버에 linux/$ARCH 에이전트가 올라가 있지 않습니다 (scripts/publish-agent.sh)"
        exit 1
    fi
    mv "$INSTALL_DIR/owlmon-agent.new" "$INSTALL_DIR/owlmon-agent"
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
# 토큰·키가 든 파일 — 만드는 순간부터 root만 읽게 (아래 chmod 600 전에 다른 사용자가 읽지 못하게)
OLD_UMASK=$(umask); umask 077
cat > "$INSTALL_DIR/config.yaml" <<YAML
# OWLmon 에이전트 설정 (install-agent-linux.sh 자동 생성)
otlp_endpoint: "$ENDPOINT"
otlp_tls: $OTLP_TLS
server_cert_sha256: "$CERT_SHA256"
# 일회용 — 처음 실행 때 이 서버 전용 키로 바뀌어 agent.key(0600)에 저장됨
enroll_token: "$ENROLL_TOKEN"

# 서비스 체크 — 사내 SMTP/DNS/업무 API 등을 추가하세요
# checks:
#   - name: "내부 메일 서버"
#     type: tcp
#     target: "mail.example.com:25"
#     interval: 60s
checks: []

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
umask "$OLD_UMASK"
echo "      $INSTALL_DIR/config.yaml"
# 새 토큰으로 다시 설치하면 예전 키는 버리고 새로 등록 (예전 키는 서버에서도 새 키로 바뀌어 무효)
[[ -n "$ENROLL_TOKEN" ]] && rm -f "$INSTALL_DIR/agent.key"

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
chmod 600 "$INSTALL_DIR/config.yaml"

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
# self-update의 바이너리 교체와 메트릭 버퍼 저장에 설치 경로 쓰기가 필요하다
ReadWritePaths=$WAL_DIR $INSTALL_DIR

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
RECENT_LOG=$(journalctl -u "$SERVICE_NAME" -n 20 --no-pager 2>/dev/null | grep -E "서버 등록|호스트 스펙|owlmon-agent 시작" | tail -2)

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
