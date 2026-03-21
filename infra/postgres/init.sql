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
