package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/seongJae/owlmon/server/db"
)

const (
	// Phase 0 결정사항: 본문 10MB, 라인당 64KB
	maxBodyBytes    = 10 * 1024 * 1024
	maxMessageBytes = 64 * 1024
)

// LogIngester는 핸들러가 로그 배치를 위탁하는 인터페이스입니다.
// 구체 구현은 server/ingest 패키지(후속 커밋)가 제공합니다.
type LogIngester interface {
	// Enqueue는 로그 배치를 큐에 넣습니다.
	// 큐가 가득 찼으면 ErrIngestQueueFull을 반환하여 핸들러가 503을 회신하게 합니다.
	Enqueue(records []db.LogRecord) error
}

// ErrIngestQueueFull은 핸들러가 503으로 변환할 큐 포화 에러입니다.
var ErrIngestQueueFull = errors.New("ingest queue full")

// LogsHandler는 OTLP/HTTP Logs 엔드포인트입니다.
type LogsHandler struct {
	ingester LogIngester
}

func NewLogsHandler(ingester LogIngester) *LogsHandler {
	return &LogsHandler{ingester: ingester}
}

// Receive는 POST /v1/logs를 처리합니다.
// 페이로드는 OpenTelemetry OTLP/HTTP Logs JSON 스펙을 따릅니다:
// https://opentelemetry.io/docs/specs/otlp/#otlphttp-request
func (h *LogsHandler) Receive(w http.ResponseWriter, r *http.Request) {
	// 1) 본문 크기 제한
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// MaxBytesReader 초과 시 *MaxBytesError 발생
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "본문이 너무 큽니다", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "본문 읽기 실패", http.StatusBadRequest)
		return
	}

	// 2) JSON 파싱
	var req otlpLogsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "JSON 파싱 실패: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 3) OTLP → LogRecord 변환
	records, rejected := convertOTLPLogs(req)

	// 4) 큐 위탁
	if len(records) > 0 {
		if err := h.ingester.Enqueue(records); err != nil {
			if errors.Is(err, ErrIngestQueueFull) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "수신 큐 포화", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "큐 위탁 실패", http.StatusInternalServerError)
			return
		}
	}

	// 5) 응답 (OTLP partial success 간략 형식)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": len(records),
		"rejected": rejected,
	})
}

// --- OTLP/HTTP JSON 페이로드 타입 (수동 정의, otlp proto 의존성 회피) ---

type otlpLogsRequest struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

type otlpResourceLogs struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}

type otlpResource struct {
	Attributes []otlpKV `json:"attributes"`
}

type otlpScopeLogs struct {
	Scope      otlpScope       `json:"scope"`
	LogRecords []otlpLogRecord `json:"logRecords"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type otlpLogRecord struct {
	TimeUnixNano   string     `json:"timeUnixNano"` // 문자열로 옴 (OTLP 스펙)
	SeverityNumber int        `json:"severityNumber"`
	SeverityText   string     `json:"severityText"`
	Body           otlpAnyVal `json:"body"`
	Attributes     []otlpKV   `json:"attributes"`
}

type otlpKV struct {
	Key   string     `json:"key"`
	Value otlpAnyVal `json:"value"`
}

// otlpAnyVal은 OTLP AnyValue (oneof). v1엔 단순 스칼라만 지원, 추후 확장.
type otlpAnyVal struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"` // OTLP 스펙: int64는 string으로
	BoolValue   *bool    `json:"boolValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

// asAny는 OTLP AnyValue를 Go any로 변환합니다.
func (v otlpAnyVal) asAny() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.IntValue != nil:
		if n, err := strconv.ParseInt(*v.IntValue, 10, 64); err == nil {
			return n
		}
		return *v.IntValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.DoubleValue != nil:
		return *v.DoubleValue
	}
	return nil
}

// flattenAttrs는 OTLP attribute 배열을 map[string]any로 변환합니다.
func flattenAttrs(kvs []otlpKV) map[string]any {
	if len(kvs) == 0 {
		return nil
	}
	m := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		m[kv.Key] = kv.Value.asAny()
	}
	return m
}

// convertOTLPLogs는 OTLP 요청을 LogRecord 슬라이스로 변환합니다.
// rejected는 검증 실패로 버려진 라인 수입니다 (필수 필드 누락 등).
func convertOTLPLogs(req otlpLogsRequest) (records []db.LogRecord, rejected int) {
	for _, rl := range req.ResourceLogs {
		resAttrs := flattenAttrs(rl.Resource.Attributes)
		hostName := stringFromAttr(resAttrs, "host.name")

		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				rec, ok := convertOneLog(lr, hostName, sl.Scope.Name, resAttrs)
				if !ok {
					rejected++
					continue
				}
				records = append(records, rec)
			}
		}
	}
	return records, rejected
}

func convertOneLog(lr otlpLogRecord, hostName, scopeName string, resAttrs map[string]any) (db.LogRecord, bool) {
	// 메시지: body.stringValue 사용. 비어 있으면 reject
	msg := ""
	if lr.Body.StringValue != nil {
		msg = *lr.Body.StringValue
	}
	if msg == "" {
		return db.LogRecord{}, false
	}
	if len(msg) > maxMessageBytes {
		// truncate (Phase 0 결정사항)
		msg = msg[:maxMessageBytes]
	}

	// 타임스탬프: 누락 시 현재 시각
	ts := time.Now().UTC()
	if lr.TimeUnixNano != "" {
		if n, err := strconv.ParseInt(lr.TimeUnixNano, 10, 64); err == nil {
			ts = time.Unix(0, n).UTC()
		}
	}

	// severity: 범위 밖이면 INFO로 fallback
	sev := db.LogLevel(lr.SeverityNumber)
	if sev < db.LogTrace || sev > 24 {
		sev = db.LogInfo
	}

	// 속성: 라인 속성 + 스코프명 + (host.name 제외) 리소스 속성
	attrs := flattenAttrs(lr.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	if scopeName != "" {
		attrs["scope.name"] = scopeName
	}
	for k, v := range resAttrs {
		if k == "host.name" {
			continue // 호스트는 별도 컬럼
		}
		if _, exists := attrs[k]; !exists {
			attrs[k] = v
		}
	}

	// source 추출: attributes["source"] 우선, 없으면 scope name fallback
	source := stringFromAttr(attrs, "source")
	if source == "" {
		source = scopeName
	}

	// host_name 누락 시 reject (필수)
	if hostName == "" {
		return db.LogRecord{}, false
	}

	return db.LogRecord{
		HostName:   hostName,
		Source:     source,
		Timestamp:  ts,
		Severity:   sev,
		Message:    msg,
		Attributes: attrs,
	}, true
}

func stringFromAttr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
