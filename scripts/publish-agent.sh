#!/bin/bash
# 에이전트를 빌드·서명해 OWLmon 서버 self-update 디렉토리에 올린다 (각 에이전트가 6시간 내 자동 업데이트)
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="${TARGET:-owlmon@192.168.0.30:owlmon/data/owlmon-server/agents/}"
OUT="$(mktemp -d)/owlmon-agent-linux-amd64"

(cd "$REPO/agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$OUT" .)
(cd "$REPO/server" && go run ./cmd/agentsign "$OUT")
# 임시 이름으로 올린 뒤 mv로 교체 — 전송 중인 반쪽 파일이 서비스되지 않게
rsync -az "$OUT" "$TARGET.owlmon-agent-linux-amd64.tmp"
rsync -az "$OUT.sig" "$TARGET.owlmon-agent-linux-amd64.sig.tmp"
ssh "${TARGET%%:*}" "cd ${TARGET#*:} && mv .owlmon-agent-linux-amd64.sig.tmp owlmon-agent-linux-amd64.sig && mv .owlmon-agent-linux-amd64.tmp owlmon-agent-linux-amd64"
echo "게시 완료: $(shasum -a 256 "$OUT" | cut -c1-12)"
