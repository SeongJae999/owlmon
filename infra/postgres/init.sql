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

-- SSL 인증서 모니터링 도메인
CREATE TABLE IF NOT EXISTS ssl_domains (
    id         BIGSERIAL PRIMARY KEY,
    domain     TEXT NOT NULL UNIQUE,
    port       INT  NOT NULL DEFAULT 443,
    memo       TEXT NOT NULL DEFAULT '',        -- 메모 (예: 학교 홈페이지)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Synthetic 모니터링 (외부에서 URL 주기 호출, 응답시간/가용성 측정)
CREATE TABLE IF NOT EXISTS synthetic_monitors (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL,                      -- 모니터 이름 (예: 학교 홈페이지)
    url              TEXT NOT NULL,                      -- http(s)://...
    method           TEXT NOT NULL DEFAULT 'GET',
    expected_status  INT  NOT NULL DEFAULT 200,
    expected_keyword TEXT NOT NULL DEFAULT '',           -- 응답 본문에 포함돼야 할 키워드 (선택)
    interval_seconds INT  NOT NULL DEFAULT 60,
    timeout_seconds  INT  NOT NULL DEFAULT 10,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS synthetic_results (
    id                BIGSERIAL PRIMARY KEY,
    monitor_id        BIGINT NOT NULL REFERENCES synthetic_monitors(id) ON DELETE CASCADE,
    success           BOOLEAN NOT NULL,
    status_code       INT NOT NULL DEFAULT 0,
    response_time_ms  INT NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    checked_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_synthetic_results_monitor_time ON synthetic_results (monitor_id, checked_at DESC);

-- DPM (Database Performance Monitoring): 외부 DB에 접속해 슬로우 쿼리/커넥션 통계 수집
CREATE TABLE IF NOT EXISTS dpm_instances (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,                  -- 사용자 표시명 (예: 학사시스템 DB)
    db_type           TEXT NOT NULL,                  -- 'postgres' | 'mysql' (Phase 2는 postgres만)
    host              TEXT NOT NULL,
    port              INT  NOT NULL DEFAULT 5432,
    username          TEXT NOT NULL,
    password_enc      TEXT NOT NULL,                  -- AES-256-GCM 암호화된 base64
    database          TEXT NOT NULL DEFAULT '',       -- 접속할 DB 이름 (postgres는 필수)
    poll_interval_sec INT  NOT NULL DEFAULT 60,
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dpm_query_stats (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     BIGINT NOT NULL REFERENCES dpm_instances(id) ON DELETE CASCADE,
    query_id        TEXT NOT NULL,                    -- pg queryid
    query_text      TEXT NOT NULL,
    calls           BIGINT NOT NULL DEFAULT 0,
    total_time_ms   DOUBLE PRECISION NOT NULL DEFAULT 0,
    mean_time_ms    DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_time_ms     DOUBLE PRECISION NOT NULL DEFAULT 0,
    rows_returned   BIGINT NOT NULL DEFAULT 0,
    collected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dpm_query_stats_instance_time ON dpm_query_stats (instance_id, collected_at DESC);

CREATE TABLE IF NOT EXISTS dpm_instance_metrics (
    id                  BIGSERIAL PRIMARY KEY,
    instance_id         BIGINT NOT NULL REFERENCES dpm_instances(id) ON DELETE CASCADE,
    connections_active  INT NOT NULL DEFAULT 0,
    connections_idle    INT NOT NULL DEFAULT 0,
    connections_max     INT NOT NULL DEFAULT 0,
    cache_hit_ratio     DOUBLE PRECISION NOT NULL DEFAULT 0,
    db_size_bytes       BIGINT NOT NULL DEFAULT 0,
    error               TEXT NOT NULL DEFAULT '',
    collected_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dpm_instance_metrics_time ON dpm_instance_metrics (instance_id, collected_at DESC);

-- 로그 수집
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS logs (
    id         BIGSERIAL PRIMARY KEY,
    timestamp  TIMESTAMPTZ NOT NULL,
    host       TEXT NOT NULL,
    source     TEXT NOT NULL,                   -- 로그 이름 (예: syslog, nginx-error)
    file_path  TEXT NOT NULL DEFAULT '',
    line       TEXT NOT NULL,
    level      TEXT NOT NULL DEFAULT '',        -- ERROR, WARN, INFO 등
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_host      ON logs (host);
CREATE INDEX IF NOT EXISTS idx_logs_level     ON logs (level);
CREATE INDEX IF NOT EXISTS idx_logs_line_trgm ON logs USING gin (line gin_trgm_ops);

-- 로그 라벨링 (운영자가 입력한 원인/조치 — 미래 LLM 학습 데이터, 영구 보관)
-- logs 테이블이 retention 정책으로 정리돼도 라벨은 살아남도록 별도 테이블.
CREATE TABLE IF NOT EXISTS log_annotations (
    id            BIGSERIAL PRIMARY KEY,
    log_id        BIGINT      NOT NULL,            -- 대상 로그 id (FK 없음 — logs 정리 후에도 라벨 보존)
    log_timestamp TIMESTAMPTZ NOT NULL,            -- 원본 로그 시각 (라벨 시점 추적용)
    annotator     TEXT        NOT NULL,            -- 라벨링한 운영자 계정
    category      TEXT,                            -- 'root_cause' | 'action_taken' | 'false_positive'
    problem       TEXT,                            -- 원인 설명 (자유 텍스트)
    solution      TEXT,                            -- 조치 내용 (자유 텍스트)
    alert_id      BIGINT REFERENCES alert_history(id) ON DELETE SET NULL, -- 연관 알림 (옵션)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_log_annotations_log_id ON log_annotations (log_id);
CREATE INDEX IF NOT EXISTS idx_log_annotations_alert  ON log_annotations (alert_id);
CREATE INDEX IF NOT EXISTS idx_log_annotations_created ON log_annotations (created_at DESC);

-- 에이전트 관리
CREATE TABLE IF NOT EXISTS agents (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,          -- 에이전트 이름 (보통 호스트명)
    key         TEXT NOT NULL UNIQUE,          -- 발급된 고유 키
    status      TEXT NOT NULL DEFAULT 'pending', -- pending, active, blocked
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen   TIMESTAMPTZ
);
