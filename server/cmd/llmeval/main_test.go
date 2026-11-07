package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSevSetAndMention(t *testing.T) {
	var c evalCase
	if err := json.Unmarshal([]byte(`{"want_sev":"high"}`), &c); err != nil || !c.WantSev.has("high") {
		t.Fatalf("단일 문자열 want_sev: %v %v", c.WantSev, err)
	}
	if err := json.Unmarshal([]byte(`{"want_sev":["medium","high"]}`), &c); err != nil || !c.WantSev.has("medium") || c.WantSev.has("low") {
		t.Fatalf("배열 want_sev: %v %v", c.WantSev, err)
	}
	if !containsAny("nas 백업 서버 확인", "192.168.0.50|NAS") || containsAny("디스크 정리", "NAS|백업") {
		t.Fatal("containsAny는 대소문자 무시 + | 중 하나 포함이어야 함")
	}
}

// 운영 analyzer와 같은 입력: 마스킹 → 호스트 상태 블록(없으면 '데이터 없음') → 추가 맥락
func TestBuildUserMsg_MatchesProdShape(t *testing.T) {
	msg := buildUserMsg(evalCase{Log: "a@b.com login failed", Context: "[자산 정보] x"})
	for _, want := range []string{"[현재 호스트 상태]\n- 사용률: 데이터 없음", "[자산 정보] x", "Host: evalhost"} {
		if !strings.Contains(msg, want) {
			t.Errorf("누락 %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "a@b.com") {
		t.Error("이메일은 마스킹돼야 함")
	}
}
