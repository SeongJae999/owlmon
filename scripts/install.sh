#!/bin/bash
# OWLmon 설치·업그레이드 — 번들을 푼 폴더에서 실행 (인터넷 불필요)
#   tar xzf owlmon-1.3.0.tar.gz && cd owlmon-1.3.0 && sudo bash install.sh [설치경로]
# 업그레이드: 같은 설치경로로 다시 실행하면 .env와 data/는 그대로 두고 이미지·설정만 교체
set -euo pipefail

BUNDLE="$(cd "$(dirname "$0")" && pwd)"
TARGET="${1:-/opt/owlmon}"
VERSION="$(cat "$BUNDLE/VERSION")"

command -v docker >/dev/null || { echo "Docker가 필요합니다"; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "Docker Compose v2가 필요합니다"; exit 1; }

echo "[1/4] 이미지 불러오기 (v$VERSION)"
docker load -i "$BUNDLE/images.tar.gz" | grep -v '^$' || true

echo "[2/4] 설정 복사 → $TARGET"
mkdir -p "$TARGET/infra" "$TARGET/data/prometheus" "$TARGET/data/owlmon-server"
# 컨테이너가 root가 아닌 사용자로 돌기 때문에 데이터 폴더 소유자를 맞춤 (prometheus=nobody, server=owlmon)
chown 65534:65534 "$TARGET/data/prometheus"
chown 100:101 "$TARGET/data/owlmon-server"
cp "$BUNDLE/docker-compose.yml" "$BUNDLE/.env.example" "$TARGET/"
cp -r "$BUNDLE/infra/." "$TARGET/infra/"
cd "$TARGET"

set_env() { # 키가 있으면 값 교체, 없으면 추가
  if grep -q "^$1=" .env; then sed -i.bak "s|^$1=.*|$1=$2|" .env && rm -f .env.bak; else echo "$1=$2" >> .env; fi
}

if [ ! -f .env ]; then
  echo "[3/4] 첫 설치 — 비밀값 자동 생성"
  cp .env.example .env
  chmod 600 .env
  set_env POSTGRES_PASSWORD "$(openssl rand -hex 16)"
  set_env OWLMON_AGENT_KEY "$(openssl rand -hex 16)"
  while :; do
    read -rsp "첫 관리자(admin) 비밀번호 (9자 이상, 영문·숫자·특수문자 중 2종류 이상): " PW; echo
    read -rsp "다시 입력: " PW2; echo
    [ "$PW" = "$PW2" ] && [ ${#PW} -ge 9 ] && break
    echo "일치하지 않거나 9자 미만입니다. 다시 입력하세요."
  done
  HASH=$(printf '%s\n' "$PW" | docker run --rm -i --entrypoint /app/owlmon-server "owlmon-server:$VERSION" hash-password)
  unset PW PW2
  set_env OWLMON_PASSWORD_HASH "'$HASH'"
else
  echo "[3/4] 기존 .env 유지 (업그레이드)"
  grep -q "^POSTGRES_PASSWORD=." .env || { echo "⚠️ .env에 POSTGRES_PASSWORD가 없습니다 — 설정 후 다시 실행하세요"; exit 1; }
fi
set_env OWLMON_VERSION "$VERSION"

echo "[4/4] 실행"
docker compose up -d --no-build --remove-orphans
sleep 5
docker compose ps --format "table {{.Name}}\t{{.Status}}"
echo
echo "설치 완료 (v$VERSION) — 웹: http://<이 서버 IP>/"
echo "에이전트 키: $TARGET/.env 의 OWLMON_AGENT_KEY (에이전트 설치 시 필요)"
