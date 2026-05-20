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

-- 에이전트 호스트 스펙 (CPU/RAM/Disk/OS 등)
-- 에이전트가 시작 시 1회 수집 후 POST → 자주 변하지 않음
CREATE TABLE IF NOT EXISTS agent_specs (
    host_name           TEXT PRIMARY KEY,                -- agents.name과 동일
    cpu_model           TEXT,                            -- "Intel(R) Core(TM) i5-14500"
    cpu_cores           INT,                             -- 논리 CPU 수
    cpu_sockets         INT,
    memory_total_bytes  BIGINT,                          -- /proc/meminfo MemTotal
    disks               JSONB,                           -- [{name,size_bytes,rotational,model}, ...]
    networks            JSONB,                           -- [{name,mac,ipv4}, ...]
    os_pretty_name      TEXT,                            -- "Debian GNU/Linux 12 (bookworm)"
    kernel_version      TEXT,                            -- uname -r
    virtualization      TEXT,                            -- "none"/"kvm"/"docker"
    arch                TEXT,                            -- "x86_64"
    collected_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_specs_updated ON agent_specs (updated_at DESC);

-- ─── 로그 룰셋 (Phase 1: 정규식 + 빈도 임계치 기반 이상 탐지) ──────────
-- 사용자가 정의하는 룰. 로그 ingest 시 평가되어 매칭되면 알림 생성.
CREATE TABLE IF NOT EXISTS log_rules (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,           -- "SSH 무차별 대입"
    pattern         TEXT NOT NULL,                  -- 정규식 (Go RE2 문법)
    severity        TEXT NOT NULL DEFAULT 'warning', -- info / warning / critical
    threshold_count INT,                            -- N회 매칭 시 알림 (null = 1회로도 알림)
    threshold_window INT,                           -- 위 N회 평가 시간 창 (초)
    cooldown_seconds INT NOT NULL DEFAULT 300,      -- 같은 룰 알림 재발송까지 최소 간격
    enabled         BOOLEAN NOT NULL DEFAULT true,
    description     TEXT,                           -- 운영자용 설명/대응 가이드
    category        TEXT,                           -- security / system / network / app
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_log_rules_enabled ON log_rules (enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_log_rules_category ON log_rules (category);

-- 룰 매칭 이력 (어떤 로그가 어떤 룰에 걸렸는지)
CREATE TABLE IF NOT EXISTS log_rule_matches (
    id          BIGSERIAL PRIMARY KEY,
    log_id      BIGINT NOT NULL REFERENCES logs(id) ON DELETE CASCADE,
    rule_id     BIGINT NOT NULL REFERENCES log_rules(id) ON DELETE CASCADE,
    matched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    host        TEXT NOT NULL,
    severity    TEXT NOT NULL                       -- 매칭 시점의 severity 스냅샷
);

CREATE INDEX IF NOT EXISTS idx_log_rule_matches_rule ON log_rule_matches (rule_id, matched_at DESC);
CREATE INDEX IF NOT EXISTS idx_log_rule_matches_host ON log_rule_matches (host, matched_at DESC);
CREATE INDEX IF NOT EXISTS idx_log_rule_matches_time ON log_rule_matches (matched_at DESC);

-- 룰별 최근 알림 발송 시각 (cooldown 평가용 — 같은 룰 5분 내 재알림 방지)
CREATE TABLE IF NOT EXISTS log_rule_alert_history (
    rule_id         BIGINT PRIMARY KEY REFERENCES log_rules(id) ON DELETE CASCADE,
    last_alerted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── 사전 정의 룰셋 (운영자가 즉시 사용 가능한 30개 기본 룰) ────────────
INSERT INTO log_rules (name, pattern, severity, category, description, cooldown_seconds) VALUES
-- 시스템 (메모리/디스크/커널)
('OOM Killer 발동',           'Out of memory.*[Kk]ill', 'critical', 'system',  '메모리 부족으로 프로세스 강제 종료. 즉시 메모리 확보·재시작 필요', 300),
('디스크 가득 참',             'No space left on device', 'critical', 'system', '디스크 100% 도달. 큰 파일·로그 정리 필요', 600),
('파일시스템 read-only',       '[Rr]emounting filesystem read-only', 'critical', 'system', '디스크 오류로 자동 read-only 전환', 600),
('커널 패닉',                  'Kernel panic.*not syncing', 'critical', 'system', '시스템 치명적 오류. 재부팅 필요', 60),
('Watchdog 타임아웃',          'soft lockup.*[Cc]pu', 'critical', 'system', 'CPU soft lockup — 커널 hang 의심', 300),
('Out of inodes',              'No space left on device.*inode', 'critical', 'system', 'inode 고갈 — 작은 파일 너무 많음', 600),
-- 보안 (SSH/sudo/방화벽)
('SSH 무차별 대입 의심',       'Failed password.*for.*from', 'warning',  'security', '비밀번호 실패 반복. fail2ban·차단 검토', 300),
('SSH 잘못된 사용자',          'Invalid user.*from', 'warning', 'security', '존재하지 않는 계정 시도 — 스캔 가능성', 300),
('sudo 권한 거부',             'sudo.*authentication failure', 'warning', 'security', 'sudo 인증 실패 — 비인가 시도 가능성', 600),
('SSH root 로그인 시도',       'Failed.*root from', 'warning', 'security', 'root 직접 로그인 시도 — 보안 정책 위반 가능성', 300),
('sshd protocol 오류',         'fatal:.*protocol', 'warning', 'security', 'SSH 프로토콜 오류 — 비정상 클라이언트', 600),
-- 네트워크
('네트워크 인터페이스 다운',   'Link is Down|carrier lost', 'critical', 'network', '네트워크 끊김 감지', 60),
('DNS 해상 실패 다발',         'Temporary failure in name resolution', 'warning', 'network', 'DNS 응답 지연/실패', 600),
('TCP RST 폭증',               'TCP.*RST', 'warning', 'network', '비정상 연결 종료 증가', 600),
('Out of socket memory',       'Out of socket memory', 'critical', 'network', '소켓 자원 고갈 — 동시 연결 한계', 300),
-- 애플리케이션 (Java/Python/web)
('Java OutOfMemoryError',      'java\.lang\.OutOfMemoryError', 'critical', 'app', 'Java 힙/스택 메모리 부족', 300),
('Java NullPointerException 폭증', 'NullPointerException', 'warning', 'app', '자바 NPE 발생 — 코드 결함 가능성', 600),
('Python Exception traceback', 'Traceback \(most recent call', 'warning', 'app', '파이썬 미처리 예외', 600),
('HTTP 5xx 폭증',              '" 5\d{2} ', 'warning', 'app', 'HTTP 5xx 응답 증가 — 서버 오류', 600),
('HTTP 503 Service Unavailable', '" 503 ', 'critical', 'app', '서비스 일시 중단', 300),
('nginx upstream timeout',     'upstream timed out', 'warning', 'app', 'nginx 백엔드 응답 지연', 600),
('nginx connection refused',   'connect\(\) failed.*Connection refused', 'critical', 'app', '백엔드 다운 의심', 300),
-- DB
('PostgreSQL too many connections', 'too many connections', 'critical', 'app', 'DB 연결 풀 고갈', 300),
('PostgreSQL deadlock',        'deadlock detected', 'warning', 'app', 'DB 데드락 발생', 600),
('MySQL too many connections', 'Too many connections', 'critical', 'app', 'MySQL 연결 한계 도달', 300),
-- 컨테이너 / k8s
('Docker container OOM',       'oom_reaper.*killed', 'critical', 'app', '컨테이너 OOM kill', 300),
('Kubernetes pod CrashLoopBackOff', 'CrashLoopBackOff', 'warning', 'app', 'Pod 반복 크래시', 600),
-- 인증서
('SSL 인증서 만료 임박 경고',  'certificate has expired|will expire', 'warning', 'security', 'SSL 인증서 갱신 필요', 86400),
-- 시간 동기화
('NTP 동기화 실패',            'no peer.*ntp|chrony.*reach 0', 'warning', 'system', '시간 동기화 실패 — 인증 오류 유발 가능', 3600),
-- 일반
('Segmentation fault',         'segfault|Segmentation fault', 'warning', 'app', '프로세스 segfault — 코드/메모리 오류', 300)
ON CONFLICT (name) DO NOTHING;
