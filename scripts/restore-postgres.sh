#!/bin/bash
# OWLmon PostgreSQL 복구 스크립트
#
# 사용 (백업 폴더가 root 전용이므로 sudo):
#   sudo bash restore-postgres.sh data/backups/owlmon-20260521-030000.sql.gz.enc
#
# 다른 서버의 백업을 복구할 때는 그 서버의 패스프레이즈(data/backups/.passphrase)를 지정:
#   sudo OWLMON_BACKUP_PASSPHRASE=... bash restore-postgres.sh <백업파일>
#
# ⚠️ 운영 DB 덮어씁니다. 확인 프롬프트 있음. 메트릭(Prometheus)은 복구 대상이 아닙니다.

set -euo pipefail
umask 077 # 사전 백업은 암호화되지 않은 DB 전체이므로 본인만 읽게 함

BACKUP_FILE="${1:-}"
CONTAINER="${CONTAINER:-owlmon-postgres}"
BACKUP_CONTAINER="${BACKUP_CONTAINER:-owlmon-backup}"
SERVER_CONTAINER="${SERVER_CONTAINER:-owlmon-server}"
DB_NAME="${DB_NAME:-owlmon}"
DB_USER="${DB_USER:-owlmon}"

if [ -z "$BACKUP_FILE" ]; then
    echo "사용: sudo bash $0 <백업파일.sql.gz.enc | 백업파일.sql.gz>"
    echo
    echo "가능한 백업 (설치 폴더의 data/backups/):"
    docker exec "$BACKUP_CONTAINER" sh -c 'ls -lh /backups/owlmon-*' 2>/dev/null | tail -10 || true
    exit 1
fi

if [ ! -f "$BACKUP_FILE" ]; then
    echo "[오류] 파일 없음 또는 읽기 권한 없음 (sudo로 실행했는지 확인): $BACKUP_FILE"
    exit 1
fi

# 백업 형식과 같은 openssl이 들어 있는 백업 컨테이너에서 복호화 (호스트에 openssl이 없어도 됨)
read_sql() {
    case "$BACKUP_FILE" in
        *.enc)
            docker exec -i -e OWLMON_BACKUP_PASSPHRASE "$BACKUP_CONTAINER" sh -c '
                P="${OWLMON_BACKUP_PASSPHRASE:-}"
                [ -n "$P" ] || P=$(cat /backups/.passphrase)
                export P
                openssl enc -d -aes-256-cbc -pbkdf2 -iter 100000 -pass env:P
            ' < "$BACKUP_FILE" | gunzip -c
            ;;
        *) gunzip -c "$BACKUP_FILE" ;;
    esac
}

# DB를 건드리기 전에 끝까지 풀어서 확인 — 패스프레이즈 오류, 파일 손상, 중간에 잘린 백업을 여기서 걸러냄
echo "백업 파일 확인 중..."
if ! COMPLETE=$(read_sql 2>/dev/null | grep -c '^-- PostgreSQL database dump complete') || [ "$COMPLETE" != "1" ]; then
    echo "[오류] 복호화에 실패했거나 온전한 백업이 아닙니다: $BACKUP_FILE"
    echo "       다른 서버의 백업이면 OWLMON_BACKUP_PASSPHRASE를 지정하세요."
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
echo "      복구하는 동안 서버가 멈추고, 안전을 위해 사전 백업이 자동 생성됩니다."
echo
read -r -p "  계속하려면 'yes' 입력: " confirm
if [ "$confirm" != "yes" ]; then
    echo "  취소"
    exit 0
fi

# 복구 중 서버가 DB에 쓰면 데이터가 섞임. 실패해도 서버는 다시 띄움
docker stop "$SERVER_CONTAINER" > /dev/null
trap 'docker start "$SERVER_CONTAINER" > /dev/null' EXIT

# ─── 1. 현재 DB 사전 백업 ─────────────────────────
PRE_BACKUP="$HOME/owlmon-backups/pre-restore-$(date '+%Y%m%d-%H%M%S').sql.gz"
mkdir -p "$(dirname "$PRE_BACKUP")"
echo
echo "[1/3] 현재 DB 사전 백업 → $PRE_BACKUP"
docker exec "$CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" --no-owner --no-acl | gzip -9 > "$PRE_BACKUP"
echo "      $(du -h "$PRE_BACKUP" | awk '{print $1}')"

# ─── 2. 복구 ──────────────────────────────────────
# 삭제와 복구를 한 트랜잭션으로 묶음 — 도중에 오류가 나면 전부 취소되어 현재 DB가 그대로 남음
echo "[2/3] 복구 중..."
if ! { echo 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'; read_sql; } \
    | docker exec -i "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" --single-transaction -v ON_ERROR_STOP=1 > /dev/null; then
    echo "[오류] 복구 실패 — 변경 사항은 모두 취소되었고 현재 DB는 그대로입니다."
    exit 1
fi

# ─── 3. 검증 ──────────────────────────────────────
echo "[3/3] 검증"
docker exec "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -qc 'ANALYZE' # 복구 직후엔 통계가 비어 있어 row 수가 0으로 나옴
TABLES=$(docker exec "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")
ROWS=$(docker exec "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT SUM(n_live_tup) FROM pg_stat_user_tables")
echo "      테이블 수: $TABLES"
echo "      총 row 수: $ROWS"

echo
echo "✅ 복구 완료"
echo "   문제 발생 시 사전 백업으로 다시 복구:"
echo "   sudo bash $0 $PRE_BACKUP"
