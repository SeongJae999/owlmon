#!/bin/bash
# 에이전트를 빌드·서명해 OWLmon 서버 self-update 디렉토리에 올린다 (각 에이전트가 6시간 내 자동 업데이트)
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${SERVER:-owlmon@192.168.0.30}"
NAME="owlmon-agent-linux-amd64"
OUT="$(mktemp -d)/$NAME"

(cd "$REPO/agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$OUT" .)
(cd "$REPO/server" && go run ./cmd/agentsign "$OUT")
rsync -az "$OUT" "$OUT.sig" "$SERVER:/tmp/"
# 볼륨 디렉토리는 컨테이너 소유라 docker cp로 넣음
ssh "$SERVER" "docker cp /tmp/$NAME.sig owlmon-server:/app/data/agents/ && docker cp /tmp/$NAME owlmon-server:/app/data/agents/ && rm /tmp/$NAME /tmp/$NAME.sig"
echo "게시 완료: $(shasum -a 256 "$OUT" | cut -c1-12)"
