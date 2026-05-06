package logtail

import (
	"encoding/json"
	"log"
	"os"
	"sync"
)

// 디스크에 보관할 최대 엔트리 수.
// 메모리 maxBufferSize(10000)보다 작게 — 디스크 용량 보호.
const maxWalSize = 5000

// WAL은 전송 실패한 로그 엔트리를 디스크에 영속화합니다.
// 에이전트가 재시작되어도 미전송 로그를 복원해 재전송할 수 있습니다.
//
// 설계: 전체 버퍼를 JSON 파일에 통째 덮어쓰기 (스냅샷 방식).
// agent/exporter/persistence.go의 메트릭 버퍼 패턴과 동일.
type WAL struct {
	path string
	mu   sync.Mutex
}

// NewWAL은 path 경로를 사용하는 WAL을 생성합니다.
// path가 빈 문자열이면 영속화 비활성 (Save/Load no-op).
func NewWAL(path string) *WAL {
	return &WAL{path: path}
}

// Save는 엔트리 슬라이스를 디스크에 저장합니다.
// 빈 슬라이스면 파일 삭제 (정리).
// maxWalSize 초과 시 가장 최근 엔트리만 유지.
// path가 빈 문자열이면 no-op.
func (w *WAL) Save(entries []LogEntry) error {
	if w.path == "" {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(entries) == 0 {
		_ = os.Remove(w.path)
		return nil
	}

	// 디스크 용량 보호: 최근 N건만 유지
	if len(entries) > maxWalSize {
		entries = entries[len(entries)-maxWalSize:]
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(w.path, data, 0600)
}

// Load는 디스크에서 엔트리를 복원합니다 (재시작 직후 1회).
// 파일 없거나 손상되었으면 빈 슬라이스 반환 + 손상 파일은 삭제.
// path가 빈 문자열이면 nil 반환.
func (w *WAL) Load() ([]LogEntry, error) {
	if w.path == "" {
		return nil, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []LogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("로그 WAL 파싱 실패, 초기화합니다: %v", err)
		_ = os.Remove(w.path)
		return nil, nil
	}

	log.Printf("로그 WAL 복원: %d건 (재전송 예정)", len(entries))
	return entries, nil
}
