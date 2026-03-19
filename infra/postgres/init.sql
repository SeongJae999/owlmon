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
