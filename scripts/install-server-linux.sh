#!/bin/bash
# OWLmon 서버 Linux 서비스 설치 스크립트
# 사용법: sudo bash install-server-linux.sh --password "비밀번호" --smtp-host "smtp.gmail.com" ...

set -e

# 기본값
USERNAME="admin"
PASSWORD=""
PROMETHEUS_URL="http://localhost:9090"
LISTEN_ADDR=":8080"
SMTP_HOST=""
SMTP_PORT="587"
SMTP_USERNAME=""
SMTP_PASSWORD=""
SMTP_FROM=""
SMTP_TO=""
POSTGRES_DSN=""
INSTALL_DIR="/opt/owlmon-server"
SERVICE_NAME="owlmon-server"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_SRC="$SCRIPT_DIR/../server"

# 인자 파싱
while [[ $# -gt 0 ]]; do
    case $1 in
        --password)      PASSWORD="$2";       shift 2 ;;
        --username)      USERNAME="$2";       shift 2 ;;
        --prometheus)    PROMETHEUS_URL="$2"; shift 2 ;;
        --listen)        LISTEN_ADDR="$2";    shift 2 ;;
        --smtp-host)     SMTP_HOST="$2";      shift 2 ;;
        --smtp-port)     SMTP_PORT="$2";      shift 2 ;;
        --smtp-username) SMTP_USERNAME="$2";  shift 2 ;;
        --smtp-password) SMTP_PASSWORD="$2";  shift 2 ;;
        --smtp-from)     SMTP_FROM="$2";      shift 2 ;;
        --smtp-to)       SMTP_TO="$2";        shift 2 ;;
        --postgres-dsn)  POSTGRES_DSN="$2";  shift 2 ;;
        --install-dir)   INSTALL_DIR="$2";    shift 2 ;;
        *) echo "알 수 없는 옵션: $1"; exit 1 ;;
    esac
done

if [[ $EUID -ne 0 ]]; then
    echo "root 권한으로 실행하세요: sudo bash $0"
    exit 1
fi

if [[ -z "$PASSWORD" ]]; then
    echo "--password 옵션이 필요합니다"
    exit 1
fi

echo "=== OWLmon 서버 설치 ==="

# 1. 비밀번호 해시 생성
echo "[1/4] 비밀번호 해시 생성 중..."
PASSWORD_HASH=$(cd "$SERVER_SRC" && go run ./cmd/hashpw "$PASSWORD" | grep '^\$2')
echo "      해시 생성 완료"

# 2. 빌드
echo "[2/4] 서버 빌드 중..."
mkdir -p "$INSTALL_DIR"
cd "$SERVER_SRC"
go build -o "$INSTALL_DIR/owlmon-server" .
echo "      빌드 완료: $INSTALL_DIR/owlmon-server"

# 3. 기존 서비스 제거
echo "[3/4] 서비스 등록 준비 중..."
systemctl stop "$SERVICE_NAME" 2>/dev/null || true
systemctl disable "$SERVICE_NAME" 2>/dev/null || true

# 4. systemd 유닛 파일 생성
echo "[4/4] systemd 서비스 등록 중..."

ENV_LINES="Environment=OWLMON_USERNAME=$USERNAME
Environment=OWLMON_PASSWORD_HASH=$PASSWORD_HASH
Environment=OWLMON_PROMETHEUS_URL=$PROMETHEUS_URL
Environment=OWLMON_LISTEN=$LISTEN_ADDR"

if [[ -n "$SMTP_HOST" ]]; then
    ENV_LINES="$ENV_LINES
Environment=SMTP_HOST=$SMTP_HOST
Environment=SMTP_PORT=$SMTP_PORT
Environment=SMTP_USERNAME=$SMTP_USERNAME
Environment=SMTP_PASSWORD=$SMTP_PASSWORD
Environment=SMTP_FROM=$SMTP_FROM
Environment=SMTP_TO=$SMTP_TO"
fi

if [[ -n "$POSTGRES_DSN" ]]; then
    ENV_LINES="$ENV_LINES
Environment=POSTGRES_DSN=$POSTGRES_DSN"
fi

cat > /etc/systemd/system/$SERVICE_NAME.service <<EOF
[Unit]
Description=OWLmon Server
Documentation=https://github.com/SeongJae999/owlmon
After=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/owlmon-server
Restart=always
RestartSec=10
$ENV_LINES
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

sleep 2
STATUS=$(systemctl is-active "$SERVICE_NAME")

echo ""
echo "=== 설치 완료 ==="
echo "서비스 이름: $SERVICE_NAME"
echo "상태: $STATUS"
echo "접속 주소: http://$(hostname -I | awk '{print $1}')$LISTEN_ADDR"
[[ -n "$SMTP_HOST" ]] && echo "이메일 알림: 활성화 ($SMTP_TO)" || echo "이메일 알림: 비활성화"
echo ""
echo "서비스 관리 명령어:"
echo "  시작:  systemctl start $SERVICE_NAME"
echo "  중지:  systemctl stop $SERVICE_NAME"
echo "  상태:  systemctl status $SERVICE_NAME"
echo "  로그:  journalctl -u $SERVICE_NAME -f"
echo ""
echo "제거하려면: sudo bash uninstall-server-linux.sh"
