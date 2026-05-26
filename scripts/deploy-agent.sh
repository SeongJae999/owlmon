#!/bin/bash
# OWLmon Agent 일괄 배포 — Mac에서 실행
#
# 사용:
#   bash deploy-agent.sh                    # 기본 호스트 4대
#   HOSTS="will-server hi-solution" bash deploy-agent.sh  # 사용자 지정
#
# 동작:
#   1. agent Linux amd64 빌드
#   2. 각 호스트에 rsync
#   3. ssh로 sudo systemctl 재시작
#
# ⚠️ 각 호스트에서 sudo 비번 입력 필요 (NOPASSWD 설정 시 자동)

set -uo pipefail

# 기본 호스트 (SSH config alias 사용)
HOSTS="${HOSTS:-willdev willkomo will hi-solution}"
AGENT_DIR="${AGENT_DIR:-/Users/skane/mango/owlmon/agent}"
BUILD_OUT="/tmp/owlmon-agent-linux-amd64"

# 색상
if [ -t 1 ]; then
    GRN=$'\033[32m'; RED=$'\033[31m'; YEL=$'\033[33m'; CYN=$'\033[36m'; RST=$'\033[0m'; BOLD=$'\033[1m'
else
    GRN=''; RED=''; YEL=''; CYN=''; RST=''; BOLD=''
fi

echo "${BOLD}╔══════════════════════════════════════════════════════════╗"
echo "║      OWLmon Agent 일괄 배포                              ║"
echo "╚══════════════════════════════════════════════════════════╝${RST}"
echo

# ─── 1. 빌드 ──────────────────────────────────────────────
echo "${CYN}[1/3] Linux amd64 빌드 (CGO=0)${RST}"
cd "$AGENT_DIR"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BUILD_OUT" .
SIZE=$(du -h "$BUILD_OUT" | awk '{print $1}')
echo "      → ${GRN}$BUILD_OUT ($SIZE)${RST}"
echo

# ─── 2~3. 호스트별 배포 ───────────────────────────────────
SUCCESS=()
FAIL=()
SKIP=()

for HOST in $HOSTS; do
    echo "${BOLD}─── $HOST ───${RST}"

    # 도달성 체크
    if ! ssh -o ConnectTimeout=5 -o BatchMode=yes "$HOST" 'echo ok' >/dev/null 2>&1; then
        echo "  ${YEL}⏭️  SSH 연결 실패 — 건너뜀${RST}"
        SKIP+=("$HOST")
        echo
        continue
    fi

    # rsync 전송
    if ! rsync -az "$BUILD_OUT" "$HOST:/tmp/owlmon-agent-new" 2>&1 | tail -3; then
        echo "  ${RED}❌ rsync 실패${RST}"
        FAIL+=("$HOST")
        echo
        continue
    fi

    # root 접속이면 sudo 생략, 일반 사용자면 sudo + 비번
    REMOTE_USER=$(ssh -o BatchMode=yes "$HOST" 'whoami' 2>/dev/null)
    if [ "$REMOTE_USER" = "root" ]; then
        SUDO=""
        echo "  ${CYN}→ root 접속 — sudo 없이 재시작${RST}"
    else
        SUDO="sudo"
        echo "  ${CYN}→ $REMOTE_USER 접속 — sudo 비번 입력 필요${RST}"
    fi

    # 한 번의 ssh 세션에서:
    #   1) 바이너리 교체
    #   2) config.yaml에 self_update 섹션 없으면 추가 (다음부턴 비번 0번)
    #   3) chown/chmod
    #   4) 재시작
    REMOTE_SCRIPT='
        set -e
        '"$SUDO"' systemctl stop owlmon-agent
        '"$SUDO"' cp /tmp/owlmon-agent-new /opt/owlmon/owlmon-agent
        '"$SUDO"' chmod +x /opt/owlmon/owlmon-agent
        '"$SUDO"' chown owlmon-agent:owlmon-agent /opt/owlmon/owlmon-agent
        if ! grep -q "^self_update:" /opt/owlmon/config.yaml 2>/dev/null; then
            '"$SUDO"' bash -c "cat >> /opt/owlmon/config.yaml <<YAML

# 자가 업데이트 (망분리 환경 자동 배포) — 6h마다 OWLmon 서버 체크 + sha256 검증 후 교체
self_update:
  enabled: true
  check_interval: 6h
YAML"
            echo \"  ☑ config.yaml에 self_update 섹션 추가됨\"
        else
            echo \"  ☑ self_update 이미 활성\"
        fi
        '"$SUDO"' systemctl start owlmon-agent
        sleep 1
        systemctl is-active owlmon-agent
    '
    if ssh -t "$HOST" "$REMOTE_SCRIPT"; then
        echo "  ${GRN}✅ $HOST — 재시작 성공${RST}"
        SUCCESS+=("$HOST")
    else
        echo "  ${RED}❌ $HOST — sudo 명령 실패 (비번 틀림 또는 권한)${RST}"
        FAIL+=("$HOST")
    fi
    echo
done

# ─── 요약 ─────────────────────────────────────────────────
echo "${BOLD}╔══════════════════════════════════════════════════════════╗"
echo "║      배포 요약                                            ║"
echo "╚══════════════════════════════════════════════════════════╝${RST}"
echo "  ${GRN}✅ 성공: ${#SUCCESS[@]}${RST}  ${YEL}⏭️  건너뜀: ${#SKIP[@]}${RST}  ${RED}❌ 실패: ${#FAIL[@]}${RST}"
[ ${#SUCCESS[@]} -gt 0 ] && echo "    • ${SUCCESS[*]}"
[ ${#SKIP[@]} -gt 0 ]    && echo "  ${YEL}⏭️  건너뜀:${RST} ${SKIP[*]}"
[ ${#FAIL[@]} -gt 0 ]    && echo "  ${RED}❌ 실패:${RST} ${FAIL[*]}"

# 실패가 있으면 non-zero
[ ${#FAIL[@]} -gt 0 ] && exit 1
exit 0
