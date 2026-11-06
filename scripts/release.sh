#!/bin/bash
# 폐쇄망 설치 번들 생성 — 인터넷 되는 PC(Docker 필요)에서 실행
#   bash scripts/release.sh 1.3.0
# 결과: release/owlmon-1.3.0.tar.gz (+ .sha256) — 이 파일 하나만 들고 가서 install.sh 실행
set -euo pipefail

VERSION="${1:?사용법: scripts/release.sh <버전> (예: 1.3.0)}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$REPO/release/owlmon-$VERSION"
PLATFORM=linux/amd64 # 설치 대상 서버 기준 (개발 Mac이 ARM이어도 amd64로 빌드)

cd "$REPO"
docker info >/dev/null 2>&1 || { echo "Docker가 실행 중이 아닙니다 (Docker Desktop을 켜세요)"; exit 1; }
[ -z "$(git status --porcelain -- server web docker-compose.yml infra)" ] || echo "⚠️ 커밋 안 된 변경이 번들에 들어갑니다"

rm -rf "$OUT" && mkdir -p "$OUT/infra"

echo "[1/4] OWLmon 이미지 빌드 ($PLATFORM, v$VERSION)"
OWLMON_VERSION="$VERSION" POSTGRES_PASSWORD=x DOCKER_DEFAULT_PLATFORM="$PLATFORM" \
  docker compose build owlmon-server owlmon-web owlmon-backup

echo "[2/4] 외부 이미지 받기 (설치 서버엔 인터넷이 없으므로 함께 묶음)"
EXTERNAL=$(OWLMON_VERSION="$VERSION" POSTGRES_PASSWORD=x docker compose config --images 2>/dev/null | grep -v '^owlmon-' | sort -u)
for img in $EXTERNAL; do docker pull --platform "$PLATFORM" "$img"; done

echo "[3/4] 이미지 저장"
# shellcheck disable=SC2086
docker save "owlmon-server:$VERSION" "owlmon-web:$VERSION" "owlmon-backup:$VERSION" $EXTERNAL | gzip > "$OUT/images.tar.gz"

echo "[4/4] 설정·스크립트 복사"
# 번들엔 소스가 없으므로 build: 블록을 빼고 image: 태그로만 실행되게 함
python3 - "$OUT/docker-compose.yml" <<'PY'
import re, sys
s = open("docker-compose.yml").read()
s = re.sub(r"\n    build:\n(      .*\n)+", "\n", s)
open(sys.argv[1], "w").write(s)
PY
grep -q "build:" "$OUT/docker-compose.yml" && { echo "compose에 build: 가 남음"; exit 1; }
cp .env.example scripts/install.sh scripts/restore-postgres.sh scripts/uninstall.sh "$OUT/"
cp -r infra/prometheus infra/otel infra/postgres "$OUT/infra/"
echo "$VERSION" > "$OUT/VERSION"
git rev-parse --short HEAD > "$OUT/COMMIT" 2>/dev/null || true

tar -C "$REPO/release" -czf "$REPO/release/owlmon-$VERSION.tar.gz" "owlmon-$VERSION"
(cd "$REPO/release" && shasum -a 256 "owlmon-$VERSION.tar.gz" > "owlmon-$VERSION.tar.gz.sha256")
rm -rf "$OUT"
echo "완료: release/owlmon-$VERSION.tar.gz ($(du -h "$REPO/release/owlmon-$VERSION.tar.gz" | cut -f1))"
