// inspect-specs는 호스트 스펙을 한 번 수집해서 JSON으로 출력하는 진단 유틸리티.
// 사용법: cd agent && go run ./cmd/inspect-specs
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/seongJae/owlmon/agent/specs"
)

func main() {
	s, err := specs.Collect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "스펙 수집 실패:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(s, "", "  ")
	fmt.Println(string(out))
}
