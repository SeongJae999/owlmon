#!/bin/bash
# OWLmon 제거 — 설치 폴더에서 실행
#   sudo bash uninstall.sh [설치경로]
# 기본은 데이터 보존: 컨테이너·이미지만 지우고 data/와 .env를 남김 (같은 경로에 다시 설치하면 데이터 그대로 사용)
# 에이전트는 각 서버에서 uninstall-agent-* 스크립트로 따로 제거
set -euo pipefail

TARGET="$(cd "${1:-$(dirname "$0")}" && pwd)"

[ "$(id -u)" = 0 ] || { echo "root 권한이 필요합니다 (data/ 폴더 소유자가 컨테이너 사용자임): sudo bash $0"; exit 1; }
# 잘못된 경로를 통째로 지우지 않도록 OWLmon 설치 폴더가 맞는지 확인
if [ "$TARGET" = "/" ] || [ ! -f "$TARGET/docker-compose.yml" ] || ! grep -q 'container_name: owlmon-server' "$TARGET/docker-compose.yml"; then
    echo "[오류] OWLmon 설치 폴더가 아닙니다: $TARGET"
    exit 1
fi

# 번들을 푼 폴더에도 같은 compose가 있어 인자 없이 실행하면 번들을 지울 수 있음
if [ -f "$TARGET/images.tar.gz" ]; then
    echo "[오류] 설치 번들 폴더입니다. 설치 경로를 지정하세요: sudo bash $0 /opt/owlmon"
    exit 1
fi

echo "OWLmon 제거 대상: $TARGET"
echo "  1) 프로그램만 제거 — 데이터(data/)와 설정(.env) 보존 (권장)"
echo "  2) 전부 삭제 — DB·메트릭·백업까지 모두 삭제, 복구 불가"
echo "  그 외) 취소"
read -r -p "선택: " MODE
case "$MODE" in
    1) ;;
    2)
        read -r -p "모든 데이터와 백업을 삭제합니다. 계속하려면 'delete' 입력: " CONFIRM
        [ "$CONFIRM" = "delete" ] || { echo "취소"; exit 0; }
        ;;
    *) echo "취소"; exit 0 ;;
esac

cd "$TARGET"
# .env가 없어도 필수 변수 검사에 막혀 제거를 못 하는 일이 없게 함
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-unused}" docker compose down --rmi all --remove-orphans

if [ "$MODE" = 2 ]; then
    cd /
    rm -rf "$TARGET"
    echo "삭제 완료: $TARGET"
else
    rm -f docker-compose.yml .env.example restore-postgres.sh uninstall.sh
    rm -rf infra
    echo "제거 완료 — 데이터와 설정은 $TARGET 에 남아 있습니다 (data/, .env)."
    echo "다시 설치: 번들 폴더에서 sudo bash install.sh $TARGET"
    echo "완전히 지우려면: sudo rm -rf $TARGET"
fi
