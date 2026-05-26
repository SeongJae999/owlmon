#!/bin/bash
# OWLmon PostgreSQL 복구 스크립트
#
# 사용:
#   bash restore-postgres.sh ~/owlmon-backups/owlmon-20260521-030000.sql.gz
#
# 동작:
#   1. 백업 파일 무결성 확인
#   2. 현재 DB 안전 백업 (pre-restore-*.sql.gz)
#   3. DB 비우고 새로 복구
#   4. 검증 — 테이블/row 카운트
#
# ⚠️ 운영 DB 덮어씁니다. 확인 프롬프트 있음.

set -euo pipefail

BACKUP_FILE="${1:-}"
CONTAINER="${CONTAINER:-owlmon-postgres}"
DB_NAME="${DB_NAME:-owlmon}"
DB_USER="${DB_USER:-owlmon}"

if [ -z "$BACKUP_FILE" ]; then
    echo "사용: bash $0 <백업파일.sql.gz>"
    echo
    echo "가능한 백업:"
    ls -lh "$HOME/owlmon-backups"/owlmon-*.sql.gz 2>/dev/null | tail -10
    exit 1
fi

if [ ! -f "$BACKUP_FILE" ]; then
    echo "[오류] 파일 없음: $BACKUP_FILE"
    exit 1
fi

# 무결성 확인
if ! gzip -t "$BACKUP_FILE" 2>/dev/null; then
    echo "[오류] gzip 파일 손상: $BACKUP_FILE"
    exit 1
fi

SIZE_HUMAN=$(du -h "$BACKUP_FILE" | awk '{print $1}')

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  OWLmon DB 복구"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  복구 파일: $BACKUP_FILE ($SIZE_HUMAN)"
echo "  대상 DB :  $CONTAINER / $DB_NAME"
echo
echo "  ⚠️  현재 $DB_NAME DB 데이터가 모두 사라지고 위 백업으로 대체됩니다."
echo "      안전을 위해 사전 백업이 자동 생성됩니다."
echo
read -r -p "  계속하려면 'yes' 입력: " confirm
if [ "$confirm" != "yes" ]; then
    echo "  취소"
    exit 0
fi

# ─── 1. 현재 DB 사전 백업 ─────────────────────────
PRE_BACKUP="$HOME/owlmon-backups/pre-restore-$(date '+%Y%m%d-%H%M%S').sql.gz"
mkdir -p "$(dirname "$PRE_BACKUP")"
echo
echo "[1/3] 현재 DB 사전 백업 → $PRE_BACKUP"
docker exec "$CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" --no-owner --no-acl | gzip -9 > "$PRE_BACKUP"
echo "      $(du -h "$PRE_BACKUP" | awk '{print $1}')"

# ─── 2. DB 비우고 복구 ────────────────────────────
echo "[2/3] DB 비우는 중 (스키마 + 데이터)..."
docker exec "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" > /dev/null

echo "      복구 중 (gzip 풀어서 stdin)..."
gunzip -c "$BACKUP_FILE" | docker exec -i "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" > /dev/null

# ─── 3. 검증 ──────────────────────────────────────
echo "[3/3] 검증"
TABLES=$(docker exec "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")
ROWS=$(docker exec "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT SUM(n_live_tup) FROM pg_stat_user_tables")
echo "      테이블 수: $TABLES"
echo "      총 row 수: $ROWS"

echo
echo "✅ 복구 완료"
echo "   문제 발생 시 사전 백업으로 다시 복구:"
echo "   bash $0 $PRE_BACKUP"
