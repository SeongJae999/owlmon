#!/bin/bash
# OWLmon PostgreSQL 백업 스크립트
#
# 사용:
#   bash backup-postgres.sh                          # 기본 (~/owlmon-backups에 저장)
#   bash backup-postgres.sh /mnt/nas/owlmon          # 다른 경로
#   BACKUP_DIR=/var/backups bash backup-postgres.sh  # 환경변수
#
# cron 등록 (매일 새벽 3시):
#   crontab -e
#   0 3 * * * /opt/owlmon/scripts/backup-postgres.sh >> /var/log/owlmon-backup.log 2>&1
#
# 보관 정책: 30일 — 그 이전 백업 자동 삭제

set -euo pipefail

# ─── 설정 ──────────────────────────────────────────
BACKUP_DIR="${1:-${BACKUP_DIR:-$HOME/owlmon-backups}}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
CONTAINER="${CONTAINER:-owlmon-postgres}"
DB_NAME="${DB_NAME:-owlmon}"
DB_USER="${DB_USER:-owlmon}"

TS=$(date '+%Y%m%d-%H%M%S')
OUT_FILE="$BACKUP_DIR/owlmon-${TS}.sql.gz"

# ─── 사전 확인 ────────────────────────────────────
echo "[$(date '+%Y-%m-%d %H:%M:%S')] OWLmon DB 백업 시작"

if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
    echo "  [오류] 컨테이너 '$CONTAINER' 실행 중 아님"
    exit 1
fi

mkdir -p "$BACKUP_DIR"

# ─── 백업 실행 ────────────────────────────────────
echo "  대상: $CONTAINER / DB=$DB_NAME"
echo "  저장: $OUT_FILE"

# pg_dump (custom format은 pg_restore 필요) — plain text + gzip이 단순/이식성 좋음
docker exec "$CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" --no-owner --no-acl \
    | gzip -9 > "$OUT_FILE"

# 백업 무결성 확인
if [ ! -s "$OUT_FILE" ]; then
    echo "  [오류] 백업 파일이 비어있음"
    rm -f "$OUT_FILE"
    exit 1
fi

SIZE_BYTES=$(stat -c%s "$OUT_FILE" 2>/dev/null || stat -f%z "$OUT_FILE")
SIZE_HUMAN=$(du -h "$OUT_FILE" | awk '{print $1}')

# gzip 무결성 검증
if ! gzip -t "$OUT_FILE" 2>/dev/null; then
    echo "  [오류] gzip 파일 손상"
    rm -f "$OUT_FILE"
    exit 1
fi

echo "  ✅ 백업 완료: $SIZE_HUMAN ($SIZE_BYTES bytes)"

# ─── 보관 정책 (오래된 백업 삭제) ─────────────────
DELETED=$(find "$BACKUP_DIR" -name 'owlmon-*.sql.gz' -mtime "+$RETENTION_DAYS" -print -delete | wc -l | tr -d ' ')
if [ "$DELETED" -gt 0 ]; then
    echo "  🧹 ${RETENTION_DAYS}일 초과 백업 ${DELETED}개 삭제"
fi

# ─── 요약 ─────────────────────────────────────────
TOTAL_COUNT=$(find "$BACKUP_DIR" -name 'owlmon-*.sql.gz' | wc -l | tr -d ' ')
TOTAL_SIZE=$(du -sh "$BACKUP_DIR" 2>/dev/null | awk '{print $1}')
echo "  📦 백업 디렉토리 현황: $TOTAL_COUNT개 / $TOTAL_SIZE"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] OWLmon DB 백업 종료"
