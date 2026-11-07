#!/bin/sh
# nginx 공식 이미지가 시작 전에 /docker-entrypoint.d/*.sh를 실행함 — 인증서가 없을 때만 자체 서명 인증서 생성
set -eu
DIR=/etc/nginx/certs
[ -s "$DIR/cert.pem" ] && [ -s "$DIR/key.pem" ] && exit 0

mkdir -p "$DIR"
# 사설 인증서라 브라우저 경고가 뜨지만 통신은 암호화됨. 기관 인증서를 받으면 이 두 파일만 교체
openssl req -x509 -nodes -newkey rsa:2048 -days 3650 \
  -subj "/CN=owlmon" -addext "subjectAltName=DNS:owlmon,DNS:localhost" \
  -keyout "$DIR/key.pem" -out "$DIR/cert.pem" 2>/dev/null
chmod 600 "$DIR/key.pem"
echo "[owlmon] 자체 서명 인증서 생성 → $DIR (기관 인증서로 교체 권장)"
