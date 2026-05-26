#!/bin/bash
# OWLmon 종합 헬스체크 — 운영 서버에서 실행
#
# 사용:
#   ssh owlmon@192.168.0.30 'bash -s' < scripts/healthcheck.sh
#   또는 운영에 복사 후 직접: bash /opt/healthcheck.sh
#
# 종료 코드:
#   0 — 정상
#   1 — 경고 있음
#   2 — 장애 있음

set -uo pipefail

# 색상 (TTY일 때만)
if [ -t 1 ]; then
    RED=$'\033[31m'; YEL=$'\033[33m'; GRN=$'\033[32m'; CYN=$'\033[36m'; RST=$'\033[0m'; BOLD=$'\033[1m'
else
    RED=''; YEL=''; GRN=''; CYN=''; RST=''; BOLD=''
fi

OK_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0
WARN_LIST=()
FAIL_LIST=()

ok()   { echo "  ${GRN}✅${RST} $1"; OK_COUNT=$((OK_COUNT+1)); }
warn() { echo "  ${YEL}⚠️ ${RST} $1"; WARN_COUNT=$((WARN_COUNT+1)); WARN_LIST+=("$1"); }
fail() { echo "  ${RED}❌${RST} $1"; FAIL_COUNT=$((FAIL_COUNT+1)); FAIL_LIST+=("$1"); }
section() { echo; echo "${BOLD}${CYN}── $1 ──${RST}"; }

# psql 헬퍼 (값만)
psql_val() {
    docker exec owlmon-postgres psql -U owlmon -d owlmon -tAc "$1" 2>/dev/null
}

echo "${BOLD}╔══════════════════════════════════════════════════════════════╗"
echo "║          OWLmon Health Check — $(date '+%Y-%m-%d %H:%M:%S')        ║"
echo "╚══════════════════════════════════════════════════════════════╝${RST}"

# ───────────────────────────────────────────────────────────────
section "1. 컨테이너 상태"
expected_containers=("owlmon-server" "owlmon-web" "owlmon-postgres" "owlmon-prometheus" "owlmon-otel-collector" "owlmon-grafana")
for c in "${expected_containers[@]}"; do
    if docker ps --format '{{.Names}}' | grep -q "^${c}$"; then
        uptime=$(docker inspect -f '{{.State.StartedAt}}' "$c" 2>/dev/null | cut -c1-19)
        ok "$c (started: $uptime)"
    else
        fail "$c 컨테이너 다운"
    fi
done

# ───────────────────────────────────────────────────────────────
section "2. API 헬스 + 응답 시간"
health=$(curl -s -m 5 http://localhost:8080/api/health 2>/dev/null)
if echo "$health" | grep -q '"status":"ok"'; then
    ms=$(curl -s -o /dev/null -w '%{time_total}' -m 5 http://localhost:8080/api/health | awk '{printf "%d", $1*1000}')
    ok "/api/health → ok (${ms}ms)"
    echo "$health" | grep -oE '"postgres":"[a-z]+"|"prometheus":"[a-z]+"' | sed 's/^/      /'
else
    fail "/api/health 응답 비정상: ${health:0:200}"
fi

web_code=$(curl -s -o /dev/null -w '%{http_code}' -m 5 http://localhost/ || echo "000")
if [ "$web_code" = "200" ]; then
    ok "웹 / → HTTP 200"
else
    fail "웹 / → HTTP $web_code"
fi

# ───────────────────────────────────────────────────────────────
section "3. PostgreSQL — 테이블 row count"
declare -A expected_min=(
    [logs]=100
    [log_rules]=10
    [alert_history]=0
    [snmp_devices]=0
    [ssl_domains]=0
    [assets]=0
)

# 실제 OWLmon 스키마 (메트릭은 Prometheus에만 저장, DB에 host_metrics 테이블 없음)
for table in logs log_rules log_rule_matches log_annotations alert_history alert_config snmp_devices ssl_domains assets agent_specs agents; do
    count=$(psql_val "SELECT COUNT(*) FROM $table" 2>/dev/null)
    if [ -z "$count" ]; then
        fail "$table 테이블 조회 실패"
    else
        min=${expected_min[$table]:-0}
        if [ "$count" -lt "$min" ]; then
            warn "$table: $count rows (기대값 ≥ $min)"
        else
            printf "  ${GRN}✅${RST} %-25s %s rows\n" "$table" "$count"
            OK_COUNT=$((OK_COUNT+1))
        fi
    fi
done

# ───────────────────────────────────────────────────────────────
section "4. 호스트별 메트릭 수신 (Prometheus)"
# OWLmon은 메트릭을 PostgreSQL이 아닌 Prometheus에 저장 → Prometheus에 직접 질의
prom_query="up{job=\"owlmon-server\"}"
prom_up=$(curl -s "http://localhost:9090/api/v1/query?query=up" 2>/dev/null)
if echo "$prom_up" | grep -q '"status":"success"'; then
    ok "Prometheus /query 응답 정상"
else
    warn "Prometheus 질의 실패 — 메트릭 수집 점검 필요"
fi

# 호스트별 메트릭 시리즈 — OTel은 system_* 이름 사용, 라벨은 host
host_metrics=$(curl -s "http://localhost:9090/api/v1/query?query=system_cpu_usage_percent" 2>/dev/null)
host_list=""
if [ -n "$host_metrics" ]; then
    host_list=$(echo "$host_metrics" | grep -oE '"host_name":"[^"]+"' | sort -u | sed 's/"host_name":"//; s/"//')
    if [ -z "$host_list" ]; then
        warn "Prometheus에 system_cpu_usage_percent 시리즈 없음 (OTLP 전송 끊김)"
    else
        while IFS= read -r h; do
            [ -z "$h" ] && continue
            cpu=$(echo "$host_metrics" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for r in d.get('data',{}).get('result',[]):
    if r.get('metric',{}).get('host_name') == '$h':
        print(f\"{float(r['value'][1]):.1f}\")
        break
" 2>/dev/null)
            printf "  ${GRN}✅${RST} %-20s CPU %s%%\n" "$h" "${cpu:-?}"
            OK_COUNT=$((OK_COUNT+1))
        done <<< "$host_list"
    fi
fi

# ───────────────────────────────────────────────────────────────
section "5. 호스트별 로그 송신 시각 (한산 vs 오프라인 구분)"
# journald include_patterns 필터로 매칭 패턴이 없으면 한산해 보일 수 있음
# → 메트릭(Prometheus) 시리즈 활성 + 로그 부재 = '한산' (정상)
# → 메트릭 부재 + 로그 부재 = '오프라인' (장애)
hosts_log=$(psql_val "SELECT host, MAX(timestamp) FROM logs GROUP BY host ORDER BY host")
if [ -z "$hosts_log" ]; then
    warn "logs 테이블에 데이터 없음"
else
    now_epoch=$(date +%s)
    while IFS='|' read -r host ts; do
        [ -z "$host" ] && continue
        ts_epoch=$(date -d "$ts" +%s 2>/dev/null || echo 0)
        diff_sec=$((now_epoch - ts_epoch))
        diff_min=$((diff_sec / 60))
        # 메트릭 시리즈 활성 여부 — OWLmon의 진짜 에이전트 헬스 신호
        if echo "$host_list" | grep -qw "$host"; then
            # 메트릭 OK = 에이전트 살아있음. journald include_patterns 매칭 안 되면 로그 한산이 정상
            printf "  ${GRN}✅${RST} %-20s 메트릭 OK · 로그 %dm 전 (한산함)\n" "$host" "$diff_min"
            OK_COUNT=$((OK_COUNT+1))
        elif [ $diff_min -lt 5 ]; then
            warn "$host 메트릭 시리즈 없음 — 로그는 ${diff_min}m 전 (수상)"
        else
            fail "$host 에이전트 죽음 확실 — 메트릭 X + 로그 ${diff_min}m 전"
        fi
    done <<< "$hosts_log"
fi

# ───────────────────────────────────────────────────────────────
section "6. 알림 발사 이력 (최근 24h)"
alert_total=$(psql_val "SELECT COUNT(*) FROM alert_history WHERE created_at > NOW() - INTERVAL '24 hours'" 2>/dev/null)
alert_critical=$(psql_val "SELECT COUNT(*) FROM alert_history WHERE created_at > NOW() - INTERVAL '24 hours' AND severity='critical'" 2>/dev/null)
alert_acked=$(psql_val "SELECT COUNT(*) FROM alert_history WHERE acked_at IS NOT NULL AND created_at > NOW() - INTERVAL '24 hours'" 2>/dev/null)
if [ -n "$alert_total" ]; then
    echo "  최근 24h 알람: 총 ${alert_total}건 (critical: ${alert_critical:-0}, acked: ${alert_acked:-0})"
    if [ "${alert_critical:-0}" -gt 50 ]; then
        warn "Critical 알람 ${alert_critical}건 — 룰 노이즈 의심"
    else
        ok "알람 발사 정상 범위"
    fi
fi

# ───────────────────────────────────────────────────────────────
section "7. 로그 룰 매칭 (최근 1h)"
rule_matches=$(psql_val "
SELECT r.name, COUNT(*) AS cnt
FROM log_rule_matches m
JOIN log_rules r ON r.id = m.rule_id
WHERE m.matched_at > NOW() - INTERVAL '1 hour'
GROUP BY r.name
ORDER BY cnt DESC
LIMIT 5
")
if [ -n "$rule_matches" ]; then
    echo "  최근 1시간 룰 매칭 Top 5:"
    echo "$rule_matches" | sed 's/^/      /' | sed 's/|/ × /'
    ok "룰 매칭 동작 중"
else
    warn "최근 1시간 룰 매칭 0건 (정상이거나 룰이 패턴 못 잡고 있음)"
fi

# 활성 룰 수
rule_active=$(psql_val "SELECT COUNT(*) FROM log_rules WHERE enabled=true")
rule_total=$(psql_val "SELECT COUNT(*) FROM log_rules")
ok "활성 룰: ${rule_active}/${rule_total}"

# ───────────────────────────────────────────────────────────────
section "8. SSL 도메인 모니터링"
ssl_total=$(psql_val "SELECT COUNT(*) FROM ssl_domains")
ssl_expire_soon=$(psql_val "SELECT COUNT(*) FROM ssl_domains WHERE expire_at < NOW() + INTERVAL '30 days'" 2>/dev/null)
if [ -n "$ssl_total" ] && [ "$ssl_total" -gt 0 ]; then
    ok "SSL 도메인: ${ssl_total}개 등록"
    if [ "${ssl_expire_soon:-0}" -gt 0 ]; then
        warn "SSL 30일 내 만료 임박: ${ssl_expire_soon}개"
    fi
else
    echo "  ${YEL}ℹ️${RST}  SSL 도메인 미등록 (선택 기능)"
fi

# ───────────────────────────────────────────────────────────────
section "9. SNMP 장비"
snmp_total=$(psql_val "SELECT COUNT(*) FROM snmp_devices")
if [ "${snmp_total:-0}" -gt 0 ]; then
    ok "SNMP 장비: ${snmp_total}개 등록"
    snmp_status=$(curl -s -m 5 http://localhost:8080/api/snmp/status 2>/dev/null)
    up_count=$(echo "$snmp_status" | grep -o '"Up":true' | wc -l)
    down_count=$(echo "$snmp_status" | grep -o '"Up":false' | wc -l)
    if [ "$down_count" -gt 0 ]; then
        warn "SNMP 응답 없는 장비: ${down_count}개"
    fi
    if [ "$up_count" -gt 0 ]; then
        ok "SNMP 응답 정상: ${up_count}개"
    fi
else
    echo "  ${YEL}ℹ️${RST}  SNMP 장비 미등록 (선택 기능)"
fi

# ───────────────────────────────────────────────────────────────
section "10. 호스트 스펙 등록률"
spec_count=$(psql_val "SELECT COUNT(DISTINCT host_name) FROM agent_specs")
prom_host_count=$(echo "$host_list" | grep -c .)
if [ "${prom_host_count:-0}" -gt 0 ]; then
    if [ "${spec_count:-0}" -eq "${prom_host_count:-0}" ]; then
        ok "스펙 등록: ${spec_count}/${prom_host_count} 호스트 (100%)"
    elif [ "${spec_count:-0}" -gt 0 ]; then
        warn "스펙 미등록 호스트: ${spec_count}/${prom_host_count} 등록됨"
    else
        warn "스펙 미등록 (모든 호스트)"
    fi
else
    echo "  ${YEL}ℹ️${RST}  호스트 없음"
fi

# ───────────────────────────────────────────────────────────────
section "11. DB 크기 + 디스크"
db_size=$(psql_val "SELECT pg_size_pretty(pg_database_size('owlmon'))")
disk_pct=$(df -h /var/lib/docker 2>/dev/null | tail -1 | awk '{print $5}' | tr -d '%')
ok "DB 크기: $db_size"
if [ -n "$disk_pct" ]; then
    if [ "$disk_pct" -lt 70 ]; then
        ok "디스크 사용률: ${disk_pct}%"
    elif [ "$disk_pct" -lt 85 ]; then
        warn "디스크 사용률: ${disk_pct}% (모니터링)"
    else
        fail "디스크 사용률: ${disk_pct}% (위험)"
    fi
fi

# ───────────────────────────────────────────────────────────────
section "12. DB 백업 시각 (cron 정상 동작?)"
# 백업 디렉토리 후보 (사용자 환경 따라 다를 수 있어 여러 곳 시도)
BACKUP_CANDIDATES=("$HOME/owlmon-backups" "/home/owlmon/owlmon-backups" "/var/backups/owlmon")
backup_dir=""
for d in "${BACKUP_CANDIDATES[@]}"; do
    if [ -d "$d" ]; then
        backup_dir="$d"
        break
    fi
done

if [ -z "$backup_dir" ]; then
    echo "  ${YEL}ℹ️${RST}  백업 디렉토리 없음 (scripts/backup-postgres.sh 미실행?)"
else
    latest=$(ls -t "$backup_dir"/owlmon-*.sql.gz 2>/dev/null | head -1)
    if [ -z "$latest" ]; then
        warn "백업 파일 없음 — $backup_dir"
    else
        # mtime을 epoch로 (Linux는 stat -c, macOS는 stat -f)
        if mtime=$(stat -c %Y "$latest" 2>/dev/null); then :
        else mtime=$(stat -f %m "$latest")
        fi
        now=$(date +%s)
        age_hour=$(( (now - mtime) / 3600 ))
        count=$(ls "$backup_dir"/owlmon-*.sql.gz 2>/dev/null | wc -l | tr -d ' ')
        total_size=$(du -sh "$backup_dir" 2>/dev/null | awk '{print $1}')
        latest_size=$(du -h "$latest" 2>/dev/null | awk '{print $1}')

        if [ "$age_hour" -lt 26 ]; then
            ok "최근 백업: ${age_hour}h 전 ($latest_size, 총 ${count}개/$total_size)"
        elif [ "$age_hour" -lt 50 ]; then
            warn "최근 백업이 ${age_hour}h 전 — daily cron 1회 빠진 듯 ($count개)"
        else
            fail "최근 백업이 ${age_hour}h 전 — cron 동작 점검 필요 ($count개)"
        fi
    fi
fi

# ───────────────────────────────────────────────────────────────
section "13. 라벨 데이터 (Phase 2 학습용 시드)"
annotation_count=$(psql_val "SELECT COUNT(*) FROM log_annotations")
annotation_with_problem=$(psql_val "SELECT COUNT(*) FROM log_annotations WHERE problem IS NOT NULL AND length(problem) > 5")
echo "  라벨 총: ${annotation_count}건 (문제 기재: ${annotation_with_problem}건)"
if [ "${annotation_count:-0}" -lt 50 ]; then
    echo "  ${YEL}ℹ️${RST}  Phase 2 학습 시작 권장치: 50건 이상 (지금: ${annotation_count})"
fi

# ───────────────────────────────────────────────────────────────
echo
echo "${BOLD}╔══════════════════════════════════════════════════════════════╗"
printf "║  결과: %s정상 %d%s · %s경고 %d%s · %s장애 %d%s%*s║\n" \
    "$GRN" "$OK_COUNT" "$RST" "$YEL" "$WARN_COUNT" "$RST" "$RED" "$FAIL_COUNT" "$RST" \
    $((40 - ${#OK_COUNT} - ${#WARN_COUNT} - ${#FAIL_COUNT})) ""
echo "╚══════════════════════════════════════════════════════════════╝${RST}"

if [ "$FAIL_COUNT" -gt 0 ]; then
    echo
    echo "${RED}${BOLD}장애 항목:${RST}"
    for x in "${FAIL_LIST[@]}"; do echo "  • $x"; done
    exit 2
fi
if [ "$WARN_COUNT" -gt 0 ]; then
    echo
    echo "${YEL}${BOLD}경고 항목:${RST}"
    for x in "${WARN_LIST[@]}"; do echo "  • $x"; done
    exit 1
fi
exit 0
