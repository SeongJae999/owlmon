#!/bin/bash
# OWLmon 에이전트 Linux 서비스 설치 스크립트
# root 권한으로 실행 필요
# 사용법: sudo bash install-agent-linux.sh --endpoint "192.168.1.10:4317"

set -e

ENDPOINT="localhost:4317"
INSTALL_DIR="/opt/owlmon"
SERVICE_NAME="owlmon-agent"
SERVICE_USER="owlmon"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_SRC="$SCRIPT_DIR/../agent"

# 인자 파싱
while [[ $# -gt 0 ]]; do
    case $1 in
        --endpoint) ENDPOINT="$2"; shift 2 ;;
        --install-dir) INSTALL_DIR="$2"; shift 2 ;;
        *) echo "알 수 없는 옵션: $1"; exit 1 ;;
    esac
done

# root 확인
if [[ $EUID -ne 0 ]]; then
    echo "root 권한으로 실행하세요: sudo bash $0"
    exit 1
fi

echo "=== OWLmon 에이전트 설치 ==="

# 1. Go 빌드
echo "[1/5] 에이전트 빌드 중..."
cd "$AGENT_SRC"
go build -o "$INSTALL_DIR/owlmon-agent" .
echo "      빌드 완료: $INSTALL_DIR/owlmon-agent"

# 2. 설치 디렉토리 준비
echo "[2/5] 설치 디렉토리 준비 중..."
mkdir -p "$INSTALL_DIR"

# config.yaml 복사
if [[ -f "$AGENT_SRC/config.yaml" ]]; then
    cp "$AGENT_SRC/config.yaml" "$INSTALL_DIR/config.yaml"
    echo "      config.yaml 복사 완료"
fi

# 3. 전용 사용자 생성 (보안)
echo "[3/5] 서비스 사용자 생성 중..."
if ! id "$SERVICE_USER" &>/dev/null; then
    useradd --system --no-create-home --shell /sbin/nologin "$SERVICE_USER"
    echo "      사용자 생성: $SERVICE_USER"
else
    echo "      사용자 이미 존재: $SERVICE_USER"
fi
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/owlmon-agent"

# 4. systemd 유닛 파일 생성
echo "[4/5] systemd 서비스 등록 중..."
cat > /etc/systemd/system/$SERVICE_NAME.service <<EOF
[Unit]
Description=OWLmon Monitoring Agent
Documentation=https://github.com/SeongJae999/owlmon
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/owlmon-agent
Restart=always
RestartSec=10
Environment=OWLMON_OTLP_ENDPOINT=$ENDPOINT
Environment=OWLMON_CONFIG=$INSTALL_DIR/config.yaml

# 로그는 journald로 수집
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
EOF

# 5. 서비스 시작
echo "[5/5] 서비스 시작 중..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

sleep 2
STATUS=$(systemctl is-active "$SERVICE_NAME")

echo ""
echo "=== 설치 완료 ==="
echo "서비스 이름: $SERVICE_NAME"
echo "상태: $STATUS"
echo "설치 경로: $INSTALL_DIR"
echo "OTLP 엔드포인트: $ENDPOINT"
echo ""
echo "서비스 관리 명령어:"
echo "  시작:  systemctl start $SERVICE_NAME"
echo "  중지:  systemctl stop $SERVICE_NAME"
echo "  상태:  systemctl status $SERVICE_NAME"
echo "  로그:  journalctl -u $SERVICE_NAME -f"
echo ""
echo "제거하려면: sudo bash uninstall-agent-linux.sh"
