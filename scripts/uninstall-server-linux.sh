#!/bin/bash
# OWLmon 서버 Linux 서비스 제거 스크립트
# 사용법: sudo bash uninstall-server-linux.sh

INSTALL_DIR="/opt/owlmon-server"
SERVICE_NAME="owlmon-server"

if [[ $EUID -ne 0 ]]; then
    echo "root 권한으로 실행하세요: sudo bash $0"
    exit 1
fi

echo "=== OWLmon 서버 제거 ==="

systemctl stop "$SERVICE_NAME" 2>/dev/null && echo "서비스 중지 완료" || true
systemctl disable "$SERVICE_NAME" 2>/dev/null && echo "자동 시작 해제 완료" || true
rm -f "/etc/systemd/system/$SERVICE_NAME.service"
systemctl daemon-reload

if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo "설치 디렉토리 제거 완료: $INSTALL_DIR"
fi

echo "제거 완료"
