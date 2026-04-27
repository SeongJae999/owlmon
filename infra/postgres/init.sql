-- OWLmon PostgreSQL 초기화 스크립트

-- 알림 히스토리
CREATE TABLE IF NOT EXISTS alert_history (
    id          BIGSERIAL PRIMARY KEY,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    host        TEXT NOT NULL,
    category    TEXT NOT NULL,        -- cpu, memory, disk, down, service
    severity    TEXT NOT NULL,        -- warning, critical
    subject     TEXT NOT NULL,
    body        TEXT NOT NULL
);

CREATE INDEX idx_alert_history_sent_at ON alert_history (sent_at DESC);
CREATE INDEX idx_alert_history_host    ON alert_history (host);

-- 알림 설정 (단일 행)
CREATE TABLE IF NOT EXISTS alert_config (
    id            INT PRIMARY KEY DEFAULT 1,
    enabled       BOOLEAN   NOT NULL DEFAULT true,
    recipients    TEXT[]    NOT NULL DEFAULT '{}',
    cpu_threshold NUMERIC   NOT NULL DEFAULT 90,
    mem_threshold NUMERIC   NOT NULL DEFAULT 95,
    disk_warn     NUMERIC   NOT NULL DEFAULT 85,
    disk_crit     NUMERIC   NOT NULL DEFAULT 90,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO alert_config (id) VALUES (1) ON CONFLICT DO NOTHING;

-- 자산 관리 (장비별 IP/위치/도입일/보증 만료 등)
CREATE TABLE IF NOT EXISTS assets (
    id               BIGSERIAL PRIMARY KEY,
    host_name        TEXT NOT NULL UNIQUE,       -- 모니터링 호스트명과 연결
    ip               TEXT NOT NULL DEFAULT '',
    location         TEXT NOT NULL DEFAULT '',   -- 위치 (예: 2층 서버실)
    description      TEXT NOT NULL DEFAULT '',   -- 장비 설명
    purchase_date    DATE,                        -- 도입일
    warranty_expires DATE,                        -- 보증 만료일
    notes            TEXT NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- SNMP 네트워크 장비
CREATE TABLE IF NOT EXISTS snmp_devices (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,                   -- 장치 이름 (예: 교무실 스위치)
    ip          TEXT NOT NULL UNIQUE,            -- IP 주소
    community   TEXT NOT NULL DEFAULT 'public', -- Community String (v2c)
    port        INT  NOT NULL DEFAULT 161,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =====================================================================
-- 로그 수집 (Phase 0)
-- 에이전트가 journald/파일/Windows Event Log에서 수집한 로그 이벤트를 저장
-- =====================================================================

-- 텍스트 유사도 검색용 확장 (message 컬럼 키워드 검색)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 로그 본체 (일별 파티션, 실제 파티션은 server/db/log_partition.go가 자동 생성/삭제)
CREATE TABLE IF NOT EXISTS logs (
    id          BIGSERIAL,
    host_name   TEXT        NOT NULL,            -- 호스트명
    source      TEXT        NOT NULL,            -- 'journald', 'file:/var/log/nginx/error.log', 'winevent:System'
    timestamp   TIMESTAMPTZ NOT NULL,            -- 로그 발생 시각
    severity    SMALLINT    NOT NULL,            -- OTLP SeverityNumber (1=trace, 5=debug, 9=info, 13=warn, 17=error, 21=fatal)
    message     TEXT        NOT NULL,            -- 로그 메시지 (민감정보 마스킹 후)
    template_id TEXT,                            -- Phase 1(Drain3) 템플릿 ID. 지금은 NULL
    attributes  JSONB,                           -- pid, unit, filename, line_no, event_id 등 동적 속성
    PRIMARY KEY (timestamp, id)                  -- 파티션 키(timestamp)를 PK에 포함 필수
) PARTITION BY RANGE (timestamp);

-- 비정상 timestamp(시계 오차, 지각 도착, 미래 시각) 로그용 fallback 파티션.
-- log_partition.go가 "logs_YYYY_MM_DD" 형식만 자동 관리하므로 logs_default는 안전하게 보존됨.
CREATE TABLE IF NOT EXISTS logs_default PARTITION OF logs DEFAULT;

-- 호스트별 최신순 조회 (대시보드)
CREATE INDEX IF NOT EXISTS idx_logs_host_ts  ON logs (host_name, timestamp DESC);
-- 경고/에러 레벨 부분 인덱스 (대부분 로그는 info 이하 → 전체 인덱스는 낭비)
CREATE INDEX IF NOT EXISTS idx_logs_severity ON logs (severity, timestamp DESC) WHERE severity >= 13;
-- 메시지 키워드 유사도 검색 (pg_trgm)
CREATE INDEX IF NOT EXISTS idx_logs_msg_trgm ON logs USING gin (message gin_trgm_ops);
-- JSONB 속성 검색 (pid, event_id 등)
CREATE INDEX IF NOT EXISTS idx_logs_attrs    ON logs USING gin (attributes);

-- 로그 라벨링 (운영자가 입력한 원인/조치 — 미래 LLM 학습 데이터, 영구 보관)
CREATE TABLE IF NOT EXISTS log_annotations (
    id            BIGSERIAL PRIMARY KEY,
    log_id        BIGINT      NOT NULL,             -- 대상 로그 id
    log_timestamp TIMESTAMPTZ NOT NULL,             -- logs 파티션 pruning용 (PK 일부)
    annotator     TEXT        NOT NULL,             -- 라벨링한 운영자 계정
    category      TEXT,                             -- 'root_cause' | 'action_taken' | 'false_positive'
    problem       TEXT,                             -- 원인 설명
    solution      TEXT,                             -- 조치 내용
    alert_id      BIGINT REFERENCES alert_history(id) ON DELETE SET NULL, -- 연관 알림 (옵션)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_log_annotations_log_id ON log_annotations (log_id);
CREATE INDEX IF NOT EXISTS idx_log_annotations_alert  ON log_annotations (alert_id);
